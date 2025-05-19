package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"

	// Nécessaire pour que client-go puisse s'authentifier auprès de k8s
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

var (
	settings         *cli.EnvSettings
	actionConfig     *action.Configuration
	appInstallNS     = "app-store-apps" // Namespace où les apps seront installées
	configuredCharts []ChartMeta        // Notre "base de données" de charts
	mu               sync.Mutex
)

// ChartMeta définit la structure pour nos charts disponibles
type ChartMeta struct {
	Name        string `json:"name"`        // Nom simple pour l'API (ex: "nginx")
	Chart       string `json:"chart"`       // Nom complet du chart (ex: "bitnami/nginx")
	Version     string `json:"version"`     // Version optionnelle du chart
	RepoURL     string `json:"repo_url"`    // URL du dépôt Helm (si c'est un chart d'un dépôt)
	Description string `json:"description"` // Description du chart
}

// ReleaseInfo définit les informations sur une release Helm installée
type ReleaseInfo struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Version    int    `json:"version"`
	Updated    string `json:"updated"`
	Status     string `json:"status"`
	Chart      string `json:"chart"`
	AppVersion string `json:"app_version"`
}

func InitHelm() {
	settings = cli.New()
	actionConfig = new(action.Configuration)

	// Utiliser le contexte Kubernetes actuel (in-cluster quand déployé)
	// Initialiser actionConfig pour fonctionner dans le namespace appInstallNS.
	if err := actionConfig.Init(settings.RESTClientGetter(), appInstallNS, os.Getenv("HELM_DRIVER"), log.Printf); err != nil {
		log.Fatalf("Failed to initialize Helm action config for namespace %s: %v", appInstallNS, err)
	}
	log.Printf("Helm initialized successfully for namespace: %s.", appInstallNS)

	// Créer le namespace pour les applications s'il n'existe pas
	// On utilise kubectl car c'est plus simple pour une opération ponctuelle
	cmd := exec.Command("kubectl", "create", "namespace", appInstallNS, "--dry-run=client", "-o", "yaml")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		log.Printf("Dry run for namespace creation failed: %v. Assuming it might exist.", err)
	} else {
		cmdApply := exec.Command("kubectl", "apply", "-f", "-")
		cmdApply.Stdin = strings.NewReader(out.String())
		if err := cmdApply.Run(); err != nil {
			log.Printf("Failed to create namespace %s, it might already exist: %v", appInstallNS, err)
		} else {
			log.Printf("Namespace %s ensured to exist.", appInstallNS)
		}
	}

	// Charger la configuration des charts
	loadChartConfig()
	// Ajouter et mettre à jour les dépôts Helm configurés
	updateHelmRepos()
}

func loadChartConfig() {
	// Pour l'instant, une liste statique. Pourrait venir d'un fichier YAML/JSON ou d'une DB.
	configuredCharts = []ChartMeta{
		{
			Name:        "nginx",
			Chart:       "bitnami/nginx",
			Version:     "15.14.0", // Spécifier une version pour la stabilité
			RepoURL:     "https://charts.bitnami.com/bitnami",
			Description: "A popular web server and reverse proxy.",
		},
		{
			Name:        "redis",
			Chart:       "bitnami/redis",
			Version:     "18.10.1",
			RepoURL:     "https://charts.bitnami.com/bitnami",
			Description: "In-memory data structure store.",
		},
		{
			Name:        "wordpress",
			Chart:       "bitnami/wordpress",
			Version:     "20.2.1",
			RepoURL:     "https://charts.bitnami.com/bitnami",
			Description: "The world's most popular blogging platform.",
		},
		// Ajoutez d'autres charts ici
	}
	log.Printf("Loaded %d chart configurations.", len(configuredCharts))
}

func updateHelmRepos() {
	mu.Lock()
	defer mu.Unlock()

	repos := make(map[string]string) // repoName -> repoURL
	for _, chart := range configuredCharts {
		if chart.RepoURL != "" {
			parts := strings.SplitN(chart.Chart, "/", 2)
			if len(parts) == 2 {
				repos[parts[0]] = chart.RepoURL
			}
		}
	}

	for name, url := range repos {
		log.Printf("Adding Helm repo: %s %s", name, url)
		repoAddCmd := exec.Command("helm", "repo", "add", name, url, "--force-update")
		if output, err := repoAddCmd.CombinedOutput(); err != nil {
			log.Printf("Error adding repo %s: %v\nOutput: %s", name, err, string(output))
		} else {
			log.Printf("Repo %s added/updated.", name)
		}
	}

	log.Println("Updating Helm repositories...")
	repoUpdateCmd := exec.Command("helm", "repo", "update")
	if output, err := repoUpdateCmd.CombinedOutput(); err != nil {
		log.Printf("Error updating Helm repos: %v\nOutput: %s", err, string(output))
	} else {
		log.Println("Helm repositories updated successfully.")
	}
}

func GetAvailableCharts() []ChartMeta {
	// Pour l'instant, retourne juste notre liste configurée.
	// À terme, pourrait faire un `helm search repo <keyword>` ou interroger une DB.
	return configuredCharts
}

