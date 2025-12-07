package helm

import (
	"fmt"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/gofrog/crypto"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func handleInstallOrUpgradeCommand(buildInfo *entities.BuildInfo, commandName string, helmArgs []string, serviceManager artifactory.ArtifactoryServicesManager) {
	chartPath, _ := getPullChartPath(commandName, helmArgs)
	if chartPath == "" {
		return
	}
	log.Debug(fmt.Sprintf("Extracting dependencies from chart: %s", chartPath))
	settings := cli.New()
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), "", "secret", func(format string, v ...interface{}) {
		log.Debug(fmt.Sprintf(format, v...))
	}); err != nil {
		log.Debug(fmt.Sprintf("Warning: failed to initialize action config: %v. Continuing with chart loading...", err))
	}
	var chartPathOptions *action.ChartPathOptions
	if commandName == "install" {
		installClient := action.NewInstall(actionConfig)
		chartPathOptions = &installClient.ChartPathOptions
	} else {
		upgradeClient := action.NewUpgrade(actionConfig)
		chartPathOptions = &upgradeClient.ChartPathOptions
	}
	updateChartPathOptionsFromArgs(chartPathOptions, helmArgs)
	resolvedChartPath, err := chartPathOptions.LocateChart(chartPath, settings)
	if err != nil {
		return
	}
	log.Debug(fmt.Sprintf("Resolved chart path: %s", resolvedChartPath))
	loadedChart, err := loader.Load(resolvedChartPath)
	if err != nil {
		return
	}
	if loadedChart.Lock == nil {
		log.Debug(fmt.Sprintf("Chart.Lock is not available: %s", loadedChart.Metadata.Name))
		return
	}
	dependencies := loadedChart.Lock.Dependencies
	if dependencies == nil {
		return
	}
	// Build cache map for non-OCI dependencies
	cacheMap := buildHelmCacheMap(settings)
	dependenciesWithChecksum := getDependenciesWithChecksums(dependencies, serviceManager, cacheMap)
	if len(dependenciesWithChecksum) > 0 && buildInfo != nil && len(buildInfo.Modules) > 0 {
		buildInfo.Modules[0].Dependencies = append(buildInfo.Modules[0].Dependencies, dependenciesWithChecksum...)
	}
}

