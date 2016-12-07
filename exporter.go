package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	xenAPI "github.com/janeprather/go-xen-api-client"
	"github.com/prometheus/client_golang/prometheus"
)

type exporterClass struct {
	metrics       []*prometheus.GaugeVec
	counter       prometheus.Counter
	replacer      *strings.Replacer
	hostname      string
	poolGatherers map[string]*poolGathererClass
}

type poolGathererClass struct {
	poolName        string
	lastKnownMaster string
	xenClients      map[string]*xenAPI.Client
}

func newExporter() *exporterClass {
	var err error
	e := &exporterClass{}
	e.hostname, err = os.Hostname()
	if err != nil {
		log.Fatalf("os.Hostname(): %s", err.Error())
	}
	e.poolGatherers = make(map[string]*poolGathererClass)
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

func (e *exporterClass) newGatherer(pool string) {
	log.Printf("Instantiating gatherer for %s\n", pool)
	gatherer := &poolGathererClass{}
	gatherer.poolName = pool
	gatherer.xenClients = make(map[string]*xenAPI.Client)
	e.poolGatherers[pool] = gatherer
}

func (e *exporterClass) gatherData() {
	log.Printf("Starting gather job for all pools")
	timeStarted := time.Now().Unix()

	// create a channel to collect data from other routines
	retCh := make(chan []*prometheus.GaugeVec)

	// launch a separate go routine for each pool
	for pool := range config.Pools {
		var ok bool
		if _, ok = e.poolGatherers[pool]; !ok {
			e.newGatherer(pool)
		}
		go e.poolGatherers[pool].gather(retCh)
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

	timeFinished := time.Now().Unix()
	lastUpdatedMetric := newMetric("last_updated",
		map[string]string{"host": e.hostname},
		float64(timeFinished))
	newMetrics = append(newMetrics, lastUpdatedMetric)
	gatherTimeMetric := newMetric("gather_time",
		map[string]string{"host": e.hostname},
		float64(timeFinished-timeStarted))
	newMetrics = append(newMetrics, gatherTimeMetric)
	log.Printf("Completed gather job for all pools in %d seconds\n",
		timeFinished-timeStarted)
	e.metrics = newMetrics
}

func (g *poolGathererClass) gather(retCh chan []*prometheus.GaugeVec) {

	var metricList []*prometheus.GaugeVec
	var defaultSRList []xenAPI.SRRef

	defer func() { retCh <- metricList }()

	timeStarted := time.Now().Unix()

	xenClient, session, err := g.getXenClient()
	if err != nil {
		log.Printf("Error getting XAPI client for %s: %s\n", g.poolName, err.Error())
		return
	}

	timeConnected := time.Now().Unix()
	log.Printf("gatherPoolData(): %s: session established in %d seconds\n",
		g.poolName, timeConnected-timeStarted)

	poolRecs, err := xenClient.Pool.GetAllRecords(session)
	if err != nil {
		log.Printf("Error getting pool records for %s: %s", g.poolName, err.Error())
		return
	}

	hostRecs, err := xenClient.Host.GetAllRecords(session)
	if err != nil {
		log.Printf("Error getting host records for %s: %s\n", g.poolName, err.Error())
		return
	}

	vmRecs, err := xenClient.VM.GetAllRecords(session)
	if err != nil {
		log.Printf("Error getting vm records for %s: %s\n", g.poolName, err.Error())
		return
	}

	vmMetricsRecs, err := xenClient.VMMetrics.GetAllRecords(session)
	if err != nil {
		log.Printf("Error getting vm metrics records for %s: %s\n", g.poolName, err.Error())
		return
	}

	hostMetricsRecs, err := xenClient.HostMetrics.GetAllRecords(session)
	if err != nil {
		log.Printf("Error getting host metrics records for %s: %s\n", g.poolName, err.Error())
		return
	}

	srRecs, err := xenClient.SR.GetAllRecords(session)
	if err != nil {
		log.Printf("Error getting sr records for %s: %s", g.poolName, err.Error())
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
					"pool":       g.poolName,
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
					"pool":       g.poolName,
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
					"pool":       g.poolName,
					"type":       srRec.Type,
					"name_label": srRec.NameLabel,
				}, float64(srRec.PhysicalUtilisation))
			metricList = append(metricList, physicalUtilisationMetric)
		}

		// set the physical_pct_allocated metric for the sr
		physicalPctAllocated := float64(0)
		if srRec.PhysicalSize > 0 {
			physicalPctAllocated = float64(srRec.PhysicalUtilisation) * 100 /
				float64(srRec.PhysicalSize)
		}
		if metricEnabled("physical_pct_allocated") {
			physicalPctAllocatedMetric := newMetric("physical_pct_allocated",
				map[string]string{
					"uuid":       srRec.UUID,
					"pool":       g.poolName,
					"type":       srRec.Type,
					"name_label": srRec.NameLabel,
				}, physicalPctAllocated)
			metricList = append(metricList, physicalPctAllocatedMetric)
		}

		// set the virtual_allocation metric for the sr
		if metricEnabled("virtual_allocation") {
			virtualAllocationMetric := newMetric("virtual_allocation",
				map[string]string{
					"uuid":       srRec.UUID,
					"pool":       g.poolName,
					"type":       srRec.Type,
					"name_label": srRec.NameLabel,
				}, float64(srRec.VirtualAllocation))
			metricList = append(metricList, virtualAllocationMetric)
		}

	}

	timeGenerated := time.Now().Unix()
	log.Printf("gatherPoolData(): %s: gather time %d seconds\n",
		g.poolName, timeGenerated-timeStarted)

}

