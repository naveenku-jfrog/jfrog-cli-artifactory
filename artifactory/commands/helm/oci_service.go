package helm

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// createServiceManagerForDependencies creates a service manager for dependency operations
func createServiceManagerForDependencies() (artifactory.ArtifactoryServicesManager, error) {
	serverDetails, err := getHelmServerDetails()
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
