# xapi_exporter

A Prometheus exporter which uses XenServer's XAPI to derive various metrics.

## Usage

```
xapi_exporter [-config <configFile>]
```

If no *configFile* is specified, xapi_exporter will attempt to read its
configuration from "xapi_exporter.yml" in the working directory.

## Config

YML config file should look something like:

```
bindaddress: ":9290"
namespace: "xenstats"

pools:
  "xen-pool-a":
  - "xen-pool-a-host-1"
  - "xen-pool-a-host-2"
  "xen-pool-b":
  - "xen-pool-b-host-1"
  - "xen-pool-b-host-2"
  - "xen-pool-b-host-3"
  - "xen-pool-b-host-4"

auth:
  username: "your_xapi_user"
  password: "your_xapi_password"
```

Note that you can optionally select *which* metrics are reported by including
an "enabledmetrics" key to an array of strings.  Without the "enabledmetrics"
option, all metrics are enabled.  If one or more metrics are specified with
this option, only those metrics will be reported.

```
enabledmetrics:
- "memory_free"
- "memory_total"
```

The above would only generate memory_free and memory_total metrics.

## Metrics

Below are available metrics.

### Pool Metrics

These metrics are relevant to XenServer pools, and will have a "pool" label
to distinguish them.

**ha_allow_overcommit**: If set to false then operations which would cause
the Pool to become overcommitted will be blocked.

**ha_enabled**: true if HA is enabled on the pool, false otherwise

**ha_host_failures_to_tolerate**: Number of host failures to tolerate before
the Pool is declared to be overcommitted

**ha_overcommitted**: True if the Pool is considered to be overcommitted i.e.
if there exist insufficient physicalk resources to tolerate the configured
number of host failures

**wlb_enabled**: true if workload balancing is enabled on the pool

### Host Metrics

These metrics are relevant to XenServer hosts, and will have a "host" label
to distinguish them.

**cpu_pct_allocated**: percent vCPUs over total CPUs on this host

**cpu_count**: the number of physical CPUs on the host

**memory_pct_allocated**: percent used memory over total memory on this host

**memory_free**: Free host memory (bytes)

**memory_total**: Total host memory (bytes)

**resident_vcpu_count**: count of vCPUs on VMs running on this host

**resident_vm_count**: count of VMs running on this host

### Storage Metrics

These metrics are relevant to XenServer SRs, and will have "pool", "uuid",
and "type" labels to distinguish them, as well as a "name_label" label for
informational purposes.

**default_storage**: true if SR is a default SR for a pool, otherwise false

**physical_pct_allocated**: percent of physical_utilisation over physical_size

**physical_size**: total physical size of the repository (in bytes)

**physical_utilisation**: physical space currently utilised on this storage
repository (in bytes). Note that for sparse disk formats, physical_utilisation
may be less than virtual_allocation.

**virtual_allocation**: sum of virtual_sizes of all VDIs in this SR (in bytes)
