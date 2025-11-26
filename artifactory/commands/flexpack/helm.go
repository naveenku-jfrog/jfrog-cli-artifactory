package flexpack

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jfrog/build-info-go/build"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/flexpack"
	ioutils "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"gopkg.in/yaml.v3"
)

// CollectHelmBuildInfoWithFlexPack collects Helm build info using FlexPack
// This follows the same pattern as Maven FlexPack in maven.go
func CollectHelmBuildInfoWithFlexPack(workingDir, buildName, buildNumber string, buildConfiguration *buildUtils.BuildConfiguration) error {
	var buildInfo *entities.BuildInfo
	var err error

	// Check if this is a command we handle with full build info collection
	switch {
	case wasHelmPushCommand():
		// For push commands: create build info with only module and artifacts (no dependencies)
		log.Debug(fmt.Sprintf("Creating build info for %s/%s (push command - artifacts only, no dependencies)", buildName, buildNumber))
		buildInfo = createHelmBuildInfoWithoutDependencies(buildName, buildNumber, workingDir)

		// Add deployed artifacts
		err = addDeployedHelmArtifactsToBuildInfo(buildInfo, workingDir)
		if err != nil {
			log.Warn("Failed to add deployed artifacts to build info: " + err.Error())
		}
		// Note: We don't add dependency OCI artifacts for push commands
	case wasHelmPackageCommand():
		// For package commands: collect build info with dependencies (dependencies are packaged in .tgz)
		// Create Helm FlexPack configuration
		config := flexpack.HelmConfig{
			WorkingDirectory: workingDir,
		}

		// Create Helm FlexPack instance
		helmFlex, err := flexpack.NewHelmFlexPack(config)
		if err != nil {
			return fmt.Errorf("failed to create Helm FlexPack: %w", err)
		}

		// Collect build info using FlexPack (includes dependencies)
		log.Debug(fmt.Sprintf("Collecting Helm build info for %s/%s in directory: %s", buildName, buildNumber, workingDir))
		buildInfo, err = helmFlex.CollectBuildInfo(buildName, buildNumber)
		if err != nil {
			return fmt.Errorf("failed to collect build info with FlexPack: %w", err)
		}
		log.Debug(fmt.Sprintf("Collected build info with %d modules", len(buildInfo.Modules)))
		for i, module := range buildInfo.Modules {
			log.Debug(fmt.Sprintf("Module[%d] ID: %s, Dependencies: %d", i, module.Id, len(module.Dependencies)))
		}

		// Add deployed artifacts (the packaged .tgz file)
		err = addDeployedHelmArtifactsToBuildInfo(buildInfo, workingDir)
		if err != nil {
			log.Warn("Failed to add deployed artifacts to build info: " + err.Error())
		}

		// Resolve version ranges to actual versions from Chart.lock
		err = resolveDependencyVersionsFromChartLock(buildInfo, workingDir)
		if err != nil {
			log.Debug("Failed to resolve dependency versions from Chart.lock: " + err.Error())
		}

		// Add OCI artifacts (manifest.json, config) for dependencies using local checksums
		err = addDependencyOCIArtifactsFromLocalChecksums(buildInfo, workingDir)
		if err != nil {
			log.Debug("Failed to add dependency OCI artifacts from local checksums: " + err.Error())
		}
	case wasHelmDependencyCommand() || wasHelmInstallOrUpgradeCommand():
		// Create Helm FlexPack configuration
		config := flexpack.HelmConfig{
			WorkingDirectory: workingDir,
		}

		// Create Helm FlexPack instance
		helmFlex, err := flexpack.NewHelmFlexPack(config)
		if err != nil {
			return fmt.Errorf("failed to create Helm FlexPack: %w", err)
		}

		// Collect build info using FlexPack (includes dependencies)
		log.Debug(fmt.Sprintf("Collecting Helm build info for %s/%s in directory: %s", buildName, buildNumber, workingDir))
		buildInfo, err = helmFlex.CollectBuildInfo(buildName, buildNumber)
		if err != nil {
			return fmt.Errorf("failed to collect build info with FlexPack: %w", err)
		}
		log.Debug(fmt.Sprintf("Collected build info with %d modules", len(buildInfo.Modules)))
		for i, module := range buildInfo.Modules {
			log.Debug(fmt.Sprintf("Module[%d] ID: %s, Dependencies: %d", i, module.Id, len(module.Dependencies)))
		}
	default:
		// For other commands, create empty build info
		log.Debug(fmt.Sprintf("Creating empty build info for %s/%s (command not requiring full build info)", buildName, buildNumber))
		buildInfo = createEmptyHelmBuildInfo(buildName, buildNumber)
	}

	// Handle different command types
	switch {
	case wasHelmDependencyCommand():
		err = resolveDependencyVersionsFromChartLock(buildInfo, workingDir)
		if err != nil {
			log.Debug("Failed to resolve dependency versions from Chart.lock: " + err.Error())
		}
		// Then add OCI artifacts (manifest.json, config) for dependencies
		err = addDependencyOCIArtifactsFromLocalChecksums(buildInfo, workingDir)
		if err != nil {
			log.Debug("Failed to add dependency OCI artifacts from local checksums: " + err.Error())
		}
	case wasHelmInstallOrUpgradeCommand():
		// For install/upgrade commands: get dependencies from helm template and add them
		err = addDependenciesFromHelmTemplate(buildInfo, workingDir)
		if err != nil {
			log.Warn("Failed to add dependencies from helm template: " + err.Error())
		}
	}

	// Save FlexPack build info for jfrog-cli rt bp compatibility
	err = saveHelmFlexPackBuildInfo(buildInfo)
	if err != nil {
		log.Warn("Failed to save build info for jfrog-cli compatibility: " + err.Error())
	} else {
		log.Info("Build info saved locally. Use 'jf rt bp " + buildName + " " + buildNumber + "' to publish it to Artifactory.")
	}

	// Set build properties on deployed artifacts if this was a push command
	// Note: package command doesn't deploy to a repository, so we skip setting properties
	if wasHelmPushCommand() {
		err = setHelmBuildPropertiesOnArtifacts(buildInfo, buildName, buildNumber)
		if err != nil {
			log.Warn("Failed to set build properties on deployed artifacts: " + err.Error())
		}
	}

	return nil
}

// createEmptyHelmBuildInfo creates an empty build info structure with only basic fields
func createEmptyHelmBuildInfo(buildName, buildNumber string) *entities.BuildInfo {
	buildInfo := entities.New()
	buildInfo.Name = buildName
	buildInfo.Number = buildNumber
	buildInfo.SetAgentName(coreutils.GetCliUserAgentName())
	buildInfo.SetAgentVersion(coreutils.GetCliUserAgentVersion())
	buildInfo.SetBuildAgentVersion(coreutils.GetClientAgentVersion())
	buildInfo.Started = time.Now().Format(entities.TimeFormat)
	serverDetails, err := config.GetDefaultServerConf()
	if err == nil && serverDetails != nil && serverDetails.User != "" {
		buildInfo.Principal = serverDetails.User
	}
	return buildInfo
}

// createHelmBuildInfoWithoutDependencies creates a build info structure with only module and properties (no dependencies)
func createHelmBuildInfoWithoutDependencies(buildName, buildNumber, workingDir string) *entities.BuildInfo {
	buildInfo := entities.New()
	buildInfo.Name = buildName
	buildInfo.Number = buildNumber

	// Set agent information
	buildInfo.SetAgentName(coreutils.GetCliUserAgentName())
	buildInfo.SetAgentVersion(coreutils.GetCliUserAgentVersion())
	buildInfo.SetBuildAgentVersion(coreutils.GetClientAgentVersion())

	// Set started time (current time)
	buildInfo.Started = time.Now().Format(entities.TimeFormat)

	// Set principal from server config if available
	serverDetails, err := config.GetDefaultServerConf()
	if err == nil && serverDetails != nil && serverDetails.User != "" {
		buildInfo.Principal = serverDetails.User
	}

	// Get chart info from Chart.yaml
	chartName, chartVersion, err := getHelmChartInfo(workingDir)
	if err != nil {
		log.Debug(fmt.Sprintf("Could not read Chart.yaml: %v, using default module ID", err))
		chartName = "helm-chart"
		chartVersion = "unknown"
	}

	// Create module with properties but no dependencies
	properties := make(map[string]string)
	chartYamlPath := filepath.Join(workingDir, "Chart.yaml")
	if data, err := os.ReadFile(chartYamlPath); err == nil {
		var chartYAML struct {
			Type        string `yaml:"type"`
			AppVersion  string `yaml:"appVersion"`
			Description string `yaml:"description"`
		}
		if err := yaml.Unmarshal(data, &chartYAML); err == nil {
			if chartYAML.Type != "" {
				properties["helm.chart.type"] = chartYAML.Type
			}
			if chartYAML.AppVersion != "" {
				properties["helm.chart.appVersion"] = chartYAML.AppVersion
			}
			if chartYAML.Description != "" {
				properties["helm.chart.description"] = chartYAML.Description
			}
		}
	}

	module := entities.Module{
		Id:           fmt.Sprintf("%s:%s", chartName, chartVersion),
		Type:         entities.Helm,
		Properties:   properties,
		Artifacts:    []entities.Artifact{},   // Will be populated by addDeployedHelmArtifactsToBuildInfo
		Dependencies: []entities.Dependency{}, // Empty - no dependencies for push commands
	}

	buildInfo.Modules = []entities.Module{module}
	return buildInfo
}

