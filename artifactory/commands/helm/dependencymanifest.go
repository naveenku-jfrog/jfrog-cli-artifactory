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

const (
	Manifest = "manifest.json"
)

// updateDependencyManifestAndConfigFile adds manifest.json and config files for all OCI dependencies
// without duplicating. This should be called after all dependencies are collected from all commands.
func updateDependencyArtifactsChecksumInBuildInfo(buildInfo *entities.BuildInfo) {
	if buildInfo == nil || len(buildInfo.Modules) == 0 {
		return
	}

	serviceManager, err := createServiceManagerForDependencies()
	if err != nil {
		log.Debug("Failed to create service manager for dependency manifest/config: " + err.Error())
		return
	}
	if serviceManager == nil {
		log.Debug("No service manager available, skipping manifest/config file addition")
		return
	}

	processedDeps := make(map[string]bool)
	for moduleIdx := range buildInfo.Modules {
		module := &buildInfo.Modules[moduleIdx]
		if len(module.Dependencies) == 0 {
			continue
		}

		existingManifestsAndConfigs := buildExistingManifestsAndConfigsMap(module)
		processModuleDependencies(module, moduleIdx, buildInfo, serviceManager, processedDeps, existingManifestsAndConfigs)
	}

	deduplicateDependencies(buildInfo)
}

// buildExistingManifestsAndConfigsMap builds a map of base dependency IDs that already have manifest/config
func buildExistingManifestsAndConfigsMap(module *entities.Module) map[string]bool {
	existingManifestsAndConfigs := make(map[string]bool)
	for _, dep := range module.Dependencies {
		if isManifestOrConfig(dep.Id) {
			baseDepId := extractBaseDependencyId(dep.Id)
			if baseDepId != "" {
				existingManifestsAndConfigs[baseDepId] = true
			}
		}
	}
	return existingManifestsAndConfigs
}

// isManifestOrConfig checks if a dependency ID represents a manifest or config file
func isManifestOrConfig(depId string) bool {
	return strings.Contains(depId, Manifest) || (strings.Contains(depId, "sha256__") && !strings.Contains(depId, ".tgz"))
}

// processModuleDependencies processes all dependencies in a module
func processModuleDependencies(module *entities.Module, moduleIdx int, buildInfo *entities.BuildInfo, serviceManager artifactory.ArtifactoryServicesManager, processedDeps, existingManifestsAndConfigs map[string]bool) {
	depIdx := 0
	for depIdx < len(module.Dependencies) {
		dep := &module.Dependencies[depIdx]
		baseDepId := getBaseDependencyId(dep)

		if shouldSkipDependency(dep, baseDepId, processedDeps, existingManifestsAndConfigs) {
			depIdx++
			continue
		}

		shouldIncrement := processDependency(dep, depIdx, module, moduleIdx, buildInfo, serviceManager)
		if shouldIncrement {
			depIdx++
		}

		processedDeps[baseDepId] = true
	}
}

// shouldSkipDependency determines if a dependency should be skipped
func shouldSkipDependency(dep *entities.Dependency, baseDepId string, processedDeps, existingManifestsAndConfigs map[string]bool) bool {
	if processedDeps[baseDepId] {
		return true
	}
	if existingManifestsAndConfigs[baseDepId] {
		log.Debug(fmt.Sprintf("Manifest/config already exist for dependency %s, skipping", baseDepId))
		return true
	}
	if isManifestOrConfig(dep.Id) {
		return true
	}
	return false
}

// processDependency processes a single dependency and returns whether to increment the index
func processDependency(dep *entities.Dependency, depIdx int, module *entities.Module, moduleIdx int, buildInfo *entities.BuildInfo, serviceManager artifactory.ArtifactoryServicesManager) bool {
	if !isOCIRepository(dep.Repository) {
		return processClassicHelmDependency(dep, depIdx, module, serviceManager)
	}

	return processOCIDependency(dep, depIdx, module, serviceManager)
}

// processClassicHelmDependency handles classic Helm dependencies
func processClassicHelmDependency(dep *entities.Dependency, depIdx int, module *entities.Module, serviceManager artifactory.ArtifactoryServicesManager) bool {
	if dep.Sha256 == "" {
		updated, err := updateClassicHelmDependencyChecksums(dep, serviceManager)
		if err != nil {
			log.Debug(fmt.Sprintf("Failed to update checksums for classic Helm dependency %s: %v", dep.Id, err))
			return true
		}
		if updated {
			log.Debug(fmt.Sprintf("Updated checksums for classic Helm dependency %s from Artifactory", dep.Id))
			return true
		}
		removeDependencyByIndex(module, depIdx)
		log.Debug(fmt.Sprintf("Removed classic Helm dependency %s without checksums (not found in Artifactory)", dep.Id))
		return false
	}
	return true
}

