package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"sync"

	"gopkg.in/yaml.v2"

	mtr "github.com/fastly/go-mtr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
)

type Exporter struct {
	mutex    sync.Mutex
	sent     *prometheus.GaugeVec
	received *prometheus.GaugeVec
}

type Config struct {
	Arguments    string `yaml:"args"`
	ReportCycles int    `yaml:"cycles"`
	Hosts        []Host `yaml:"hosts"`
}

type Host struct {
	Name  string `yaml:"name"`
	Alias string `yaml:"alias"`
}

type targetFeedback struct {
	Target string
	Hosts  []*mtr.Host
}

var config Config

const (
	Namespace = "mtr"
)

func NewExporter() *Exporter {
	var (
		hop_id = "hop_id"
		hop_ip = "hop_ip"
		server = "server"
	)

	return &Exporter{
		sent: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Name:      "sent",
				Help:      "packets sent",
			},
			[]string{server, hop_id, hop_ip},
		),
		received: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Name:      "received",
				Help:      "packets received",
			},
			[]string{server, hop_id, hop_ip},
		),
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	e.sent.Describe(ch)
	e.received.Describe(ch)
}

func (e *Exporter) collect(ch chan<- prometheus.Metric) error {
	jobs := make(chan string, 1024)
	results := make(chan *targetFeedback)
	defer close(results)

	for w := 1; w <= len(config.Hosts); w++ {
		go worker(w, jobs, results)
	}

	for _, host := range config.Hosts {
		jobs <- host.Name
	}
	close(jobs)

	for tf := range results {
		fmt.Println(tf)
		for _, host := range tf.Hosts {
			e.sent.WithLabelValues(tf.Target, strconv.Itoa(host.Hop), host.IP.String()).Set(float64(host.Sent))
			e.received.WithLabelValues(tf.Target, strconv.Itoa(host.Hop), host.IP.String()).Set(float64(host.Received))
		}
	}

	e.sent.Collect(ch)
	e.received.Collect(ch)
	return nil
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if err := e.collect(ch); err != nil {
		log.Errorf("Error scraping mtr: %s", err)
	}
	return
}

func trace(host string, results chan<- *targetFeedback) {
	// run MTR and wait for it to complete
	cycles := config.ReportCycles
	args := config.Arguments
	arg := fmt.Sprintf("--%v", args)
	a := mtr.New(cycles, host, arg)
	<-a.Done

	// output result
	if a.Error != nil {
		log.Errorln("%v", a.Error)
	} else {
		results <- &targetFeedback{
			Target: host,
			Hosts:  a.Hosts,
		}
	}
}

func worker(id int, jobs <-chan string, results chan<- *targetFeedback) {
	for job := range jobs {
		log.Infoln("worker", id, "processing job", job)
		trace(job, results)
	}
}

func main() {
	var (
		configFile    = flag.String("config.file", "mtr.yaml", "MTR exporter configuration file.")
		listenAddress = flag.String("web.listen-address", ":9116", "The address to listen on for HTTP requests.")
		showVersion   = flag.Bool("version", false, "Print version information.")
	)

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

	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		log.Fatalf("Error parsing config file: %s", err)
	}

	prometheus.MustRegister(version.NewCollector("mtr_exporter"))
	exporter := NewExporter()
	prometheus.MustRegister(exporter)

	fmt.Println(config)

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
