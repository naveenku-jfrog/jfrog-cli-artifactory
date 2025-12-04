package helm

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	ioutils "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-client-go/artifactory"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// executeAQLQuery executes an AQL query and returns parsed results
func executeAQLQuery(serviceManager artifactory.ArtifactoryServicesManager, query string) (*servicesUtils.AqlSearchResult, error) {
	log.Debug(fmt.Sprintf("Executing AQL query: %s", query))

	stream, err := serviceManager.Aql(query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute AQL query: %w", err)
	}

	var closeErr error
	defer func() {
		if closeErr != nil {
			log.Debug(fmt.Sprintf("Failed to close AQL stream: %v", closeErr))
		}
		ioutils.Close(stream, &closeErr)
	}()

	result, err := io.ReadAll(stream)
	if err != nil {
		return nil, fmt.Errorf("failed to read AQL query result: %w", err)
	}

	parsedResult := new(servicesUtils.AqlSearchResult)
	if err := json.Unmarshal(result, parsedResult); err != nil {
		return nil, fmt.Errorf("failed to parse AQL result: %w", err)
	}

	return parsedResult, nil
}

// OCIFileSearcher searches for OCI files in Artifactory
// This follows Single Responsibility Principle (SRP) - only responsible for OCI file searching
type OCIFileSearcher struct {
	serviceManager artifactory.ArtifactoryServicesManager
}

// NewOCIFileSearcher creates a new OCI file searcher
func NewOCIFileSearcher(serviceManager artifactory.ArtifactoryServicesManager) *OCIFileSearcher {
	return &OCIFileSearcher{serviceManager: serviceManager}
}

// SearchInDirectory searches for OCI artifacts in a specific directory
func (s *OCIFileSearcher) SearchInDirectory(repo, dirPath string) (map[string]*servicesUtils.ResultItem, error) {
	log.Debug(fmt.Sprintf("Searching for OCI artifacts in directory: %s/%s", repo, dirPath))

	// Build AQL query for OCI artifacts (manifest.json and sha256__* files)
	query := fmt.Sprintf(`items.find({
		"repo": "%s",
		"path": "%s",
		"$or": [
			{"name": {"$match": "manifest.json"}},
			{"name": {"$match": "sha256__*"}}
		]
	}).include("repo", "path", "name", "sha256", "actual_sha1", "actual_md5")`, repo, dirPath)

	log.Debug(fmt.Sprintf("AQL Query for OCI artifacts: %s", query))

	result, err := executeAQLQuery(s.serviceManager, query)
	if err != nil {
		return nil, err
	}

	if len(result.Results) == 0 {
		return nil, nil
	}

	artifacts := make(map[string]*servicesUtils.ResultItem, len(result.Results))
	for _, resultItem := range result.Results {
		itemCopy := resultItem
		artifacts[resultItem.Name] = &itemCopy
		log.Debug(fmt.Sprintf("Found OCI artifact: %s (path: %s/%s, sha256: %s) in search path: %s",
			resultItem.Name, resultItem.Path, resultItem.Name, resultItem.Sha256, dirPath))
	}

	return artifacts, nil
}

// getParentPath extracts the parent path from a given path
func getParentPath(path string) string {
	pathParts := strings.Split(path, "/")
	if len(pathParts) > 1 {
		return pathParts[0]
	}
	return ""
}

// searchDependencyOCIFilesByPath searches for OCI artifacts directly by chart name/version path
func searchDependencyOCIFilesByPath(serviceManager artifactory.ArtifactoryServicesManager, repo, versionPath, _ string, _ string) (map[string]*servicesUtils.ResultItem, string, error) {
	repoName := extractRepositoryNameFromURL(repo)
	if repoName == "" {
		return nil, "", fmt.Errorf("could not extract repository name from: %s", repo)
	}

	log.Debug(fmt.Sprintf("Searching for OCI artifacts in path: %s/%s", repoName, versionPath))

	searcher := NewOCIFileSearcher(serviceManager)

	artifacts, err := searcher.SearchInDirectory(repoName, versionPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to search OCI artifacts: %w", err)
	}

	if len(artifacts) == 0 {
		parentPath := getParentPath(versionPath)
		if parentPath != "" {
			log.Debug(fmt.Sprintf("No OCI artifacts found in %s/%s, trying parent directory: %s/%s", repoName, versionPath, repoName, parentPath))
			return searchDependencyOCIFilesByPath(serviceManager, repoName, parentPath, "", "")
		}

		log.Debug(fmt.Sprintf("No OCI artifacts found for dependency in path: %s/%s (no parent directory to try)", repoName, versionPath))
		return nil, "", fmt.Errorf("no OCI artifacts found for dependency in path: %s/%s", repoName, versionPath)
	}

	dirPath := fmt.Sprintf("%s/%s", repoName, versionPath)
	return artifacts, dirPath, nil
}