// processOCIDependency handles OCI dependencies
func processOCIDependency(dep *entities.Dependency, depIdx int, module *entities.Module, serviceManager artifactory.ArtifactoryServicesManager) bool {
	if dep.Sha256 == "" {
		addedLayers, err := addAllOCILayersForDependency(dep, module, serviceManager)
		if err != nil {
			log.Debug(fmt.Sprintf("Failed to add all OCI layers for dependency %s: %v", dep.Id, err))
			return true
		}
		if addedLayers > 0 {
			removeDependencyByIndex(module, depIdx)
			log.Debug(fmt.Sprintf("Removed dependency %s without checksums after adding %d OCI layers from Artifactory", dep.Id, addedLayers))
			return false
		}
		return true
	}

	if err := addManifestAndConfigForDependency(dep, module, serviceManager); err != nil {
		log.Debug(fmt.Sprintf("Failed to add manifest/config for dependency %s: %v", dep.Id, err))
	}
	return true
}

// getBaseDependencyId extracts the base dependency ID from a dependency
// For example: "repo/chart-name/1.0.0/manifest.json" -> "chart-name:1.0.0"
func getBaseDependencyId(dep *entities.Dependency) string {
	versionPath := extractDependencyPath(dep.Id)
	if versionPath != "" {
		return dep.Id
	}

	parts := strings.Split(dep.Id, "/")
	if len(parts) >= 2 {
		lastPart := parts[len(parts)-1]
		secondLastPart := parts[len(parts)-2]
		if lastPart == Manifest || strings.HasPrefix(lastPart, "sha256__") {
			return fmt.Sprintf("%s:%s", secondLastPart, "unknown")
		}
		return fmt.Sprintf("%s:%s", secondLastPart, lastPart)
	}

	return dep.Id
}

// extractBaseDependencyId extracts base dependency ID from a manifest/config dependency ID
// For example: "repo/chart-name/1.0.0/manifest.json" -> "chart-name:1.0.0"
func extractBaseDependencyId(depId string) string {
	parts := strings.Split(depId, "/")
	if len(parts) < 3 {
		return ""
	}

	name := parts[len(parts)-3]
	version := parts[len(parts)-2]
	return fmt.Sprintf("%s:%s", name, version)
}

// addManifestAndConfigForDependency adds manifest.json and config files for a single OCI dependency
func addManifestAndConfigForDependency(dep *entities.Dependency, module *entities.Module, serviceManager artifactory.ArtifactoryServicesManager) error {
	versionPath := extractDependencyPath(dep.Id)
	if versionPath == "" {
		return fmt.Errorf("could not extract version path from dependency ID %s", dep.Id)
	}

	repoName := extractRepositoryNameFromURL(dep.Repository)
	if repoName == "" {
		return fmt.Errorf("could not extract repo name from: %s", dep.Repository)
	}

	hasManifest, hasConfig := checkExistingManifestAndConfig(module, versionPath)
	if hasManifest && hasConfig {
		return nil
	}

	ociArtifacts, _, err := searchDependencyOCIFilesByPath(serviceManager, dep.Repository, versionPath, "", "")
	if err != nil {
		log.Debug(fmt.Sprintf("Failed to search OCI artifacts for dependency %s: %v", dep.Id, err))
		return nil
	}

	return addManifestAndConfigFromArtifacts(module, ociArtifacts, hasManifest, hasConfig, serviceManager, repoName, dep.Id)
}

// checkExistingManifestAndConfig checks if manifest.json and config already exist in dependencies
func checkExistingManifestAndConfig(module *entities.Module, versionPath string) (bool, bool) {
	hasManifest := false
	hasConfig := false

	for _, existingDep := range module.Dependencies {
		if !strings.Contains(existingDep.Id, versionPath) {
			continue
		}

		if strings.Contains(existingDep.Id, Manifest) {
			hasManifest = true
		}

		if strings.Contains(existingDep.Id, "sha256__") && !strings.Contains(existingDep.Id, Manifest) && !strings.Contains(existingDep.Id, ".tgz") {
			hasConfig = true
		}
	}

	return hasManifest, hasConfig
}

// addManifestAndConfigFromArtifacts adds manifest.json and config files from OCI artifacts
func addManifestAndConfigFromArtifacts(module *entities.Module, ociArtifacts map[string]*servicesUtils.ResultItem, hasManifest, hasConfig bool, serviceManager artifactory.ArtifactoryServicesManager, repoName, depId string) error {
	manifestItem, found := ociArtifacts[Manifest]
	if !found || hasManifest {
		return nil
	}

	addOCIDependency(module, manifestItem)
	log.Debug(fmt.Sprintf("Added manifest.json dependency for %s", depId))

	manifestContent, err := downloadFileContentFromArtifactory(serviceManager, repoName, manifestItem.Path, manifestItem.Name)
	if err != nil {
		return err
	}

	configLayerSha256, _, err := extractLayerChecksumsFromManifest(manifestContent)
	if err != nil {
		return err
	}
	if configLayerSha256 == "" || hasConfig {
		return nil
	}

	configFileName := fmt.Sprintf("sha256__%s", configLayerSha256)
	configItem, found := ociArtifacts[configFileName]
	if !found {
		return nil
	}

	addOCIDependency(module, configItem)
	log.Debug(fmt.Sprintf("Added config dependency for %s", depId))

	return nil
}

