package helm

import (
	"fmt"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/artifactory"
)

func handleDependencyCommand(buildInfo *entities.BuildInfo, serviceManager artifactory.ArtifactoryServicesManager, workingDir, buildName, buildNumber, project string) error {
	buildInfo, err := collectBuildInfoWithFlexPack(workingDir, buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to collect build info: %w", err)
	}
	if buildInfo == nil {
		return fmt.Errorf("no build info collected, skipping further processing")
	}
	updateDependencyOCILayersInBuildInfo(buildInfo, serviceManager)
	removeDuplicateDependencies(buildInfo)
	err = saveBuildInfo(buildInfo, buildName, buildNumber, project)
	if err != nil {
		return fmt.Errorf("failed to save build info")
	}
	return nil
}
