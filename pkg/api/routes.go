package api

import (
	"github.com/gin-gonic/gin"
)

// SetupRouter configures the Gin router with all API routes.
func SetupRouter(handler *APIHandler) *gin.Engine {
	router := gin.Default()

	// Basic CORS middleware (allow all for now, refine for production)
	router.Use(CORSMiddleware())

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "UP"})
	})

	apiGroup := router.Group("/api")
	{
		// Chart catalog endpoints
		apiGroup.GET("/charts", handler.GetChartsHandler)

		// Release management endpoints
		apiGroup.POST("/charts/:chartName/install", handler.InstallChartHandler)
		apiGroup.GET("/releases", handler.ListReleasesHandler)
		apiGroup.GET("/releases/:releaseName/status", handler.GetReleaseStatusHandler)
		apiGroup.DELETE("/releases/:releaseName", handler.UninstallReleaseHandler)

		// Metrics streaming endpoint
		apiGroup.GET("/metrics/stream", handler.MetricsStreamHandler)
	}
	return router
}