// getDependenciesWithChecksums gets checksums for chart dependencies from the specified registry
// For non-OCI dependencies, it first checks the Helm cache before querying Artifactory
func getDependenciesWithChecksums(chartDeps []*chart.Dependency, serviceManager artifactory.ArtifactoryServicesManager, cacheMap map[string]string) []entities.Dependency {
	if len(chartDeps) == 0 {
		return []entities.Dependency{}
	}
	var dependencies []entities.Dependency
	for _, chartDep := range chartDeps {
		depId := fmt.Sprintf("%s:%s", chartDep.Name, chartDep.Version)
		repository := ExtractPathFromURL(chartDep.Repository)
		dep := entities.Dependency{
			Id:         depId,
			Type:       "helm",
			Repository: chartDep.Repository,
		}
		if isOCIRepository(chartDep.Repository) {
			versionPath := fmt.Sprintf("%s/%s", chartDep.Name, chartDep.Version)
			searchPattern := fmt.Sprintf("%s/%s/*", repository, versionPath)
			ociArtifacts, err := searchDependencyOCIFilesByPath(serviceManager, searchPattern)
			if err != nil {
				log.Debug(fmt.Sprintf("Failed to search OCI artifacts for dependency %s: %v", depId, err))
				dependencies = append(dependencies, dep)
				continue
			}
			if len(ociArtifacts) > 0 {
				for _, resultItem := range ociArtifacts {
					dep.Id = resultItem.Name
					dep.Checksum = entities.Checksum{
						Sha1:   resultItem.Actual_Sha1,
						Sha256: resultItem.Sha256,
						Md5:    resultItem.Actual_Md5,
					}
					dependencies = append(dependencies, dep)
					log.Debug(fmt.Sprintf("Found OCI checksums for dependency %s: sha256=%s", depId, dep.Sha256))
				}
			}
		} else {
			// Try to find in cache - check both with repo and without (for flat cache structure)
			cacheKey := buildCacheKey(chartDep.Name, chartDep.Version)
			cachedPath, found := cacheMap[cacheKey]
			if found {
				log.Debug(fmt.Sprintf("Found dependency %s in Helm cache: %s", depId, cachedPath))
				fileDetails, err := crypto.GetFileDetails(cachedPath, true)
				if err != nil {
					log.Debug(fmt.Sprintf("Failed to get checksums from cache for %s: %v", depId, err))
				} else {
					dep.Id = filepath.Base(cachedPath)
					dep.Checksum = entities.Checksum{
						Sha1:   fileDetails.Checksum.Sha1,
						Sha256: fileDetails.Checksum.Sha256,
						Md5:    fileDetails.Checksum.Md5,
					}
					dependencies = append(dependencies, dep)
					log.Debug(fmt.Sprintf("Found classic Helm checksums from cache for dependency %s: sha256=%s", depId, dep.Sha256))
					continue
				}
			}
			// Not found in cache, search Artifactory
			resultItem, err := searchClassicHelmChart(serviceManager, repository, chartDep.Name, chartDep.Version)
			if err != nil {
				log.Debug(fmt.Sprintf("Classic Helm chart not found for dependency %s: %v", depId, err))
				dependencies = append(dependencies, dep)
				continue
			}
			dep.Id = resultItem.Name
			dep.Checksum = entities.Checksum{
				Sha1:   resultItem.Actual_Sha1,
				Sha256: resultItem.Sha256,
				Md5:    resultItem.Actual_Md5,
			}
			dependencies = append(dependencies, dep)
			log.Debug(fmt.Sprintf("Found classic Helm checksums from Artifactory for dependency %s: sha256=%s", depId, dep.Sha256))
		}
	}
	return dependencies
}

// ExtractPathFromURL extracts the path from a URL, regardless of the scheme
func ExtractPathFromURL(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	path := parsedURL.Path
	return strings.TrimPrefix(path, "/")
}

// buildHelmCacheMap scans the Helm repository cache and builds a map of available charts
func buildHelmCacheMap(settings *cli.EnvSettings) map[string]string {
	cacheMap := make(map[string]string)
	cacheDir := settings.RepositoryCache
	if cacheDir == "" {
		log.Debug("Helm repository cache directory not configured")
		return cacheMap
	}
	log.Debug(fmt.Sprintf("Scanning Helm cache directory: %s", cacheDir))
	err := filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".tgz") {
			return nil
		}
		relPath, err := filepath.Rel(cacheDir, path)
		if err != nil {
			return err
		}
		parts := strings.Split(relPath, string(filepath.Separator))
		var fileName string
		switch {
		case len(parts) >= 2:
			fileName = parts[len(parts)-1]
		case len(parts) == 1:
			fileName = parts[0]
		default:
			return nil
		}

		nameWithoutExt := strings.TrimSuffix(fileName, ".tgz")
		lastDash := strings.LastIndex(nameWithoutExt, "-")
		if lastDash == -1 {
			return nil
		}
		chartName := nameWithoutExt[:lastDash]
		version := nameWithoutExt[lastDash+1:]

		cacheKey := fmt.Sprintf("%s:%s", chartName, version)
		cacheMap[cacheKey] = path
		log.Debug(fmt.Sprintf("Found cached chart: %s -> %s", cacheKey, path))
		return nil
	})
	if err != nil {
		log.Debug(fmt.Sprintf("Error scanning Helm cache directory: %v", err))
	}
	log.Debug(fmt.Sprintf("Built cache map with %d entries", len(cacheMap)))
	return cacheMap
}

// buildCacheKey creates a cache key from repository, chart name, and version
func buildCacheKey(chartName, version string) string {
	return fmt.Sprintf("%s:%s", chartName, version)
}
