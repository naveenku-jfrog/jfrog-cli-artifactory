package helm

import (
	"fmt"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"path"
	"strings"

	"github.com/jfrog/gofrog/crypto"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/registry"
	orasregistry "oras.land/oras-go/v2/registry"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

func handlePushCommand(buildInfo *entities.BuildInfo, helmArgs []string, serviceManager artifactory.ArtifactoryServicesManager) {
	filePath := getPushChartPath(helmArgs)
	if filePath == "" {
		return
	}
	registryURL := getPushRegistryURL(helmArgs)
	if registryURL == "" {
		return
	}
	log.Debug(fmt.Sprintf("Processing push command for chart: %s to registry: %s", filePath, registryURL))
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
	isOCI := registry.IsOCI(registryURL)
	if !isOCI {
		if buildInfo != nil && len(buildInfo.Modules) > 0 {
			buildInfo.Modules[0].Artifacts = append(buildInfo.Modules[0].Artifacts, artifact)
		}
		return
	}
	var artifacts []entities.Artifact
	var searchPattern string
	chartName, chartVersion, err := getChartDetails(artifact.Name)
	if err != nil {
		log.Debug(fmt.Sprintf("Could not extract chart name/version from artifact: %s", artifact.Name))
		return
	}
	searchPattern = fmt.Sprintf("%s/%s/%s/", artifact.Path, chartName, chartVersion)
	ociArtifacts, err := searchDependencyOCIFilesByPath(serviceManager, searchPattern)
	if err != nil {
		log.Debug(fmt.Sprintf("Failed to search OCI artifacts for chart at path %s: %v", artifact.Path, err))
		return
	}
	for _, ociArtifact := range ociArtifacts {
		artifacts = append(artifacts, entities.Artifact{
			Name:                   ociArtifact.Name,
			Path:                   path.Join(ociArtifact.Path, ociArtifact.Name),
			OriginalDeploymentRepo: ociArtifact.Repo,
			Checksum: entities.Checksum{
				Sha1:   ociArtifact.Actual_Sha1,
				Md5:    ociArtifact.Actual_Md5,
				Sha256: ociArtifact.Sha256,
			},
		})
	}
	if buildInfo != nil && len(buildInfo.Modules) > 0 {
		buildInfo.Modules[0].Artifacts = artifacts
	}
}

// getPushChartPath extracts the chart path from helm push command arguments
func getPushChartPath(helmArgs []string) string {
	for i := 1; i < len(helmArgs); i++ {
		arg := helmArgs[i]
		if strings.HasPrefix(arg, "--") {
			if i+1 < len(helmArgs) && !strings.HasPrefix(helmArgs[i+1], "--") {
				i++
			}
			continue
		}
		return arg
	}
	return ""
}

// getPushRegistryURL extracts the registry URL from helm push command arguments
func getPushRegistryURL(helmArgs []string) string {
	positionalCount := 0
	for i := 1; i < len(helmArgs); i++ {
		arg := helmArgs[i]
		if strings.HasPrefix(arg, "--") {
			if i+1 < len(helmArgs) && !strings.HasPrefix(helmArgs[i+1], "--") {
				i++
			}
			continue
		}
		positionalCount++
		if positionalCount == 2 {
			return arg
		}
	}
	return ""
}

// getUploadedFileDeploymentPath extracts the deployment path from the OCI registry URL argument
// Example: oci://example.com/my-repo/folder -> returns "my-repo/folder"
func getUploadedFileDeploymentPath(registryURL string) string {
	if registryURL == "" {
		return ""
	}
	raw := strings.TrimPrefix(registryURL, registry.OCIScheme+"://")
	ref, err := parseOCIReference(raw)
	if err != nil {
		log.Debug(fmt.Sprintf("Failed to parse OCI reference %s: %v", registryURL, err))
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
