package helm

import (
	"fmt"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"strings"
)

func handlePullCommand(buildInfo *entities.BuildInfo, helmArgs []string, serviceManager artifactory.ArtifactoryServicesManager) {
	positionalArgs := getPositionalArguments(helmArgs)
	if len(positionalArgs) == 0 {
		log.Debug("Skipping pulling because no chart path given")
		return
	}
	chartPath := positionalArgs[0]
	isOCI := isOCIRepository(chartPath)
	if isOCI {
		_, chartVersion, err := coreutils.ExtractStringOptionFromArgs(helmArgs, "version")
		if err != nil {
			log.Error(errorutils.CheckError(err), "Failed to extract version from string option")
		}
		repo, chartName := getRepoAndChartName(ExtractPathFromURL(chartPath))
		searchPattern := fmt.Sprintf("%s/%s/%s/", repo, chartName, chartVersion)
		resultMap, err := searchDependencyOCIFilesByPath(serviceManager, searchPattern)
		if err != nil {
			log.Debug("Failed to search OCI layers for ", chartPath)
			return
		}
		if len(resultMap) == 0 {
			log.Debug("No OCI layers found for chart: ", chartPath)
			return
		}
		artifactManifest, err := getManifest(resultMap, serviceManager, repo)
		if err != nil {
			log.Debug("Failed to get manifest")
			return
		}
		if artifactManifest == nil {
			log.Debug("Could not find image manifest in Artifactory")
			return
		}
		layerDigests := make([]struct{ Digest, MediaType string }, len(artifactManifest.Layers))
		for i, layerItem := range artifactManifest.Layers {
			layerDigests[i] = struct{ Digest, MediaType string }{
				Digest:    layerItem.Digest,
				MediaType: layerItem.MediaType,
			}
		}
		artifactsLayers, err := ocicontainer.ExtractLayersFromManifestData(resultMap, artifactManifest.Config.Digest, layerDigests)
		if err != nil {
			log.Debug("Failed to extract OCI layers for ", chartPath)
			return
		}
		var dependencies []entities.Dependency
		for _, artLayer := range artifactsLayers {
			dependencies = append(dependencies, artLayer.ToDependency())
		}
		if buildInfo != nil && len(buildInfo.Modules) > 0 {
			buildInfo.Modules[0].Dependencies = dependencies
		}
	}
}

func getRepoAndChartName(path string) (string, string) {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return "", ""
	}
	return parts[len(parts)-2], parts[len(parts)-1]
}
