package helm

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"

	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// HelmCommand represents a Helm command execution
type HelmCommand struct {
	cmdName            string
	helmArgs           []string
	serverDetails      *config.ServerDetails
	workingDirectory   string
	serverId           string
	username           string
	password           string
	buildConfiguration *buildUtils.BuildConfiguration
}

// NewHelmCommand creates a new HelmCommand instance
func NewHelmCommand() *HelmCommand {
	return &HelmCommand{}
}

// CommandName returns the command name for this Helm command
func (hc *HelmCommand) CommandName() string {
	return hc.cmdName
}

// SetHelmCmdName sets the Helm command name
func (hc *HelmCommand) SetHelmCmdName(cmdName string) *HelmCommand {
	hc.cmdName = cmdName
	return hc
}

// SetHelmArgs sets the Helm command arguments
func (hc *HelmCommand) SetHelmArgs(helmArgs []string) *HelmCommand {
	hc.helmArgs = helmArgs
	return hc
}

// SetBuildConfiguration sets the build configuration
func (hc *HelmCommand) SetBuildConfiguration(buildConfiguration *buildUtils.BuildConfiguration) *HelmCommand {
	hc.buildConfiguration = buildConfiguration
	return hc
}

// SetServerDetails sets the server details
func (hc *HelmCommand) SetServerDetails(serverDetails *config.ServerDetails) *HelmCommand {
	hc.serverDetails = serverDetails
	return hc
}

// SetWorkingDirectory sets the working directory
func (hc *HelmCommand) SetWorkingDirectory(workingDirectory string) *HelmCommand {
	hc.workingDirectory = workingDirectory
	return hc
}

// SetUsername sets the username
func (hc *HelmCommand) SetUsername(username string) *HelmCommand {
	hc.username = username
	return hc
}

// SetPassword sets the password
func (hc *HelmCommand) SetPassword(password string) *HelmCommand {
	hc.password = password
	return hc
}

// SetServerId sets the server ID
func (hc *HelmCommand) SetServerId(serverId string) *HelmCommand {
	hc.serverId = serverId
	return hc
}

// ServerDetails returns the server details
func (hc *HelmCommand) ServerDetails() (*config.ServerDetails, error) {
	return hc.serverDetails, nil
}

// Run executes the Helm command
func (hc *HelmCommand) Run() error {
	if hc.requiresCredentialsInArguments() {
		hc.appendCredentialsInArguments()
	}
	err := hc.performRegistryLogin()
	if err != nil {
		return err
	}
	if err := hc.executeHelmCommand(); err != nil {
		return errorutils.CheckErrorf("helm %s failed: %w", hc.cmdName, err)
	}

	if err := hc.collectBuildInfoIfNeeded(); err != nil {
		return errorutils.CheckError(err)
	}

	return nil
}

// requiresCredentialsInArguments checks if the command requires credentials to be appended to arguments
func (hc *HelmCommand) requiresCredentialsInArguments() bool {
	cmdName := hc.cmdName
	return cmdName == "registry" || cmdName == "repo" || cmdName == "dependency" || cmdName == "upgrade" || cmdName == "install" || cmdName == "pull" || cmdName == "push"
}

// appendCredentialsInArguments appends the username and password to arguments
func (hc *HelmCommand) appendCredentialsInArguments() {
	user, pass := hc.getCredentials()
	if user == "" || pass == "" {
		log.Debug("No credentials available for helm registry login")
		return
	}
	hc.helmArgs = append(hc.helmArgs, fmt.Sprintf("--username=%s", user))
	hc.helmArgs = append(hc.helmArgs, fmt.Sprintf("--password=%s", pass))
	return
}

// executeHelmCommand executes the native Helm command
func (hc *HelmCommand) executeHelmCommand() error {
	log.Info(fmt.Sprintf("Running Helm %s.", hc.cmdName))

	helmCmd := exec.Command("helm", append(hc.helmArgs)...)
	helmCmd.Stdout = os.Stdout
	helmCmd.Stderr = os.Stderr
	helmCmd.Stdin = os.Stdin
	helmCmd.Dir = hc.workingDirectory

	return helmCmd.Run()
}

