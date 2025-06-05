package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"app-store-api/pkg/appcatalog"
	"app-store-api/pkg/helm"
	"app-store-api/pkg/metrics"
)

// APIHandler holds dependencies for API handlers.
type APIHandler struct {
	catalogService *appcatalog.Service
	helmClient     *helm.HelmClient
	metricsService *metrics.Service
}

// NewAPIHandler creates a new APIHandler.
func NewAPIHandler(cs *appcatalog.Service, hc *helm.HelmClient, ms *metrics.Service) *APIHandler {
	return &APIHandler{
		catalogService: cs,
		helmClient:     hc,
		metricsService: ms,
	}
}

// GetChartsHandler handles requests to list available charts.
func (h *APIHandler) GetChartsHandler(c *gin.Context) {
	charts := h.catalogService.GetAvailableCharts()
	c.JSON(http.StatusOK, charts)
}

// InstallChartHandler handles requests to install a chart.
func (h *APIHandler) InstallChartHandler(c *gin.Context) {
	chartSimpleName := c.Param("chartName")

	var req helm.InstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if err.Error() != "EOF" {
			log.Printf("Warning: Could not bind JSON for install request for chart '%s': %v", chartSimpleName, err)
		}
		if req.Values == nil {
			req.Values = make(map[string]interface{})
		}
	}

	chartMeta, err := h.catalogService.GetChartByName(chartSimpleName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Convert appcatalog.ChartMeta to helm.ChartDefinition for InstallChart
	helmChartDef := helm.ChartDefinition{
		Name:    chartMeta.Name,
		Chart:   chartMeta.Chart,
		Version: chartMeta.Version,
		RepoURL: chartMeta.RepoURL,
	}

	release, err := h.helmClient.InstallChart(helmChartDef, req.ReleaseName, req.Values) // PASSING CONVERTED TYPE
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Chart '%s' installed successfully as release '%s'", chartMeta.Chart, release.Name),
		"release": gin.H{
			"name":      release.Name,
			"namespace": release.Namespace,
			"version":   release.Version,
			"status":    release.Info.Status.String(),
		},
	})
}

// ListReleasesHandler handles requests to list installed releases.
func (h *APIHandler) ListReleasesHandler(c *gin.Context) {
	releases, err := h.helmClient.ListInstalledReleases()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, releases)
}

// GetReleaseStatusHandler handles requests for a specific release's status.
func (h *APIHandler) GetReleaseStatusHandler(c *gin.Context) {
	releaseName := c.Param("releaseName")
	status, err := h.helmClient.GetReleaseStatus(releaseName)
	if err != nil {
		if strings.Contains(err.Error(), "release: not found") || strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Release '%s' not found.", releaseName)})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, status)
}

// UninstallReleaseHandler handles requests to uninstall a release.
func (h *APIHandler) UninstallReleaseHandler(c *gin.Context) {
	releaseName := c.Param("releaseName")
	_, err := h.helmClient.UninstallRelease(releaseName)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Release '%s' not found.", releaseName)})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Release '%s' uninstalled successfully.", releaseName)})
}

// MetricsStreamHandler establishes an SSE connection to stream cluster metrics.
func (h *APIHandler) MetricsStreamHandler(c *gin.Context) {
	log.Println("Client connected for metrics stream")
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*") // Or your specific frontend origin

	messageChan := make(chan string)
	defer func() {
		close(messageChan)
		log.Println("Client disconnected from metrics stream")
	}()

	// Goroutine to periodically fetch and send metrics
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-c.Request.Context().Done(): // Client disconnected
				return
			case <-ticker.C:
				if h.metricsService == nil {
					log.Println("Metrics service not available, cannot send metrics.")
					// Optionally send an error event to client
					// fmt.Fprintf(c.Writer, "event: error\ndata: Metrics service unavailable\n\n")
					// c.Writer.Flush()
					continue // Or break/return if we don't want to keep trying
				}
				clusterMetrics, err := h.metricsService.GetClusterMetricsSnapshot()
				if err != nil {
					log.Printf("Error getting cluster metrics snapshot: %v", err)
					// Optionally send an error event to client
					// fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", err.Error())
					// c.Writer.Flush()
					continue
				}

				jsonData, err := json.Marshal(clusterMetrics)
				if err != nil {
					log.Printf("Error marshalling metrics to JSON: %v", err)
					continue
				}
				messageChan <- string(jsonData)
			}
		}
	}()

	// Keep the connection open and send messages
	// Use io.WriteString for simpler SSE formatting
	c.Stream(func(w io.Writer) bool {
		if msg, ok := <-messageChan; ok {
			_, err := fmt.Fprintf(w, "data: %s\n\n", msg)
			if err != nil {
				return false
			} // SSE format: "data: <json_string>\n\n"
			return true // Keep connection open
		}
		return false // Close connection if channel is closed
	})
}
