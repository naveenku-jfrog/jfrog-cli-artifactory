package helm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveHelmRepositoryAliasInRepository tests the resolveHelmRepositoryAlias function
// Note: This test is also in helm_test.go, keeping this version for repository.go specific testing
func TestResolveHelmRepositoryAliasInRepository(t *testing.T) {
	tests := []struct {
		name          string
		alias         string
		reposYaml     string
		expectedURL   string
		expectedError bool
		setEnv        bool
		envPath       string
	}{
		{
			name:  "Resolve alias with @ prefix",
			alias: "@bitnami",
			reposYaml: `repositories:
  - name: bitnami
    url: https://charts.bitnami.com/bitnami`,
			expectedURL:   "https://charts.bitnami.com/bitnami",
			expectedError: false,
		},
		{
			name:  "Resolve alias without @ prefix",
			alias: "bitnami",
			reposYaml: `repositories:
  - name: bitnami
    url: https://charts.bitnami.com/bitnami`,
			expectedURL:   "https://charts.bitnami.com/bitnami",
			expectedError: false,
		},
		{
			name:  "Multiple repositories",
			alias: "@stable",
			reposYaml: `repositories:
  - name: bitnami
    url: https://charts.bitnami.com/bitnami
  - name: stable
    url: https://charts.helm.sh/stable`,
			expectedURL:   "https://charts.helm.sh/stable",
			expectedError: false,
		},
		{
			name:          "Repository not found",
			alias:         "@nonexistent",
			reposYaml:     `repositories: []`,
			expectedError: true,
		},
		{
			name:          "Invalid YAML",
			alias:         "@bitnami",
			reposYaml:     `repositories: [invalid`,
			expectedError: true,
		},
		{
			name:  "Resolve with HELM_REPOSITORY_CONFIG env var",
			alias: "@test",
			reposYaml: `repositories:
  - name: test
    url: https://example.com/test`,
			expectedURL:   "https://example.com/test",
			expectedError: false,
			setEnv:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			var reposYamlPath string

			// Always use a temporary file for testing to avoid conflicts with user's actual config
			if tt.setEnv && tt.envPath != "" {
				reposYamlPath = tt.envPath
			} else {
				reposYamlPath = filepath.Join(tempDir, "repositories.yaml")
				err := os.Setenv("HELM_REPOSITORY_CONFIG", reposYamlPath)
				if err != nil {
					return
				}
				defer func() {
					_ = os.Unsetenv("HELM_REPOSITORY_CONFIG")
				}()
			}

			// Create directory if it doesn't exist
			err := os.MkdirAll(filepath.Dir(reposYamlPath), 0755)
			if err != nil {
				return
			}

			if tt.reposYaml != "" {
				err := os.WriteFile(reposYamlPath, []byte(tt.reposYaml), 0644)
				require.NoError(t, err)
			}

			url, err := resolveHelmRepositoryAlias(tt.alias)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedURL, url)
			}
		})
	}
}

// TestExtractRepositoryNameFromURL tests the extractRepositoryNameFromURL function
func TestExtractRepositoryNameFromURL(t *testing.T) {
	tests := []struct {
		name         string
		repository   string
		expectedRepo string
	}{
		{
			name:         "OCI URL with artifactory",
			repository:   "oci://example.com/artifactory/my-repo",
			expectedRepo: "artifactory", // Function returns first path segment
		},
		{
			name:         "OCI URL without artifactory",
			repository:   "oci://example.com/my-repo",
			expectedRepo: "my-repo",
		},
		{
			name:         "HTTPS URL",
			repository:   "https://charts.example.com/repo",
			expectedRepo: "repo",
		},
		{
			name:         "Non-URL string",
			repository:   "my-repo",
			expectedRepo: "my-repo",
		},
		{
			name:         "Empty string",
			repository:   "",
			expectedRepo: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRepositoryNameFromURL(tt.repository)
			assert.Equal(t, tt.expectedRepo, result)
		})
	}
}

