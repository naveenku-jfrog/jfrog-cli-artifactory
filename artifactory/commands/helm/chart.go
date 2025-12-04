package helm

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// getHelmChartInfo extracts chart name and version from Chart.yaml
func getHelmChartInfo(workingDir string) (string, string, error) {
	chartYamlPath := filepath.Join(workingDir, "Chart.yaml")
	data, err := os.ReadFile(chartYamlPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read Chart.yaml: %w", err)
	}

	var chartYAML struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	}

	if err := yaml.Unmarshal(data, &chartYAML); err != nil {
		return "", "", fmt.Errorf("failed to parse Chart.yaml: %w", err)
	}

	return chartYAML.Name, chartYAML.Version, nil
}