// saveHelmFlexPackBuildInfo saves Helm FlexPack build info for jfrog-cli rt bp compatibility
func saveHelmFlexPackBuildInfo(buildInfo *entities.BuildInfo) error {
	service := build.NewBuildInfoService()
	buildInstance, err := service.GetOrCreateBuildWithProject(buildInfo.Name, buildInfo.Number, "")
	if err != nil {
		return fmt.Errorf("failed to create build: %w", err)
	}
	return buildInstance.SaveBuildInfo(buildInfo)
}

// getHelmCommandPosition finds the position of "helm" in os.Args and returns the index of the command after it
// Returns -1 if "helm" is not found or if there's no command after it
func getHelmCommandPosition() int {
	args := os.Args
	for i, arg := range args {
		if arg == "helm" && i+1 < len(args) {
			return i + 1
		}
	}
	return -1
}

// wasHelmPushCommand checks if the current command was a Helm push command
// This is used to determine if we should set build properties (only for push, not package)
func wasHelmPushCommand() bool {
	cmdPos := getHelmCommandPosition()
	if cmdPos == -1 {
		return false
	}
	args := os.Args
	if cmdPos < len(args) && args[cmdPos] == "push" {
		return true
	}
	return false
}

// wasHelmPackageCommand checks if the current command was a Helm package command
func wasHelmPackageCommand() bool {
	cmdPos := getHelmCommandPosition()
	if cmdPos == -1 {
		return false
	}
	args := os.Args
	if cmdPos < len(args) && args[cmdPos] == "package" {
		return true
	}
	return false
}

// wasHelmDependencyCommand checks if the current command was a Helm dependency update or build command
func wasHelmDependencyCommand() bool {
	cmdPos := getHelmCommandPosition()
	if cmdPos == -1 {
		return false
	}
	args := os.Args
	// Check if command is "dependency" and next arg is "update" or "build"
	if cmdPos < len(args) && args[cmdPos] == "dependency" {
		if cmdPos+1 < len(args) {
			subCmd := args[cmdPos+1]
			return subCmd == "update" || subCmd == "build"
		}
	}
	return false
}

// wasHelmInstallOrUpgradeCommand checks if the current command was a Helm install, upgrade, or template command
func wasHelmInstallOrUpgradeCommand() bool {
	cmdPos := getHelmCommandPosition()
	if cmdPos == -1 {
		return false
	}
	args := os.Args
	if cmdPos < len(args) {
		cmd := args[cmdPos]
		if cmd == "install" || cmd == "upgrade" || cmd == "template" {
			return true
		}
	}
	return false
}

// addDeployedHelmArtifactsToBuildInfo adds deployed Helm chart artifacts to build info
func addDeployedHelmArtifactsToBuildInfo(buildInfo *entities.BuildInfo, workingDir string) error {
	// Get server details from configuration
	serverDetails, err := config.GetDefaultServerConf()
	if err != nil {
		return fmt.Errorf("failed to get server details: %w", err)
	}

	if serverDetails == nil {
		log.Debug("No server details configured, skipping artifact collection")
		return nil
	}

	// Create service manager
	serviceManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create services manager: %w", err)
	}

	// Get chart name and version from Chart.yaml
	chartName, chartVersion, err := getHelmChartInfo(workingDir)
	if err != nil {
		log.Debug("Could not get chart info, skipping artifact collection: " + err.Error())
		return err
	}

	// Try to extract repository from helm args or use default
	repoName, err := getHelmRepositoryFromArgs(serviceManager)
	if err != nil {
		log.Debug("Could not determine Helm repository, skipping artifact collection: " + err.Error())
		return err
	}

	// Search for the chart in Artifactory (following Docker pattern)
	artifacts, err := searchHelmChartArtifacts(chartName, chartVersion, repoName, serviceManager)
	if err != nil {
		return fmt.Errorf("failed to search for Helm chart artifacts: %w", err)
	}

	if len(artifacts) == 0 {
		log.Debug("No Helm chart artifacts found in Artifactory")
		return nil
	}

	if len(buildInfo.Modules) > 0 {
		for moduleIdx := range buildInfo.Modules {
			buildInfo.Modules[moduleIdx].Artifacts = append(buildInfo.Modules[moduleIdx].Artifacts, artifacts...)
			log.Debug(fmt.Sprintf("Added %d Helm chart artifacts to module[%d]: %s", len(artifacts), moduleIdx, buildInfo.Modules[moduleIdx].Id))
		}
		log.Info(fmt.Sprintf("Added %d Helm chart artifacts to build info with checksums from Artifactory across %d modules", len(artifacts), len(buildInfo.Modules)))
	} else {
		log.Warn("No modules found in build info, cannot add artifacts")
	}

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
	defer ioutils.Close(reader, &err)

	var artifacts []entities.Artifact
	for resultItem := new(servicesUtils.ResultItem); reader.NextRecord(resultItem) == nil; resultItem = new(servicesUtils.ResultItem) {
		if resultItem.Type == "folder" {
			continue
		}
		artifact := convertHelmResultItemToArtifact(resultItem)
		artifacts = append(artifacts, artifact)
		log.Debug(fmt.Sprintf("Including artifact: %s (path: %s/%s, modified: %s)",
			artifact.Name, artifact.Path, artifact.Name, resultItem.Modified))
	}

	log.Debug(fmt.Sprintf("Total artifacts found and included: %d", len(artifacts)))

	if len(artifacts) == 0 {
		log.Debug("No Helm chart artifacts found in Artifactory")
		return nil, nil
	}

	return artifacts, nil
}

// convertHelmResultItemToArtifact converts a ResultItem to entities.Artifact with proper type field
func convertHelmResultItemToArtifact(item *servicesUtils.ResultItem) entities.Artifact {
	artifact := item.ToArtifact()

	// Type field is not set - it will be omitted from JSON output
	artifact.Type = ""

	return artifact
}

// getHelmChartInfo extracts chart name and version from Chart.yaml
func getHelmChartInfo(workingDir string) (string, string, error) {
	chartYamlPath := filepath.Join(workingDir, "Chart.yaml")
	data, err := os.ReadFile(chartYamlPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read Chart.yaml: %w", err)
	}

	var chartYAML struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	}

	if err := yaml.Unmarshal(data, &chartYAML); err != nil {
		return "", "", fmt.Errorf("failed to parse Chart.yaml: %w", err)
	}

	return chartYAML.Name, chartYAML.Version, nil
}

// getHelmRepositoryFromArgs tries to extract repository name from helm command arguments
func getHelmRepositoryFromArgs(_ artifactory.ArtifactoryServicesManager) (string, error) {
	args := os.Args
	for i, arg := range args {
		// Look for registry URL in helm push command: helm push <chart> <registry-url>
		if arg == "push" && i+2 < len(args) {
			registryURL := args[i+2]
			// Extract repo from registry URL
			// Format: oci://<host>/artifactory/<repo> or oci://<host>/<repo>
			registryURL = strings.TrimPrefix(registryURL, "oci://")
			if strings.Contains(registryURL, "://") {
				parts := strings.Split(registryURL, "://")
				if len(parts) > 1 {
					registryURL = parts[1]
				}
			}

			parts := strings.Split(registryURL, "/")
			for j, part := range parts {
				if part == "artifactory" && j+1 < len(parts) {
					return parts[j+1], nil
				}
			}
			// If no "artifactory" found, return the last part
			if len(parts) > 0 {
				return parts[len(parts)-1], nil
			}
		}
	}
	return "", fmt.Errorf("could not extract repository from helm command arguments")
}

