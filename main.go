package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	NozzleTemperature int     `json:"temp_nozzle"`
	BedTemperature    int     `json:"temp_bed"`
	Material          string  `json:"material"`
	ZPosition         float64 `json:"pos_z_mm"`
	PrintingSpeed     int     `json:"printing_speed"`
	FlowFactor        int     `json:"flow_factor"`
	Progress          int     `json:"progress"`
	PrintDuration     string  `json:"print_dur"`
	TimeEstimated     string  `json:"time_est"`
	TimeZone          string  `json:"time_zone"`
	ProjectName       string  `json:"project_name"`
}

// SessionCollector exposes session metrics
type Collector struct {
	prusaConnectHost            string
	nozzleTemperatureDescroptor *prometheus.Desc
	bedTemperatureDescroptor    *prometheus.Desc
	zPositionDescroptor         *prometheus.Desc
	printingSpeedDescroptor     *prometheus.Desc
	flowFactorDescroptor        *prometheus.Desc
	progressDescroptor          *prometheus.Desc
	printDurationDescroptor     *prometheus.Desc
	timeEstimatedDescroptor     *prometheus.Desc
	timeZoneDescroptor          *prometheus.Desc
}

// NewSessionCollector takes a transmission.Client and returns a SessionCollector
func NewCollector(host string) *Collector {
	return &Collector{
		prusaConnectHost: host,
		nozzleTemperatureDescroptor: prometheus.NewDesc(
			"prusa_connect_temp_nozzle",
			"Temperature of the print nozzle in celsius",
			[]string{},
			nil,
		),
		bedTemperatureDescroptor: prometheus.NewDesc(
			"prusa_connect_temp_bed",
			"Temperature of the print bed in celsius",
			[]string{},
			nil,
		),
		zPositionDescroptor: prometheus.NewDesc(
			"prusa_connect_z_pozition",
			"Vertical pozition of the print head in millimeters",
			[]string{},
			nil,
		),
		printingSpeedDescroptor: prometheus.NewDesc(
			"prusa_connect_printing_speed",
			"Printing speed as a percentage",
			[]string{},
			nil,
		),
		flowFactorDescroptor: prometheus.NewDesc(
			"prusa_connect_flow_factor",
			"Flow factor",
			[]string{},
			nil,
		),
		progressDescroptor: prometheus.NewDesc(
			"prusa_connect_progress",
			"Print completeness as a percentage",
			[]string{},
			nil,
		),
		printDurationDescroptor: prometheus.NewDesc(
			"prusa_connect_print_duration",
			"Time passed since the current print job started in seconds",
			[]string{},
			nil,
		),
		timeEstimatedDescroptor: prometheus.NewDesc(
			"prusa_connect_time_estimated",
			"Estimated time remaining of the current print job in seconds",
			[]string{},
			nil,
		),
	}
}

// Describe implements the prometheus.Collector interface
func (c Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.nozzleTemperatureDescroptor
	ch <- c.bedTemperatureDescroptor
	ch <- c.zPositionDescroptor
	ch <- c.printingSpeedDescroptor
	ch <- c.flowFactorDescroptor
	ch <- c.progressDescroptor
	ch <- c.printDurationDescroptor
	ch <- c.timeEstimatedDescroptor
}

