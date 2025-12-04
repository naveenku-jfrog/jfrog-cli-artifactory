package helm

import (
	"fmt"

	"github.com/jfrog/build-info-go/entities"
	ioutils "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// addDeployedHelmArtifactsToBuildInfo adds deployed Helm chart artifacts to build info
func addDeployedHelmArtifactsToBuildInfo(buildInfo *entities.BuildInfo, workingDir string) error {
	serverDetails, err := getHelmServerDetails()
	if err != nil {
		return fmt.Errorf("failed to get server details: %w", err)
	}

	if serverDetails == nil {
		log.Debug("No server details configured, skipping artifact collection")
		return nil
	}

	serviceManager, err := createServiceManager(serverDetails)
	if err != nil {
		return fmt.Errorf("failed to create services manager: %w", err)
	}

	chartName, chartVersion, err := getHelmChartInfo(workingDir)
	if err != nil {
		log.Debug("Could not get chart info, skipping artifact collection: " + err.Error())
		return err
	}

	repoName, err := getHelmRepositoryFromArgs()
	if err != nil {
		log.Debug("Could not determine Helm repository, skipping artifact collection: " + err.Error())
		return err
	}

	artifacts, err := searchHelmChartArtifacts(chartName, chartVersion, repoName, serviceManager)
	if err != nil {
		return fmt.Errorf("failed to search for Helm chart artifacts: %w", err)
	}

	if len(artifacts) == 0 {
		log.Debug("No Helm chart artifacts found in Artifactory")
		return nil
	}

	return addArtifactsToModules(buildInfo, artifacts)
}

// createServiceManager creates a service manager from server details
func createServiceManager(serverDetails *config.ServerDetails) (artifactory.ArtifactoryServicesManager, error) {
	serviceManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create services manager: %w", err)
	}

	return serviceManager, nil
}

// addArtifactsToModules adds artifacts to all modules in build info
func addArtifactsToModules(buildInfo *entities.BuildInfo, artifacts []entities.Artifact) error {
	if len(buildInfo.Modules) == 0 {
		log.Warn("No modules found in build info, cannot add artifacts")
		return nil
	}

	for moduleIdx := range buildInfo.Modules {
		buildInfo.Modules[moduleIdx].Artifacts = append(buildInfo.Modules[moduleIdx].Artifacts, artifacts...)
		log.Debug(fmt.Sprintf("Added %d Helm chart artifacts to module[%d]: %s", len(artifacts), moduleIdx, buildInfo.Modules[moduleIdx].Id))
	}

	log.Info(fmt.Sprintf("Added %d Helm chart artifacts to build info with checksums from Artifactory across %d modules", len(artifacts), len(buildInfo.Modules)))
	return nil
}

// searchHelmChartArtifacts searches Artifactory for Helm chart artifacts and retrieves checksums
func searchHelmChartArtifacts(chartName, chartVersion, repoName string, serviceManager artifactory.ArtifactoryServicesManager) ([]entities.Artifact, error) {
	searchPattern := fmt.Sprintf("%s/%s/%s/*", repoName, chartName, chartVersion)
	log.Debug(fmt.Sprintf("Searching for Helm chart artifacts with pattern: %s", searchPattern))

	searchParams := services.NewSearchParams()
	searchParams.Pattern = searchPattern
	searchParams.Recursive = true

	reader, err := serviceManager.SearchFiles(searchParams)
	if err != nil {
		return nil, fmt.Errorf("failed to search for Helm chart artifacts: %w", err)
	}

	var closeErr error
	defer func() {
		if closeErr != nil {
			log.Debug(fmt.Sprintf("Failed to close search reader: %v", closeErr))
		}
		ioutils.Close(reader, &closeErr)
	}()

	artifacts := collectArtifactsFromSearch(reader)
	return artifacts, nil
}

// collectArtifactsFromSearch collects artifacts from search results
func collectArtifactsFromSearch(reader *content.ContentReader) []entities.Artifact {
	var artifacts []entities.Artifact

	for resultItem := new(servicesUtils.ResultItem); reader.NextRecord(resultItem) == nil; resultItem = new(servicesUtils.ResultItem) {
		if resultItem.Type == "folder" {
			continue
		}

		artifact := resultItem.ToArtifact()
		artifacts = append(artifacts, artifact)
		log.Debug(fmt.Sprintf("Including artifact: %s (path: %s/%s, modified: %s)",
			artifact.Name, artifact.Path, artifact.Name, resultItem.Modified))
	}

	log.Debug(fmt.Sprintf("Total artifacts found and included: %d", len(artifacts)))

	if len(artifacts) == 0 {
		log.Debug("No Helm chart artifacts found in Artifactory")
		return nil
	}

	return artifacts
}
