package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"helm.sh/helm/v3/pkg/action"
)

// TestUpdateChartPathOptionsFromArgs tests the updateChartPathOptionsFromArgs function
func TestUpdateChartPathOptionsFromArgs(t *testing.T) {
	tests := []struct {
		name              string
		helmArgs          []string
		expectedRepo      string
		expectedVersion   string
		expectedUser      string
		expectedPass      string
		expectedPlainHTTP bool
	}{
		{
			name:            "Set repo and version",
			helmArgs:        []string{"--repo", "https://charts.example.com", "--version", "1.2.3"},
			expectedRepo:    "https://charts.example.com",
			expectedVersion: "1.2.3",
		},
		{
			name:         "Set repo with equals",
			helmArgs:     []string{"--repo=https://charts.example.com"},
			expectedRepo: "https://charts.example.com",
		},
		{
			name:         "Set username and password",
			helmArgs:     []string{"--username", "user", "--password", "pass"},
			expectedUser: "user",
			expectedPass: "pass",
		},
		{
			name:              "Set plain-http flag",
			helmArgs:          []string{"--plain-http"},
			expectedPlainHTTP: true,
		},
		{
			name:              "Set multiple flags",
			helmArgs:          []string{"--repo", "https://charts.example.com", "--version", "1.2.3", "--username", "user", "--plain-http"},
			expectedRepo:      "https://charts.example.com",
			expectedVersion:   "1.2.3",
			expectedUser:      "user",
			expectedPlainHTTP: true,
		},
		{
			name:            "Short flags",
			helmArgs:        []string{"-r", "https://charts.example.com", "-v", "1.2.3", "-u", "user"},
			expectedRepo:    "https://charts.example.com",
			expectedVersion: "1.2.3",
			expectedUser:    "user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chartPathOptions := &action.ChartPathOptions{}
			updateChartPathOptionsFromArgs(chartPathOptions, tt.helmArgs)

			if tt.expectedRepo != "" {
				assert.Equal(t, tt.expectedRepo, chartPathOptions.RepoURL)
			}
			if tt.expectedVersion != "" {
				assert.Equal(t, tt.expectedVersion, chartPathOptions.Version)
			}
			if tt.expectedUser != "" {
				assert.Equal(t, tt.expectedUser, chartPathOptions.Username)
			}
			if tt.expectedPass != "" {
				assert.Equal(t, tt.expectedPass, chartPathOptions.Password)
			}
			if tt.expectedPlainHTTP {
				assert.True(t, chartPathOptions.PlainHTTP)
			}
		})
	}
}

