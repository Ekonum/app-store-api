package appcatalog

import (
	"fmt"
	"log" // Consider structured logging

	"app-store-api/pkg/helm" // Local import
)

// Service provides operations for the application catalog.
type Service struct {
	charts     []ChartMeta
	helmClient *helm.HelmClient
}

// NewService creates a new catalog service.
func NewService(chartConfigPath string, hc *helm.HelmClient) (*Service, error) {
	charts, err := LoadChartRegistryFromFile(chartConfigPath)
	if err != nil {
		return nil, fmt.Errorf("could not load chart registry: %w", err)
	}
	log.Printf("Loaded %d chart configurations from %s", len(charts), chartConfigPath)

	// Convert appcatalog.ChartMeta to helm.ChartDefinition for UpdateRepos
	helmChartDefinitions := make([]helm.ChartDefinition, len(charts))
	for i, cm := range charts {
		helmChartDefinitions[i] = helm.ChartDefinition{
			Name:    cm.Name,
			Chart:   cm.Chart,
			Version: cm.Version,
			RepoURL: cm.RepoURL,
		}
	}

	if err := hc.UpdateRepos(helmChartDefinitions); err != nil { // PASSING CONVERTED TYPE
		log.Printf("Warning: Initial Helm repo update failed: %v", err)
	}

	return &Service{
		charts:     charts,
		helmClient: hc,
	}, nil
}

// GetAvailableCharts returns the list of charts available for installation.
func (s *Service) GetAvailableCharts() []ChartMeta {
	return s.charts
}

// GetChartByName returns a chart's metadata by its simple name.
func (s *Service) GetChartByName(name string) (*ChartMeta, error) {
	for _, chart := range s.charts {
		if chart.Name == name {
			return &chart, nil
		}
	}
	return nil, fmt.Errorf("chart '%s' not found in configured list", name)
}
