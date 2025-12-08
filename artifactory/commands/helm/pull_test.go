package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Note: Most functions in pull.go are integration functions that require
// Helm SDK and actual chart files. Unit tests for these would require
// significant mocking. This file can be expanded with integration tests
// or mocks as needed.

// TestGetPullChartPathForPullCommand tests getPullChartPath specifically for pull command
func TestGetPullChartPathForPullCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expected    string
		expectedErr bool
	}{
		{
			name:        "Pull with chart name",
			args:        []string{"nginx"},
			expected:    "nginx",
			expectedErr: false,
		},
		{
			name:        "Pull with flags",
			args:        []string{"--repo", "https://charts.example.com", "nginx"},
			expected:    "nginx",
			expectedErr: false,
		},
		{
			name:        "Pull with no args",
			args:        []string{},
			expected:    "",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getPullChartPath("pull", tt.args)
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