// setHelmBuildPropertiesOnArtifacts sets build properties on deployed Helm chart artifacts
// Uses artifacts already in buildInfo instead of searching again to avoid duplicate AQL queries
func setHelmBuildPropertiesOnArtifacts(buildInfo *entities.BuildInfo, buildName, buildNumber string) error {
	if buildInfo == nil || len(buildInfo.Modules) == 0 {
		return nil
	}

	serverDetails, err := config.GetDefaultServerConf()
	if err != nil {
		return fmt.Errorf("failed to get server details: %w", err)
	}
	if serverDetails == nil {
		log.Debug("No server details configured, skipping build properties setting")
		return nil
	}

	serviceManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create services manager: %w", err)
	}

	// Get repository name (needed for setting properties)
	repoName, err := getHelmRepositoryFromArgs(serviceManager)
	if err != nil {
		log.Warn("Could not determine Helm repository, skipping build properties: " + err.Error())
		return err
	}

	// Extract artifacts from buildInfo and convert to ResultItem format
	resultItems := extractArtifactsFromBuildInfo(buildInfo, repoName)
	if len(resultItems) == 0 {
		log.Debug("No artifacts found in build info to set properties on")
		return nil
	}

	buildProps := createBuildPropertiesString(buildName, buildNumber)
	return setPropertiesOnArtifactsList(serviceManager, resultItems, buildProps)
}

// extractArtifactsFromBuildInfo extracts artifacts from buildInfo and converts them to ResultItem format
func extractArtifactsFromBuildInfo(buildInfo *entities.BuildInfo, defaultRepo string) []servicesUtils.ResultItem {
	var resultItems []servicesUtils.ResultItem

	for _, module := range buildInfo.Modules {
		for _, artifact := range module.Artifacts {
			// Get repo from artifact's OriginalDeploymentRepo if available, otherwise use default
			repo := defaultRepo
			if artifact.OriginalDeploymentRepo != "" {
				repo = artifact.OriginalDeploymentRepo
			}

			path := artifact.Path
			name := artifact.Name

			// Remove the filename from the path if it's at the end
			if name != "" && strings.HasSuffix(path, name) {
				path = strings.TrimSuffix(path, name)
				path = strings.TrimSuffix(path, "/")
			}

			resultItem := servicesUtils.ResultItem{
				Repo: repo,
				Path: path,
				Name: name,
			}
			resultItems = append(resultItems, resultItem)
		}
	}

	return resultItems
}

// createBuildPropertiesString creates build properties string
func createBuildPropertiesString(buildName, buildNumber string) string {
	timestamp := fmt.Sprintf("%d", time.Now().UnixNano()/int64(time.Millisecond))
	return fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", buildName, buildNumber, timestamp)
}

// setPropertiesOnArtifactsList sets build properties on all artifacts in a single batch call
func setPropertiesOnArtifactsList(serviceManager artifactory.ArtifactoryServicesManager, artifacts []servicesUtils.ResultItem, buildProps string) error {
	if len(artifacts) == 0 {
		return nil
	}

	filePath, err := writeResultItemsToFile(artifacts)
	if err != nil {
		return fmt.Errorf("failed to write artifacts to file: %w", err)
	}
	defer func() {
		if removeErr := os.Remove(filePath); removeErr != nil {
			log.Debug(fmt.Sprintf("Failed to remove temporary file %s: %s", filePath, removeErr))
		}
	}()

	reader := content.NewContentReader(filePath, content.DefaultKey)
	defer ioutils.Close(reader, &err)

	propsParams := services.NewPropsParams()
	propsParams.Reader = reader
	propsParams.Props = buildProps

	_, err = serviceManager.SetProps(propsParams)
	if err != nil {
		return fmt.Errorf("failed to set properties on artifacts: %w", err)
	}

	log.Info("Successfully set build properties on deployed Helm artifacts")
	return nil
}

// writeResultItemsToFile writes ResultItems to a temporary file and returns the file path
func writeResultItemsToFile(items []servicesUtils.ResultItem) (filePath string, err error) {
	writer, err := content.NewContentWriter("results", true, false)
	if err != nil {
		return "", fmt.Errorf("failed to create content writer: %w", err)
	}

	// Write all items
	for _, item := range items {
		writer.Write(item)
	}

	// Close writer to ensure all data is written (ContentWriter writes asynchronously)
	filePath = writer.GetFilePath()
	if closeErr := writer.Close(); closeErr != nil {
		return "", fmt.Errorf("failed to close writer: %w", closeErr)
	}

	return filePath, nil
}

// addDependencyOCIArtifactsFromLocalChecksums adds OCI artifacts (manifest.json, config) for dependencies
func addDependencyOCIArtifactsFromLocalChecksums(buildInfo *entities.BuildInfo, workingDir string) error {
	if buildInfo == nil || len(buildInfo.Modules) == 0 {
		return nil
	}
	serviceManager, err := createServiceManagerForDependencies()
	if err != nil {
		return err
	}
	if serviceManager == nil {
		return nil
	}

	for moduleIdx := range buildInfo.Modules {
		module := &buildInfo.Modules[moduleIdx]
		processModuleDependenciesForOCI(module, moduleIdx, buildInfo, serviceManager, workingDir)
	}
	return nil
}

// createServiceManagerForDependencies creates a service manager for dependency operations
func createServiceManagerForDependencies() (artifactory.ArtifactoryServicesManager, error) {
	serverDetails, err := config.GetDefaultServerConf()
	if err != nil {
		return nil, fmt.Errorf("failed to get server details: %w", err)
	}
	if serverDetails == nil {
		log.Debug("No server details configured, skipping dependency OCI artifact collection")
		return nil, nil
	}
	serviceManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create services manager: %w", err)
	}
	return serviceManager, nil
}

// processModuleDependenciesForOCI processes dependencies for a module and adds OCI artifacts
func processModuleDependenciesForOCI(module *entities.Module, moduleIdx int, buildInfo *entities.BuildInfo, serviceManager artifactory.ArtifactoryServicesManager, workingDir string) {
	log.Debug(fmt.Sprintf("Processing module[%d]: %s with %d dependencies", moduleIdx, module.Id, len(module.Dependencies)))
	processedDeps := make(map[string]bool)
	for i := range module.Dependencies {
		dep := &module.Dependencies[i]
		if processedDeps[dep.Id] || dep.Sha256 == "" {
			if dep.Sha256 == "" {
				log.Debug(fmt.Sprintf("Dependency %s has no SHA256 checksum, skipping OCI artifact search", dep.Id))
			}
			continue
		}
		processedDeps[dep.Id] = true

		if err := processDependencyOCIArtifacts(dep, i, module, moduleIdx, buildInfo, serviceManager, workingDir); err != nil {
			log.Warn(fmt.Sprintf("Failed to process OCI artifacts for dependency %s: %v", dep.Id, err))
		}
	}
	log.Debug(fmt.Sprintf("Module[%d] %s: Processed %d dependencies", moduleIdx, module.Id, len(module.Dependencies)))
}

// processDependencyOCIArtifacts processes OCI artifacts for a single dependency
func processDependencyOCIArtifacts(dep *entities.Dependency, depIdx int, module *entities.Module, moduleIdx int, buildInfo *entities.BuildInfo, serviceManager artifactory.ArtifactoryServicesManager, workingDir string) error {
	layerSha256 := dep.Sha256
	log.Debug(fmt.Sprintf("Searching for OCI artifacts for dependency %s", dep.Id))

	versionPath, depRepo := extractDependencyPathAndRepo(dep.Id, workingDir, serviceManager)
	if versionPath == "" {
		// Fallback to SHA256 search if we can't extract name/version
		log.Debug(fmt.Sprintf("Could not extract version path from dependency ID %s, falling back to SHA256 search", dep.Id))
		ociArtifacts, dirPath, err := searchDependencyOCIFilesByLayerSha256(layerSha256, serviceManager, versionPath, depRepo)
		if err != nil {
			return fmt.Errorf("failed to search OCI artifacts: %w", err)
		}
		log.Debug(fmt.Sprintf("Found OCI artifacts for dependency %s in directory: %s", dep.Id, dirPath))
		return addOCIArtifactsToDependencies(dep, depIdx, module, moduleIdx, buildInfo, ociArtifacts, layerSha256)
	}

	// Search directly by name/version path (more efficient than SHA256 search)
	log.Debug(fmt.Sprintf("Searching for OCI artifacts for dependency %s in repository %s, path: %s", dep.Id, depRepo, versionPath))
	ociArtifacts, dirPath, err := searchDependencyOCIFilesByPath(serviceManager, depRepo, versionPath, "", layerSha256)
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to search OCI artifacts for dependency %s in path %s/%s: %v", dep.Id, depRepo, versionPath, err))
		// Try fallback to SHA256 search
		log.Debug(fmt.Sprintf("Attempting fallback SHA256 search for dependency %s", dep.Id))
		ociArtifacts, dirPath, err = searchDependencyOCIFilesByLayerSha256(layerSha256, serviceManager, versionPath, depRepo)
		if err != nil {
			return fmt.Errorf("failed to search OCI artifacts (both path and SHA256 search failed): %w", err)
		}
		log.Debug(fmt.Sprintf("Found OCI artifacts for dependency %s using SHA256 fallback in directory: %s", dep.Id, dirPath))
	} else {
		log.Debug(fmt.Sprintf("Found OCI artifacts for dependency %s in directory: %s", dep.Id, dirPath))
	}
	return addOCIArtifactsToDependencies(dep, depIdx, module, moduleIdx, buildInfo, ociArtifacts, layerSha256)
}