func GetChartByName(name string) (*ChartMeta, error) {
	for _, chart := range configuredCharts {
		if chart.Name == name {
			return &chart, nil
		}
	}
	return nil, fmt.Errorf("chart %s not found in configured list", name)
}

func InstallChart(chartName string, releaseName string, values map[string]interface{}) (*release.Release, error) {
	mu.Lock()
	defer mu.Unlock()

	cfgChart, err := GetChartByName(chartName)
	if err != nil {
		return nil, err
	}

	if releaseName == "" {
		releaseName = cfgChart.Name // Par défaut, utiliser le nom du chart pour la release
	}

	// Vérifier si la release existe déjà (dans appInstallNS via actionConfig)
	histClient := action.NewHistory(actionConfig)
	histClient.Max = 1
	if _, err := histClient.Run(releaseName); err == nil {
		return nil, fmt.Errorf("release %s already exists in namespace %s", releaseName, appInstallNS)
	}

	client := action.NewInstall(actionConfig)
	client.Namespace = appInstallNS // Explicitement pour l'installation
	client.ReleaseName = releaseName
	client.Version = cfgChart.Version // Utiliser la version spécifiée dans ChartMeta
	client.Wait = true                // Attendre que les ressources soient prêtes (optionnel)
	client.Timeout = 5 * time.Minute  // Timeout pour l'installation

	log.Printf("Locating chart %s version %s...", cfgChart.Chart, client.Version)
	cp, err := client.ChartPathOptions.LocateChart(cfgChart.Chart, settings)
	if err != nil {
		log.Printf("Error locating chart %s (version %s): %v. Attempting repo update.", cfgChart.Chart, client.Version, err)
		// Tenter une mise à jour des dépôts et réessayer si le chart n'est pas trouvé
		updateHelmRepos()
		cp, err = client.ChartPathOptions.LocateChart(cfgChart.Chart, settings)
		if err != nil {
			return nil, fmt.Errorf("could not locate chart %s (version %s) after repo update: %w", cfgChart.Chart, client.Version, err)
		}
	}
	log.Printf("Found chart at path: %s", cp)

	chartRequested, err := loader.Load(cp)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart %s: %w", cfgChart.Chart, err)
	}

	log.Printf("Installing chart %s as release %s in namespace %s", cfgChart.Chart, releaseName, appInstallNS)
	rel, err := client.Run(chartRequested, values)
	if err != nil {
		return nil, fmt.Errorf("failed to install chart %s: %w", cfgChart.Chart, err)
	}

	log.Printf("Successfully installed chart %s as release %s", rel.Chart.Metadata.Name, rel.Name)
	return rel, nil
}

func ListInstalledReleases() ([]ReleaseInfo, error) {
	mu.Lock()
	defer mu.Unlock()

	client := action.NewList(actionConfig) // actionConfig est maintenant initialisé pour appInstallNS
	client.AllNamespaces = false           // Lister uniquement dans appInstallNS (défini dans actionConfig)
	client.SetStateMask()                  // Lister tous les états (deployed, failed, etc.)

	results, err := client.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}

	var releases []ReleaseInfo
	for _, rel := range results {
		releases = append(releases, ReleaseInfo{
			Name:       rel.Name,
			Namespace:  rel.Namespace,
			Version:    rel.Version,
			Updated:    rel.Info.LastDeployed.Format(time.RFC3339),
			Status:     rel.Info.Status.String(),
			Chart:      rel.Chart.Metadata.Name,
			AppVersion: rel.Chart.Metadata.AppVersion,
		})
	}
	return releases, nil
}

func UninstallRelease(releaseName string) (*release.UninstallReleaseResponse, error) {
	mu.Lock()
	defer mu.Unlock()

	// actionConfig est initialisé pour appInstallNS, donc Uninstall opérera dans ce namespace.
	client := action.NewUninstall(actionConfig)
	client.Timeout = 5 * time.Minute

	log.Printf("Uninstalling release %s from namespace %s", releaseName, appInstallNS)
	res, err := client.Run(releaseName)
	if err != nil {
		// Vérifier si la release n'existe pas
		if strings.Contains(err.Error(), "release: not found") {
			return nil, fmt.Errorf("release %s not found in namespace %s", releaseName, appInstallNS)
		}
		return nil, fmt.Errorf("failed to uninstall release %s: %w", releaseName, err)
	}
	log.Printf("Successfully uninstalled release %s", releaseName)
	return res, nil
}

// GetReleaseStatus utilise `helm status` via os/exec car c'est plus simple pour obtenir un JSON complet.
// La librairie Go de Helm est un peu verbeuse pour cette opération.
func GetReleaseStatus(releaseName string) (map[string]interface{}, error) {
	mu.Lock()
	defer mu.Unlock()

	cmd := exec.Command("helm", "status", releaseName, "-n", appInstallNS, "-o", "json")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error getting status for release %s: %v\nStderr: %s", releaseName, err, stderr.String())
	}

	var statusData map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &statusData); err != nil {
		return nil, fmt.Errorf("error unmarshalling status for release %s: %v", releaseName, err)
	}

	return statusData, nil
}
