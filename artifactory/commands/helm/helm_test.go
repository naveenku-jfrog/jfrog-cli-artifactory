package helm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetHelmChartInfo tests the getHelmChartInfo function
func TestGetHelmChartInfo(t *testing.T) {
	tests := []struct {
		name          string
		chartYaml     string
		expectedName  string
		expectedVer   string
		expectedError bool
	}{
		{
			name: "Valid Chart.yaml",
			chartYaml: `name: test-chart
version: 1.0.0`,
			expectedName:  "test-chart",
			expectedVer:   "1.0.0",
			expectedError: false,
		},
		{
			name: "Chart.yaml with appVersion",
			chartYaml: `name: my-app
version: 2.3.4
appVersion: "1.0"`,
			expectedName:  "my-app",
			expectedVer:   "2.3.4",
			expectedError: false,
		},
		{
			name:          "Missing Chart.yaml",
			chartYaml:     "",
			expectedError: true,
		},
		{
			name: "Invalid YAML",
			chartYaml: `name: test-chart
version: 1.0.0
invalid: [unclosed`,
			expectedError: true,
		},
		{
			name:          "Missing name field",
			chartYaml:     `version: 1.0.0`,
			expectedError: false, // Function doesn't validate required fields, just reads what's there
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tempDir := t.TempDir()
			chartYamlPath := filepath.Join(tempDir, "Chart.yaml")

			if tt.chartYaml != "" {
				err := os.WriteFile(chartYamlPath, []byte(tt.chartYaml), 0644)
				require.NoError(t, err)
			}

			name, version, err := getHelmChartInfo(tempDir)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				if tt.expectedName != "" {
					assert.Equal(t, tt.expectedName, name)
				}
				if tt.expectedVer != "" {
					assert.Equal(t, tt.expectedVer, version)
				}
			}
		})
	}
}

// Note: TestValidateHelmArgs was removed because validateHelmArgs function was removed.
// This function was part of template.go which has been deleted.

// Note: TestIsVersionRange and TestResolveDependencyVersionsFromChartLock were removed
// because isVersionRange and resolveDependencyVersionsFromChartLock functions were removed.
// Dependency and version calculation logic is now handled by FlexPack (build-info-go),
// not in jfrog-cli-artifactory.

// TestResolveHelmRepositoryAlias tests the resolveHelmRepositoryAlias function
func TestResolveHelmRepositoryAlias(t *testing.T) {
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

// Note: TestGetRepositoryForDependency was removed because getRepositoryForDependency function was removed.
// Repository extraction logic is now handled by FlexPack (build-info-go), not in jfrog-cli-artifactory.

// TestGetHelmRepositoryFromArgs tests the getHelmRepositoryFromArgs function
// Note: This function uses os.Args, so we need to test it carefully
func TestGetHelmRepositoryFromArgs(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedRepo  string
		expectedError bool
	}{
		{
			name:          "Extract repo from oci:// URL with artifactory",
			args:          []string{"jf", "helm", "push", "chart.tgz", "oci://example.com/artifactory/my-repo"},
			expectedRepo:  "my-repo",
			expectedError: false,
		},
		{
			name:          "Extract repo from oci:// URL without artifactory",
			args:          []string{"jf", "helm", "push", "chart.tgz", "oci://example.com/my-repo"},
			expectedRepo:  "my-repo",
			expectedError: false,
		},
		{
			name:          "No push command in args",
			args:          []string{"jf", "helm", "package", "chart"},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original args
			originalArgs := os.Args
			defer func() {
				os.Args = originalArgs
				resetCachedFlags()
			}()

			// Reset cached flags before each test
			resetCachedFlags()

			// Set test args
			os.Args = tt.args

			repo, err := getHelmRepositoryFromArgs()

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedRepo, repo)
			}
		})
	}
}

// Note: TestExtractHelmArgsForTemplate was removed because extractHelmArgsForTemplate function was removed.
// This function was part of template.go which has been deleted.

// Note: TestFilterDependenciesByChartsDirectory was removed because filterDependenciesByChartsDirectory function was removed.
// This function was part of template.go which has been deleted.