// extractDependencyPathAndRepo extracts version path and repository for a dependency
func extractDependencyPathAndRepo(depId, workingDir string, serviceManager artifactory.ArtifactoryServicesManager) (string, string) {
	var versionPath string
	parts := strings.Split(depId, ":")
	if len(parts) == 2 {
		versionPath = fmt.Sprintf("%s/%s", parts[0], parts[1])
	}
	depRepo := getRepositoryForDependency(parts[0], workingDir)
	if depRepo == "" {
		depRepo, _ = getHelmRepositoryFromArgs(serviceManager)
	}
	return versionPath, depRepo
}

// addOCIArtifactsToDependencies adds OCI artifacts to dependencies, updating IDs and adding new dependencies
func addOCIArtifactsToDependencies(dep *entities.Dependency, depIdx int, module *entities.Module, moduleIdx int, buildInfo *entities.BuildInfo, ociArtifacts map[string]*servicesUtils.ResultItem, layerSha256 string) error {
	layerFileName := fmt.Sprintf("sha256__%s", layerSha256)
	for name, resultItem := range ociArtifacts {
		if name == layerFileName {
			updateDependencyID(dep, depIdx, moduleIdx, buildInfo, resultItem, name)
			continue
		}
		addOCIDependency(module, resultItem, name)
	}
	return nil
}

// updateDependencyID updates the dependency ID to include the full path from Artifactory
func updateDependencyID(dep *entities.Dependency, depIdx, moduleIdx int, buildInfo *entities.BuildInfo, resultItem *servicesUtils.ResultItem, layerFileName string) {
	oldId := dep.Id
	var newId string
	if resultItem.Path != "" {
		newId = fmt.Sprintf("%s/%s", resultItem.Path, resultItem.Name)
	} else {
		newId = resultItem.Name
	}
	buildInfo.Modules[moduleIdx].Dependencies[depIdx].Id = newId
	log.Debug(fmt.Sprintf("Updated dependency ID from '%s' to '%s' (layer file: %s, repo: %s, path: %s, name: %s)",
		oldId, newId, layerFileName, resultItem.Repo, resultItem.Path, resultItem.Name))
	log.Info(fmt.Sprintf("Updated dependency ID: %s -> %s", oldId, newId))
	log.Debug(fmt.Sprintf("Skipping layer file %s for dependency %s (already present as main dependency)", layerFileName, dep.Id))
}

// addOCIDependency adds an OCI artifact as a separate dependency
func addOCIDependency(module *entities.Module, resultItem *servicesUtils.ResultItem, name string) {
	ociDependency := entities.Dependency{
		Id: fmt.Sprintf("%s/%s", resultItem.Path, resultItem.Name),
		Checksum: entities.Checksum{
			Sha1:   resultItem.Actual_Sha1,
			Sha256: resultItem.Sha256,
			Md5:    resultItem.Actual_Md5,
		},
	}
	module.Dependencies = append(module.Dependencies, ociDependency)
	log.Debug(fmt.Sprintf("Added OCI artifact as dependency: %s (path: %s/%s)",
		name, resultItem.Path, resultItem.Name))
}

// searchDependencyOCIFilesByLayerSha256 searches for OCI artifacts (manifest.json, config) using AQL query with the layer SHA256
func searchDependencyOCIFilesByLayerSha256(layerSha256 string, serviceManager artifactory.ArtifactoryServicesManager, versionPath string, repository string) (map[string]*servicesUtils.ResultItem, string, error) {
	layerFile, err := findLayerFileBySha256(layerSha256, serviceManager, repository)
	if err != nil {
		return nil, "", err
	}

	possiblePaths := buildPossibleOCIPaths(versionPath, layerFile.Path)
	return searchOCIFilesInPaths(serviceManager, layerFile.Repo, possiblePaths, layerSha256)
}

// findLayerFileBySha256 finds the layer file by its SHA256 checksum
func findLayerFileBySha256(layerSha256 string, serviceManager artifactory.ArtifactoryServicesManager, repository string) (*servicesUtils.ResultItem, error) {
	log.Debug(fmt.Sprintf("Searching for dependency layer file using AQL with SHA256: %s (repository: %s)", layerSha256, repository))
	layerQuery := createAqlQueryForLayerFile(layerSha256, repository)
	log.Debug(fmt.Sprintf("AQL Query for layer file: %s", layerQuery))

	stream, err := serviceManager.Aql(layerQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to execute AQL query for layer file: %w", err)
	}
	defer ioutils.Close(stream, &err)

	result, err := io.ReadAll(stream)
	if err != nil {
		return nil, fmt.Errorf("failed to read AQL query result: %w", err)
	}

	parsedResult := new(servicesUtils.AqlSearchResult)
	if err := json.Unmarshal(result, parsedResult); err != nil {
		return nil, fmt.Errorf("failed to parse AQL result: %w", err)
	}

	if len(parsedResult.Results) == 0 {
		return nil, fmt.Errorf("layer file not found with SHA256: %s", layerSha256)
	}

	layerFile := &parsedResult.Results[0]
	log.Debug(fmt.Sprintf("Found layer file at: %s/%s/%s", layerFile.Repo, layerFile.Path, layerFile.Name))
	return layerFile, nil
}

// buildPossibleOCIPaths builds a list of possible paths where OCI artifacts might be located
func buildPossibleOCIPaths(versionPath, layerPath string) []string {
	var possiblePaths []string
	if versionPath != "" {
		possiblePaths = append(possiblePaths, versionPath)
		log.Debug(fmt.Sprintf("Will also search in version-specific directory: %s", versionPath))
	}
	possiblePaths = append(possiblePaths, layerPath)

	// Try to extract version-specific path from layer path (fallback)
	pathParts := strings.Split(layerPath, "/")
	if len(pathParts) >= 2 && pathParts[len(pathParts)-1] == "_uploads" {
		parentPath := strings.Join(pathParts[:len(pathParts)-1], "/")
		if parentPath != "" && parentPath != versionPath {
			possiblePaths = append(possiblePaths, parentPath)
		}
	}
	log.Debug(fmt.Sprintf("Searching for OCI artifacts in directories: %v", possiblePaths))
	return possiblePaths
}

// searchOCIFilesInPaths searches for OCI files in multiple possible paths
func searchOCIFilesInPaths(serviceManager artifactory.ArtifactoryServicesManager, repo string, possiblePaths []string, layerSha256 string) (map[string]*servicesUtils.ResultItem, string, error) {
	ociArtifacts := make(map[string]*servicesUtils.ResultItem)
	for _, searchPath := range possiblePaths {
		found, dirPath, err := searchOCIFilesInPath(serviceManager, repo, searchPath)
		if err != nil {
			log.Debug(fmt.Sprintf("Failed to search OCI artifacts in path %s: %v", searchPath, err))
			continue
		}
		if len(found) > 0 {
			for name, item := range found {
				if _, exists := ociArtifacts[name]; !exists {
					ociArtifacts[name] = item
				}
			}
			return ociArtifacts, dirPath, nil
		}
	}

	if len(ociArtifacts) == 0 {
		return nil, "", fmt.Errorf("no OCI artifacts found for layer SHA256: %s in any of the searched paths", layerSha256)
	}
	return ociArtifacts, "", nil
}

// searchOCIFilesInPath searches for OCI files in a specific path
func searchOCIFilesInPath(serviceManager artifactory.ArtifactoryServicesManager, repo, searchPath string) (map[string]*servicesUtils.ResultItem, string, error) {
	log.Debug(fmt.Sprintf("Searching for OCI artifacts in directory: %s/%s", repo, searchPath))
	ociQuery := createAqlQueryForOCIFilesInDirectory(repo, searchPath)
	log.Debug(fmt.Sprintf("AQL Query for OCI artifacts: %s", ociQuery))

	ociStream, err := serviceManager.Aql(ociQuery)
	if err != nil {
		return nil, "", err
	}
	defer ioutils.Close(ociStream, &err)

	ociResult, err := io.ReadAll(ociStream)
	if err != nil {
		return nil, "", err
	}

	ociParsedResult := new(servicesUtils.AqlSearchResult)
	if err := json.Unmarshal(ociResult, ociParsedResult); err != nil {
		return nil, "", err
	}

	if len(ociParsedResult.Results) == 0 {
		return nil, "", nil
	}

	artifacts := make(map[string]*servicesUtils.ResultItem)
	for _, resultItem := range ociParsedResult.Results {
		itemCopy := resultItem
		artifacts[resultItem.Name] = &itemCopy
		log.Debug(fmt.Sprintf("Found OCI artifact: %s (path: %s/%s, sha256: %s) in search path: %s",
			resultItem.Name, resultItem.Path, resultItem.Name, resultItem.Sha256, searchPath))
	}

	dirPath := fmt.Sprintf("%s/%s", repo, searchPath)
	return artifacts, dirPath, nil
}

