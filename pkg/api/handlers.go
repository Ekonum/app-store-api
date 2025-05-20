package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"app-store-api/pkg/appcatalog"
	"app-store-api/pkg/helm"
)

// APIHandler holds dependencies for API handlers.
type APIHandler struct {
	catalogService *appcatalog.Service
	helmClient     *helm.HelmClient
}

// NewAPIHandler creates a new APIHandler.
func NewAPIHandler(cs *appcatalog.Service, hc *helm.HelmClient) *APIHandler {
	return &APIHandler{
		catalogService: cs,
		helmClient:     hc,
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
