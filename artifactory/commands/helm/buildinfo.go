package helm

import (
	"fmt"

	"github.com/jfrog/build-info-go/build"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/flexpack"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// CollectHelmBuildInfoWithFlexPack collects Helm build info using FlexPack
func CollectHelmBuildInfoWithFlexPack(workingDir, buildName, buildNumber string) error {
	buildInstance, err := getOrCreateBuild(buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to get or create build: %w", err)
	}

	buildInfo, err := collectBuildInfoWithFlexPack(workingDir, buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to collect build info: %w", err)
	}
	buildInfo.Modules[0].Dependencies[0].Sha256 = ""
	buildInfo.Modules[0].Dependencies[0].Sha1 = ""
	buildInfo.Modules[0].Dependencies[0].Md5 = ""
	buildInfo.Modules[0].Dependencies[0].Repository = "https://rteco549demo.jfrogdev.org/artifactory/rteco549-classic-helm"
	handlePushCommand(buildInfo, workingDir)
	updateDependencyArtifactsChecksumInBuildInfo(buildInfo)

	return saveBuildInfo(buildInstance, buildInfo, buildName, buildNumber)
}

// getOrCreateBuild gets or creates a build from the build service
func getOrCreateBuild(buildName, buildNumber string) (*build.Build, error) {
	buildInfoService := build.NewBuildInfoService()
	buildInstance, err := buildInfoService.GetOrCreateBuildWithProject(buildName, buildNumber, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get or create build: %w", err)
	}

	return buildInstance, nil
}

// collectBuildInfoWithFlexPack collects build info using FlexPack
func collectBuildInfoWithFlexPack(workingDir, buildName, buildNumber string) (*entities.BuildInfo, error) {
	helmConfig := flexpack.HelmConfig{
		WorkingDirectory: workingDir,
	}

	helmFlex, err := flexpack.NewHelmFlexPack(helmConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Helm FlexPack: %w", err)
	}

	buildInfo, err := helmFlex.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to collect build info with FlexPack: %w", err)
	}

	return buildInfo, nil
}

// handlePushCommand handles special processing for push command
func handlePushCommand(buildInfo *entities.BuildInfo, workingDir string) {
	if getHelmCommandName() != "push" {
		return
	}

	if err := addDeployedHelmArtifactsToBuildInfo(buildInfo, workingDir); err != nil {
		log.Warn("Failed to add deployed artifacts to build info: " + err.Error())
	}
}

// saveBuildInfo saves build info to the build instance
func saveBuildInfo(buildInstance *build.Build, buildInfo *entities.BuildInfo, buildName, buildNumber string) error {
	if err := buildInstance.SaveBuildInfo(buildInfo); err != nil {
		log.Warn("Failed to save build info for jfrog-cli compatibility: " + err.Error())
		return err
	}

	log.Info(fmt.Sprintf("Build info saved locally. Use 'jf rt bp %s %s' to publish it to Artifactory.", buildName, buildNumber))
	return nil
}
