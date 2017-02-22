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
	dropped  *prometheus.GaugeVec
	lost     *prometheus.GaugeVec
	mean     *prometheus.GaugeVec
	best     *prometheus.GaugeVec
	worst    *prometheus.GaugeVec
	standard *prometheus.GaugeVec
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

type TargetFeedback struct {
	Target string
	Alias  string
	Hosts  []*mtr.Host
}

var config Config

const (
	Namespace = "mtr"
)

func NewExporter() *Exporter {
	var (
		alias  = "alias"
		server = "server"
		hop_id = "hop_id"
		hop_ip = "hop_ip"
	)

	return &Exporter{
		sent: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Name:      "sent",
				Help:      "packets sent",
			},
			[]string{alias, server, hop_id, hop_ip},
		),
		received: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Name:      "received",
				Help:      "packets received",
			},
			[]string{alias, server, hop_id, hop_ip},
		),
		dropped: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Name:      "dropped",
				Help:      "packets dropped",
			},
			[]string{alias, server, hop_id, hop_ip},
		),
		lost: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Name:      "lost",
				Help:      "packets lost in percent",
			},
			[]string{alias, server, hop_id, hop_ip},
		),
		mean: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Name:      "mean",
				Help:      "mean time of all packets in microseconds",
			},
			[]string{alias, server, hop_id, hop_ip},
		),
		best: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Name:      "best",
				Help:      "best time for a packet in microseconds",
			},
			[]string{alias, server, hop_id, hop_ip},
		),
		worst: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Name:      "worst",
				Help:      "worst time for a packet in microseconds",
			},
			[]string{alias, server, hop_id, hop_ip},
		),
		standard: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Name:      "standard",
				Help:      "standard deviation of the latencies to each hop",
			},
			[]string{alias, server, hop_id, hop_ip},
		),
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	e.sent.Describe(ch)
	e.received.Describe(ch)
}

func (e *Exporter) collect(ch chan<- prometheus.Metric) error {
	results := make(chan *TargetFeedback)
	wg := &sync.WaitGroup{}
	wg.Add(len(config.Hosts))

	for w, host := range config.Hosts {
		go worker(w, host, results, wg)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for tf := range results {
		for _, host := range tf.Hosts {
			e.sent.WithLabelValues(tf.Alias, tf.Target, strconv.Itoa(host.Hop), host.IP.String()).Set(float64(host.Sent))
			e.received.WithLabelValues(tf.Alias, tf.Target, strconv.Itoa(host.Hop), host.IP.String()).Set(float64(host.Received))
			e.dropped.WithLabelValues(tf.Alias, tf.Target, strconv.Itoa(host.Hop), host.IP.String()).Set(float64(host.Dropped))
			e.lost.WithLabelValues(tf.Alias, tf.Target, strconv.Itoa(host.Hop), host.IP.String()).Set(host.LostPercent * 100)
			e.mean.WithLabelValues(tf.Alias, tf.Target, strconv.Itoa(host.Hop), host.IP.String()).Set(host.Mean)
			e.best.WithLabelValues(tf.Alias, tf.Target, strconv.Itoa(host.Hop), host.IP.String()).Set(float64(host.Best))
			e.worst.WithLabelValues(tf.Alias, tf.Target, strconv.Itoa(host.Hop), host.IP.String()).Set(float64(host.Worst))
			e.standard.WithLabelValues(tf.Alias, tf.Target, strconv.Itoa(host.Hop), host.IP.String()).Set(host.StandardDev)
		}
	}

	e.sent.Collect(ch)
	e.received.Collect(ch)
	e.dropped.Collect(ch)
	e.lost.Collect(ch)
	e.mean.Collect(ch)
	e.best.Collect(ch)
	e.worst.Collect(ch)
	e.standard.Collect(ch)
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

func trace(host Host, results chan<- *TargetFeedback) {
	// run MTR and wait for it to complete
	cycles := config.ReportCycles
	args := config.Arguments
	arg := fmt.Sprintf("--%v", args)
	a := mtr.New(cycles, host.Name, arg)
	<-a.Done

	// output result
	if a.Error != nil {
		log.Errorln("%v", a.Error)
	} else {
		results <- &TargetFeedback{
			Target: host.Name,
			Alias:  host.Alias,
			Hosts:  a.Hosts,
		}
	}
}

func worker(id int, host Host, results chan<- *TargetFeedback, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Infoln("worker", id, "processing job", host.Name, "aliased as", host.Alias)
	trace(host, results)
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
