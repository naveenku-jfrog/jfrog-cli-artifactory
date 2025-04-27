package python

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/ini.v1"
)

func TestConfigurePypirc(t *testing.T) {
	// Create a temp directory for the test using t.TempDir()
	tempDir := t.TempDir()

	// Mock the home directory
	t.Setenv("HOME", tempDir)
	// Also set USERPROFILE for Windows
	t.Setenv("USERPROFILE", tempDir)

	// Test parameters
	testRepoURL := "https://artifactory.example.com/artifactory/api/pypi/pypi-virtual/"
	testRepoName := "pypi-virtual"
	testUsername := "testuser"
	testPassword := "testpass"

	// Call the function to be tested
	err := ConfigurePypirc(testRepoURL, testRepoName, testUsername, testPassword)
	require.NoError(t, err, "ConfigurePypirc failed")

	// Verify the file was created
	pypircPath := filepath.Join(tempDir, ".pypirc")
	exists, err := fileutils.IsFileExists(pypircPath, false)
	require.NoError(t, err, "Error checking if file exists")
	require.True(t, exists, "The .pypirc file was not created")

	// Check file permissions
	fileInfo, err := os.Stat(pypircPath)
	require.NoError(t, err, "Error getting file info")
	assert.Equal(t, os.FileMode(0600), fileInfo.Mode().Perm(), "File permissions are incorrect")

	// Parse the created file with more relaxed parsing
	cfg, err := ini.LoadSources(ini.LoadOptions{
		Loose:            true,
		Insensitive:      true,
		IgnoreInlineComment: true,
	}, pypircPath)
	require.NoError(t, err, "Error loading INI file")

	// Check distutils section
	distutils, err := cfg.GetSection("distutils")
	require.NoError(t, err, "distutils section not found")

	// Check index-servers key
	indexServers := distutils.Key("index-servers").String()
	require.NotEmpty(t, indexServers, "index-servers key is empty")

	// Check if pypi is in index-servers
	assert.True(t, strings.Contains(indexServers, "pypi"), "pypi not found in index-servers")

	// Check pypi section
	pypi, err := cfg.GetSection("pypi")
	require.NoError(t, err, "pypi section not found")

	// Check repository URL
	assert.Equal(t, testRepoURL, pypi.Key("repository").String(), "Repository URL is incorrect")

	// Check username
	assert.Equal(t, testUsername, pypi.Key("username").String(), "Username is incorrect")

	// Check password
	assert.Equal(t, testPassword, pypi.Key("password").String(), "Password is incorrect")
}

func TestConfigurePypircWithExistingFile(t *testing.T) {
	// Create a temp directory for the test using t.TempDir()
	tempDir := t.TempDir()

	// Mock the home directory - set both variables for cross-platform
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)

	// Create an existing .pypirc file with valid INI format
	pypircPath := filepath.Join(tempDir, ".pypirc")
	// Use Windows-compatible line endings and formatting
	existingContent := "[distutils]\r\nindex-servers = existing-repo\r\n\r\n[existing-repo]\r\nrepository = https://example.com/repo\r\nusername = user\r\npassword = pass\r\n"
	err := os.WriteFile(pypircPath, []byte(existingContent), 0600)
	require.NoError(t, err, "Error creating existing .pypirc file")

	// Test parameters
	testRepoURL := "https://artifactory.example.com/artifactory/api/pypi/pypi-virtual/"
	testRepoName := "pypi-virtual"
	testUsername := "testuser"
	testPassword := "testpass"

	// Call the function to be tested
	err = ConfigurePypirc(testRepoURL, testRepoName, testUsername, testPassword)
	require.NoError(t, err, "ConfigurePypirc failed")

	// Parse the updated file with relaxed parsing for Windows compatibility
	cfg, err := ini.LoadSources(ini.LoadOptions{
		Loose:            true,
		Insensitive:      true,
		IgnoreInlineComment: true,
	}, pypircPath)
	require.NoError(t, err, "Error loading INI file")

	// Check if both repositories are in index-servers
	indexServers := cfg.Section("distutils").Key("index-servers").String()
	assert.True(t, strings.Contains(indexServers, "pypi"), "pypi not found in index-servers")
	assert.True(t, strings.Contains(indexServers, "existing-repo"), "existing-repo not found in index-servers")

	// Verify existing repo section is still there
	existingRepo, err := cfg.GetSection("existing-repo")
	require.NoError(t, err, "existing-repo section not found")
	assert.Equal(t, "https://example.com/repo", existingRepo.Key("repository").String())

	// Verify pypi section has been added with correct values
	pypi, err := cfg.GetSection("pypi")
	require.NoError(t, err, "pypi section not found")
	assert.Equal(t, testRepoURL, pypi.Key("repository").String())
	assert.Equal(t, testUsername, pypi.Key("username").String())
	assert.Equal(t, testPassword, pypi.Key("password").String())
}
