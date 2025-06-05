package main

import (
	"fmt"
	"log" // Standard library logger
	"os"  // For os.Exit

	"app-store-api/pkg/api"
	"app-store-api/pkg/appcatalog"
	"app-store-api/pkg/config"
	"app-store-api/pkg/helm"
	"app-store-api/pkg/metrics"

	"github.com/gin-gonic/gin"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned"
)

func main() {
	// Load application configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Set Gin mode
	gin.SetMode(cfg.GinMode)

	// ---- Initialize Kubernetes and Metrics Clients ----
	var k8sRestConfig *rest.Config
	var k8sInitErr error

	// Detect if running in-cluster or using a local kubeconfig
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != "" {
		log.Println("Detected in-cluster environment. Using in-cluster config.")
		k8sRestConfig, k8sInitErr = rest.InClusterConfig()
	} else {
		log.Printf("Not an in-cluster environment. Using kubeconfig from: %s", cfg.KubeconfigPath)
		if cfg.KubeconfigPath == "" {
			log.Fatalf("Kubeconfig path is not set for out-of-cluster configuration and not in-cluster.")
		}
		k8sRestConfig, k8sInitErr = clientcmd.BuildConfigFromFlags("", cfg.KubeconfigPath)
	}
	if k8sInitErr != nil {
		log.Fatalf("Failed to get Kubernetes REST config: %v", k8sInitErr)
	}

	kubeClientset, err := kubernetes.NewForConfig(k8sRestConfig)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes clientset: %v", err)
	}

	metricsClientset, err := metricsv1beta1.NewForConfig(k8sRestConfig)
	if err != nil {
		log.Printf("Warning: Failed to create Metrics clientset: %v. Metrics features will be unavailable.", err)
		// metricsClientset will be nil, this is handled by the metricsService
	}
	// ---- End Kubernetes and Metrics Clients Initialization ----

	// Initialize Helm client (passing the initialized Kubernetes clientset)
	helmClient, err := helm.NewHelmClient(cfg, kubeClientset)
	if err != nil {
		log.Fatalf("Failed to initialize Helm client: %v", err)
	}

	// Initialize App Catalog Service
	catalogService, err := appcatalog.NewService(cfg.ChartConfigPath, helmClient)
	if err != nil {
		log.Fatalf("Failed to initialize App Catalog service: %v", err)
	}

	// Initialize Metrics Service
	metricsService := metrics.NewService(kubeClientset, metricsClientset)

	// Initialize API Handler with dependencies
	apiHandler := api.NewAPIHandler(catalogService, helmClient, metricsService)

	// Setup router
	router := api.SetupRouter(apiHandler)

	// Start server
	listenAddr := fmt.Sprintf(":%s", cfg.ListenPort)
	log.Printf("API server starting on %s in %s mode", listenAddr, cfg.GinMode)
       if err := router.Run(listenAddr); err != nil {
               log.Fatalf("Failed to run server: %v", err)
       }
}
