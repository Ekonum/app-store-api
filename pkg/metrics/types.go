package metrics

// NodeMetrics represents metrics for a single Kubernetes node.
type NodeMetrics struct {
	Name                   string  `json:"name"`
	CPUUsageMilliCores     int64   `json:"cpu_usage_milli_cores"`     // Actual milliCores used
	MemoryUsageBytes       int64   `json:"memory_usage_bytes"`        // Actual bytes used
	CPUAvailableMilliCores int64   `json:"cpu_available_milli_cores"` // Total allocatable milliCores
	MemoryAvailableBytes   int64   `json:"memory_available_bytes"`    // Total allocatable bytes
	CPUUsagePercentage     float64 `json:"cpu_usage_percentage"`
	MemUsagePercentage     float64 `json:"mem_usage_percentage"`
}

// ClusterMetrics aggregates metrics for the entire cluster.
type ClusterMetrics struct {
	TotalCPUUsageMilliCores    int64         `json:"total_cpu_usage_milli_cores"`
	TotalCPUCapacityMilliCores int64         `json:"total_cpu_capacity_milli_cores"`
	TotalMemoryUsageBytes      int64         `json:"total_memory_usage_bytes"`
	TotalMemoryCapacityBytes   int64         `json:"total_memory_capacity_bytes"`
	AverageCPUUsagePercentage  float64       `json:"average_cpu_usage_percentage"`
	AverageMemUsagePercentage  float64       `json:"average_mem_usage_percentage"`
	Nodes                      []NodeMetrics `json:"nodes,omitempty"`
}
