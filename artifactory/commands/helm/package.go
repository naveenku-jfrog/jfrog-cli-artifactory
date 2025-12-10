package helm

import (
	"fmt"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"path/filepath"
)

func handlePackageCommand(buildInfo *entities.BuildInfo, args []string, serviceManager artifactory.ArtifactoryServicesManager, buildName, buildNumber, project string) error {
	packagePaths := getPaths(args)
	for _, path := range packagePaths {
		absolutePath, err := filepath.Abs(path)
		buildInfo, err = collectBuildInfoWithFlexPack(absolutePath, buildName, buildNumber)
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
	}
	return nil
}
