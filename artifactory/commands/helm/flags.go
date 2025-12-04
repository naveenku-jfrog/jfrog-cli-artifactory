package helm

import (
	"fmt"
	"os"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// helmFlags contains all parsed flags and arguments from os.Args
type helmFlags struct {
	commandName string
	serverId    string
	username    string
	password    string
	url         string
	registryURL string
	helmArgs    []string
}

var cachedFlags *helmFlags

// resetCachedFlags resets the cached flags (used for testing)
func resetCachedFlags() {
	cachedFlags = nil
}

// parseHelmFlags parses os.Args once and extracts all Helm-related flags
func parseHelmFlags() *helmFlags {
	if cachedFlags != nil {
		return cachedFlags
	}

	flags := &helmFlags{}
	args := os.Args

	extractCommandAndRegistry(flags, args)
	parseFlagsAndArgs(flags, args)

	cachedFlags = flags
	return flags
}

// extractCommandAndRegistry extracts command name and registry URL from args
func extractCommandAndRegistry(flags *helmFlags, args []string) {
	if len(args) < 3 || args[1] != "helm" {
		return
	}

	flags.commandName = args[2]

	// For push command: jf helm push <chart> <registry-url>
	if flags.commandName == "push" && len(args) >= 5 {
		flags.registryURL = args[4]
	}
}

// parseFlagsAndArgs parses all flags and filters helm arguments in a single pass
func parseFlagsAndArgs(flags *helmFlags, args []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if handleFlagWithValue(flags, &i, arg, args) || handleFlagWithEquals(flags, arg) {
			continue
		}
		collectHelmArg(flags, i, arg, args)
	}
}

// handleFlagWithValue handles flags with separate values (--flag value)
func handleFlagWithValue(flags *helmFlags, i *int, arg string, args []string) bool {
	if *i+1 >= len(args) {
		return false
	}
	nextArg := args[*i+1]
	switch arg {
	case "--server-id":
		if flags.serverId == "" {
			flags.serverId = nextArg
		}
		*i++
		return true
	case "--username", "--user":
		if flags.username == "" {
			flags.username = nextArg
		}
		*i++
		return true
	case "--password":
		if flags.password == "" {
			flags.password = nextArg
		}
		*i++
		return true
	case "--url":
		if flags.url == "" {
			flags.url = nextArg
		}
		*i++
		return true
	case "--build-name", "--build-number", "--project", "--module":
		*i++
		return true
	case "--skip-login":
		return true
	}
	return false
}

// handleFlagWithEquals handles flags with equals format (--flag=value)
func handleFlagWithEquals(flags *helmFlags, arg string) bool {
	if !strings.Contains(arg, "=") {
		return false
	}

	parts := strings.SplitN(arg, "=", 2)
	if len(parts) != 2 {
		return false
	}

	flagName := parts[0]
	flagValue := parts[1]

	switch flagName {
	case "--server-id":
		if flags.serverId == "" {
			flags.serverId = flagValue
		}
		return true
	case "--username", "--user":
		if flags.username == "" {
			flags.username = flagValue
		}
		return true
	case "--password":
		if flags.password == "" {
			flags.password = flagValue
		}
		return true
	case "--url":
		if flags.url == "" {
			flags.url = flagValue
		}
		return true
	case "--build-name", "--build-number", "--project", "--module":
		return true
	}

	return false
}

// collectHelmArg collects helm arguments, skipping JFrog-specific flags
func collectHelmArg(flags *helmFlags, i int, arg string, args []string) {
	if i < 3 || args[1] != "helm" {
		return
	}
	jfrogFlags := []string{
		"--build-",
		"--project=",
		"--module=",
		"--server-id",
		"--url",
		"--skip-login",
	}
	for _, flag := range jfrogFlags {
		if strings.HasPrefix(arg, flag) {
			return
		}
	}
	flags.helmArgs = append(flags.helmArgs, arg)
}

// getHelmCommandName returns the helm command name from os.Args
func getHelmCommandName() string {
	return parseHelmFlags().commandName
}

// getHelmServerId returns the server-id flag value from os.Args
func getHelmServerId() string {
	return parseHelmFlags().serverId
}

// getHelmUsername returns the username flag value from os.Args
func getHelmUsername() string {
	return parseHelmFlags().username
}

// getHelmPassword returns the password flag value from os.Args
func getHelmPassword() string {
	return parseHelmFlags().password
}

// getHelmUrl returns the url flag value from os.Args
func getHelmUrl() string {
	return parseHelmFlags().url
}

// getHelmServerDetails gets server details by server-id if provided, otherwise returns default server
func getHelmServerDetails() (*config.ServerDetails, error) {
	username := getHelmUsername()
	password := getHelmPassword()
	url := getHelmUrl()

	serverDetails, err := getBaseServerDetails()
	if err != nil {
		return nil, err
	}

	applyFlagOverrides(serverDetails, username, password, url)

	return serverDetails, nil
}

// getBaseServerDetails gets base server configuration
func getBaseServerDetails() (*config.ServerDetails, error) {
	serverId := getHelmServerId()
	if serverId != "" {
		serverDetails, err := config.GetSpecificConfig(serverId, true, false)
		if err != nil {
			return nil, fmt.Errorf("failed to get server details for server-id '%s': %w", serverId, err)
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
func applyFlagOverrides(serverDetails *config.ServerDetails, username, password, url string) {
	if url != "" {
		serverDetails.ArtifactoryUrl = url
		serverDetails.Url = url
	}

	if username != "" && password != "" {
		newServerDetails := *serverDetails
		newServerDetails.User = username
		newServerDetails.Password = password
		newServerDetails.AccessToken = ""
		*serverDetails = newServerDetails
		log.Debug("Using username and password from command-line flags for authentication")
	} else if username != "" || password != "" {
		log.Debug("Both --username and --password must be provided to use command-line credentials. Using server configuration credentials instead.")
	}
}
