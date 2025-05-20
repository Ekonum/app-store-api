package helm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log" // Consider replacing with a structured logger in a real app
	"os/exec"
	"strings"
	"sync"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	// "app-store-api/pkg/appcatalog" // REMOVED THIS IMPORT
	"app-store-api/pkg/config"
)

// HelmClient interacts with Helm and Kubernetes.
type HelmClient struct {
	config       *config.AppConfig
	settings     *cli.EnvSettings
	actionConfig *action.Configuration
	kubeClient   kubernetes.Interface
	repoUpdateMu sync.Mutex
}

// NewHelmClient creates a new HelmClient.
func NewHelmClient(cfg *config.AppConfig) (*HelmClient, error) {
	settings := cli.New()
	settings.KubeConfig = cfg.KubeconfigPath
	settings.SetNamespace(cfg.AppInstallNamespace)

	actionCfg := new(action.Configuration)
	err := actionCfg.Init(settings.RESTClientGetter(), cfg.AppInstallNamespace, cfg.HelmDriver, log.Printf)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Helm action configuration for namespace %s: %w", cfg.AppInstallNamespace, err)
	}

	var k8sConfig *rest.Config
	k8sConfig, err = rest.InClusterConfig()
	if err != nil {
		log.Printf("Not in cluster, attempting to use kubeconfig from %s: %v", settings.KubeConfig, err)
		k8sConfig, err = clientcmd.BuildConfigFromFlags("", settings.KubeConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
		}
	}
	kubeClient, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	hc := &HelmClient{
		config:       cfg,
		settings:     settings,
		actionConfig: actionCfg,
		kubeClient:   kubeClient,
	}

	if _, err := hc.kubeClient.CoreV1().Namespaces().Get(context.Background(), cfg.AppInstallNamespace, metav1.GetOptions{}); err != nil {
		if strings.Contains(err.Error(), "not found") {
			log.Printf("Namespace %s not found, attempting to create it using kubectl.", cfg.AppInstallNamespace)
			cmd := exec.Command("kubectl", "create", "namespace", cfg.AppInstallNamespace)
			if output, errCreate := cmd.CombinedOutput(); errCreate != nil {
				log.Printf("Failed to create namespace %s: %v. Output: %s. Please ensure it exists.", cfg.AppInstallNamespace, errCreate, string(output))
			} else {
				log.Printf("Namespace %s created successfully.", cfg.AppInstallNamespace)
			}
		} else {
			log.Printf("Error checking namespace %s: %v", cfg.AppInstallNamespace, err)
		}
	}
	return hc, nil
}

// UpdateRepos adds and updates Helm repositories based on the provided chart definitions.
func (hc *HelmClient) UpdateRepos(charts []ChartDefinition) error { // MODIFIED PARAMETER TYPE
	hc.repoUpdateMu.Lock()
	defer hc.repoUpdateMu.Unlock()

	addedRepos := make(map[string]bool)
	for _, chart := range charts {
		if chart.RepoURL == "" {
			continue
		}
		parts := strings.SplitN(chart.Chart, "/", 2)
		if len(parts) < 2 {
			log.Printf("Skipping repo for chart '%s': invalid format, expected repo/chartname", chart.Chart)
			continue
		}
		repoName := parts[0]
		if _, ok := addedRepos[repoName]; ok {
			continue
		}

		log.Printf("Ensuring Helm repo: %s %s", repoName, chart.RepoURL)
		entry := &repo.Entry{Name: repoName, URL: chart.RepoURL}

		r, err := repo.NewChartRepository(entry, getter.All(hc.settings))
		if err != nil {
			return fmt.Errorf("failed to create chart repository for %s: %w", repoName, err)
		}
		r.CachePath = hc.settings.RepositoryCache

		if _, err := r.DownloadIndexFile(); err != nil {
			log.Printf("Warning: failed to download index for repo %s (%s): %v. Will try to add anyway.", repoName, chart.RepoURL, err)
		}

		cmd := exec.Command("helm", "repo", "add", repoName, chart.RepoURL, "--force-update")
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("Error adding/updating repo %s: %v\nOutput: %s", repoName, err, string(output))
		} else {
			log.Printf("Repo %s added/updated successfully.", repoName)
			addedRepos[repoName] = true
		}
	}

	log.Println("Updating all Helm repositories...")
	cmd := exec.Command("helm", "repo", "update")
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("Warning: Error updating Helm repositories: %v\nOutput: %s", err, string(output))
		return fmt.Errorf("helm repo update failed: %w; output: %s", err, string(output))
	}
	log.Println("Helm repositories updated successfully.")
	return nil
}

