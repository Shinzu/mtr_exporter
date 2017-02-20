package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"gopkg.in/yaml.v2"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
)

var (
	configFile    = flag.String("config.file", "mtr.yml", "MTR exporter configuration file.")
	listenAddress = flag.String("web.listen-address", ":9116", "The address to listen on for HTTP requests.")
	showVersion   = flag.Bool("version", false, "Print version information.")
)

type Config struct {
	Protocol     string          `yaml:"protocol"` // Defaults to "tcp"
	reportCycles int             `yaml:"cycles"`   // Defaults to 30
	Hosts        map[string]Host `yaml:"hosts"`
}

type Host struct {
	Host  string `yaml:"host"`
	Alias string `yaml:"alias"`
}

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Fprintln(os.Stdout, version.Print("mtr_exporter"))
		os.Exit(0)
	}

	log.Infoln("Starting mtr_exporter", version.Info())
	log.Infoln("Build context", version.BuildContext())

	yamlFile, err := ioutil.ReadFile(*configFile)

	if err != nil {
		log.Fatalf("Error reading config file: %s", err)
	}

	config := Config{}

	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		log.Fatalf("Error parsing config file: %s", err)
	}

	http.Handle("/metrics", prometheus.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
            <head><title>MTR Exporter</title></head>
            <body>
            <h1>MTR Exporter</h1>
            <p><a href="/metrics">Metrics</a></p>
            </body>
            </html>`))
	})

	log.Infoln("Listening on", *listenAddress)
	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		log.Fatalf("Error starting HTTP server: %s", err)
	}
}
