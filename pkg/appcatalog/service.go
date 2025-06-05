package appcatalog

import (
	"fmt"
	"log"

	"app-store-api/pkg/helm"
)

type Service struct {
	charts     []ChartMeta
	helmClient *helm.HelmClient
}

func NewService(chartConfigPath string, hc *helm.HelmClient) (*Service, error) {
	charts, err := LoadChartRegistryFromFile(chartConfigPath)
	if err != nil {
		return nil, fmt.Errorf("could not load chart registry: %w", err)
	}
	log.Printf("Loaded %d chart configurations from %s", len(charts), chartConfigPath)

	// Convert appcatalog.ChartMeta to helm.ChartDefinition
	helmChartDefinitions := make([]helm.ChartDefinition, len(charts))
	for i, cm := range charts {
		helmChartDefinitions[i] = helm.ChartDefinition{
			Name:    cm.Name,
			Chart:   cm.Chart,
			Version: cm.Version,
			RepoURL: cm.RepoURL,
		}
	}

	// Run initial repo update in a separate goroutine so it doesn't block startup
	go func() {
		log.Println("Starting initial Helm repo update in background...")
		if err := hc.UpdateRepos(helmChartDefinitions); err != nil {
			log.Printf("Warning: Initial background Helm repo update failed: %v", err)
		} else {
			log.Println("Initial background Helm repo update completed.")
		}
	}()

	return &Service{
		charts:     charts,
		helmClient: hc,
	}, nil
}

func (s *Service) GetAvailableCharts() []ChartMeta {
	return s.charts
}

func (s *Service) GetChartByName(name string) (*ChartMeta, error) {
	for _, chart := range s.charts {
		if chart.Name == name {
			return &chart, nil
		}
	}
	return nil, fmt.Errorf("chart '%s' not found in configured list", name)
}