// createAqlQueryForLayerFile creates an AQL query to find the OCI layer file by its SHA256 checksum
func createAqlQueryForLayerFile(layerSha256 string, repository string) string {
	sha256Hex := strings.TrimPrefix(layerSha256, "sha256:")
	var query string
	if repository != "" {
		query = fmt.Sprintf(`items.find({
		"repo": "%s",
		"sha256": "%s"
	}).include("repo", "path", "name", "sha256", "actual_sha1", "actual_md5")`, repository, sha256Hex)
	} else {
		query = fmt.Sprintf(`items.find({
		"sha256": "%s"
	}).include("repo", "path", "name", "sha256", "actual_sha1", "actual_md5")`, sha256Hex)
	}
	return query
}

// searchDependencyOCIFilesByPath searches for OCI artifacts directly by chart name/version path
func searchDependencyOCIFilesByPath(serviceManager artifactory.ArtifactoryServicesManager, repo, versionPath, _ string, layerSha256 string) (map[string]*servicesUtils.ResultItem, string, error) {
	log.Debug(fmt.Sprintf("Searching for OCI artifacts in path: %s/%s", repo, versionPath))

	// Search for OCI artifacts directly in the version-specific directory
	ociQuery := createAqlQueryForOCIFilesInDirectory(repo, versionPath)
	log.Debug(fmt.Sprintf("AQL Query for OCI artifacts: %s", ociQuery))

	ociStream, err := serviceManager.Aql(ociQuery)
	if err != nil {
		return nil, "", fmt.Errorf("failed to execute AQL query for OCI artifacts: %w", err)
	}
	defer ioutils.Close(ociStream, &err)

	ociResult, err := io.ReadAll(ociStream)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read AQL query result: %w", err)
	}

	ociParsedResult := new(servicesUtils.AqlSearchResult)
	if err := json.Unmarshal(ociResult, ociParsedResult); err != nil {
		return nil, "", fmt.Errorf("failed to parse AQL result: %w", err)
	}

	if len(ociParsedResult.Results) == 0 {
		// If not found in version path, try parent directory as fallback
		log.Debug(fmt.Sprintf("No OCI artifacts found in %s/%s, trying parent directory", repo, versionPath))
		pathParts := strings.Split(versionPath, "/")
		if len(pathParts) > 1 {
			parentPath := pathParts[0]
			log.Debug(fmt.Sprintf("Trying parent path: %s/%s", repo, parentPath))
			return searchDependencyOCIFilesByPath(serviceManager, repo, parentPath, "", layerSha256)
		}
		log.Debug(fmt.Sprintf("No OCI artifacts found for dependency in path: %s/%s (no parent directory to try)", repo, versionPath))
		return nil, "", fmt.Errorf("no OCI artifacts found for dependency in path: %s/%s", repo, versionPath)
	}

	// Build artifacts map
	ociArtifacts := make(map[string]*servicesUtils.ResultItem)
	for _, resultItem := range ociParsedResult.Results {
		itemCopy := resultItem
		ociArtifacts[resultItem.Name] = &itemCopy
		log.Debug(fmt.Sprintf("Found OCI artifact: %s (path: %s/%s, sha256: %s)",
			resultItem.Name, resultItem.Path, resultItem.Name, resultItem.Sha256))
	}

	dirPath := fmt.Sprintf("%s/%s", repo, versionPath)
	return ociArtifacts, dirPath, nil
}

// createAqlQueryForOCIFilesInDirectory creates an AQL query to find all OCI artifacts in a specific directory
func createAqlQueryForOCIFilesInDirectory(repo, dirPath string) string {
	// AQL query to find manifest.json and all sha256__* files in the directory
	// Note: In Artifactory AQL, checksum fields are: sha256 (correct), actual_sha1, actual_md5
	query := fmt.Sprintf(`items.find({
		"repo": "%s",
		"path": "%s",
		"$or": [
			{"name": {"$match": "manifest.json"}},
			{"name": {"$match": "sha256__*"}}
		]
	}).include("repo", "path", "name", "sha256", "actual_sha1", "actual_md5")`, repo, dirPath)
	return query
}

// resolveHelmRepositoryAlias resolves a Helm repository alias (e.g., "@bitnami") to its URL
// by reading from Helm's repositories.yaml configuration file
func resolveHelmRepositoryAlias(alias string) (string, error) {
	// Clean the alias (remove the "@" prefix)
	repoName := strings.TrimPrefix(alias, "@")

	// Find the repositories.yaml file path
	var configPath string
	if envPath := os.Getenv("HELM_REPOSITORY_CONFIG"); envPath != "" {
		// Validate path to prevent path traversal attacks
		if strings.Contains(envPath, "..") {
			return "", fmt.Errorf("path traversal detected in HELM_REPOSITORY_CONFIG: %s", envPath)
		}
		absPath, err := filepath.Abs(envPath)
		if err != nil {
			return "", fmt.Errorf("failed to resolve absolute path for HELM_REPOSITORY_CONFIG: %w", err)
		}
		cleanedPath := filepath.Clean(absPath)
		if cleanedPath != absPath {
			return "", fmt.Errorf("invalid path detected in HELM_REPOSITORY_CONFIG: %s", envPath)
		}
		configPath = cleanedPath
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}

		// Platform-specific default paths
		switch runtime.GOOS {
		case "darwin": // macOS
			configPath = filepath.Join(home, "Library/Preferences/helm/repositories.yaml")
		case "linux":
			configPath = filepath.Join(home, ".config/helm/repositories.yaml")
		case "windows":
			configPath = filepath.Join(home, "AppData/Roaming/helm/repositories.yaml")
		default:
			return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
		}
	}

	// Read the file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read repositories.yaml at %s: %w", configPath, err)
	}

	// Parse YAML
	var repoFile struct {
		Repositories []struct {
			Name string `yaml:"name"`
			Url  string `yaml:"url"`
		} `yaml:"repositories"`
	}

	if err := yaml.Unmarshal(data, &repoFile); err != nil {
		return "", fmt.Errorf("failed to parse repositories.yaml: %w", err)
	}

	// Find the matching repository
	for _, repo := range repoFile.Repositories {
		if repo.Name == repoName {
			return repo.Url, nil
		}
	}

	return "", fmt.Errorf("repository alias '%s' not found in Helm repositories.yaml", alias)
}

// getRepositoryForDependency extracts the repository from Chart.yaml for a specific dependency
// Returns the repository name (extracted from OCI URL if needed) or empty string if not found
// Handles repository aliases (e.g., "@bitnami") by resolving them from Helm's repositories.yaml
func getRepositoryForDependency(depName, workingDir string) string {
	chartYamlPath := filepath.Join(workingDir, "Chart.yaml")
	data, err := os.ReadFile(chartYamlPath)
	if err != nil {
		log.Debug(fmt.Sprintf("Could not read Chart.yaml to get repository for dependency %s: %v", depName, err))
		return ""
	}

	var chartYAML struct {
		Dependencies []struct {
			Name       string `yaml:"name"`
			Repository string `yaml:"repository"`
		} `yaml:"dependencies"`
	}

	if err := yaml.Unmarshal(data, &chartYAML); err != nil {
		log.Debug(fmt.Sprintf("Could not parse Chart.yaml to get repository for dependency %s: %v", depName, err))
		return ""
	}

	// Find the dependency and extract repository
	for _, dep := range chartYAML.Dependencies {
		if dep.Name == depName {
			if dep.Repository == "" {
				return ""
			}

			repoURL := dep.Repository

			// Check if repository is an alias (starts with "@")
			if strings.HasPrefix(repoURL, "@") {
				resolvedURL, err := resolveHelmRepositoryAlias(repoURL)
				if err != nil {
					log.Debug(fmt.Sprintf("Failed to resolve repository alias %s for dependency %s: %v", repoURL, depName, err))
					return ""
				}
				repoURL = resolvedURL
			}

			// Extract repository name from OCI URL
			// Format: oci://<host>/artifactory/<repo> or oci://<host>/<repo>
			repoURL = strings.TrimPrefix(repoURL, "oci://")
			if strings.Contains(repoURL, "://") {
				parts := strings.Split(repoURL, "://")
				if len(parts) > 1 {
					repoURL = parts[1]
				}
			}

			parts := strings.Split(repoURL, "/")
			for j, part := range parts {
				if part == "artifactory" && j+1 < len(parts) {
					return parts[j+1]
				}
			}
			// If no "artifactory" found, return the last part
			if len(parts) > 0 {
				return parts[len(parts)-1]
			}
			return ""
		}
	}

	return ""
}

