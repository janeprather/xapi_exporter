package main

import "github.com/prometheus/client_golang/prometheus"

var metricHelp = map[string]string{
	// special metric for prometheus exporter overhead
	"last_updated": "seconds since epoch of last stats collection",

	// xapi derived metrics
	"cpu_count":         "the number of physical CPUs on the host",
	"cpu_pct_allocated": "percent vCPUs over total CPUs on this host",
	"default_storage":   "true if SR is a default SR for a pool, otherwise false",
	"ha_allow_overcommit": "If set to false then operations which would cause " +
		"the Pool to become overcommitted will be blocked.",
	"ha_enabled": "true if HA is enabled on the pool, false otherwise",
	"ha_host_failures_to_tolerate": "Number of host failures to tolerate " +
		"before the Pool is declared to be overcommitted",
	"ha_overcommitted": "True if the Pool is considered to be overcommitted " +
		"i.e. if there exist insufficient physicalk resources to tolerate the " +
		"configured number of host failures",
	"memory_free":            "Free host memory (bytes)",
	"memory_pct_allocated":   "percent used memory over total memory on this host",
	"memory_total":           "Total host memory (bytes)",
	"physical_pct_allocated": "percent of physical_utilisation over physical_size",
	"physical_size":          "total physical size of the repository (in bytes)",
	"physical_utilisation": "physical space currently utilised on this storage " +
		"repository (in bytes). Note that for sparse disk formats, " +
		"physical_utilisation may be less than virtual_allocation.",
	"resident_vcpu_count": "count of vCPUs on VMs running on this host",
	"resident_vm_count":   "count of VMs running on this host",
	"virtual_allocation":  "sum of virtual_sizes of all VDIs in this SR (in bytes)",
	"wlb_enabled":         "true if workload balancing is enabled on the pool",
}

func newMetric(name string, labelValues map[string]string, value float64) *prometheus.GaugeVec {
	var metricLabels []string
	for key := range labelValues {
		metricLabels = append(metricLabels, key)
	}
	metric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: config.NameSpace,
		Name:      name,
		Help:      metricHelp[name],
	}, metricLabels)
	metric.With(prometheus.Labels(labelValues)).Set(value)
	return metric
}

func metricEnabled(name string) bool {
	if len(config.EnabledMetrics) == 0 {
		return true
	}
	for _, enabledName := range config.EnabledMetrics {
		if name == enabledName {
			return true
		}
	}
	return false
}
