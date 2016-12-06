package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
)

const appTitle = "XAPI Data Exporter"

const httpIndex = "<html><head><title>%s</title></head>" +
	"<body><h1>%s</h1><a href=\"%s\">View Metrics</a>" +
	"</body></html>"

const metricsPath = "/metrics"

func main() {
	var err error

	// parse commandline flags
	flag.Parse()

	// load configuration
	initConfig()

	// instantiate exporter object
	exporter := newExporter()

	// register the exporter with prom package
	prometheus.MustRegister(exporter)

	// register the prometheus handler with http service
	http.Handle(metricsPath, prometheus.Handler())

	// unless the metricsPath is /, offer a minimal page with a link at /
	if len(metricsPath) > 0 && metricsPath != "/" {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(fmt.Sprintf(httpIndex, appTitle, appTitle, metricsPath)))
		})
	}

	log.Printf("Starting HTTP service on %s\n", config.BindAddress)
	err = http.ListenAndServe(config.BindAddress, nil)
	if err != nil {
		log.Fatal(err)
	}
}