// addDependenciesFromHelmTemplate collects dependencies for install/upgrade/template commands
func addDependenciesFromHelmTemplate(buildInfo *entities.BuildInfo, workingDir string) error {
	if buildInfo == nil || len(buildInfo.Modules) == 0 {
		return nil
	}

	serviceManager, err := createServiceManagerForDependencies()
	if err != nil {
		return err
	}
	if serviceManager == nil {
		return nil
	}
	isTemplateCmd := isHelmTemplateCommand()
	if !isTemplateCmd {
		if err := runHelmTemplateForValidation(workingDir); err != nil {
			log.Debug(fmt.Sprintf("Helm template validation failed (non-fatal): %v", err))
		}
	}
	dependencies, err := getFilteredDependencies(workingDir, isTemplateCmd)
	if err != nil {
		return err
	}
	if len(dependencies) == 0 {
		return nil
	}
	return addDependenciesToModules(buildInfo, dependencies, serviceManager, workingDir, isTemplateCmd)
}

// runHelmTemplateForValidation runs helm template for validation
func runHelmTemplateForValidation(workingDir string) error {
	helmArgs := extractHelmArgsForTemplate()

	log.Debug(fmt.Sprintf("Running helm template with arguments: %v (for validation)", helmArgs))
	templateCmd := exec.Command("helm", append([]string{"template"}, helmArgs...)...)
	templateCmd.Dir = workingDir
	templateOutput, err := templateCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm template validation failed: %v, output: %s", err, string(templateOutput))
	}

	log.Debug(fmt.Sprintf("Helm template validation successful (output length: %d bytes)", len(templateOutput)))
	return nil
}

// getFilteredDependencies gets dependencies and filters them based on command type
func getFilteredDependencies(workingDir string, isTemplateCmd bool) ([]entities.Dependency, error) {
	allDependencies, err := getDependenciesFromChartYAML(workingDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get dependencies from Chart.yaml: %w", err)
	}

	if isTemplateCmd {
		log.Debug("Using all dependencies from Chart.yaml (template command doesn't download dependencies)")
		return allDependencies, nil
	}

	actualDependencies, err := filterDependenciesByChartsDirectory(allDependencies, workingDir)
	if err != nil {
		log.Debug(fmt.Sprintf("Failed to filter dependencies by charts directory: %v, using all dependencies", err))
		return allDependencies, nil
	}
	return actualDependencies, nil
}

// addDependenciesToModules adds dependencies to all modules in build info
func addDependenciesToModules(buildInfo *entities.BuildInfo, dependencies []entities.Dependency, serviceManager artifactory.ArtifactoryServicesManager, workingDir string, isTemplateCmd bool) error {
	log.Debug(fmt.Sprintf("Found %d dependencies in Chart.yaml", len(dependencies)))

	hasChecksums := checkIfDependenciesHaveChecksums(buildInfo)
	if !hasChecksums {
		// Dependencies don't have checksums yet, try to find them by name/version
		for moduleIdx := range buildInfo.Modules {
			module := &buildInfo.Modules[moduleIdx]
			addDependenciesToModule(module, moduleIdx, dependencies, serviceManager, workingDir)
		}
	} else {
		log.Debug("Dependencies already have checksums from build-info-go, skipping name/version search")
	}

	// Add OCI artifacts (manifest.json, config) for dependencies using their SHA256 checksums
	if err := addDependencyOCIArtifactsFromArtifactory(buildInfo, workingDir); err != nil {
		log.Debug("Failed to add dependency OCI artifacts from Artifactory: " + err.Error())
	}

	// Deduplicate dependencies after OCI artifacts are added (IDs may have been updated)
	deduplicateDependencies(buildInfo)

	commandType := "install/upgrade"
	if isTemplateCmd {
		commandType = "template"
	}

	// Count actual dependencies added (from buildInfo, not from Chart.yaml)
	totalDeps := 0
	for _, module := range buildInfo.Modules {
		totalDeps += len(module.Dependencies)
	}
	log.Info(fmt.Sprintf("Added %d dependencies from helm %s to build info", totalDeps, commandType))
	return nil
}

// checkIfDependenciesHaveChecksums checks if dependencies in buildInfo already have SHA256 checksums
func checkIfDependenciesHaveChecksums(buildInfo *entities.BuildInfo) bool {
	for _, module := range buildInfo.Modules {
		for _, dep := range module.Dependencies {
			if dep.Sha256 != "" {
				return true
			}
		}
	}
	return false
}

// addDependenciesToModule adds dependencies to a single module by searching Artifactory
func addDependenciesToModule(module *entities.Module, moduleIdx int, dependencies []entities.Dependency, serviceManager artifactory.ArtifactoryServicesManager, workingDir string) {
	log.Debug(fmt.Sprintf("Processing module[%d]: %s", moduleIdx, module.Id))

	for _, dep := range dependencies {
		depEntity, err := searchDependencyInArtifactory(dep, serviceManager, workingDir)
		if err != nil {
			log.Debug(fmt.Sprintf("Failed to find dependency %s in Artifactory: %v", dep.Id, err))
			continue
		}

		module.Dependencies = append(module.Dependencies, depEntity)
		log.Debug(fmt.Sprintf("Added dependency: %s (sha256: %s)", depEntity.Id, depEntity.Sha256))
	}
}

// isHelmTemplateCommand checks if the current command was a Helm template command
func isHelmTemplateCommand() bool {
	cmdPos := getHelmCommandPosition()
	if cmdPos == -1 {
		return false
	}
	args := os.Args
	if cmdPos < len(args) && args[cmdPos] == "template" {
		return true
	}
	return false
}

// extractHelmArgsForTemplate extracts helm arguments from os.Args, removing build flags
// Returns arguments suitable for helm template command (without "install"/"upgrade"/"template" command name)
func extractHelmArgsForTemplate() []string {
	args := os.Args
	var helmArgs []string
	foundHelm := false
	skipNext := false
	skipCommand := false // Skip install/upgrade/template command name

	for i, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}

		// Find where "helm" command starts
		if arg == "helm" && i+1 < len(args) {
			foundHelm = true
			skipCommand = true // Next arg will be install/upgrade/template, skip it
			continue
		}

		if !foundHelm {
			continue
		}

		// Skip the command name (install/upgrade/template) and --install flag
		if skipCommand {
			skipCommand = false
			if arg == "install" || arg == "upgrade" || arg == "template" {
				continue
			}
			// If it's "upgrade --install", handle --install flag
			if arg == "--install" {
				continue
			}
		}

		// Skip build flags
		if arg == "--build-name" || arg == "--build-number" || arg == "--project" || arg == "--module" {
			skipNext = true // Skip the value
			continue
		}
		if strings.HasPrefix(arg, "--build-name=") || strings.HasPrefix(arg, "--build-number=") ||
			strings.HasPrefix(arg, "--project=") || strings.HasPrefix(arg, "--module=") {
			continue
		}

		// Skip server-id flag
		if arg == "--server-id" {
			skipNext = true
			continue
		}
		if strings.HasPrefix(arg, "--server-id=") {
			continue
		}

		// Skip skip-login flag
		if arg == "--skip-login" {
			continue
		}

		// Add all other arguments
		helmArgs = append(helmArgs, arg)
	}

	return helmArgs
}

// getDependenciesFromChartYAML reads Chart.yaml and returns dependency information
func getDependenciesFromChartYAML(workingDir string) ([]entities.Dependency, error) {
	chartYamlPath := filepath.Join(workingDir, "Chart.yaml")
	data, err := os.ReadFile(chartYamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read Chart.yaml: %w", err)
	}

	var chartYAML struct {
		Dependencies []struct {
			Name       string `yaml:"name"`
			Version    string `yaml:"version"`
			Repository string `yaml:"repository"`
		} `yaml:"dependencies"`
	}

	if err := yaml.Unmarshal(data, &chartYAML); err != nil {
		return nil, fmt.Errorf("failed to parse Chart.yaml: %w", err)
	}

	// Try to get resolved versions from Chart.lock
	lockVersions := make(map[string]string)
	lockPath := filepath.Join(workingDir, "Chart.lock")
	if lockData, err := os.ReadFile(lockPath); err == nil {
		var lockYAML struct {
			Dependencies []struct {
				Name    string `yaml:"name"`
				Version string `yaml:"version"`
			} `yaml:"dependencies"`
		}
		if err := yaml.Unmarshal(lockData, &lockYAML); err == nil {
			for _, dep := range lockYAML.Dependencies {
				lockVersions[dep.Name] = dep.Version
			}
		}
	}

	var dependencies []entities.Dependency
	for _, dep := range chartYAML.Dependencies {
		version := dep.Version
		// Use resolved version from Chart.lock if available
		if resolvedVersion, found := lockVersions[dep.Name]; found {
			version = resolvedVersion
		}
		dependencies = append(dependencies, entities.Dependency{
			Id: fmt.Sprintf("%s:%s", dep.Name, version),
		})
	}

	return dependencies, nil
}

