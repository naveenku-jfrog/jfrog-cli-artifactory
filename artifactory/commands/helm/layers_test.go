package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestParseDependencyID tests the parseDependencyID function
func TestParseDependencyID(t *testing.T) {
	tests := []struct {
		name          string
		depId         string
		expectedName  string
		expectedVer   string
		expectedError bool
	}{
		{
			name:          "Valid dependency ID",
			depId:         "nginx:1.2.3",
			expectedName:  "nginx",
			expectedVer:   "1.2.3",
			expectedError: false,
		},
		{
			name:          "Invalid - no colon",
			depId:         "nginx",
			expectedError: true,
		},
		{
			name:          "Invalid - multiple colons",
			depId:         "nginx:1.2.3:extra",
			expectedError: true,
		},
		{
			name:          "Empty string",
			depId:         "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, ver, err := parseDependencyID(tt.depId)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedName, name)
				assert.Equal(t, tt.expectedVer, ver)
			}
		})
	}
}

// TestExtractDependencyPathInLayers tests the extractDependencyPath function (from repository.go but used in layers)
// Note: This test is also in repository_test.go, keeping this version for layers.go specific testing
func TestExtractDependencyPathInLayers(t *testing.T) {
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
