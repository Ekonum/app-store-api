package metrics

import (
	"context"
	"fmt"
	"log"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned"
)

// Service handles fetching and processing of Kubernetes metrics.
type Service struct {
	kubeClient    kubernetes.Interface
	metricsClient metricsv1beta1.Interface
}

// NewService creates a new metrics service.
func NewService(kc kubernetes.Interface, mc metricsv1beta1.Interface) *Service {
	if mc == nil {
		log.Println("Metrics client is nil in NewService, metrics features will be limited.")
	}
	return &Service{kubeClient: kc, metricsClient: mc}
}

// GetClusterMetricsSnapshot fetches a snapshot of current cluster, node, and pod metrics.
func (s *Service) GetClusterMetricsSnapshot() (*ClusterMetrics, error) {
	startTime := time.Now() // Log start time
	if s.metricsClient == nil {
		return nil, fmt.Errorf("metrics client not initialized, ensure Metrics Server is installed and API has permissions")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	log.Println("Fetching node metrics from Metrics Server...")
	nodeMetricsList, err := s.metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Error fetching node metrics: %v (took %v)", err, time.Since(startTime))
		return nil, fmt.Errorf("failed to list node metrics: %w", err)
	}
	log.Printf("Fetched %d node metrics (took %v)", len(nodeMetricsList.Items), time.Since(startTime))

	log.Println("Fetching node list from Kubernetes API...")
	nodesListStartTime := time.Now()
	nodes, err := s.kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{}) // Use the same context
	if err != nil {
		log.Printf("Error fetching node list: %v (total time for metrics: %v)", err, time.Since(startTime))
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	log.Printf("Fetched %d nodes from API (node list took %v, total metrics time so far: %v)", len(nodes.Items), time.Since(nodesListStartTime), time.Since(startTime))

	var detailedNodeMetrics []NodeMetrics
	var totalCPUUsageMilliCores, totalCPUCapacityMilliCores int64
	var totalMemUsageBytes, totalMemCapacityBytes int64

	for _, n := range nodes.Items {
		var nodeCPUUsage, nodeMemUsage resource.Quantity
		foundMetrics := false
		for _, nm := range nodeMetricsList.Items {
			if nm.Name == n.Name {
				nodeCPUUsage = nm.Usage["cpu"]
				nodeMemUsage = nm.Usage["memory"]
				foundMetrics = true
				break
			}
		}

		if !foundMetrics {
			log.Printf("Warning: Metrics not found for node %s", n.Name)
			nodeCPUUsage = *resource.NewMilliQuantity(0, resource.DecimalSI)
			nodeMemUsage = *resource.NewQuantity(0, resource.BinarySI)
		}

		allocatableCPU := n.Status.Allocatable["cpu"]
		allocatableMem := n.Status.Allocatable["memory"]

		nodeCPUUsageMilli := nodeCPUUsage.MilliValue()
		nodeMemUsageBytes := nodeMemUsage.Value()
		allocatableCPUMilli := allocatableCPU.MilliValue()
		allocatableMemBytes := allocatableMem.Value()

		totalCPUUsageMilliCores += nodeCPUUsageMilli
		totalCPUCapacityMilliCores += allocatableCPUMilli
		totalMemUsageBytes += nodeMemUsageBytes
		totalMemCapacityBytes += allocatableMemBytes

		cpuUsagePercent := 0.0
		if allocatableCPUMilli > 0 {
			cpuUsagePercent = float64(nodeCPUUsageMilli*100) / float64(allocatableCPUMilli)
		}
		memUsagePercent := 0.0
		if allocatableMemBytes > 0 {
			memUsagePercent = float64(nodeMemUsageBytes*100) / float64(allocatableMemBytes)
		}

		detailedNodeMetrics = append(detailedNodeMetrics, NodeMetrics{
			Name:                   n.Name,
			CPUUsageMilliCores:     nodeCPUUsageMilli,
			MemoryUsageBytes:       nodeMemUsageBytes,
			CPUAvailableMilliCores: allocatableCPUMilli,
			MemoryAvailableBytes:   allocatableMemBytes,
			CPUUsagePercentage:     cpuUsagePercent,
			MemUsagePercentage:     memUsagePercent,
		})
	}

	avgCPUUsage := 0.0
	if totalCPUCapacityMilliCores > 0 {
		avgCPUUsage = float64(totalCPUUsageMilliCores*100) / float64(totalCPUCapacityMilliCores)
	}
	avgMemUsage := 0.0
	if totalMemCapacityBytes > 0 {
		avgMemUsage = float64(totalMemUsageBytes*100) / float64(totalMemCapacityBytes)
	}

	log.Printf("Cluster metrics snapshot calculation completed (total time: %v)", time.Since(startTime))
	return &ClusterMetrics{
		TotalCPUUsageMilliCores:    totalCPUUsageMilliCores,
		TotalCPUCapacityMilliCores: totalCPUCapacityMilliCores,
		TotalMemoryUsageBytes:      totalMemUsageBytes,
		TotalMemoryCapacityBytes:   totalMemCapacityBytes,
		AverageCPUUsagePercentage:  avgCPUUsage,
		AverageMemUsagePercentage:  avgMemUsage,
		Nodes:                      detailedNodeMetrics,
	}, nil
}
