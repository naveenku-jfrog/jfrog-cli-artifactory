package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGetPushChartPathAndRegistryURL tests the getPushChartPathAndRegistryURL function
func TestGetPushChartPathAndRegistryURL(t *testing.T) {
	tests := []struct {
		name           string
		helmArgs       []string
		expectedPath   string
		expectedRegURL string
	}{
		{
			name:           "Simple chart path and registry URL",
			helmArgs:       []string{"chart.tgz", "oci://registry/repo"},
			expectedPath:   "chart.tgz",
			expectedRegURL: "oci://registry/repo",
		},
		{
			name:           "Chart path and registry URL with flags",
			helmArgs:       []string{"chart.tgz", "oci://registry/repo", "--build-name=test"},
			expectedPath:   "chart.tgz",
			expectedRegURL: "oci://registry/repo",
		},
		{
			name:           "Chart path and registry URL with flags before",
			helmArgs:       []string{"--build-name=test", "chart.tgz", "oci://registry/repo"},
			expectedPath:   "chart.tgz",
			expectedRegURL: "oci://registry/repo",
		},
		{
			name:           "Only one positional arg",
			helmArgs:       []string{"chart.tgz"},
			expectedPath:   "chart.tgz",
			expectedRegURL: "",
		},
		{
			name:           "No positional args",
			helmArgs:       []string{"--build-name=test"},
			expectedPath:   "",
			expectedRegURL: "",
		},
		{
			name:           "Empty args",
			helmArgs:       []string{},
			expectedPath:   "",
			expectedRegURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chartPath, registryURL := getPushChartPathAndRegistryURL(tt.helmArgs)
			assert.Equal(t, tt.expectedPath, chartPath)
			assert.Equal(t, tt.expectedRegURL, registryURL)
		})
	}
}

// TestGetUploadedFileDeploymentPath tests the getUploadedFileDeploymentPath function
func TestGetUploadedFileDeploymentPath(t *testing.T) {
	tests := []struct {
		name         string
		registryURL  string
		expectedPath string
	}{
		{
			name:         "Simple OCI URL",
			registryURL:  "oci://example.com/my-repo",
			expectedPath: "my-repo",
		},
		{
			name:         "OCI URL with path",
			registryURL:  "oci://example.com/my-repo/folder",
			expectedPath: "my-repo/folder",
		},
		{
			name:         "Empty URL",
			registryURL:  "",
			expectedPath: "",
		},
		{
			name:         "Invalid OCI reference",
			registryURL:  "oci://",
			expectedPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getUploadedFileDeploymentPath(tt.registryURL)
			assert.Equal(t, tt.expectedPath, result)
		})
	}
}

// TestParseOCIReference tests the parseOCIReference function
func TestParseOCIReference(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		expectedReg   string
		expectedRepo  string
		expectedRef   string
		expectedError bool
	}{
		{
			name:         "Valid OCI reference",
			raw:          "example.com/my-repo:1.0.0",
			expectedReg:  "example.com",
			expectedRepo: "my-repo",
			expectedRef:  "1.0.0",
		},
		{
			name:         "OCI reference without tag",
			raw:          "example.com/my-repo",
			expectedReg:  "example.com",
			expectedRepo: "my-repo",
			expectedRef:  "",
		},
		{
			name:         "OCI reference with nested path",
			raw:          "example.com/my-repo/folder:1.0.0",
			expectedReg:  "example.com",
			expectedRepo: "my-repo/folder",
			expectedRef:  "1.0.0",
		},
		{
			name:          "Invalid reference",
			raw:           "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseOCIReference(tt.raw)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedReg, result.Registry)
				assert.Equal(t, tt.expectedRepo, result.Repository)
				assert.Equal(t, tt.expectedRef, result.Reference)
			}
		})
	}
}
