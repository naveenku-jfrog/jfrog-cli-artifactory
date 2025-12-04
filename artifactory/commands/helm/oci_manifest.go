package helm

import (
	"encoding/json"
	"fmt"
	"strings"

	artutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-client-go/artifactory"
)

// downloadFileContentFromArtifactory downloads a file from Artifactory and returns its content
func downloadFileContentFromArtifactory(serviceManager artifactory.ArtifactoryServicesManager, repo, path, name string) ([]byte, error) {
	relativePath := buildRelativePath(repo, path, name)

	var manifestData map[string]interface{}
	if err := artutils.RemoteUnmarshal(serviceManager, relativePath, &manifestData); err != nil {
		return nil, fmt.Errorf("failed to download and unmarshal file from Artifactory: %w", err)
	}

	content, err := json.Marshal(manifestData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest data: %w", err)
	}

	return content, nil
}

// buildRelativePath builds the relative path for RemoteUnmarshal
func buildRelativePath(repo, path, name string) string {
	if path == "" {
		return fmt.Sprintf("%s/%s", repo, name)
	}

	return fmt.Sprintf("%s/%s/%s", repo, path, name)
}

// extractLayerChecksumsFromManifest parses manifest.json and extracts config and manifest layer checksums
func extractLayerChecksumsFromManifest(manifestContent []byte) (string, string, error) {
	var manifest struct {
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
		Layers []struct {
			Digest string `json:"digest"`
		} `json:"layers"`
	}

	if err := json.Unmarshal(manifestContent, &manifest); err != nil {
		return "", "", fmt.Errorf("failed to parse manifest.json: %w", err)
	}

	configLayerSha256 := strings.TrimPrefix(manifest.Config.Digest, "sha256:")

	var manifestLayerSha256 string
	if len(manifest.Layers) > 0 {
		manifestLayerSha256 = strings.TrimPrefix(manifest.Layers[0].Digest, "sha256:")
	}

	return configLayerSha256, manifestLayerSha256, nil
}
