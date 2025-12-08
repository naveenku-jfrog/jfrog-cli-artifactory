package helm

import (
	"fmt"
	"strings"

	"github.com/jfrog/build-info-go/entities"
	ioutils "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// updateDependencyManifestAndConfigFile adds manifest.json and config files for all OCI dependencies
func updateDependencyOCILayersInBuildInfo(buildInfo *entities.BuildInfo, serviceManager artifactory.ArtifactoryServicesManager) {
	if buildInfo == nil || len(buildInfo.Modules) == 0 {
		return
	}
	processedDeps := make(map[string]bool)
	for moduleIdx := range buildInfo.Modules {
		module := &buildInfo.Modules[moduleIdx]
		if len(module.Dependencies) == 0 {
			continue
		}
		processModuleDependencies(module, serviceManager, processedDeps)
	}
}

// processModuleDependencies processes all dependencies in a module
func processModuleDependencies(module *entities.Module, serviceManager artifactory.ArtifactoryServicesManager, processedDeps map[string]bool) {
	lengthOfDeps := len(module.Dependencies)
	for depIdx := 0; depIdx < lengthOfDeps; depIdx++ {
		dep := &module.Dependencies[0]
		processDependency(dep, 0, module, serviceManager, processedDeps)
	}
}

// processDependency processes a single dependency and returns whether to increment the index
func processDependency(dep *entities.Dependency, _ int, module *entities.Module, serviceManager artifactory.ArtifactoryServicesManager, processedDeps map[string]bool) {
	if !isOCIRepository(dep.Repository) {
		processClassicHelmDependency(dep, module, serviceManager, processedDeps)
		return
	}
	processOCIDependency(dep, module, serviceManager, processedDeps)
}

// processClassicHelmDependency handles classic Helm dependencies
func processClassicHelmDependency(dep *entities.Dependency, module *entities.Module, serviceManager artifactory.ArtifactoryServicesManager, processedDeps map[string]bool) {
	if processedDeps[dep.Sha256] {
		return
	}
	if dep.Sha256 == "" {
		err := updateClassicHelmDependencyChecksums(dep, serviceManager)
		if err != nil {
			log.Debug("Failed to update checksums for classic Helm dependency ", dep.Id, " : ", err)
			return
		}
		processedDeps[dep.Sha256] = true
		removeDependencyByIndex(module)
		log.Debug("Removed classic Helm dependency ", dep.Id, " without checksums (not found in Artifactory)")
		return
	}
}

// processOCIDependency handles OCI dependencies
func processOCIDependency(dep *entities.Dependency, module *entities.Module, serviceManager artifactory.ArtifactoryServicesManager, processedDeps map[string]bool) bool {
	addedLayers, err := addOCILayersForDependency(dep, module, serviceManager, processedDeps)
	if err != nil {
		log.Debug("Failed to add OCI layers for dependency ", dep.Id, " : ", err)
		return true
	}
	if addedLayers > 0 {
		removeDependencyByIndex(module)
		log.Debug("Removed dependency ", dep.Id, " after adding ", addedLayers, " OCI layers")
		return false
	}
	return true
}

// addOCILayersForDependency adds all OCI layers for a dependency that has checksums
func addOCILayersForDependency(dep *entities.Dependency, module *entities.Module, serviceManager artifactory.ArtifactoryServicesManager, processedDeps map[string]bool) (int, error) {
	versionPath := extractDependencyPath(dep.Id)
	if versionPath == "" {
		return 0, fmt.Errorf("could not extract version path from dependency ID %s", dep.Id)
	}
	repoName := extractRepositoryNameFromURL(dep.Repository)
	if repoName == "" {
		return 0, fmt.Errorf("could not extract repo name from: %s", dep.Repository)
	}
	searchPattern := fmt.Sprintf("%s/%s/*", repoName, versionPath)
	ociArtifacts, err := searchDependencyOCIFilesByPath(serviceManager, searchPattern)
	if err != nil {
		log.Debug("Failed to search OCI artifacts for dependency ", dep.Id, " : ", err)
		return 0, nil
	}
	if len(ociArtifacts) == 0 {
		return 0, nil
	}
	addedCount := 0
	for name, resultItem := range ociArtifacts {
		if processedDeps[resultItem.Sha256] {
			continue
		}
		addOCILayer(module, resultItem)
		addedCount++
		processedDeps[resultItem.Sha256] = true
		log.Debug("Added OCI artifact as dependency: ", name, " (path: ", resultItem.Path, "/", name, ")")
	}
	return addedCount, nil
}

// removeDependencyByIndex removes a dependency from the module by its index
func removeDependencyByIndex(module *entities.Module) {
	if len(module.Dependencies) == 0 {
		return
	}
	module.Dependencies = module.Dependencies[1:]
}

// updateClassicHelmDependencyChecksums searches for a classic Helm chart .tgz file in Artifactory
func updateClassicHelmDependencyChecksums(dep *entities.Dependency, serviceManager artifactory.ArtifactoryServicesManager) error {
	depName, depVersion, err := parseDependencyID(dep.Id)
	if err != nil {
		return fmt.Errorf("could not parse dependency ID %s: %w", dep.Id, err)
	}
	repoName := extractRepositoryNameFromURL(dep.Repository)
	if repoName == "" {
		return fmt.Errorf("could not extract repository name from: %s", dep.Repository)
	}
	log.Debug("Classic Helm dependency ", dep.Id, " has no checksums, searching for .tgz file in Artifactory")
	resultItem, err := searchClassicHelmChart(serviceManager, repoName, depName, depVersion)
	if err != nil {
		log.Debug("Classic Helm chart not found for dependency ", dep.Id, " : ", err)
		return nil
	}
	dep.Checksum = entities.Checksum{
		Sha1:   resultItem.Actual_Sha1,
		Sha256: resultItem.Sha256,
		Md5:    resultItem.Actual_Md5,
	}
	dep.Sha256 = resultItem.Sha256
	log.Debug("Found classic Helm chart for dependency ", dep.Id, " : ", resultItem.Name, " (sha256: ", dep.Sha256, ")")
	return nil
}

// parseDependencyID parses dependency ID into name and version
func parseDependencyID(depId string) (string, string, error) {
	parts := strings.Split(depId, ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid dependency ID format: %s", depId)
	}
	return parts[0], parts[1], nil
}

// searchClassicHelmChart searches for classic Helm chart .tgz file
func searchClassicHelmChart(serviceManager artifactory.ArtifactoryServicesManager, repoName, depName, depVersion string) (*servicesUtils.ResultItem, error) {
	searchPattern := fmt.Sprintf("%s/%s-%s*.tgz", repoName, depName, depVersion)
	log.Debug("Searching for classic Helm chart with pattern: ", searchPattern)

	searchParams := services.NewSearchParams()
	searchParams.Pattern = searchPattern
	searchParams.Recursive = false

	reader, err := serviceManager.SearchFiles(searchParams)
	if err != nil {
		return nil, fmt.Errorf("failed to search for classic Helm chart: %w", err)
	}

	var closeErr error
	defer func() {
		if closeErr != nil {
			log.Debug("Failed to close search reader: ", closeErr)
		}
		ioutils.Close(reader, &closeErr)
	}()
	for item := new(servicesUtils.ResultItem); reader.NextRecord(item) == nil; item = new(servicesUtils.ResultItem) {
		if item.Type != "folder" && strings.HasSuffix(item.Name, ".tgz") {
			return item, nil
		}
	}
	return nil, fmt.Errorf("classic Helm chart .tgz file not found")
}

// searchDependencyOCIFilesByPath searches for OCI artifacts using a search pattern
func searchDependencyOCIFilesByPath(serviceManager artifactory.ArtifactoryServicesManager, searchPattern string) (map[string]*servicesUtils.ResultItem, error) {
	log.Debug("Searching for OCI artifacts with pattern: ", searchPattern)

	searchParams := services.NewSearchParams()
	searchParams.Pattern = searchPattern
	searchParams.Recursive = false

	reader, err := serviceManager.SearchFiles(searchParams)
	if err != nil {
		return nil, fmt.Errorf("failed to search for OCI artifacts: %w", err)
	}

	var closeErr error
	defer func() {
		if closeErr != nil {
			log.Debug("Failed to close search reader: ", closeErr)
		}
		ioutils.Close(reader, &closeErr)
	}()

	artifacts := make(map[string]*servicesUtils.ResultItem)
	for item := new(servicesUtils.ResultItem); reader.NextRecord(item) == nil; item = new(servicesUtils.ResultItem) {
		if item.Type != "folder" && (item.Name == "manifest.json" || strings.HasPrefix(item.Name, "sha256__")) {
			itemCopy := *item
			artifacts[item.Name] = &itemCopy
			log.Debug("Found OCI artifact: ", item.Name, " (path: ", item.Path, "/", item.Name, ", sha256: ", item.Sha256, ")")
		}
	}
	return artifacts, nil
}

// addOCIDependency adds an OCI artifact as a separate dependency
func addOCILayer(module *entities.Module, resultItem *servicesUtils.ResultItem) {
	ociDependency := entities.Dependency{
		Id: fmt.Sprintf("%s/%s/%s", resultItem.Repo, resultItem.Path, resultItem.Name),
		Checksum: entities.Checksum{
			Sha1:   resultItem.Actual_Sha1,
			Sha256: resultItem.Sha256,
			Md5:    resultItem.Actual_Md5,
		},
	}

	module.Dependencies = append(module.Dependencies, ociDependency)
	log.Debug("Added OCI artifact as dependency: ", resultItem.Name, " (path: ", resultItem.Path, "/", resultItem.Name, ")")
}