func (g *poolGathererClass) getXenClient() (
	xenClient *xenAPI.Client, session xenAPI.SessionRef, err error) {
	var hostList = config.Pools[g.poolName]

	// if a lastKnownMaster exists in our host list, bump it to the top
	if len(g.lastKnownMaster) != 0 {
		var newHostList = []string{g.lastKnownMaster}

		for key, host := range hostList {
			if host == g.lastKnownMaster {
				hostList = append(hostList[:key], hostList[key+1:]...)
				break
			}
			var ips []net.IP
			ips, err = net.LookupIP(host)
			if err != nil {
				ipMatch := false
				for _, ip := range ips {
					if g.lastKnownMaster == ip.String() {
						hostList = append(hostList[:key], hostList[key+1:]...)
						ipMatch = true
						break
					}
				}
				if ipMatch {
					break
				}
			}
		}
		hostList = newHostList
	}

	for _, host := range hostList {
		xenClient, session, err = g.tryXenClient(host)
		if err == nil {
			return xenClient, session, nil
		}
		log.Printf("tryXenClient(): %s: %s\n", host, err.Error())
	}

	return nil, "", fmt.Errorf(
		"%s: unable to authenticate into a master host", g.poolName)
}

func (g *poolGathererClass) tryXenClient(host string) (
	xenClient *xenAPI.Client, session xenAPI.SessionRef, err error) {

	var ok bool
	if xenClient, ok = g.xenClients[host]; !ok {
		log.Printf("Instantiating xenAPI.Client for %s\n", host)
		// no xapi client exists for this host, create a new one
		xenClient, err = xenAPI.NewClient("https://"+host, nil)
		if err != nil {
			return nil, "", fmt.Errorf("NewClient(): %s: %s\n", host, err.Error())
		}
	}

	sessionCh := make(chan xenAPI.SessionRef)
	errCh := make(chan error)
	go func(xenClient *xenAPI.Client, sessionCh chan xenAPI.SessionRef,
		errCh chan error) {

		session, err = xenClient.Session.LoginWithPassword(
			config.Auth.Username, config.Auth.Password,
			"1.0", "xapi_exporter")
		if err != nil {
			errCh <- err
		} else {
			sessionCh <- session
		}
	}(xenClient, sessionCh, errCh)

	select {
	case err := <-errCh:
		errParts := strings.Split(err.Error(), " ")
		if errParts[2] == "HOST_IS_SLAVE" {
			return g.tryXenClient(errParts[3])
		}
		return nil, "", fmt.Errorf(
			"LoginWithPassword(): %s: %s\n", host, err.Error())
	case <-time.After(time.Second * config.TimeoutLogin):
		return nil, "", fmt.Errorf(
			"LoginWithPassword(): timeout after %d seconds", config.TimeoutLogin)
	case session = <-sessionCh:
	}
	g.xenClients[host] = xenClient
	g.lastKnownMaster = host
	return
}
