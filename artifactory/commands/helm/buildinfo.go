package helm

import (
	"fmt"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/flexpack"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildtool "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// CollectHelmBuildInfoWithFlexPack collects Helm build info using FlexPack
func CollectHelmBuildInfoWithFlexPack(workingDir, buildName, buildNumber, project, commandName string, helmArgs []string, serverDetails *config.ServerDetails) error {
	buildInfo, err := collectBuildInfoWithFlexPack(workingDir, buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to collect build info: %w", err)
	}
	if buildInfo == nil {
		log.Debug("No build info collected, skipping further processing")
		return nil
	}
	serviceManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create services manager: %w", err)
	}
	updateDependencyOCILayersInBuildInfo(buildInfo, serviceManager)
	switch commandName {
	case "push":
		handlePushCommand(buildInfo, helmArgs, serviceManager)
	case "pull", "fetch":
		handlePullCommand(buildInfo, helmArgs, serviceManager)
	case "install", "upgrade":
		handleInstallOrUpgradeCommand(buildInfo, commandName, helmArgs, serviceManager)
	}
	return saveBuildInfo(buildInfo, buildName, buildNumber, project)
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

// saveBuildInfo saves build info to the build instance
func saveBuildInfo(buildInfo *entities.BuildInfo, buildName, buildNumber, project string) error {
	if err := buildtool.SaveBuildInfo(buildName, buildNumber, project, buildInfo); err != nil {
		log.Warn("Failed to save build info for jfrog-cli compatibility: ", err.Error())
		return err
	}
	log.Info("Build info saved locally. Use 'jf rt bp ", buildName, " ", buildNumber, "' to publish it to Artifactory.")
	return nil
}
