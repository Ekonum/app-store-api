package appcatalog

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ChartMeta defines the metadata for an available chart.
type ChartMeta struct {
	Name        string `json:"name" yaml:"name"`                             // User-friendly name (e.g., "nginx")
	Chart       string `json:"chart" yaml:"chart"`                           // Full chart name (e.g., "bitnami/nginx")
	Version     string `json:"version,omitempty" yaml:"version,omitempty"`   // Optional chart version
	RepoURL     string `json:"repo_url,omitempty" yaml:"repo_url,omitempty"` // Helm repository URL (if applicable)
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// DefaultValues map[string]interface{} `json:"default_values,omitempty" yaml:"default_values,omitempty"` // Future: default values
}

// ChartRegistry holds the list of configured charts.
type ChartRegistry struct {
	Charts []ChartMeta `yaml:"charts"`
}

// LoadChartRegistryFromFile loads chart configurations from a YAML file.
func LoadChartRegistryFromFile(filePath string) ([]ChartMeta, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read chart config file %s: %w", filePath, err)
	}

	var registry ChartRegistry
	err = yaml.Unmarshal(data, &registry)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal chart config from %s: %w", filePath, err)
	}
	return registry.Charts, nil
}
