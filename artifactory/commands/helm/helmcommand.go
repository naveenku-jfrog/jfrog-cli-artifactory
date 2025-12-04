package helm

import (
	"fmt"
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
	cmdName          string
	helmArgs         []string
	configuration    *buildUtils.BuildConfiguration
	serverDetails    *config.ServerDetails
	workingDirectory string
	collectBuildInfo bool
	serverId         string
	username         string
	password         string
	url              string
	skipLogin        bool
}

// NewHelmCommand creates a new HelmCommand instance
func NewHelmCommand() *HelmCommand {
	return &HelmCommand{}
}

// CommandName returns the command name for this Helm command
func (hc *HelmCommand) CommandName() string {
	return "rt_helm"
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
	hc.configuration = buildConfiguration
	return hc
}

// SetServerDetails sets the server details
func (hc *HelmCommand) SetServerDetails(serverDetails *config.ServerDetails) *HelmCommand {
	hc.serverDetails = serverDetails
	return hc
}

// ResolveServerDetails resolves server details using stored serverId, username, password, url
func (hc *HelmCommand) ResolveServerDetails() error {
	serverDetails, err := hc.getBaseServerDetails()
	if err != nil {
		return err
	}

	hc.applyFlagOverrides(serverDetails)
	hc.serverDetails = serverDetails

	return nil
}

// getBaseServerDetails gets base server configuration
func (hc *HelmCommand) getBaseServerDetails() (*config.ServerDetails, error) {
	if hc.serverId != "" {
		serverDetails, err := config.GetSpecificConfig(hc.serverId, true, false)
		if err != nil {
			return nil, fmt.Errorf("failed to get server details for server-id '%s': %w", hc.serverId, err)
		}
		return serverDetails, nil
	}

	serverDetails, err := config.GetDefaultServerConf()
	if err != nil {
		return nil, fmt.Errorf("failed to get default server details: %w", err)
	}

	return serverDetails, nil
}

// applyFlagOverrides applies flag-based overrides to server details
func (hc *HelmCommand) applyFlagOverrides(serverDetails *config.ServerDetails) {
	if hc.url != "" {
		serverDetails.ArtifactoryUrl = hc.url
		serverDetails.Url = hc.url
	}

	if hc.username != "" && hc.password != "" {
		newServerDetails := *serverDetails
		newServerDetails.User = hc.username
		newServerDetails.Password = hc.password
		newServerDetails.AccessToken = ""
		*serverDetails = newServerDetails
		log.Debug("Using username and password from command-line flags for authentication")
	} else if hc.username != "" || hc.password != "" {
		log.Debug("Both --username and --password must be provided to use command-line credentials. Using server configuration credentials instead.")
	}
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

// SetUrl sets the URL
func (hc *HelmCommand) SetUrl(url string) *HelmCommand {
	hc.url = url
	return hc
}

// SetServerId sets the server ID
func (hc *HelmCommand) SetServerId(serverId string) *HelmCommand {
	hc.serverId = serverId
	return hc
}

// SetSkipLogin sets whether to skip login
func (hc *HelmCommand) SetSkipLogin(skipLogin bool) *HelmCommand {
	hc.skipLogin = skipLogin
	return hc
}

// ServerDetails returns the server details
func (hc *HelmCommand) ServerDetails() (*config.ServerDetails, error) {
	return hc.serverDetails, nil
}

// Run executes the Helm command
func (hc *HelmCommand) Run() error {
	if err := hc.ensureServerDetails(); err != nil {
		log.Debug("Could not resolve server details: " + err.Error())
	}

	if !hc.skipLogin {
		if err := hc.performHelmRegistryLogin(); err != nil {
			log.Debug("Helm registry login attempt completed with warning: " + err.Error())
		}
	}

	if err := hc.executeHelmCommand(); err != nil {
		return errorutils.CheckErrorf("helm %s failed: %w", hc.cmdName, err)
	}

	if err := hc.collectBuildInfoIfNeeded(); err != nil {
		return errorutils.CheckError(err)
	}

	return nil
}

// ensureServerDetails ensures server details are resolved
func (hc *HelmCommand) ensureServerDetails() error {
	if hc.serverDetails == nil {
		return hc.ResolveServerDetails()
	}
	return nil
}

// executeHelmCommand executes the native Helm command
func (hc *HelmCommand) executeHelmCommand() error {
	log.Info(fmt.Sprintf("Running Helm %s.", hc.cmdName))

	helmCmd := exec.Command("helm", append([]string{hc.cmdName}, hc.helmArgs...)...)
	helmCmd.Stdout = os.Stdout
	helmCmd.Stderr = os.Stderr
	helmCmd.Stdin = os.Stdin
	helmCmd.Dir = hc.workingDirectory

	return helmCmd.Run()
}

// collectBuildInfoIfNeeded collects build info if configuration is provided
func (hc *HelmCommand) collectBuildInfoIfNeeded() error {
	if hc.configuration == nil {
		return nil
	}

	isCollectBuildInfo, err := hc.configuration.IsCollectBuildInfo()
	if err != nil {
		return errorutils.CheckError(err)
	}

	if !isCollectBuildInfo {
		return nil
	}

	log.Info("Collecting build info for executed helm command...")

	buildName, err := hc.configuration.GetBuildName()
	if err != nil {
		return errorutils.CheckError(err)
	}

	buildNumber, err := hc.configuration.GetBuildNumber()
	if err != nil {
		return errorutils.CheckError(err)
	}

	err = CollectHelmBuildInfoWithFlexPack(hc.workingDirectory, buildName, buildNumber)
	return errorutils.CheckError(err)
}

// performHelmRegistryLogin performs helm registry login using stored credentials
func (hc *HelmCommand) performHelmRegistryLogin() error {
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
	if hc.url != "" {
		parsedURL, err := url.Parse(hc.url)
		if err != nil {
			return "", fmt.Errorf("failed to parse URL: %w", err)
		}
		return parsedURL.Host, nil
	}

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
	cmdLogin.Stdout = os.Stderr
	cmdLogin.Stderr = os.Stderr

	if err := cmdLogin.Run(); err != nil {
		return fmt.Errorf("helm registry login failed: %w", err)
	}

	log.Debug(fmt.Sprintf("Helm registry login to %s successful.", registryURL))
	return nil
}