// TestExtractDependencyPathInRepository tests the extractDependencyPath function
// Note: This test is also in layers_test.go, keeping this version for repository.go specific testing
func TestExtractDependencyPathInRepository(t *testing.T) {
	tests := []struct {
		name         string
		depId        string
		expectedPath string
	}{
		{
			name:         "Valid dependency ID",
			depId:        "nginx:1.2.3",
			expectedPath: "nginx/1.2.3",
		},
		{
			name:         "Invalid format - no colon",
			depId:        "nginx",
			expectedPath: "",
		},
		{
			name:         "Invalid format - multiple colons",
			depId:        "nginx:1.2.3:extra",
			expectedPath: "",
		},
		{
			name:         "Empty string",
			depId:        "",
			expectedPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDependencyPath(tt.depId)
			assert.Equal(t, tt.expectedPath, result)
		})
	}
}

// TestIsOCIRepository tests the isOCIRepository function
func TestIsOCIRepository(t *testing.T) {
	tests := []struct {
		name       string
		repository string
		expected   bool
	}{
		{
			name:       "OCI URL",
			repository: "oci://example.com/repo",
			expected:   true,
		},
		{
			name:       "HTTPS URL",
			repository: "https://charts.example.com/repo",
			expected:   false,
		},
		{
			name:       "Empty string",
			repository: "",
			expected:   false,
		},
		{
			name:       "Non-URL string",
			repository: "my-repo",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isOCIRepository(tt.repository)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRemoveProtocolPrefix tests the removeProtocolPrefix function
func TestRemoveProtocolPrefix(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "OCI URL",
			url:      "oci://example.com/repo",
			expected: "example.com/repo",
		},
		{
			name:     "HTTPS URL",
			url:      "https://example.com/repo",
			expected: "example.com/repo",
		},
		{
			name:     "HTTP URL",
			url:      "http://example.com/repo",
			expected: "example.com/repo",
		},
		{
			name:     "URL with custom scheme",
			url:      "custom://example.com/repo",
			expected: "example.com/repo",
		},
		{
			name:     "No protocol",
			url:      "example.com/repo",
			expected: "example.com/repo",
		},
		{
			name:     "Empty string",
			url:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeProtocolPrefix(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseHelmRepoListOutput tests the parseHelmRepoListOutput function
func TestParseHelmRepoListOutput(t *testing.T) {
	tests := []struct {
		name          string
		output        string
		repoName      string
		expectedURL   string
		expectedError bool
	}{
		{
			name: "Find repository in output",
			output: `NAME            URL
bitnami         https://charts.bitnami.com/bitnami
stable          https://charts.helm.sh/stable`,
			repoName:      "bitnami",
			expectedURL:   "https://charts.bitnami.com/bitnami",
			expectedError: false,
		},
		{
			name: "Find second repository",
			output: `NAME            URL
bitnami         https://charts.bitnami.com/bitnami
stable          https://charts.helm.sh/stable`,
			repoName:      "stable",
			expectedURL:   "https://charts.helm.sh/stable",
			expectedError: false,
		},
		{
			name: "Repository not found",
			output: `NAME            URL
bitnami         https://charts.bitnami.com/bitnami`,
			repoName:      "nonexistent",
			expectedError: true,
		},
		{
			name:          "Empty output",
			output:        "",
			repoName:      "bitnami",
			expectedError: true,
		},
		{
			name:          "Output with only header",
			output:        `NAME            URL`,
			repoName:      "bitnami",
			expectedError: true,
		},
		{
			name: "Output with extra whitespace",
			output: `NAME            URL
  bitnami         https://charts.bitnami.com/bitnami  `,
			repoName:      "bitnami",
			expectedURL:   "https://charts.bitnami.com/bitnami",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseHelmRepoListOutput([]byte(tt.output), tt.repoName)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedURL, result)
			}
		})
	}
}
