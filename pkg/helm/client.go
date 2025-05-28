package helm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log" // Consider replacing with a structured logger in a real app
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions" // Needed for ConfigFlags
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"app-store-api/pkg/config"
)

// HelmClient interacts with Helm and Kubernetes.
type HelmClient struct {
	config       *config.AppConfig
	settings     *cli.EnvSettings // Still useful for RepositoryCache, etc.
	actionConfig *action.Configuration
	kubeClient   kubernetes.Interface
	repoUpdateMu sync.Mutex
}

// NewHelmClient creates a new HelmClient.
func NewHelmClient(cfg *config.AppConfig, kubeClientset kubernetes.Interface) (*HelmClient, error) {
	settings := cli.New()
	settings.SetNamespace(cfg.AppInstallNamespace)

	actionCfg := new(action.Configuration)

	configFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	configFlags.Namespace = &cfg.AppInstallNamespace

	inCluster := os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != ""
	if !inCluster {
		if cfg.KubeconfigPath == "" {
			return nil, fmt.Errorf("kubeconfig path is not set for out-of-cluster configuration")
		}
		configFlags.KubeConfig = &cfg.KubeconfigPath
		settings.KubeConfig = cfg.KubeconfigPath // Also for CLI settings if used directly
	}
	// If in-cluster, configFlags with empty KubeConfig path will use in-cluster mechanisms.

	err := actionCfg.Init(configFlags, cfg.AppInstallNamespace, cfg.HelmDriver, log.Printf)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Helm action configuration: %w", err)
	}

	hc := &HelmClient{
		config:       cfg,
		settings:     settings,
		actionConfig: actionCfg,
		kubeClient:   kubeClientset, // Use the passed clientset
	}

	// Namespace check (optional, good to have)
	if _, errNs := hc.kubeClient.CoreV1().Namespaces().Get(context.Background(), cfg.AppInstallNamespace, metav1.GetOptions{}); errNs != nil {
		if strings.Contains(errNs.Error(), "not found") {
			log.Printf("Target application namespace %s not found. Please ensure it exists.", cfg.AppInstallNamespace)
		} else {
			log.Printf("Error checking namespace %s: %v", cfg.AppInstallNamespace, errNs)
		}
	}
	return hc, nil
}

// UpdateRepos adds and updates Helm repositories based on the provided chart definitions.
func (hc *HelmClient) UpdateRepos(charts []ChartDefinition) error {
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
		// For `helm repo add/update`, it's often more reliable to shell out to the Helm CLI
		// as it handles more edge cases and auth.
		// Ensure KUBECONFIG and HELM_NAMESPACE are set if necessary.
		cmd := exec.Command("helm", "repo", "add", repoName, chart.RepoURL, "--force-update")
		cmd.Env = os.Environ()            // Pass current environment
		if hc.settings.KubeConfig != "" { // Propagate KubeConfig if specified
			cmd.Env = append(cmd.Env, "KUBECONFIG="+hc.settings.KubeConfig)
		}
		// Helm CLI commands often respect HELM_NAMESPACE for where to look for some config,
		// though repo commands are usually global or use specific config files.
		// cmd.Env = append(cmd.Env, "HELM_NAMESPACE="+hc.config.AppInstallNamespace)

		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("Error adding/updating repo %s with Helm CLI: %v\nOutput: %s", repoName, err, string(output))
		} else {
			log.Printf("Repo %s added/updated successfully via Helm CLI.", repoName)
			addedRepos[repoName] = true
		}
	}

	if len(addedRepos) > 0 { // Only update if repos were actually added/changed by this call.
		log.Println("Updating all Helm repositories via Helm CLI...")
		cmd := exec.Command("helm", "repo", "update")
		cmd.Env = os.Environ()
		if hc.settings.KubeConfig != "" {
			cmd.Env = append(cmd.Env, "KUBECONFIG="+hc.settings.KubeConfig)
		}

		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("Warning: Error updating Helm repositories via Helm CLI: %v\nOutput: %s", err, string(output))
			// Do not return error here, as it might be a transient issue for one repo.
			// The application can often proceed.
		} else {
			log.Println("Helm repositories updated successfully via Helm CLI.")
		}
	}
	return nil
}

// InstallChart installs a Helm chart.
func (hc *HelmClient) InstallChart(chartDef ChartDefinition, releaseName string, values map[string]interface{}) (*release.Release, error) {
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
	client.Namespace = hc.config.AppInstallNamespace // Target namespace for chart resources
	client.ReleaseName = releaseName
	client.Version = chartDef.Version
	client.Wait = true
	client.Timeout = hc.config.HelmTimeout

	// Use hc.settings for LocateChart as it contains repository configurations
	chartPathOptions := client.ChartPathOptions
	chartPathOptions.Version = chartDef.Version // Ensure version is set for locating
	// Set settings for ChartPathOptions to use repository config
	// This is crucial for LocateChart to find charts in repositories.
	// client.ChartPathOptions.SetEnvSettings(hc.settings) // This method doesn't exist.
	// Instead, LocateChart itself takes settings.

	log.Printf("Locating chart '%s' version '%s'...", chartDef.Chart, chartDef.Version)
	cp, err := chartPathOptions.LocateChart(chartDef.Chart, hc.settings) // Pass hc.settings here
	if err != nil {
		log.Printf("Error locating chart %s (version %s): %v. Attempting repo update before retry.", chartDef.Chart, client.Version, err)
		if errUpdate := hc.UpdateRepos([]ChartDefinition{chartDef}); errUpdate != nil {
			log.Printf("Repo update failed during chart location for %s: %v", chartDef.Chart, errUpdate)
		}
		cp, err = chartPathOptions.LocateChart(chartDef.Chart, hc.settings) // Retry
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
	cmd.Env = os.Environ()
	if hc.settings.KubeConfig != "" {
		cmd.Env = append(cmd.Env, "KUBECONFIG="+hc.settings.KubeConfig)
	}
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