// collectBuildInfoIfNeeded collects build info if configuration is provided
func (hc *HelmCommand) collectBuildInfoIfNeeded() error {
	if hc.buildConfiguration == nil {
		return nil
	}
	isCollectBuildInfo, err := hc.buildConfiguration.IsCollectBuildInfo()
	if err != nil {
		return errorutils.CheckError(err)
	}
	if !isCollectBuildInfo {
		return nil
	}
	log.Info("Collecting build info for executed helm command...")
	buildName, err := hc.buildConfiguration.GetBuildName()
	if err != nil {
		return errorutils.CheckError(err)
	}
	buildNumber, err := hc.buildConfiguration.GetBuildNumber()
	if err != nil {
		return errorutils.CheckError(err)
	}
	project := hc.buildConfiguration.GetProject()
	err = CollectHelmBuildInfoWithFlexPack(hc.workingDirectory, buildName, buildNumber, project, hc.cmdName, hc.helmArgs, hc.serverDetails)
	return errorutils.CheckError(err)
}

// performRegistryLogin performs helm registry login using stored credentials
func (hc *HelmCommand) performRegistryLogin() error {
	if hc.serverDetails == nil {
		log.Debug("No server details available for helm registry login")
		return nil
	}
	registryURL, err := hc.getRegistryURL()
	if err != nil {
		log.Debug(fmt.Sprintf("Failed to get registry URL: %v", err))
		return nil
	}
	if registryURL == "" {
		log.Debug("No server URL available for helm registry login")
		return nil
	}
	user, pass := hc.getCredentials()
	if user == "" || pass == "" {
		log.Debug("No credentials available for helm registry login")
		return nil
	}
	return hc.executeHelmLogin(registryURL, user, pass)
}

// getRegistryURL extracts registry URL from server details
func (hc *HelmCommand) getRegistryURL() (string, error) {
	if hc.serverDetails.ArtifactoryUrl != "" {
		parsedURL, err := url.Parse(hc.serverDetails.ArtifactoryUrl)
		if err != nil {
			return "", fmt.Errorf("failed to parse Artifactory URL: %w", err)
		}
		return parsedURL.Host, nil
	}

	if hc.serverDetails.Url != "" {
		parsedURL, err := url.Parse(hc.serverDetails.Url)
		if err != nil {
			return "", fmt.Errorf("failed to parse URL: %w", err)
		}
		return parsedURL.Host, nil
	}

	return "", nil
}

// getCredentials extracts credentials from command or server details
func (hc *HelmCommand) getCredentials() (string, string) {
	user := hc.username
	pass := hc.password

	if user == "" {
		user = hc.serverDetails.User
	}

	if pass == "" {
		pass = hc.serverDetails.Password
	}

	if hc.serverDetails.AccessToken != "" && pass == "" {
		if user == "" {
			user = auth.ExtractUsernameFromAccessToken(hc.serverDetails.AccessToken)
		}
		pass = hc.serverDetails.AccessToken
	}

	return user, pass
}

// executeHelmLogin executes the helm registry login command
func (hc *HelmCommand) executeHelmLogin(registryURL, user, pass string) error {
	log.Debug(fmt.Sprintf("Performing helm registry login to %s with user %s", registryURL, user))

	cmdLogin := exec.Command("helm", "registry", "login", registryURL, "--username", user, "--password-stdin")
	cmdLogin.Stdin = strings.NewReader(pass)
	cmdLogin.Stdout = io.Discard
	cmdLogin.Stderr = os.Stderr

	if err := cmdLogin.Run(); err != nil {
		return fmt.Errorf("helm registry login failed: %w", err)
	}

	log.Debug(fmt.Sprintf("Helm registry login to %s successful.", registryURL))
	return nil
}
