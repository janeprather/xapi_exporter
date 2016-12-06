package main

import (
	"log"
	"os"
	"strconv"
	"strings"

	xenAPI "github.com/janeprather/go-xen-api-client"
	"github.com/prometheus/client_golang/prometheus"
)

type exporterClass struct {
	metrics  []*prometheus.GaugeVec
	counter  prometheus.Counter
	replacer *strings.Replacer
	hostname string
}

func newExporter() *exporterClass {
	var err error
	e := &exporterClass{}
	e.hostname, err = os.Hostname()
	if err != nil {
		log.Fatalf("os.Hostname(): %s", err.Error())
	}
	lastUpdatedMetric := newMetric("last_updated",
		map[string]string{"host": e.hostname},
		0)
	e.metrics = append(e.metrics, lastUpdatedMetric)
	return e
}

// Describe calls Describe(ch) for all stored metrics
func (e *exporterClass) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range e.metrics {
		m.Describe(ch)
	}
}

// Collect gathers data, then calls Collect(ch) for all stored metrics
func (e *exporterClass) Collect(ch chan<- prometheus.Metric) {
	e.gatherData()

	for _, m := range e.metrics {
		m.Collect(ch)
	}
}

func (e *exporterClass) gatherData() {

	// create a channel to collect data from other routines
	retCh := make(chan []*prometheus.GaugeVec)

	// launch a separate go routine for each pool
	for pool := range config.Pools {
		go gatherPoolData(pool, retCh)
	}

	// create an array in which to gather results
	var retLists [][]*prometheus.GaugeVec

	for {
		// wait for results from one of the gather routines
		metricList := <-retCh
		retLists = append(retLists, metricList)

		// break if we have results back for all the routines
		if len(retLists) == len(config.Pools) {
			break
		}
	}

	var newMetrics []*prometheus.GaugeVec
	for _, metricsList := range retLists {
		for _, metric := range metricsList {
			newMetrics = append(newMetrics, metric)
		}
	}

	lastUpdatedMetric := newMetric("last_updated",
		map[string]string{"host": e.hostname},
		0)
	newMetrics = append(newMetrics, lastUpdatedMetric)
	e.metrics = newMetrics
}