// Note: TestCreateBuildPropertiesString was removed because createBuildPropertiesString function was removed.
// This function was part of template.go which has been deleted.

// Note: TestIsDependencyInChartsDirectory was removed because isDependencyInChartsDirectory function was removed.
// This function was part of template.go which has been deleted.

// Note: TestAddChartNameVariants was removed because addChartNameVariants function was removed.
// This function was part of template.go which has been deleted.

// Note: TestExtractDependencyPathAndRepo and TestCreateEmptyHelmBuildInfo were removed
// because extractDependencyPathAndRepo and createEmptyHelmBuildInfo functions were removed.
// Dependency path extraction and build info creation logic is now handled by FlexPack (build-info-go),
// not in jfrog-cli-artifactory.

// Note: TestCreateHelmBuildInfoWithoutDependencies was removed because createHelmBuildInfoWithoutDependencies
// function was removed. Build info creation logic is now handled by FlexPack (build-info-go),
// not in jfrog-cli-artifactory.

// TestGetHelmCommandName tests the getHelmCommandName function
func TestGetHelmCommandName(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedName string
	}{
		{
			name:         "Get helm push command name",
			args:         []string{"jf", "helm", "push", "chart.tgz"},
			expectedName: "push",
		},
		{
			name:         "Helm not found - wrong command",
			args:         []string{"jf", "mvn", "install"},
			expectedName: "",
		},
		{
			name:         "Helm at end without command",
			args:         []string{"jf", "helm"},
			expectedName: "",
		},
		{
			name:         "Get helm package command name",
			args:         []string{"jf", "helm", "package", "chart"},
			expectedName: "package",
		},
		{
			name:         "Get helm install command name",
			args:         []string{"jf", "helm", "install", "my-release", "my-chart"},
			expectedName: "install",
		},
		{
			name:         "Get helm dependency command name",
			args:         []string{"jf", "helm", "dependency", "update"},
			expectedName: "dependency",
		},
		{
			name:         "Get helm command with jfrog",
			args:         []string{"jfrog", "helm", "push", "chart.tgz"},
			expectedName: "push",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalArgs := os.Args
			defer func() {
				os.Args = originalArgs
				resetCachedFlags()
			}()

			resetCachedFlags()
			os.Args = tt.args
			result := getHelmCommandName()
			assert.Equal(t, tt.expectedName, result)
		})
	}
}

// TestGetHelmServerId tests the getHelmServerId function
func TestGetHelmServerId(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		expectedId string
	}{
		{
			name:       "Extract server-id with space",
			args:       []string{"jf", "helm", "push", "chart.tgz", "oci://example.com/repo", "--server-id", "my-server"},
			expectedId: "my-server",
		},
		{
			name:       "Extract server-id with equals",
			args:       []string{"jf", "helm", "push", "chart.tgz", "oci://example.com/repo", "--server-id=my-server"},
			expectedId: "my-server",
		},
		{
			name:       "No server-id flag",
			args:       []string{"jf", "helm", "push", "chart.tgz", "oci://example.com/repo"},
			expectedId: "",
		},
		{
			name:       "Server-id at end without value",
			args:       []string{"jf", "helm", "push", "chart.tgz", "--server-id"},
			expectedId: "",
		},
		{
			name:       "Server-id with other flags",
			args:       []string{"jf", "helm", "install", "my-release", "my-chart", "--server-id", "server1", "--build-name=build"},
			expectedId: "server1",
		},
		{
			name:       "Multiple server-id flags (first one wins)",
			args:       []string{"jf", "helm", "push", "chart.tgz", "--server-id", "first", "--server-id", "second"},
			expectedId: "first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalArgs := os.Args
			defer func() {
				os.Args = originalArgs
				resetCachedFlags()
			}()

			resetCachedFlags()
			os.Args = tt.args
			result := getHelmServerId()
			assert.Equal(t, tt.expectedId, result)
		})
	}
}