// filterDependenciesByChartsDirectory filters dependencies to only include those that are actually
// present in the charts/ directory. This ensures we only include dependencies whose conditions
// evaluated to true during helm install/upgrade.
func filterDependenciesByChartsDirectory(dependencies []entities.Dependency, workingDir string) ([]entities.Dependency, error) {
	chartsDir := filepath.Join(workingDir, "charts")
	actualChartNames, err := buildChartNamesMap(chartsDir)
	if err != nil {
		return nil, err
	}

	log.Debug(fmt.Sprintf("Found %d chart entries in charts/ directory: %v", len(actualChartNames), actualChartNames))
	return filterDependenciesByChartNames(dependencies, actualChartNames), nil
}

// buildChartNamesMap builds a map of chart names found in the charts directory
func buildChartNamesMap(chartsDir string) (map[string]bool, error) {
	info, err := os.Stat(chartsDir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug("charts/ directory does not exist, no dependencies were included")
			return make(map[string]bool), nil
		}
		return nil, fmt.Errorf("failed to check charts directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("charts path exists but is not a directory")
	}

	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read charts directory: %w", err)
	}

	actualChartNames := make(map[string]bool)
	for _, entry := range entries {
		addChartNameVariants(entry.Name(), actualChartNames)
	}

	return actualChartNames, nil
}

// addChartNameVariants adds chart name variants to the map
func addChartNameVariants(name string, chartNames map[string]bool) {
	baseName := strings.TrimSuffix(name, ".tgz")
	chartNames[baseName] = true

	parts := strings.Split(baseName, "-")
	if len(parts) >= 2 {
		chartNames[parts[0]] = true
	}
}

// filterDependenciesByChartNames filters dependencies based on chart names map
func filterDependenciesByChartNames(dependencies []entities.Dependency, actualChartNames map[string]bool) []entities.Dependency {
	var filteredDeps []entities.Dependency
	for _, dep := range dependencies {
		if isDependencyInChartsDirectory(dep, actualChartNames) {
			filteredDeps = append(filteredDeps, dep)
			log.Debug(fmt.Sprintf("Including dependency %s (found in charts/ directory)", dep.Id))
		} else {
			log.Debug(fmt.Sprintf("Excluding dependency %s (not found in charts/ directory, condition may be false or values disabled it)", dep.Id))
		}
	}

	log.Debug(fmt.Sprintf("Filtered %d dependencies from %d total (based on charts/ directory)", len(filteredDeps), len(dependencies)))
	return filteredDeps
}

// isDependencyInChartsDirectory checks if a dependency is present in the charts directory
func isDependencyInChartsDirectory(dep entities.Dependency, actualChartNames map[string]bool) bool {
	parts := strings.Split(dep.Id, ":")
	if len(parts) != 2 {
		log.Debug(fmt.Sprintf("Skipping dependency with invalid ID format: %s", dep.Id))
		return false
	}
	depName := parts[0]

	for chartName := range actualChartNames {
		if chartName == depName || strings.HasPrefix(chartName, depName+"-") {
			return true
		}
	}
	return false
}

// resolveDependencyVersionsFromChartLock resolves version ranges in dependency IDs to actual versions from Chart.lock
// This is needed because build-info-go might use version ranges from Chart.yaml instead of resolved versions from Chart.lock
func resolveDependencyVersionsFromChartLock(buildInfo *entities.BuildInfo, workingDir string) error {
	if buildInfo == nil || len(buildInfo.Modules) == 0 {
		return nil
	}

	lockVersions, err := readResolvedVersionsFromChartLock(workingDir)
	if err != nil {
		return err
	}
	if len(lockVersions) == 0 {
		return nil
	}

	log.Debug(fmt.Sprintf("Resolved %d dependency versions from Chart.lock", len(lockVersions)))
	return updateDependencyVersionsInModules(buildInfo, lockVersions)
}

// readResolvedVersionsFromChartLock reads resolved versions from Chart.lock
func readResolvedVersionsFromChartLock(workingDir string) (map[string]string, error) {
	lockPath := filepath.Join(workingDir, "Chart.lock")
	lockData, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug("Chart.lock not found, skipping version resolution")
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read Chart.lock: %w", err)
	}

	var chartLock struct {
		Dependencies []struct {
			Name    string `yaml:"name"`
			Version string `yaml:"version"`
		} `yaml:"dependencies"`
	}

	if err := yaml.Unmarshal(lockData, &chartLock); err != nil {
		return nil, fmt.Errorf("failed to parse Chart.lock: %w", err)
	}

	lockVersions := make(map[string]string)
	for _, lockDep := range chartLock.Dependencies {
		lockVersions[lockDep.Name] = lockDep.Version
	}

	if len(lockVersions) == 0 {
		log.Debug("No dependencies found in Chart.lock")
	}

	return lockVersions, nil
}

// updateDependencyVersionsInModules updates dependency versions in all modules
func updateDependencyVersionsInModules(buildInfo *entities.BuildInfo, lockVersions map[string]string) error {
	for moduleIdx := range buildInfo.Modules {
		module := &buildInfo.Modules[moduleIdx]
		for i := range module.Dependencies {
			dep := &module.Dependencies[i]
			updateDependencyVersionIfNeeded(dep, lockVersions)
		}
	}
	return nil
}

// updateDependencyVersionIfNeeded updates a dependency version if it's a range and a resolved version exists
func updateDependencyVersionIfNeeded(dep *entities.Dependency, lockVersions map[string]string) {
	parts := strings.Split(dep.Id, ":")
	if len(parts) != 2 {
		return
	}
	depName := parts[0]
	currentVersion := parts[1]

	if !isVersionRange(currentVersion) {
		return
	}

	if resolvedVersion, found := lockVersions[depName]; found {
		oldId := dep.Id
		newId := fmt.Sprintf("%s:%s", depName, resolvedVersion)
		dep.Id = newId
		log.Debug(fmt.Sprintf("Resolved dependency version: %s -> %s", oldId, newId))
	} else {
		log.Debug(fmt.Sprintf("No resolved version found in Chart.lock for dependency: %s", depName))
	}
}

// isVersionRange checks if a version string is a version range
func isVersionRange(version string) bool {
	return strings.Contains(version, "x") ||
		strings.Contains(version, "*") ||
		strings.Contains(version, ">") ||
		strings.Contains(version, "<") ||
		strings.Contains(version, "~") ||
		strings.Contains(version, "^")
}

// searchDependencyInArtifactory searches for a dependency in Artifactory and returns its checksums
func searchDependencyInArtifactory(dep entities.Dependency, serviceManager artifactory.ArtifactoryServicesManager, workingDir string) (entities.Dependency, error) {
	// Extract name and version from dependency ID
	// Format can be: "name:version" or "path/name" (if already updated from Artifactory)
	var depName, depVersion string
	if strings.Contains(dep.Id, ":") {
		// Format: name:version
		parts := strings.Split(dep.Id, ":")
		if len(parts) != 2 {
			return dep, fmt.Errorf("invalid dependency ID format: %s", dep.Id)
		}
		depName = parts[0]
		depVersion = parts[1]
	} else {
		pathParts := strings.Split(dep.Id, "/")
		if len(pathParts) >= 2 {
			// First part is the name, second part is usually the version
			depName = pathParts[0]
			if len(pathParts) >= 2 {
				depVersion = pathParts[1]
			} else {
				return dep, fmt.Errorf("cannot extract version from dependency ID: %s", dep.Id)
			}
		} else {
			return dep, fmt.Errorf("cannot extract name/version from dependency ID: %s", dep.Id)
		}
	}

	// Get repository for this dependency from Chart.yaml
	depRepo := getRepositoryForDependency(depName, workingDir)
	if depRepo == "" {
		var err error
		depRepo, err = getHelmRepositoryFromArgs(serviceManager)
		if err != nil {
			return dep, fmt.Errorf("could not determine repository for dependency %s: %w", dep.Id, err)
		}
	}

	// Search for the .tar file using name and version
	// OCI Helm charts are stored as: <repo>/<name>/<version>/<name>-<version>.tgz
	searchPattern := fmt.Sprintf("%s/%s/%s/*.tgz", depRepo, depName, depVersion)
	log.Debug(fmt.Sprintf("Searching for dependency %s with pattern: %s", dep.Id, searchPattern))

	searchParams := services.NewSearchParams()
	searchParams.Pattern = searchPattern
	searchParams.Recursive = false

	reader, err := serviceManager.SearchFiles(searchParams)
	if err != nil {
		return dep, fmt.Errorf("failed to search for dependency: %w", err)
	}
	defer ioutils.Close(reader, &err)

	var resultItem *servicesUtils.ResultItem
	for item := new(servicesUtils.ResultItem); reader.NextRecord(item) == nil; item = new(servicesUtils.ResultItem) {
		if item.Type != "folder" {
			resultItem = item
			break
		}
	}

	if resultItem == nil {
		return dep, fmt.Errorf("dependency %s not found in Artifactory", dep.Id)
	}
	dep.Checksum = entities.Checksum{
		Sha1:   resultItem.Actual_Sha1,
		Sha256: resultItem.Sha256,
		Md5:    resultItem.Actual_Md5,
	}
	log.Debug(fmt.Sprintf("Found dependency %s in Artifactory: %s (sha256: %s, path: %s/%s)", depName, dep.Id, dep.Sha256, resultItem.Path, resultItem.Name))
	return dep, nil
}