// Collect implements the prometheus.Collector interface
func (c Collector) Collect(ch chan<- prometheus.Metric) {
	resp, err := http.Get(c.prusaConnectHost + "/api/telemetry")
	if err != nil {
		log.Printf("failed to get telemetry: %v", err)
		return
	}
	var metrics Metrics
	err = json.NewDecoder(resp.Body).Decode(&metrics)
	if err != nil {
		log.Printf("failed to decode response: %v", err)
		return
	}

	ch <- prometheus.MustNewConstMetric(c.nozzleTemperatureDescroptor, prometheus.GaugeValue, float64(metrics.NozzleTemperature))
	ch <- prometheus.MustNewConstMetric(c.bedTemperatureDescroptor, prometheus.GaugeValue, float64(metrics.BedTemperature))
	ch <- prometheus.MustNewConstMetric(c.zPositionDescroptor, prometheus.GaugeValue, float64(metrics.ZPosition))
	ch <- prometheus.MustNewConstMetric(c.printingSpeedDescroptor, prometheus.GaugeValue, float64(metrics.PrintingSpeed))
	ch <- prometheus.MustNewConstMetric(c.flowFactorDescroptor, prometheus.GaugeValue, float64(metrics.FlowFactor))
	ch <- prometheus.MustNewConstMetric(c.progressDescroptor, prometheus.GaugeValue, float64(metrics.Progress))
	duration, err := parseDuration(metrics.PrintDuration)
	if err == nil {
		ch <- prometheus.MustNewConstMetric(c.printDurationDescroptor, prometheus.GaugeValue, float64(duration/time.Second))
	}
	estimated, err := strconv.ParseFloat(metrics.TimeEstimated, 64)
	if err == nil {
		ch <- prometheus.MustNewConstMetric(c.timeEstimatedDescroptor, prometheus.GaugeValue, float64(estimated))
	}
}

func main() {
	log.Println("Starting prusa-connect-exporter")

	prusaConnectHost, ok := os.LookupEnv("PRUSA_CONNECT_HOST")
	if !ok {
		log.Fatal("MIssing enviromental variable: PRUSA_CONNECT_ADDRESS")
	}
	prusaConnectExporterPort, ok := os.LookupEnv("PRUSA_CONNECT_EXPORTER_PORT")
	if !ok {
		prusaConnectExporterPort = "8080"
	}
	prusaConnectExporterPath, ok := os.LookupEnv("PRUSA_CONNECT_EXPORTER_PATH")
	if !ok {
		prusaConnectExporterPath = "/metrics"
	}

	prometheus.MustRegister(NewCollector(prusaConnectHost))

	http.Handle(prusaConnectExporterPath, promhttp.Handler())

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
			<head><title>Prusa Connect Exporter</title></head>
			<body>
			<h1>Prusa Connect Exporter</h1>
			<p><a href="` + prusaConnectExporterPath + `">Metrics</a></p>
			</body>
			</html>`))
	})

	err := http.ListenAndServe(":"+prusaConnectExporterPort, nil)

	log.Fatalf("Unable to start the server: %v", err)
}

var unitMap = map[string]uint64{
	"s": uint64(time.Second),
	"m": uint64(time.Minute),
	"h": uint64(time.Hour),
	"d": uint64(24 * time.Hour),
}

func parseDuration(s string) (time.Duration, error) {
	orig := s
	var d uint64
	for s != "" {
		// Consume whitespace
		i := 0
		for ; i < len(s); i++ {
			c := s[i]
			if c != ' ' {
				break
			}
		}
		s = s[i:]

		// Consume value
		var v uint64
		i = 0
		for ; i < len(s); i++ {
			c := s[i]
			if c < '0' || c > '9' {
				break
			}
			if v > 1<<63/10 {
				// overflow
				return 0, fmt.Errorf("invalid duration \"%v\"", orig)
			}
			v = v*10 + uint64(c) - '0'
			if v > 1<<63 {
				// overflow
				return 0, fmt.Errorf("invalid duration \"%v\"", orig)
			}
		}
		if i == 0 {
			return 0, fmt.Errorf("missing value in duration \"%v\"", orig)
		}
		s = s[i:]

		// Consume unit
		i = 0
		for ; i < len(s); i++ {
			c := s[i]
			if c == ' ' || '0' <= c && c <= '9' {
				break
			}
		}
		if i == 0 {
			return 0, fmt.Errorf("missing unit in duration \"%v\"", orig)
		}
		u := s[:i]
		s = s[i:]
		unit, ok := unitMap[u]
		if !ok {
			return 0, fmt.Errorf("unknown unit \"%v\" in duration \"%v\"", u, orig)
		}

		if v > 1<<63/unit {
			// overflow
			return 0, fmt.Errorf("invalid duration \"%v\"", orig)
		}
		v *= unit
		d += v
		if d > 1<<63 {
			return 0, fmt.Errorf("invalid duration \"%v\"", orig)
		}
	}
	return time.Duration(d), nil
}