// TestGetHelmUsername tests the getHelmUsername function
func TestGetHelmUsername(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		expectedId string
	}{
		{
			name:       "Extract username with space",
			args:       []string{"jf", "helm", "push", "chart.tgz", "oci://example.com/repo", "--username", "myuser"},
			expectedId: "myuser",
		},
		{
			name:       "Extract username with equals",
			args:       []string{"jf", "helm", "push", "chart.tgz", "oci://example.com/repo", "--username=myuser"},
			expectedId: "myuser",
		},
		{
			name:       "Extract user flag (alternative)",
			args:       []string{"jf", "helm", "push", "chart.tgz", "oci://example.com/repo", "--user", "myuser"},
			expectedId: "myuser",
		},
		{
			name:       "No username flag",
			args:       []string{"jf", "helm", "push", "chart.tgz", "oci://example.com/repo"},
			expectedId: "",
		},
		{
			name:       "Username at end without value",
			args:       []string{"jf", "helm", "push", "chart.tgz", "--username"},
			expectedId: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalArgs := os.Args
			defer func() {
				os.Args = originalArgs
				resetCachedFlags()
			}()

			resetCachedFlags()
			os.Args = tt.args
			result := getHelmUsername()
			assert.Equal(t, tt.expectedId, result)
		})
	}
}

// TestGetHelmPassword tests the getHelmPassword function
func TestGetHelmPassword(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		expectedId string
	}{
		{
			name:       "Extract password with space",
			args:       []string{"jf", "helm", "push", "chart.tgz", "oci://example.com/repo", "--password", "mypass"},
			expectedId: "mypass",
		},
		{
			name:       "Extract password with equals",
			args:       []string{"jf", "helm", "push", "chart.tgz", "oci://example.com/repo", "--password=mypass"},
			expectedId: "mypass",
		},
		{
			name:       "No password flag",
			args:       []string{"jf", "helm", "push", "chart.tgz", "oci://example.com/repo"},
			expectedId: "",
		},
		{
			name:       "Password at end without value",
			args:       []string{"jf", "helm", "push", "chart.tgz", "--password"},
			expectedId: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalArgs := os.Args
			defer func() {
				os.Args = originalArgs
				resetCachedFlags()
			}()

			resetCachedFlags()
			os.Args = tt.args
			result := getHelmPassword()
			assert.Equal(t, tt.expectedId, result)
		})
	}
}

// Note: TestPerformHelmRegistryLoginIfNeeded was removed because login is now handled
// in HelmCommand.Run() method, not as a standalone function. Login functionality
// is tested as part of HelmCommand integration tests.

// TestHelmCommandNameSwitch tests the switch statement logic with helmCommandName
func TestHelmCommandNameSwitch(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedName string
	}{
		{
			name:         "Helm push command",
			args:         []string{"jf", "helm", "push", "chart.tgz", "oci://example.com/repo"},
			expectedName: "push",
		},
		{
			name:         "Helm package command",
			args:         []string{"jf", "helm", "package", "chart"},
			expectedName: "package",
		},
		{
			name:         "Helm dependency command",
			args:         []string{"jf", "helm", "dependency", "update"},
			expectedName: "dependency",
		},
		{
			name:         "Helm install command",
			args:         []string{"jf", "helm", "install", "release", "chart"},
			expectedName: "install",
		},
		{
			name:         "Helm upgrade command",
			args:         []string{"jf", "helm", "upgrade", "release", "chart"},
			expectedName: "upgrade",
		},
		{
			name:         "Helm template command",
			args:         []string{"jf", "helm", "template", "release", "chart"},
			expectedName: "template",
		},
		{
			name:         "No helm command - wrong command",
			args:         []string{"jf", "mvn", "install"},
			expectedName: "",
		},
		{
			name:         "No helm command - insufficient args",
			args:         []string{"jf"},
			expectedName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalArgs := os.Args
			defer func() {
				os.Args = originalArgs
				resetCachedFlags()
			}()

			resetCachedFlags()
			os.Args = tt.args
			result := getHelmCommandName()
			assert.Equal(t, tt.expectedName, result)
		})
	}
}
