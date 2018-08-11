package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"gopkg.in/yaml.v2"

	mtr "github.com/Shinzu/go-mtr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
)

type Exporter struct {
	mutex    sync.Mutex
	sent     *prometheus.CounterVec
	received *prometheus.CounterVec
	dropped  *prometheus.CounterVec
	lost     *prometheus.CounterVec
	latency     *prometheus.SummaryVec
	failed     *prometheus.CounterVec
}

type Config struct {
	Arguments    []string `yaml:"args"`
	Hosts        []Host   `yaml:"hosts"`
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
		sent: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Name:      "sent",
				Help:      "packets sent",
			},
			[]string{alias, server, hop_id, hop_ip},
		),
		received: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Name:      "received",
				Help:      "packets received",
			},
			[]string{alias, server, hop_id, hop_ip},
		),
		dropped: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Name:      "dropped",
				Help:      "packets dropped",
			},
			[]string{alias, server, hop_id, hop_ip},
		),
		lost: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Name:      "lost",
				Help:      "packets lost in percent",
			},
			[]string{alias, server, hop_id, hop_ip},
		),
		latency: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Namespace:  Namespace,
				Name:       "latency",
				Help:       "packet latency in microseconds",
				Objectives: map[float64]float64{
					0.5: 0.05,
					0.9: 0.01,
					0.99: 0.001,
				},
				MaxAge: prometheus.DefMaxAge,
				AgeBuckets: prometheus.DefAgeBuckets,
				BufCap: prometheus.DefBufCap,
			},
			[]string{alias, server, hop_id, hop_ip},
		),
		failed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Name:      "failed",
				Help:      "MTR runs failed",
			},
			[]string{alias, server},
		),
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	e.sent.Describe(ch)
	e.received.Describe(ch)
	e.dropped.Describe(ch)
	e.lost.Describe(ch)
	e.latency.Describe(ch)
	e.failed.Describe(ch)
}

func (e *Exporter) collect() error {
	for {
		results := make(chan *TargetFeedback)
		wg := &sync.WaitGroup{}
		wg.Add(len(config.Hosts))

		for w, host := range config.Hosts {
			go func(w int, host Host) {
				log.Infoln("worker", w, "processing job", host.Name, "aliased as", host.Alias)
				err := worker(w, host, results, wg)
				if err != nil {
				  log.Errorf("worker %d failed job %v aliased as %v: %v\n", w, host.Name, host.Alias, err)
					e.failed.WithLabelValues(host.Alias, host.Name).Inc()
				} else {
					log.Infoln("worker", w, "finished job", host.Name, "aliased as", host.Alias)
				}
			}(w, host)
		}

		go func() {
			wg.Wait()
			close(results)
		}()

		for tf := range results {
			for _, host := range tf.Hosts {
				e.sent.WithLabelValues(tf.Alias, tf.Target, strconv.Itoa(host.Hop), host.IP.String()).Add(float64(host.Sent))
				e.received.WithLabelValues(tf.Alias, tf.Target, strconv.Itoa(host.Hop), host.IP.String()).Add(float64(host.Received))
				e.dropped.WithLabelValues(tf.Alias, tf.Target, strconv.Itoa(host.Hop), host.IP.String()).Add(float64(host.Dropped))
				e.lost.WithLabelValues(tf.Alias, tf.Target, strconv.Itoa(host.Hop), host.IP.String()).Add(host.LostPercent * 100)
				e.latency.WithLabelValues(tf.Alias, tf.Target, strconv.Itoa(host.Hop), host.IP.String()).Observe(host.Mean)
			}
		}
		time.Sleep(1)
	}
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.sent.Collect(ch)
	e.received.Collect(ch)
	e.dropped.Collect(ch)
	e.lost.Collect(ch)
	e.latency.Collect(ch)
	e.failed.Collect(ch)
	return
}

func trace(host Host, results chan<- *TargetFeedback) error {
	// run MTR and wait for it to complete
	a := mtr.New(1, host.Name, config.Arguments...)
	<-a.Done

	// output result
	if a.Error == nil {
		results <- &TargetFeedback{
			Target: host.Name,
			Alias:  host.Alias,
			Hosts:  a.Hosts,
		}
	}
	return a.Error
}

func worker(id int, host Host, results chan<- *TargetFeedback, wg *sync.WaitGroup) error {
	defer wg.Done()
	return trace(host, results)
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

	go exporter.collect()

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
