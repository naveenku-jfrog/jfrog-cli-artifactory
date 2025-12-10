package helm

import (
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	"github.com/jfrog/jfrog-client-go/artifactory"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/gofrog/crypto"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"helm.sh/helm/v3/pkg/registry"
)

func handlePushCommand(buildInfo *entities.BuildInfo, helmArgs []string, serviceManager artifactory.ArtifactoryServicesManager) {
	filePath, registryURL := getPushChartPathAndRegistryURL(helmArgs)
	if filePath == "" || registryURL == "" {
		return
	}
	chartName, chartVersion, err := getChartDetails(filePath)
	if err != nil {
		log.Debug("Could not extract chart name/version from artifact: ", filePath)
		return
	}
	appendModuleAndBuildAgentIfAbsent(buildInfo, chartName, chartVersion)
	log.Debug("Processing push command for chart: ", filePath, " to registry: ", registryURL)
	deploymentPath := getUploadedFileDeploymentPath(registryURL)
	repoName := extractRepositoryNameFromURL(registryURL)
	fileDetails, err := crypto.GetFileDetails(filePath, true)
	if err != nil {
		return
	}
	fileName := filePath
	if strings.Contains(filePath, "/") {
		parts := strings.Split(filePath, "/")
		fileName = parts[len(parts)-1]
	}
	isOCI := registry.IsOCI(registryURL)
	if !isOCI {
		artifact := entities.Artifact{
			Name:                   fileName,
			Path:                   deploymentPath,
			OriginalDeploymentRepo: repoName,
			Checksum: entities.Checksum{
				Sha1:   fileDetails.Checksum.Sha1,
				Md5:    fileDetails.Checksum.Md5,
				Sha256: fileDetails.Checksum.Sha256,
			},
		}
		buildInfo.Modules[0].Artifacts = append(buildInfo.Modules[0].Artifacts, artifact)
		return
	}
	searchPattern := fmt.Sprintf("%s/%s/%s/", repoName, chartName, chartVersion)
	resultMap, err := searchDependencyOCIFilesByPath(serviceManager, searchPattern)
	if err != nil {
		log.Debug("Failed to search OCI artifacts for ", chartName, " : ", chartVersion)
		return
	}
	if len(resultMap) == 0 {
		log.Debug("No OCI artifacts found for chart: ", chartName, " : ", chartVersion)
		return
	}
	artifactManifest, err := getManifest(resultMap, serviceManager, repoName)
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
		log.Debug("Failed to extract OCI artifacts for ", chartName, " : ", chartVersion)
		return
	}
	var artifacts []entities.Artifact
	for _, artLayer := range artifactsLayers {
		artifacts = append(artifacts, artLayer.ToArtifact())
	}
	buildInfo.Modules[0].Artifacts = artifacts
}
