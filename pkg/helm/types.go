package helm

// ReleaseInfo defines information about an installed Helm release.
type ReleaseInfo struct {
	Name         string           `json:"name"`
	Namespace    string           `json:"namespace"`
	Version      int              `json:"version"`
	Updated      string           `json:"updated"` // ISO 8601 format
	Status       string           `json:"status"`
	Chart        string           `json:"chart"`         // Name of the chart (e.g., "nginx")
	ChartVersion string           `json:"chart_version"` // Version of the chart (e.g., "1.16.0")
	AppVersion   string           `json:"app_version"`   // Application version from chart metadata
	NodePorts    map[string]int32 `json:"node_ports,omitempty"`
}

// InstallRequest represents the payload for a chart installation request.
type InstallRequest struct {
	ReleaseName string                 `json:"release_name,omitempty"` // Optional name for the Helm release
	Values      map[string]interface{} `json:"values,omitempty"`       // Helm values to customize the installation
}

// ChartDefinition is used by HelmClient to install charts and update repos.
// It's a subset of appcatalog.ChartMeta to avoid import cycles.
type ChartDefinition struct {
	Name    string // User-friendly name (e.g., "nginx")
	Chart   string // Full chart name (e.g., "bitnami/nginx")
	Version string // Chart version
	RepoURL string // Helm repository URL
	// DefaultValues map[string]interface{} // Future: default values for installation
}