func gatherPoolData(pool string, retCh chan []*prometheus.GaugeVec) {
	var metricList []*prometheus.GaugeVec
	var defaultSRList []xenAPI.SRRef

	xenClient, session, err := getXenClient(pool)
	if err != nil {
		log.Printf("Error getting XAPI client for %s: %s\n", pool, err.Error())
		retCh <- metricList
		return
	}

	hostRecs, err := xenClient.Host.GetAllRecords(session)
	if err != nil {
		log.Printf("Error getting host records for %s: %s\n", pool, err.Error())
		retCh <- metricList
		return
	}

	vmRecs, err := xenClient.VM.GetAllRecords(session)
	if err != nil {
		log.Printf("Error getting vm records for %s: %s\n", pool, err.Error())
		retCh <- metricList
		return
	}

	vmMetricsRecs, err := xenClient.VMMetrics.GetAllRecords(session)
	if err != nil {
		log.Printf("Error getting vm metrics records for %s: %s\n", pool, err.Error())
		retCh <- metricList
		return
	}

	hostMetricsRecs, err := xenClient.HostMetrics.GetAllRecords(session)
	if err != nil {
		log.Printf("Error getting host metrics records for %s: %s\n", pool, err.Error())
		retCh <- metricList
		return
	}

	for _, hostRec := range hostRecs {
		// tally vcpus and vms
		vCPUCount := 0
		vmCount := 0
		for _, vmRef := range hostRec.ResidentVMs {
			if vmRec, ok := vmRecs[vmRef]; ok && !vmRec.IsControlDomain {
				vmCount++
				if vmMetricsRec, ok := vmMetricsRecs[vmRec.Metrics]; ok {
					vCPUCount += vmMetricsRec.VCPUsNumber
				}
			}
		}

		cpuCount, _ := strconv.ParseFloat(hostRec.CPUInfo["cpu_count"], 64)

		// set cpu_count metric for the host
		if metricEnabled("cpu_count") {
			cpuCountMetric := newMetric("cpu_count",
				map[string]string{"host": hostRec.Hostname},
				cpuCount)
			metricList = append(metricList, cpuCountMetric)
		}

		// set cpu_allocation metric for the host
		if metricEnabled("cpu_allocation") {
			cpuAllocationMetric := newMetric("cpu_allocation",
				map[string]string{"host": hostRec.Hostname},
				float64(vCPUCount)*100/cpuCount)
			metricList = append(metricList, cpuAllocationMetric)
		}

		// set memory_total metric for host
		if metricEnabled("memory_total") {
			memoryTotalMetric := newMetric("memory_total",
				map[string]string{"host": hostRec.Hostname},
				float64(hostMetricsRecs[hostRec.Metrics].MemoryTotal))
			metricList = append(metricList, memoryTotalMetric)
		}

		// set memory_free metric for host
		if metricEnabled("memory_free") {
			memoryFreeMetric := newMetric("memory_free",
				map[string]string{"host": hostRec.Hostname},
				float64(hostMetricsRecs[hostRec.Metrics].MemoryFree))
			metricList = append(metricList, memoryFreeMetric)
		}

		// set memory_allocation metric for host
		if metricEnabled("memory_allocation") {
			hostMetricsRec := hostMetricsRecs[hostRec.Metrics]
			memoryAllocationMetric := newMetric("memory_allocation",
				map[string]string{"host": hostRec.Hostname},
				float64(hostMetricsRec.MemoryTotal-hostMetricsRec.MemoryFree)*100/
					float64(hostMetricsRec.MemoryTotal))
			metricList = append(metricList, memoryAllocationMetric)
		}

		// set resident_vcpu_count metric for host
		if metricEnabled("resident_vcpu_count") {
			residentVCPUCountMetric := newMetric("resident_vcpu_count",
				map[string]string{"host": hostRec.Hostname},
				float64(vCPUCount))
			metricList = append(metricList, residentVCPUCountMetric)
		}

		// set resident_vm_count metric for host
		if metricEnabled("resident_vm_count") {
			residentVMCountMetric := newMetric("resident_vm_count",
				map[string]string{"host": hostRec.Hostname},
				float64(vmCount))
			metricList = append(metricList, residentVMCountMetric)
		}
	}

	// fetch collection of pool records and, if successful, relevant metrics
	poolRecs, err := xenClient.Pool.GetAllRecords(session)
	if err != nil {
		log.Printf("Error getting pool records for %s: %s", pool, err.Error())
		retCh <- metricList
		return
	}
	for _, poolRec := range poolRecs {
		defaultSRList = append(defaultSRList, poolRec.DefaultSR)

		// set ha_allow_overcommit metric for pool
		if metricEnabled("ha_allow_overcommit") {
			haAllowOvercommitMetric := newMetric("ha_allow_overcommit",
				map[string]string{"pool": poolRec.NameLabel},
				boolFloat(poolRec.HaAllowOvercommit))
			metricList = append(metricList, haAllowOvercommitMetric)
		}

		// set ha_enabled metric for pool
		if metricEnabled("ha_enabled") {
			haEnabledMetric := newMetric("ha_enabled",
				map[string]string{"pool": poolRec.NameLabel},
				boolFloat(poolRec.HaEnabled))
			metricList = append(metricList, haEnabledMetric)
		}

		// set ha_host_failures_to_tolerate metric for pool
		if metricEnabled("ha_host_failures_to_tolerate") {
			haHostFailuresToTolerateMetric := newMetric("ha_host_failures_to_tolerate",
				map[string]string{"pool": poolRec.NameLabel},
				float64(poolRec.HaHostFailuresToTolerate))
			metricList = append(metricList, haHostFailuresToTolerateMetric)
		}

		// set the ha_overcommitted metric for pool
		if metricEnabled("ha_overcommitted") {
			haOvercommittedMetric := newMetric("ha_overcommitted",
				map[string]string{"pool": poolRec.NameLabel},
				boolFloat(poolRec.HaOvercommitted))
			metricList = append(metricList, haOvercommittedMetric)
		}

		// set the wlb_enabled metric for the pool
		if metricEnabled("wlb_enabled") {
			wlbEnabledMetric := newMetric("wlb_enabled",
				map[string]string{"pool": poolRec.NameLabel},
				boolFloat(poolRec.WlbEnabled))
			metricList = append(metricList, wlbEnabledMetric)
		}

	}

	srRecs, err := xenClient.SR.GetAllRecords(session)
	if err != nil {
		log.Printf("Error getting sr records for %s: %s", pool, err.Error())
		retCh <- metricList
		return
	}

	for srRef, srRec := range srRecs {
		defaultSR := false
		for _, defSR := range defaultSRList {
			if defSR == srRef {
				defaultSR = true
			}
		}

		// set the default_storage metric for the sr
		if metricEnabled("default_storage") {
			defaultStorageMetric := newMetric("default_storage",
				map[string]string{
					"uuid":       srRec.UUID,
					"pool":       pool,
					"type":       srRec.Type,
					"name_label": srRec.NameLabel,
				}, boolFloat(defaultSR))
			metricList = append(metricList, defaultStorageMetric)
		}

		// set the physical_size metric for the sr
		if metricEnabled("physical_size") {
			physicalSizeMetric := newMetric("physical_size",
				map[string]string{
					"uuid":       srRec.UUID,
					"pool":       pool,
					"type":       srRec.Type,
					"name_label": srRec.NameLabel,
				}, float64(srRec.PhysicalSize))
			metricList = append(metricList, physicalSizeMetric)
		}

		// set the physical_utilisation metric for the sr
		if metricEnabled("physical_utilisation") {
			physicalUtilisationMetric := newMetric("physical_utilisation",
				map[string]string{
					"uuid":       srRec.UUID,
					"pool":       pool,
					"type":       srRec.Type,
					"name_label": srRec.NameLabel,
				}, float64(srRec.PhysicalUtilisation))
			metricList = append(metricList, physicalUtilisationMetric)
		}

		// set the physical_pct_allocated metric for the sr
		if metricEnabled("physical_pct_allocated") {
			physicalPctAllocatedMetric := newMetric("physical_pct_allocated",
				map[string]string{
					"uuid":       srRec.UUID,
					"pool":       pool,
					"type":       srRec.Type,
					"name_label": srRec.NameLabel,
				}, float64(srRec.PhysicalUtilisation)*100/float64(srRec.PhysicalSize))
			metricList = append(metricList, physicalPctAllocatedMetric)
		}

		// set the virtual_allocation metric for the sr
		if metricEnabled("virtual_allocation") {
			virtualAllocationMetric := newMetric("virtual_allocation",
				map[string]string{
					"uuid":       srRec.UUID,
					"pool":       pool,
					"type":       srRec.Type,
					"name_label": srRec.NameLabel,
				}, float64(srRec.VirtualAllocation))
			metricList = append(metricList, virtualAllocationMetric)
		}

	}

	retCh <- metricList
}
