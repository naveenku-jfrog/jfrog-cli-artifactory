package helm

import (
	"github.com/jfrog/jfrog-client-go/utils/log"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/registry"
	orasregistry "oras.land/oras-go/v2/registry"
	"strings"
)

func needBuildInfo(cmdName string) bool {
	buildInfoNeededCommands := map[string]bool{
		"dependency": true,
		"package":    true,
		"push":       true,
	}
	return buildInfoNeededCommands[cmdName]
}

func getChartDetails(filePath string) (string, string, error) {
	chart, err := loader.Load(filePath)
	if err != nil {
		return "", "", err
	}
	name := chart.Metadata.Name
	version := chart.Metadata.Version
	return name, version, nil
}

// getUploadedFileDeploymentPath extracts the deployment path from the OCI registry URL argument
func getUploadedFileDeploymentPath(registryURL string) string {
	if registryURL == "" {
		return ""
	}
	raw := strings.TrimPrefix(registryURL, registry.OCIScheme+"://")
	ref, err := parseOCIReference(raw)
	if err != nil {
		log.Debug("Failed to parse OCI reference ", registryURL, " : ", err)
		return ""
	}
	return ref.Repository
}

func getPushChartPathAndRegistryURL(helmArgs []string) (chartPath, registryURL string) {
	var positionalArgs []string
	for _, arg := range helmArgs {
		if arg == "push" || strings.HasPrefix(arg, "-") {
			continue
		}
		positionalArgs = append(positionalArgs, arg)
	}
	if len(positionalArgs) > 0 {
		chartPath = positionalArgs[0]
	}
	if len(positionalArgs) > 1 {
		registryURL = positionalArgs[1]
	}
	return
}

// parseOCIReference parses an OCI reference using the same approach as Helm SDK
func parseOCIReference(raw string) (*ociReference, error) {
	orasRef, err := orasregistry.ParseReference(raw)
	if err != nil {
		return nil, err
	}
	return &ociReference{
		Registry:   orasRef.Registry,
		Repository: orasRef.Repository,
		Reference:  orasRef.Reference,
	}, nil
}

// ociReference represents a parsed OCI reference (similar to Helm SDK's reference struct)
type ociReference struct {
	Registry   string
	Repository string
	Reference  string
}
