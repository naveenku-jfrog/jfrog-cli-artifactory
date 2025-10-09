package rbsearch

import (
	"errors"
	rtUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

type RBSearchCommand struct {
	serverDetails *config.ServerDetails
	subCmdName    string
}

func NewRBSearchCommand() *RBSearchCommand {
	return &RBSearchCommand{}
}

func (rbSearch *RBSearchCommand) SetSubCmdName(subCmdName string) {
	rbSearch.subCmdName = subCmdName
}

func (rbSearch *RBSearchCommand) SetServerDetails(serverDetails *config.ServerDetails) {
	rbSearch.serverDetails = serverDetails
}

func (rbSearch *RBSearchCommand) CommandName() string {
	return "release-bundle-search"
}

func (rbSearch *RBSearchCommand) ServerDetails() (*config.ServerDetails, error) {
	return rbSearch.serverDetails, nil
}

func (rbSearch *RBSearchCommand) Run() error {
	lcServicesManager, err := rtUtils.CreateLifecycleServiceManager(rbSearch.serverDetails, false)
	if err != nil {
		return err
	}
	switch rbSearch.subCmdName {
	case "names":
		return lcServicesManager.ReleaseBundlesSearchNames()
	case "versions":
		return lcServicesManager.ReleaseBundlesSearchVersions()
	case "artifacts":
		return lcServicesManager.ReleaseBundlesSearchArtifacts()
	case "environment":
		return lcServicesManager.ReleaseBundlesSearchEnvironment()
	case "status":
		return lcServicesManager.ReleaseBundlesSearchStatus()
	case "signature":
		return lcServicesManager.ReleaseBundlesSearchSignature()
	default:
		return errorutils.CheckError(errors.New("Unknown SubCommand: " + rbSearch.subCmdName))
	}
}
