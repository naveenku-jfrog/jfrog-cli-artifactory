package helm

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	oci          = "oci://"
	schemeHttp   = "https"
	schemeSecure = "http"
)

// extractRepositoryNameFromURL extracts the repository name from an OCI or HTTPS URL
func extractRepositoryNameFromURL(repository string) string {
	if repository == "" {
		return ""
	}

	if !strings.Contains(repository, "://") {
		return repository
	}

	repoURL := removeProtocolPrefix(repository)
	if repoURL == "" {
		return repository
	}

	parts := strings.Split(repoURL, "/")
	if len(parts) > 1 && parts[0] == "" {
		if len(parts) > 2 && parts[2] != "" {
			return parts[2]
		}
	} else if len(parts) > 1 && parts[1] != "" {
		return parts[1]
	}
	return ""
}

// removeProtocolPrefix removes protocol prefix from URL
func removeProtocolPrefix(repository string) string {
	prefixes := []string{oci, schemeHttp + "://", schemeSecure + "://"}

	for _, prefix := range prefixes {
		if strings.HasPrefix(repository, prefix) {
			return strings.TrimPrefix(repository, prefix)
		}
	}

	parts := strings.Split(repository, "://")
	if len(parts) > 1 {
		return parts[1]
	}

	return repository
}

// extractDependencyPath extracts version path from dependency ID
func extractDependencyPath(depId string) string {
	parts := strings.Split(depId, ":")
	if len(parts) != 2 {
		return ""
	}

	return fmt.Sprintf("%s/%s", parts[0], parts[1])
}

// isOCIRepository checks if a repository is OCI-compatible
func isOCIRepository(repository string) bool {
	if repository == "" {
		return false
	}

	return strings.HasPrefix(repository, oci)
}

// resolveHelmRepositoryAlias resolves a Helm repository alias to its URL using helm repo list
func resolveHelmRepositoryAlias(alias string) (string, error) {
	repoName := strings.TrimPrefix(alias, "@")

	cmd := exec.Command("helm", "repo", "list")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to execute helm repo list: %w", err)
	}

	return parseHelmRepoListOutput(output, repoName)
}

// parseHelmRepoListOutput parses helm repo list output to find repository URL
func parseHelmRepoListOutput(output []byte, repoName string) (string, error) {
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "NAME") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == repoName {
			return fields[1], nil
		}
	}

	return "", fmt.Errorf("repository alias '%s' not found in Helm repositories", repoName)
}