// addAllOCILayersForDependency adds all OCI layers for a dependency that doesn't have checksums
// Returns the number of layers added and any error
func addAllOCILayersForDependency(dep *entities.Dependency, module *entities.Module, serviceManager artifactory.ArtifactoryServicesManager) (int, error) {
	versionPath := extractDependencyPath(dep.Id)
	if versionPath == "" {
		return 0, fmt.Errorf("could not extract version path from dependency ID %s", dep.Id)
	}

	repoName := extractRepositoryNameFromURL(dep.Repository)
	if repoName == "" {
		return 0, fmt.Errorf("could not extract repository name from: %s", dep.Repository)
	}

	log.Debug(fmt.Sprintf("Dependency %s has no checksums, searching for all OCI layers in Artifactory", dep.Id))

	ociArtifacts, _, err := searchDependencyOCIFilesByPath(serviceManager, dep.Repository, versionPath, "", "")
	if err != nil {
		log.Debug(fmt.Sprintf("Failed to search OCI artifacts for dependency %s: %v", dep.Id, err))
		return 0, nil
	}

	if len(ociArtifacts) == 0 {
		log.Debug(fmt.Sprintf("No OCI artifacts found for dependency %s", dep.Id))
		return 0, nil
	}

	addedCount := 0
	for name, resultItem := range ociArtifacts {
		if isAlreadyAdded(module, versionPath, name) {
			continue
		}

		addOCIDependency(module, resultItem)
		addedCount++
		log.Debug(fmt.Sprintf("Added OCI layer %s for dependency %s", name, dep.Id))
	}

	if addedCount > 0 {
		log.Debug(fmt.Sprintf("Added %d OCI layers for dependency %s", addedCount, dep.Id))
	}

	return addedCount, nil
}

// isAlreadyAdded checks if an OCI artifact is already present in module dependencies
func isAlreadyAdded(module *entities.Module, versionPath, artifactName string) bool {
	for _, existingDep := range module.Dependencies {
		if strings.Contains(existingDep.Id, versionPath) && strings.Contains(existingDep.Id, artifactName) {
			return true
		}
	}
	return false
}

// removeDependencyByIndex removes a dependency from the module by its index
func removeDependencyByIndex(module *entities.Module, index int) {
	if index < 0 || index >= len(module.Dependencies) {
		return
	}

	module.Dependencies = append(module.Dependencies[:index], module.Dependencies[index+1:]...)
}

// updateClassicHelmDependencyChecksums searches for a classic Helm chart .tgz file in Artifactory
// and updates the dependency with checksums. Returns true if checksums were updated, false if not found.
func updateClassicHelmDependencyChecksums(dep *entities.Dependency, serviceManager artifactory.ArtifactoryServicesManager) (bool, error) {
	depName, depVersion, err := parseDependencyID(dep.Id)
	if err != nil {
		return false, fmt.Errorf("could not parse dependency ID %s: %w", dep.Id, err)
	}

	repoName := extractRepositoryNameFromURL(dep.Repository)
	if repoName == "" {
		return false, fmt.Errorf("could not extract repository name from: %s", dep.Repository)
	}

	log.Debug(fmt.Sprintf("Classic Helm dependency %s has no checksums, searching for .tgz file in Artifactory", dep.Id))

	resultItem, err := searchClassicHelmChart(serviceManager, repoName, depName, depVersion)
	if err != nil {
		log.Debug(fmt.Sprintf("Classic Helm chart not found for dependency %s: %v", dep.Id, err))
		return false, nil
	}

	dep.Checksum = entities.Checksum{
		Sha1:   resultItem.Actual_Sha1,
		Sha256: resultItem.Sha256,
		Md5:    resultItem.Actual_Md5,
	}
	dep.Sha256 = resultItem.Sha256

	log.Debug(fmt.Sprintf("Found classic Helm chart for dependency %s: %s (sha256: %s)", dep.Id, resultItem.Name, dep.Sha256))
	return true, nil
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
	log.Debug(fmt.Sprintf("Searching for classic Helm chart with pattern: %s", searchPattern))

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
			log.Debug(fmt.Sprintf("Failed to close search reader: %v", closeErr))
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
