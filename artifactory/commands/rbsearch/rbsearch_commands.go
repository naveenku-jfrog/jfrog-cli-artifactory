package rbsearch

import (
	"errors"
	rtUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

type RbSearchCommand struct {
	serverDetails *config.ServerDetails
	subCmdName    string
}

func NewRbSearchCommand() *RbSearchCommand {
	return &RbSearchCommand{}
}

func (rbs *RbSearchCommand) SetSubCmdName(subCmdName string) {
	rbs.subCmdName = subCmdName
}

func (rbs *RbSearchCommand) SetServerDetails(serverDetails *config.ServerDetails) {
	rbs.serverDetails = serverDetails
}

func (rbs *RbSearchCommand) CommandName() string {
	return "release-bundle-search"
}

func (rbs *RbSearchCommand) ServerDetails() (*config.ServerDetails, error) {
	return rbs.serverDetails, nil
}

func (rbs *RbSearchCommand) Run() error {
	lcServicesManager, err := rtUtils.CreateLifecycleServiceManager(rbs.serverDetails, false)
	if err != nil {
		return err
	}
	switch rbs.subCmdName {
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
		return errorutils.CheckError(errors.New("Unknown SubCommand: " + rbs.subCmdName))
	}
}
