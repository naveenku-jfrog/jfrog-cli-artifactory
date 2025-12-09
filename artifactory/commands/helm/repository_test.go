package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