// InstallChart installs a Helm chart.
// Takes helm.ChartDefinition now.
func (hc *HelmClient) InstallChart(chartDef ChartDefinition, releaseName string, values map[string]interface{}) (*release.Release, error) { // MODIFIED PARAMETER TYPE
	if releaseName == "" {
		releaseName = chartDef.Name
	}

	histClient := action.NewHistory(hc.actionConfig)
	histClient.Max = 1
	if history, err := histClient.Run(releaseName); err == nil && len(history) > 0 {
		return nil, fmt.Errorf("release '%s' already exists in namespace '%s'", releaseName, hc.config.AppInstallNamespace)
	} else if err != nil && !strings.Contains(err.Error(), "release: not found") {
		return nil, fmt.Errorf("error checking history for release %s: %w", releaseName, err)
	}

	client := action.NewInstall(hc.actionConfig)
	client.Namespace = hc.config.AppInstallNamespace
	client.ReleaseName = releaseName
	client.Version = chartDef.Version
	client.Wait = true
	client.Timeout = hc.config.HelmTimeout

	chartPathOptions := client.ChartPathOptions
	chartPathOptions.Version = chartDef.Version

	log.Printf("Locating chart '%s' version '%s'...", chartDef.Chart, chartDef.Version)
	cp, err := chartPathOptions.LocateChart(chartDef.Chart, hc.settings)
	if err != nil {
		log.Printf("Error locating chart %s (version %s): %v. Attempting repo update before retry.", chartDef.Chart, client.Version, err)
		if errUpdate := hc.UpdateRepos([]ChartDefinition{chartDef}); errUpdate != nil { // Update only the relevant repo
			log.Printf("Repo update failed during chart location for %s: %v", chartDef.Chart, errUpdate)
		}
		cp, err = chartPathOptions.LocateChart(chartDef.Chart, hc.settings)
		if err != nil {
			return nil, fmt.Errorf("could not locate chart '%s' (version '%s') after repo update: %w", chartDef.Chart, chartDef.Version, err)
		}
	}
	log.Printf("Found chart at path: %s", cp)

	chartRequested, err := loader.Load(cp)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart from path %s: %w", cp, err)
	}

	log.Printf("Installing chart '%s' as release '%s' in namespace '%s'", chartRequested.Name(), releaseName, hc.config.AppInstallNamespace)
	rel, err := client.Run(chartRequested, values)
	if err != nil {
		return nil, fmt.Errorf("failed to install chart '%s': %w", chartRequested.Name(), err)
	}

	log.Printf("Successfully installed chart '%s' (version %s) as release '%s'", rel.Chart.Metadata.Name, rel.Chart.Metadata.Version, rel.Name)
	return rel, nil
}

// ListInstalledReleases lists all releases in the configured namespace.
func (hc *HelmClient) ListInstalledReleases() ([]ReleaseInfo, error) {
	listClient := action.NewList(hc.actionConfig)
	listClient.AllNamespaces = false
	listClient.SetStateMask()

	results, err := listClient.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to list Helm releases: %w", err)
	}

	var releasesInfo []ReleaseInfo
	if results == nil {
		return []ReleaseInfo{}, nil
	}

	for _, rel := range results {
		nodePorts := hc.getReleaseNodePorts(rel.Name, rel.Namespace)
		releasesInfo = append(releasesInfo, ReleaseInfo{
			Name:         rel.Name,
			Namespace:    rel.Namespace,
			Version:      rel.Version,
			Updated:      rel.Info.LastDeployed.Time.Format(time.RFC3339),
			Status:       rel.Info.Status.String(),
			Chart:        rel.Chart.Metadata.Name,
			ChartVersion: rel.Chart.Metadata.Version,
			AppVersion:   rel.Chart.Metadata.AppVersion,
			NodePorts:    nodePorts,
		})
	}
	return releasesInfo, nil
}

func (hc *HelmClient) getReleaseNodePorts(releaseName, namespace string) map[string]int32 {
	nodePorts := make(map[string]int32)
	labelSelector := fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName)

	serviceList, err := hc.kubeClient.CoreV1().Services(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		log.Printf("Warning: Could not list services for release '%s' in namespace '%s': %v", releaseName, namespace, err)
		return nodePorts
	}

	for _, service := range serviceList.Items {
		if service.Spec.Type == "NodePort" || service.Spec.Type == "LoadBalancer" {
			for _, port := range service.Spec.Ports {
				if port.NodePort > 0 {
					portName := port.Name
					if portName == "" {
						portName = fmt.Sprintf("%d", port.Port)
					}
					nodePorts[portName] = port.NodePort
				}
			}
			if len(nodePorts) > 0 {
				break
			}
		}
	}
	return nodePorts
}

// UninstallRelease uninstalls a Helm release.
func (hc *HelmClient) UninstallRelease(releaseName string) (*release.UninstallReleaseResponse, error) {
	uninstallClient := action.NewUninstall(hc.actionConfig)
	uninstallClient.Timeout = hc.config.HelmTimeout

	log.Printf("Uninstalling release '%s' from namespace '%s'", releaseName, hc.config.AppInstallNamespace)
	res, err := uninstallClient.Run(releaseName)
	if err != nil {
		if strings.Contains(err.Error(), "release: not found") {
			return nil, fmt.Errorf("release '%s' not found in namespace '%s'", releaseName, hc.config.AppInstallNamespace)
		}
		return nil, fmt.Errorf("failed to uninstall release '%s': %w", releaseName, err)
	}
	log.Printf("Successfully uninstalled release '%s'", releaseName)
	return res, nil
}

// GetReleaseStatus retrieves the status of a specific release.
func (hc *HelmClient) GetReleaseStatus(releaseName string) (map[string]interface{}, error) {
	cmd := exec.Command("helm", "status", releaseName, "-n", hc.config.AppInstallNamespace, "-o", "json")
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error getting status for release '%s': %w. Stderr: %s", releaseName, err, stderr.String())
	}

	var statusData map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &statusData); err != nil {
		return nil, fmt.Errorf("error unmarshalling status JSON for release '%s': %w", releaseName, err)
	}
	return statusData, nil
}
