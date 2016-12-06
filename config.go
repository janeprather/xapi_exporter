package main

import (
	"flag"
	"io/ioutil"
	"log"
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
	TimeoutData  time.Duration
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
}
