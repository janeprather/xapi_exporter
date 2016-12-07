package main

import (
	"flag"
	"io/ioutil"
	"log"
	"regexp"
	"time"

	yaml "gopkg.in/yaml.v2"
)

var configFile = flag.String("config", "xapi_exporter.yml", "config filename")

var config *configClass

type configClass struct {
	BindAddress    string
	NameSpace      string
	EnabledMetrics []string
	Pools          map[string][]string
	Auth           struct {
		Username string
		Password string
	}
	TimeoutLogin time.Duration
}

func initConfig() {
	config = &configClass{}

	data, err := ioutil.ReadFile(*configFile)
	if err != nil {
		log.Fatalf("Unable to read config: %s", err.Error())
	}

	err = yaml.Unmarshal(data, config)
	if err != nil {
		log.Fatalf("Unable to parse config: %s", err.Error())
	}

	validateConfig()
}

func validateConfig() {
	hasErrors := false

	// ensure that bindaddress is a valid pattern
	if match, err := regexp.MatchString("(i?)^[0-9a-z\\-\\_\\.]*:[0-9]+$",
		config.BindAddress); err != nil || !match {
		log.Println("config: bindaddress must be in \"host:port\" or \":port\" " +
			"format")
		hasErrors = true
	}

	// ensure that prometheus metric namespace is reasonable
	if match, err := regexp.MatchString("(i?)^[a-z0-9]+$",
		config.NameSpace); err != nil || !match {
		log.Println("config: namespace must be reasonable metrics namespace " +
			"(try \"xen\")")
		hasErrors = true
	}

	// ensure that username has been specified
	if config.Auth.Username == "" {
		log.Println("config: auth/username must be specified")
		hasErrors = true
	}

	// ensure that password has been specified
	if config.Auth.Password == "" {
		log.Println("config: auth/password must be specified")
		hasErrors = true
	}

	// ensure that a timeout value isn't missing/zero
	if config.TimeoutLogin == 0 {
		log.Println("config: timeoutlogin should be non-zero")
		hasErrors = true
	}

	// ensure at least one pool is configured from which to gather data
	if len(config.Pools) == 0 {
		log.Println("config: no pools configured")
		hasErrors = true
	}

	// if enabledmetrics is specified, ensure only valid metrics are listed
	if len(config.EnabledMetrics) > 0 {
		for _, mName := range config.EnabledMetrics {
			if _, ok := metricHelp[mName]; !ok {
				log.Printf("config: enabledmetrics contains invalid metric name: %s\n",
					mName)
				hasErrors = true
			}
		}
	}

	// if any errors occurred, abort the program
	if hasErrors {
		log.Fatalln("Aborting due to configuration errors.")
	}
}
