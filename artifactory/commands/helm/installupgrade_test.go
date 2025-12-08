package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExtractPathFromURL tests the ExtractPathFromURL function
func TestExtractPathFromURL(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		expected string
	}{
		{
			name:     "HTTPS URL with path",
			rawURL:   "https://charts.example.com/my-repo",
			expected: "my-repo",
		},
		{
			name:     "HTTPS URL with leading slash",
			rawURL:   "https://charts.example.com/my-repo",
			expected: "my-repo",
		},
		{
			name:     "OCI URL",
			rawURL:   "oci://registry.example.com/repo",
			expected: "repo",
		},
		{
			name:     "URL without path",
			rawURL:   "https://charts.example.com",
			expected: "",
		},
		{
			name:     "Invalid URL",
			rawURL:   "not-a-url",
			expected: "not-a-url", // Function returns path even for invalid URLs
		},
		{
			name:     "Empty string",
			rawURL:   "",
			expected: "",
		},
		{
			name:     "URL with nested path",
			rawURL:   "https://charts.example.com/my-repo/subfolder",
			expected: "my-repo/subfolder",
		},
		{
			name:     "URL with trailing slash",
			rawURL:   "https://charts.example.com/my-repo/",
			expected: "my-repo/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractPathFromURL(tt.rawURL)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBuildCacheKey tests the buildCacheKey function
func TestBuildCacheKey(t *testing.T) {
	tests := []struct {
		name      string
		chartName string
		version   string
		expected  string
	}{
		{
			name:      "Simple chart and version",
			chartName: "nginx",
			version:   "1.2.3",
			expected:  "nginx:1.2.3",
		},
		{
			name:      "Chart with dash",
			chartName: "my-test-chart",
			version:   "0.1.0",
			expected:  "my-test-chart:0.1.0",
		},
		{
			name:      "Chart with version containing dashes",
			chartName: "nginx",
			version:   "1.2.3-alpha",
			expected:  "nginx:1.2.3-alpha",
		},
		{
			name:      "Empty chart name",
			chartName: "",
			version:   "1.0.0",
			expected:  ":1.0.0",
		},
		{
			name:      "Empty version",
			chartName: "nginx",
			version:   "",
			expected:  "nginx:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildCacheKey(tt.chartName, tt.version)
			assert.Equal(t, tt.expected, result)
		})
	}
}
