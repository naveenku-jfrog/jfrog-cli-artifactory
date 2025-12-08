package helm

import (
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	"github.com/jfrog/jfrog-client-go/artifactory"

	"github.com/jfrog/gofrog/crypto"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/registry"
	orasregistry "oras.land/oras-go/v2/registry"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

func handlePushCommand(buildInfo *entities.BuildInfo, helmArgs []string, serviceManager artifactory.ArtifactoryServicesManager) {
	filePath, registryURL := getPushChartPathAndRegistryURL(helmArgs)
	if filePath == "" || registryURL == "" {
		return
	}
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
		if buildInfo != nil && len(buildInfo.Modules) > 0 {
			buildInfo.Modules[0].Artifacts = append(buildInfo.Modules[0].Artifacts, artifact)
		}
		return
	}
	var searchPattern string
	chartName, chartVersion, err := getChartDetails(fileName)
	if err != nil {
		log.Debug("Could not extract chart name/version from artifact: ", fileName)
		return
	}
	searchPattern = fmt.Sprintf("%s/%s/%s/", repoName, chartName, chartVersion)
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
	if buildInfo != nil && len(buildInfo.Modules) > 0 {
		buildInfo.Modules[0].Artifacts = artifacts
	}
}

func getPushChartPathAndRegistryURL(helmArgs []string) (chartPath, registryURL string) {
	booleanFlags := map[string]bool{
		"--debug": true, "--plain-http": true, "--insecure-skip-tls-verify": true,
		"--verify": true, "--dry-run": true, "--help": true,
	}
	var positionalArgs []string
	for i := 0; i < len(helmArgs); i++ {
		arg := helmArgs[i]
		if arg == "push" {
			continue
		}
		if strings.HasPrefix(arg, "--") {
			if strings.Contains(arg, "=") {
				continue
			}
			if booleanFlags[arg] {
				continue
			}
			if i+1 < len(helmArgs) {
				i++
			}
			continue
		}
		positionalArgs = append(positionalArgs, arg)
	}
	if len(positionalArgs) > 0 {
		chartPath = positionalArgs[0]
	}
	if len(positionalArgs) > 1 {
		registryURL = positionalArgs[1]
	}
	return
}

// getUploadedFileDeploymentPath extracts the deployment path from the OCI registry URL argument
func getUploadedFileDeploymentPath(registryURL string) string {
	if registryURL == "" {
		return ""
	}
	raw := strings.TrimPrefix(registryURL, registry.OCIScheme+"://")
	ref, err := parseOCIReference(raw)
	if err != nil {
		log.Debug("Failed to parse OCI reference ", registryURL, " : ", err)
		return ""
	}
	return ref.Repository
}

// parseOCIReference parses an OCI reference using the same approach as Helm SDK
func parseOCIReference(raw string) (*ociReference, error) {
	orasRef, err := orasregistry.ParseReference(raw)
	if err != nil {
		return nil, err
	}
	return &ociReference{
		Registry:   orasRef.Registry,
		Repository: orasRef.Repository,
		Reference:  orasRef.Reference,
	}, nil
}

// ociReference represents a parsed OCI reference (similar to Helm SDK's reference struct)
type ociReference struct {
	Registry   string
	Repository string
	Reference  string
}

func getChartDetails(filePath string) (string, string, error) {
	chart, err := loader.Load(filePath)
	if err != nil {
		return "", "", err
	}
	name := chart.Metadata.Name
	version := chart.Metadata.Version
	return name, version, nil
}
