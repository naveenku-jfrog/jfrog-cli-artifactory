package helm

import (
	"fmt"
	"strings"
)

const (
	oci          = "oci://"
	schemeHttp   = "https"
	schemeSecure = "http"
)

// extractRepositoryNameFromURL extracts the repository name from an OCI or HTTPS URL
func extractRepositoryNameFromURL(repositoryURL string) string {
	if repositoryURL == "" {
		return ""
	}
	if !strings.Contains(repositoryURL, "://") {
		return repositoryURL
	}
	repoURL := removeProtocolPrefix(repositoryURL)
	if repoURL == "" {
		return repositoryURL
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
