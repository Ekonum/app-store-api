package config

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"k8s.io/client-go/util/homedir"
)

// AppConfig holds the application configuration.
type AppConfig struct {
	ListenPort          string
	GinMode             string
	AppInstallNamespace string
	KubeconfigPath      string
	HelmDriver          string
	HelmTimeout         time.Duration
	ChartConfigPath     string // Path to a YAML/JSON file defining available charts
}

// LoadConfig loads configuration from environment variables or defaults.
func LoadConfig() (*AppConfig, error) {
	// Default Kubeconfig path
	var defaultKubeconfig string
	if home := homedir.HomeDir(); home != "" {
		defaultKubeconfig = filepath.Join(home, ".kube", "config")
	}

	helmTimeoutStr := getEnv("HELM_TIMEOUT_SECONDS", "300") // Default 5 minutes
	helmTimeoutSec, err := strconv.Atoi(helmTimeoutStr)
	if err != nil {
		log.Printf("Warning: Invalid HELM_TIMEOUT_SECONDS value '%s', using default 300s. Error: %v", helmTimeoutStr, err)
		helmTimeoutSec = 300
	}

	return &AppConfig{
		ListenPort:          getEnv("APP_PORT", "8080"),
		GinMode:             getEnv("GIN_MODE", "debug"), // "release" for production
		AppInstallNamespace: getEnv("APP_INSTALL_NAMESPACE", "app-store-apps"),
		KubeconfigPath:      getEnv("KUBECONFIG", defaultKubeconfig),
		HelmDriver:          getEnv("HELM_DRIVER", "secret"), // "secret", "configmap", or "memory"
		HelmTimeout:         time.Duration(helmTimeoutSec) * time.Second,
		ChartConfigPath:     getEnv("CHART_CONFIG_PATH", "charts.yaml"), // Example path
	}, nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
