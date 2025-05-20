package main

import (
	"fmt"
	"log" // Standard library logger
	"os"  // For os.Exit

	"app-store-api/pkg/api"
	"app-store-api/pkg/appcatalog"
	"app-store-api/pkg/config"
	"app-store-api/pkg/helm"

	"github.com/gin-gonic/gin"
)

func main() {
	// Load application configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Set Gin mode
	gin.SetMode(cfg.GinMode)

	// Initialize Helm client
	helmClient, err := helm.NewHelmClient(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize Helm client: %v", err)
	}

	// Initialize App Catalog Service
	catalogService, err := appcatalog.NewService(cfg.ChartConfigPath, helmClient)
	if err != nil {
		log.Fatalf("Failed to initialize App Catalog service: %v", err)
	}

	// Initialize API Handler with dependencies
	apiHandler := api.NewAPIHandler(catalogService, helmClient)

	// Setup router
	router := api.SetupRouter(apiHandler)

	// Start server
	listenAddr := fmt.Sprintf(":%s", cfg.ListenPort)
	log.Printf("API server starting on %s in %s mode", listenAddr, cfg.GinMode)
	if err := router.Run(listenAddr); err != nil {
		log.Fatalf("Failed to run server: %v", err)
		os.Exit(1)
	}
}
