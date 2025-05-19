package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	log.Println("Starting App Store API...")
	InitHelm() // Initialiser Helm et charger la config des charts

	router := gin.Default()
	// CORS Middleware (permettre les requêtes depuis n'importe quelle origine pour le dev)
	// À terme, à configurer plus précisément pour le front.
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	api := router.Group("/api")
	{
		// Endpoints pour les charts disponibles ("catalogue")
		api.GET("/charts", getChartsHandler) // Lister les charts configurés

		// Endpoints pour les installations (releases Helm)
		api.POST("/charts/:chartName/install", installChartHandler)       // Installer un chart (ex: /api/charts/nginx/install)
		api.GET("/releases", listReleasesHandler)                         // Lister les releases installées
		api.GET("/releases/:releaseName/status", getReleaseStatusHandler) // Obtenir le statut d'une release
		api.DELETE("/releases/:releaseName", uninstallReleaseHandler)     // Désinstaller une release
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Port par défaut
	}
	log.Printf("API server listening on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
