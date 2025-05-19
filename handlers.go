package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"strings"
)

// InstallRequest est la structure pour le corps de la requête d'installation
type InstallRequest struct {
	ReleaseName string                 `json:"release_name"` // Nom optionnel pour la release Helm
	Values      map[string]interface{} `json:"values"`       // Valeurs Helm pour customiser l'installation
}

func getChartsHandler(c *gin.Context) {
	charts := GetAvailableCharts()
	c.JSON(http.StatusOK, charts)
}

func installChartHandler(c *gin.Context) {
	chartName := c.Param("chartName") // Nom simple défini dans ChartMeta (ex: "nginx")

	var req InstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Si le corps est vide ou invalide, on procède avec les valeurs par défaut
		// mais on logue l'erreur pour débogage
		if err.Error() != "EOF" { // EOF signifie corps vide, ce qui est ok ici
			log.Printf("Warning: could not bind JSON for install request: %v", err)
		}
		// Initialiser Values si nil pour éviter panic plus tard
		if req.Values == nil {
			req.Values = make(map[string]interface{})
		}
	}

	// Si ReleaseName n'est pas fourni, Helm générera un nom ou on pourrait utiliser chartName.
	// Notre fonction InstallChart gère cela en utilisant chartName par défaut.
	release, err := InstallChart(chartName, req.ReleaseName, req.Values)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Chart installed successfully",
		"release": gin.H{
			"name":      release.Name,
			"namespace": release.Namespace,
			"version":   release.Version,
			"status":    release.Info.Status.String(),
		},
	})
}

func listReleasesHandler(c *gin.Context) {
	releases, err := ListInstalledReleases()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, releases)
}

func getReleaseStatusHandler(c *gin.Context) {
	releaseName := c.Param("releaseName")
	status, err := GetReleaseStatus(releaseName)
	if err != nil {
		if strings.Contains(err.Error(), "release: not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, status)
}

func uninstallReleaseHandler(c *gin.Context) {
	releaseName := c.Param("releaseName")
	_, err := UninstallRelease(releaseName)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Release %s uninstalled successfully", releaseName)})
}