// TestGetPullChartPath tests the getPullChartPath function
func TestGetPullChartPath(t *testing.T) {
	tests := []struct {
		name        string
		cmdName     string
		args        []string
		expected    string
		expectedErr bool
	}{
		{
			name:        "Pull command - valid",
			cmdName:     "pull",
			args:        []string{"pull", "chart-name"},
			expected:    "chart-name",
			expectedErr: false,
		},
		{
			name:        "Pull command - missing chart",
			cmdName:     "pull",
			args:        []string{},
			expected:    "",
			expectedErr: true,
		},
		{
			name:        "Install command - valid",
			cmdName:     "install",
			args:        []string{"install", "release-name", "chart-name"},
			expected:    "chart-name",
			expectedErr: false,
		},
		{
			name:        "Install command - with generate-name",
			cmdName:     "install",
			args:        []string{"install", "--generate-name", "chart-name"},
			expected:    "chart-name",
			expectedErr: false,
		},
		{
			name:        "Upgrade command - valid",
			cmdName:     "upgrade",
			args:        []string{"upgrade", "release-name", "chart-name"},
			expected:    "chart-name",
			expectedErr: false,
		},
		{
			name:        "Upgrade command - missing args",
			cmdName:     "upgrade",
			args:        []string{"release-name"},
			expected:    "",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getPullChartPath(tt.cmdName, tt.args)
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestHasGenerateNameFlag tests the hasGenerateNameFlag function
func TestHasGenerateNameFlag(t *testing.T) {
	tests := []struct {
		name     string
		helmArgs []string
		expected bool
	}{
		{
			name:     "Has --generate-name",
			helmArgs: []string{"--generate-name", "chart"},
			expected: true,
		},
		{
			name:     "Has -g",
			helmArgs: []string{"-g", "chart"},
			expected: true,
		},
		{
			name:     "Has --generate-name=value",
			helmArgs: []string{"--generate-name=true", "chart"},
			expected: true,
		},
		{
			name:     "No generate-name flag",
			helmArgs: []string{"chart"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasGenerateNameFlag(tt.helmArgs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConvertShortFlag tests the convertShortFlag function
func TestConvertShortFlag(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		expected string
	}{
		{
			name:     "Convert -u to --username",
			flag:     "-u",
			expected: "--username",
		},
		{
			name:     "Convert -p to --password",
			flag:     "-p",
			expected: "--password",
		},
		{
			name:     "Convert -v to --version",
			flag:     "-v",
			expected: "--version",
		},
		{
			name:     "Convert -r to --repo",
			flag:     "-r",
			expected: "--repo",
		},
		{
			name:     "Unknown short flag",
			flag:     "-f",
			expected: "-f",
		},
		{
			name:     "Long flag unchanged",
			flag:     "--repo",
			expected: "--repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertShortFlag(tt.flag)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetPositionalArguments tests the getPositionalArguments function
func TestGetPositionalArguments(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "Simple positional args",
			args:     []string{"release", "chart"},
			expected: []string{"release", "chart"},
		},
		{
			name:     "Args with flags",
			args:     []string{"--dry-run", "release", "--wait", "chart"},
			expected: []string{"release", "chart"},
		},
		{
			name:     "Only flags",
			args:     []string{"--dry-run", "--wait"},
			expected: []string{},
		},
		{
			name:     "Empty args",
			args:     []string{},
			expected: []string{},
		},
		{
			name:     "Args with generate-name flag",
			args:     []string{"--generate-name", "chart"},
			expected: []string{"chart"},
		},
		{
			name:     "Args with multiple boolean flags",
			args:     []string{"--debug", "--dry-run", "release", "chart"},
			expected: []string{"release", "chart"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPositionalArguments(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSetStringFlag tests the setStringFlag function
func TestSetStringFlag(t *testing.T) {
	tests := []struct {
		name              string
		flagName          string
		value             string
		expectedRepo      string
		expectedVersion   string
		expectedUser      string
		expectedPass      string
		expectedCaFile    string
		expectedCertFile  string
		expectedKeyFile   string
		expectedKeyring   string
		shouldSet         bool
	}{
		{
			name:            "Set repo",
			flagName:        "--repo",
			value:           "https://charts.example.com",
			expectedRepo:    "https://charts.example.com",
			shouldSet:       true,
		},
		{
			name:          "Set version",
			flagName:      "--version",
			value:         "1.2.3",
			expectedVersion: "1.2.3",
			shouldSet:     true,
		},
		{
			name:          "Set username",
			flagName:      "--username",
			value:         "user",
			expectedUser:  "user",
			shouldSet:     true,
		},
		{
			name:          "Set password",
			flagName:      "--password",
			value:         "pass",
			expectedPass:  "pass",
			shouldSet:     true,
		},
		{
			name:          "Set ca-file",
			flagName:      "--ca-file",
			value:         "/path/to/ca.crt",
			expectedCaFile: "/path/to/ca.crt",
			shouldSet:     true,
		},
		{
			name:           "Set cert-file",
			flagName:       "--cert-file",
			value:          "/path/to/cert.crt",
			expectedCertFile: "/path/to/cert.crt",
			shouldSet:      true,
		},
		{
			name:          "Set key-file",
			flagName:      "--key-file",
			value:         "/path/to/key.key",
			expectedKeyFile: "/path/to/key.key",
			shouldSet:     true,
		},
		{
			name:          "Set keyring",
			flagName:      "--keyring",
			value:         "/path/to/keyring",
			expectedKeyring: "/path/to/keyring",
			shouldSet:     true,
		},
		{
			name:      "Unknown flag",
			flagName:  "--unknown",
			value:     "value",
			shouldSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chartPathOptions := &action.ChartPathOptions{}
			result := setStringFlag(chartPathOptions, tt.flagName, tt.value)

			assert.Equal(t, tt.shouldSet, result)
			if tt.expectedRepo != "" {
				assert.Equal(t, tt.expectedRepo, chartPathOptions.RepoURL)
			}
			if tt.expectedVersion != "" {
				assert.Equal(t, tt.expectedVersion, chartPathOptions.Version)
			}
			if tt.expectedUser != "" {
				assert.Equal(t, tt.expectedUser, chartPathOptions.Username)
			}
			if tt.expectedPass != "" {
				assert.Equal(t, tt.expectedPass, chartPathOptions.Password)
			}
			if tt.expectedCaFile != "" {
				assert.Equal(t, tt.expectedCaFile, chartPathOptions.CaFile)
			}
			if tt.expectedCertFile != "" {
				assert.Equal(t, tt.expectedCertFile, chartPathOptions.CertFile)
			}
			if tt.expectedKeyFile != "" {
				assert.Equal(t, tt.expectedKeyFile, chartPathOptions.KeyFile)
			}
			if tt.expectedKeyring != "" {
				assert.Equal(t, tt.expectedKeyring, chartPathOptions.Keyring)
			}
		})
	}
}

// TestSetBoolFlag tests the setBoolFlag function
func TestSetBoolFlag(t *testing.T) {
	tests := []struct {
		name                    string
		flagName                string
		expectedInsecureSkipTLS bool
		expectedPlainHTTP       bool
		expectedPassCredentials bool
		expectedVerify          bool
		shouldSet               bool
	}{
		{
			name:                    "Set insecure-skip-tls-verify",
			flagName:                "--insecure-skip-tls-verify",
			expectedInsecureSkipTLS: true,
			shouldSet:               true,
		},
		{
			name:                    "Set insecure-skip-verify",
			flagName:                "--insecure-skip-verify",
			expectedInsecureSkipTLS: true,
			shouldSet:               true,
		},
		{
			name:              "Set plain-http",
			flagName:          "--plain-http",
			expectedPlainHTTP: true,
			shouldSet:         true,
		},
		{
			name:                    "Set pass-credentials",
			flagName:                "--pass-credentials",
			expectedPassCredentials: true,
			shouldSet:               true,
		},
		{
			name:           "Set verify",
			flagName:       "--verify",
			expectedVerify: true,
			shouldSet:      true,
		},
		{
			name:      "Unknown flag",
			flagName:  "--unknown",
			shouldSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chartPathOptions := &action.ChartPathOptions{}
			result := setBoolFlag(chartPathOptions, tt.flagName)

			assert.Equal(t, tt.shouldSet, result)
			if tt.expectedInsecureSkipTLS {
				assert.True(t, chartPathOptions.InsecureSkipTLSverify)
			}
			if tt.expectedPlainHTTP {
				assert.True(t, chartPathOptions.PlainHTTP)
			}
			if tt.expectedPassCredentials {
				assert.True(t, chartPathOptions.PassCredentialsAll)
			}
			if tt.expectedVerify {
				assert.True(t, chartPathOptions.Verify)
			}
		})
	}
}