// addDependencyOCIArtifactsFromArtifactory adds OCI artifacts (manifest.json, config) for dependencies
// by searching Artifactory using the dependency's .tar file SHA256
// This is used for install/upgrade commands where dependencies are found in Artifactory
func addDependencyOCIArtifactsFromArtifactory(buildInfo *entities.BuildInfo, workingDir string) error {
	if buildInfo == nil || len(buildInfo.Modules) == 0 {
		return nil
	}

	serviceManager, err := createServiceManagerForDependencies()
	if err != nil {
		return err
	}
	if serviceManager == nil {
		return nil
	}

	for moduleIdx := range buildInfo.Modules {
		module := &buildInfo.Modules[moduleIdx]
		processModuleDependenciesForOCIFromArtifactory(module, moduleIdx, buildInfo, serviceManager, workingDir)
	}

	return nil
}

// processModuleDependenciesForOCIFromArtifactory processes dependencies for a module and adds OCI artifacts from Artifactory
func processModuleDependenciesForOCIFromArtifactory(module *entities.Module, moduleIdx int, buildInfo *entities.BuildInfo, serviceManager artifactory.ArtifactoryServicesManager, workingDir string) {
	log.Debug(fmt.Sprintf("Processing module[%d]: %s with %d dependencies", moduleIdx, module.Id, len(module.Dependencies)))

	processedDeps := make(map[string]bool)
	for i := range module.Dependencies {
		dep := &module.Dependencies[i]
		if processedDeps[dep.Id] || dep.Sha256 == "" {
			if dep.Sha256 == "" {
				log.Debug(fmt.Sprintf("Dependency %s has no SHA256 checksum, skipping OCI artifact search", dep.Id))
			}
			continue
		}
		processedDeps[dep.Id] = true

		if err := processDependencyOCIArtifactsFromArtifactory(dep, i, module, moduleIdx, buildInfo, serviceManager, workingDir); err != nil {
			log.Debug(fmt.Sprintf("Failed to process OCI artifacts for dependency %s: %v", dep.Id, err))
		}
	}

	log.Debug(fmt.Sprintf("Module[%d] %s: Processed %d dependencies", moduleIdx, module.Id, len(module.Dependencies)))
}

// processDependencyOCIArtifactsFromArtifactory processes OCI artifacts for a single dependency from Artifactory
func processDependencyOCIArtifactsFromArtifactory(dep *entities.Dependency, depIdx int, module *entities.Module, moduleIdx int, buildInfo *entities.BuildInfo, serviceManager artifactory.ArtifactoryServicesManager, workingDir string) error {
	layerSha256 := dep.Sha256
	log.Debug(fmt.Sprintf("Searching for OCI artifacts for dependency %s", dep.Id))

	_, versionPath, depRepo := extractDependencyInfoFromID(dep.Id, workingDir, serviceManager)
	if versionPath == "" {
		// Fallback to SHA256 search if we can't extract name/version
		log.Debug(fmt.Sprintf("Could not extract version path from dependency ID %s, falling back to SHA256 search", dep.Id))
		ociArtifacts, dirPath, err := searchDependencyOCIFilesByLayerSha256(layerSha256, serviceManager, versionPath, depRepo)
		if err != nil {
			return fmt.Errorf("failed to search OCI artifacts: %w", err)
		}
		log.Debug(fmt.Sprintf("Found OCI artifacts for dependency %s in directory: %s", dep.Id, dirPath))
		return addOCIArtifactsToDependencies(dep, depIdx, module, moduleIdx, buildInfo, ociArtifacts, layerSha256)
	}

	// Search directly by name/version path (more efficient than SHA256 search)
	ociArtifacts, dirPath, err := searchDependencyOCIFilesByPath(serviceManager, depRepo, versionPath, "", layerSha256)
	if err != nil {
		return fmt.Errorf("failed to search OCI artifacts: %w", err)
	}

	log.Debug(fmt.Sprintf("Found OCI artifacts for dependency %s in directory: %s", dep.Id, dirPath))
	return addOCIArtifactsToDependencies(dep, depIdx, module, moduleIdx, buildInfo, ociArtifacts, layerSha256)
}

// deduplicateDependencies removes duplicate dependencies from all modules
// Duplicates are identified by matching SHA256 checksums (same dependency with different IDs)
func deduplicateDependencies(buildInfo *entities.BuildInfo) {
	if buildInfo == nil || len(buildInfo.Modules) == 0 {
		return
	}

	for moduleIdx := range buildInfo.Modules {
		module := &buildInfo.Modules[moduleIdx]
		if len(module.Dependencies) == 0 {
			continue
		}

		// Keep the dependency with the most complete path (longest ID)
		seenChecksums := make(map[string]int)
		uniqueDeps := make([]entities.Dependency, 0, len(module.Dependencies))

		for _, dep := range module.Dependencies {
			if dep.Sha256 == "" {
				// If no SHA256, keep it (might be a placeholder or OCI artifact)
				uniqueDeps = append(uniqueDeps, dep)
				continue
			}

			// Check if we've seen this SHA256 before
			if existingIdx, found := seenChecksums[dep.Sha256]; found {
				// Compare IDs - keep the one with the longer/more complete path
				existingDep := uniqueDeps[existingIdx]
				if len(dep.Id) > len(existingDep.Id) {
					uniqueDeps[existingIdx] = dep
					log.Debug(fmt.Sprintf("Removing duplicate dependency %s (keeping %s with same SHA256: %s)",
						existingDep.Id, dep.Id, dep.Sha256))
				} else {
					// Keep the existing one
					log.Debug(fmt.Sprintf("Removing duplicate dependency %s (keeping %s with same SHA256: %s)",
						dep.Id, existingDep.Id, dep.Sha256))
				}
			} else {
				// First time seeing this SHA256, add it
				seenChecksums[dep.Sha256] = len(uniqueDeps)
				uniqueDeps = append(uniqueDeps, dep)
			}
		}

		// Update module dependencies
		if len(uniqueDeps) < len(module.Dependencies) {
			log.Debug(fmt.Sprintf("Deduplicated dependencies in module[%d]: %d -> %d", moduleIdx, len(module.Dependencies), len(uniqueDeps)))
			module.Dependencies = uniqueDeps
		}
	}
}

// extractDependencyInfoFromID extracts dependency name, version path, and repository from dependency ID
func extractDependencyInfoFromID(depId, workingDir string, serviceManager artifactory.ArtifactoryServicesManager) (string, string, string) {
	var depName string
	var versionPath string

	if strings.Contains(depId, ":") {
		parts := strings.Split(depId, ":")
		if len(parts) == 2 {
			depName = parts[0]
			versionPath = fmt.Sprintf("%s/%s", parts[0], parts[1])
		}
	} else {
		pathParts := strings.Split(depId, "/")
		if len(pathParts) >= 2 {
			depName = pathParts[0]
			if len(pathParts) >= 3 {
				versionPath = fmt.Sprintf("%s/%s", pathParts[0], pathParts[1])
			}
		}
	}

	depRepo := getRepositoryForDependency(depName, workingDir)
	if depRepo == "" {
		depRepo, _ = getHelmRepositoryFromArgs(serviceManager)
	}

	return depName, versionPath, depRepo
}
