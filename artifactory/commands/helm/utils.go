package helm

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"strings"

	"github.com/jfrog/jfrog-client-go/utils/log"
	"helm.sh/helm/v3/pkg/action"
)

// updateChartPathOptionsFromArgs parses all Helm flags from helmArgs and maps them directly to ChartPathOptions
func updateChartPathOptionsFromArgs(chartPathOptions *action.ChartPathOptions, helmArgs []string) {
	for i := 0; i < len(helmArgs); i++ {
		arg := helmArgs[i]

		// Skip non-flag arguments
		if !strings.HasPrefix(arg, "-") {
			continue
		}

		// Handle flags with value in same argument: --flag=value or -f=value
		if strings.Contains(arg, "=") {
			parts := strings.SplitN(arg, "=", 2)
			flagName := parts[0]
			value := parts[1]
			// Convert short flags to long flags for processing
			longFlag := convertShortFlag(flagName)
			setStringFlag(chartPathOptions, longFlag, value)
			continue
		}

		// Handle boolean flags
		longFlag := convertShortFlag(arg)
		if setBoolFlag(chartPathOptions, longFlag) {
			continue
		}

		// Handle flags with separate value: --flag value or -f value
		if i+1 < len(helmArgs) {
			nextArg := helmArgs[i+1]
			if !strings.HasPrefix(nextArg, "-") {
				longFlag := convertShortFlag(arg)
				if setStringFlag(chartPathOptions, longFlag, nextArg) {
					i++ // Skip the value in next iteration
					continue
				}
			}
		}
	}
}

// convertShortFlag converts short flags to their long form equivalents
// Only handles flags that are part of ChartPathOptions
func convertShortFlag(flag string) string {
	shortToLong := map[string]string{
		"-u": "--username",
		"-p": "--password",
		"-v": "--version",
		"-r": "--repo",
	}
	if long, exists := shortToLong[flag]; exists {
		return long
	}
	return flag
}

// setStringFlag sets a string flag on ChartPathOptions
func setStringFlag(chartPathOptions *action.ChartPathOptions, flagName, value string) bool {
	switch flagName {
	case "--ca-file":
		chartPathOptions.CaFile = value
		log.Debug("Setting ", flagName, "=", value)
		return true
	case "--cert-file":
		chartPathOptions.CertFile = value
		log.Debug("Setting ", flagName, "=", value)
		return true
	case "--key-file":
		chartPathOptions.KeyFile = value
		log.Debug("Setting ", flagName, "=", value)
		return true
	case "--keyring":
		chartPathOptions.Keyring = value
		log.Debug("Setting ", flagName, "=", value)
		return true
	case "--password":
		chartPathOptions.Password = value
		log.Debug("Setting ", flagName, "=", value)
		return true
	case "--repo":
		chartPathOptions.RepoURL = value
		log.Debug("Setting ", flagName, "=", value)
		return true
	case "--username", "--user":
		chartPathOptions.Username = value
		log.Debug("Setting ", flagName, "=", value)
		return true
	case "--version":
		chartPathOptions.Version = value
		log.Debug("Setting ", flagName, "=", value)
		return true
	default:
		return false
	}
}

// setBoolFlag sets a boolean flag on ChartPathOptions
func setBoolFlag(chartPathOptions *action.ChartPathOptions, flagName string) bool {
	switch flagName {
	case "--insecure-skip-tls-verify", "--insecure-skip-verify":
		chartPathOptions.InsecureSkipTLSverify = true
		return true
	case "--plain-http":
		chartPathOptions.PlainHTTP = true
		return true
	case "--pass-credentials":
		chartPathOptions.PassCredentialsAll = true
		return true
	case "--verify":
		chartPathOptions.Verify = true
		return true
	default:
		return false
	}
}

func getPullChartPath(cmdName string, args []string) (string, error) {
	positionalArgs := getPositionalArguments(args)
	switch cmdName {
	case "pull":
		if len(positionalArgs) < 2 {
			return "", errors.New("this command requires at least 1 argument: chart name")
		}
		return positionalArgs[1], nil
	case "upgrade":
		if len(positionalArgs) < 3 {
			return "", errors.New("this command requires 2 arguments: release name and chart name")
		}
		return positionalArgs[2], nil
	case "install":
		generateName := hasGenerateNameFlag(args)
		if generateName {
			if len(positionalArgs) < 2 {
				return "", errors.New("this command with --generate-name requires at least 1 argument: chart name")
			}
			return positionalArgs[1], nil
		}
		if len(positionalArgs) < 3 {
			return "", errors.New("this command requires 2 arguments: release name and chart name")
		}
		return positionalArgs[2], nil
	default:
		return "", fmt.Errorf("unsupported command: %s", cmdName)
	}
}

func getPositionalArguments(args []string) []string {
	flags := pflag.NewFlagSet("helm-parser", pflag.ContinueOnError)
	boolFlags := []string{
		"debug", "dry-run", "wait", "atomic", "create-namespace",
		"cleanup-on-fail", "devel", "dependency-update", "generate-name",
		"insecure-skip-tls-verify", "verify", "plain-http",
	}
	for _, name := range boolFlags {
		flags.Bool(name, false, "")
	}
	flags.ParseErrorsAllowlist.UnknownFlags = true
	err := flags.Parse(args)
	if err != nil {
		return []string{}
	}
	positionalArgs := flags.Args()
	return positionalArgs
}

// hasGenerateNameFlag checks if --generate-name or -g flag is present
func hasGenerateNameFlag(helmArgs []string) bool {
	for _, arg := range helmArgs {
		if arg == "--generate-name" || arg == "-g" || strings.HasPrefix(arg, "--generate-name=") {
			return true
		}
	}
	return false
}
