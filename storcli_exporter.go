package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	argStorcliPath   = flag.String("storcli_path", envOrDefault("STORCLI_PATH", "/opt/MegaRAID/storcli/storcli64"), "Path to MegaRAID StorCLI or PercCLI binary. By default '/opt/MegaRAID/storcli/storcli64'.")
	argMetricsPath   = flag.String("metrics_path", envOrDefault("METRICS_PATH", "/metrics"), "Path under which to expose Prometheus metrics. By default '/metrics'.")
	argMetricsPrefix = flag.String("metrics_prefix", envOrDefault("METRICS_PREFIX", "storcli"), "Prefix for Prometheus metrics. By default 'storcli'.")
	argListenAddress = flag.String("listen_address", envOrDefault("LISTEN_ADDRESS", ":9326"), "Listen address for this exporter. By default ':9326'.")
)

func envOrDefault(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok && value != "" {
		return value
	}

	return fallback
}

// Response is the response we fetch from StorCLI/PercCLI command output
type Response struct {
	Controllers []struct {
		ResponseData struct {
			VirtualDrives int `json:"Virtual Drives"`
			VDLIST        []struct {
				Position string `json:"DG/VD"`
				Type     string `json:"TYPE"`
				State    string `json:"State"`
				Size     string `json:"Size"`
			} `json:"VD LIST"`
			PhysicalDrives int `json:"Physical Drives"`
			PDLIST         []struct {
				Device   int    `json:"DID"`
				Position string `json:"EID:Slt"`
				State    string `json:"State"`
				Media    string `json:"Med"`
				Model    string `json:"Model"`
				Size     string `json:"Size"`
				Type     string `json:"Type"`
			} `json:"PD LIST"`
			DriveGroups    int `json:"Drive Groups"`
			HardwareConfig struct {
				Temperature   int `json:"ROC temperature(Degree Celsius)"`
				CacheCade     int `json:"Current Size of CacheCade (GB)"`
				FirmwareCache int `json:"Current Size of FW Cache (MB)"`
			} `json:"HwCfg"`
			TOPOLOGYLIST []struct {
				DiskGroup int         `json:"DG"`
				Array     interface{} `json:"Arr"`
				Row       interface{} `json:"Row"`
				Position  string      `json:"EID:Slot"`
				Device    interface{} `json:"DID"`
				State     string      `json:"State"`
				Size      string      `json:"Size"`
				Type      string      `json:"Type"`
			} `json:"TOPOLOGY"`
		} `json:"Response Data"`
	} `json:"Controllers"`
}

type DiskDetailedInfo struct {
	shieldCounter          float64
	mediaErrorCount        float64
	otherErrorCount        float64
	driveTemperature       float64
	predictiveFailureCount float64
}

// Exporter is struct defining StorCLI/PercCLI exporter
type Exporter struct {
	physicalDriveStatus    *prometheus.Desc
	virtualDriveStatus     *prometheus.Desc
	physicalDriveCount     *prometheus.Desc
	virtualDriveCount      *prometheus.Desc
	driveGroupsCount       *prometheus.Desc
	topologyStatus         *prometheus.Desc
	scrapeSuccess          *prometheus.Desc
	temperature            *prometheus.Desc
	cacheCade              *prometheus.Desc
	firmwareCache          *prometheus.Desc
	driveTemperature       *prometheus.Desc
	shieldCounter          *prometheus.Desc
	mediaErrorCount        *prometheus.Desc
	otherErrorCount        *prometheus.Desc
	predictiveFailureCount *prometheus.Desc
}

func fetchStorcliOutput() (resp Response, err error) {
	output, err := exec.Command(*argStorcliPath, "/call", "show", "all", "J").Output()
	if err != nil {
		return Response{}, fmt.Errorf("Failed to execute command: %s", err)
	}
	if err != nil {
		fmt.Print(err)
	}
	var response Response
	err = json.Unmarshal(output, &response)
	if err != nil {
		return Response{}, fmt.Errorf("Failed to unmarshal JSON: %s", err)
	}
	return response, nil
}

func fetchStorcliOutputDisks(controllerNumber string, enclosureNumber string, slotNumber string) DiskDetailedInfo {
	output, err := exec.Command(*argStorcliPath, "/call/eall/sall", "show", "all", "J").Output()
	if err != nil {
		return DiskDetailedInfo{}
	}
	if err != nil {
		fmt.Print(err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(output, &data); err != nil {
		fmt.Println("Error:", err)
		return DiskDetailedInfo{}
	}
	ctrlNum, err := strconv.Atoi(controllerNumber)
	responseData := data["Controllers"].([]interface{})[ctrlNum].(map[string]interface{})["Response Data"].(map[string]interface{})

	driveTemperature := math.NaN()

	detailedInfo := responseData["Drive /c"+controllerNumber+"/e"+enclosureNumber+"/s"+slotNumber+" - Detailed Information"].(map[string]interface{})
	state := detailedInfo["Drive /c"+controllerNumber+"/e"+enclosureNumber+"/s"+slotNumber+" State"].(map[string]interface{})

	driveTemperatureStr := state["Drive Temperature"].(string)

	driveTemperatureParts := strings.Split(driveTemperatureStr, "C")
	if len(driveTemperatureParts) != 2 {
		log.Printf("Failed to parse temperature: %s", err)
	} else {
		driveTemperatureValue := strings.TrimSpace(driveTemperatureParts[0])
		driveTemperatureInt, err := strconv.Atoi(driveTemperatureValue)
		driveTemperature = float64(driveTemperatureInt)
		if err != nil {
			fmt.Print(err)
			driveTemperature = math.NaN()
		}
	}

	shieldCounter := state["Shield Counter"].(float64)
	mediaErrorCount := state["Media Error Count"].(float64)
	otherErrorCount := state["Other Error Count"].(float64)
	predictiveFailureCount := state["Predictive Failure Count"].(float64)

	return DiskDetailedInfo{
		shieldCounter:          shieldCounter,
		mediaErrorCount:        mediaErrorCount,
		otherErrorCount:        otherErrorCount,
		driveTemperature:       driveTemperature,
		predictiveFailureCount: predictiveFailureCount,
	}
}

// NewExporter creates a new object of type Exporter
func NewExporter() *Exporter {
	return &Exporter{
		scrapeSuccess:          ScrapeSuccess,
		virtualDriveCount:      VirtualDrivesCount,
		physicalDriveCount:     PhysicalDrivesCount,
		virtualDriveStatus:     VirtualDriveStatus,
		physicalDriveStatus:    PhysicalDriveStatus,
		driveGroupsCount:       DriveGroupsCount,
		topologyStatus:         TopologyStatus,
		temperature:            Temperature,
		cacheCade:              CacheCade,
		firmwareCache:          FirmwareCache,
		driveTemperature:       DriveTemperature,
		shieldCounter:          ShieldCounter,
		mediaErrorCount:        MediaErrorCount,
		otherErrorCount:        OtherErrorCount,
		predictiveFailureCount: PredictiveFailureCount,
	}
}

// Describe describes the Prometheus metrics
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.physicalDriveStatus
	ch <- e.virtualDriveStatus
	ch <- e.physicalDriveCount
	ch <- e.virtualDriveCount
	ch <- e.driveGroupsCount
	ch <- e.topologyStatus
	ch <- e.scrapeSuccess
	ch <- e.temperature
	ch <- e.cacheCade
	ch <- e.firmwareCache
	ch <- e.driveTemperature
	ch <- e.shieldCounter
	ch <- e.mediaErrorCount
	ch <- e.otherErrorCount
	ch <- e.predictiveFailureCount
}

// Collect collects the Prometheus metrics
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	response, err := fetchStorcliOutput()
	if err != nil {
		ch <- prometheus.MustNewConstMetric(e.scrapeSuccess, prometheus.GaugeValue, 0)
		log.Printf("Failed to fetch StorCLI output: %s", err)
	}
	for controllerNumber, controller := range response.Controllers {
		ch <- prometheus.MustNewConstMetric(e.virtualDriveCount, prometheus.GaugeValue, float64(controller.ResponseData.VirtualDrives), strconv.Itoa(controllerNumber))
		ch <- prometheus.MustNewConstMetric(e.physicalDriveCount, prometheus.GaugeValue, float64(controller.ResponseData.PhysicalDrives), strconv.Itoa(controllerNumber))
		ch <- prometheus.MustNewConstMetric(e.driveGroupsCount, prometheus.GaugeValue, float64(controller.ResponseData.DriveGroups), strconv.Itoa(controllerNumber))
		ch <- prometheus.MustNewConstMetric(e.temperature, prometheus.GaugeValue, float64(controller.ResponseData.HardwareConfig.Temperature), strconv.Itoa(controllerNumber))
		ch <- prometheus.MustNewConstMetric(e.cacheCade, prometheus.GaugeValue, float64(controller.ResponseData.HardwareConfig.CacheCade), strconv.Itoa(controllerNumber))
		ch <- prometheus.MustNewConstMetric(e.firmwareCache, prometheus.GaugeValue, float64(controller.ResponseData.HardwareConfig.FirmwareCache), strconv.Itoa(controllerNumber))
		for _, virtualDrive := range controller.ResponseData.VDLIST {
			ch <- prometheus.MustNewConstMetric(
				e.virtualDriveStatus, prometheus.GaugeValue, 1.0,
				strconv.Itoa(controllerNumber), virtualDrive.Position, virtualDrive.Type, virtualDrive.Size, virtualDrive.State,
			)
		}
		for _, physicalDrive := range controller.ResponseData.PDLIST {
			ch <- prometheus.MustNewConstMetric(
				e.physicalDriveStatus, prometheus.GaugeValue, 1.0,
				strconv.Itoa(controllerNumber), physicalDrive.Position, strconv.Itoa(physicalDrive.Device), physicalDrive.Model,
				physicalDrive.State, physicalDrive.Media, physicalDrive.Size,
			)

			parts := strings.Split(physicalDrive.Position, ":")
			if len(parts) != 2 {
				log.Printf("Failed to process StorCLI output: %s", err)
				continue
			}
			enclosureNumber := parts[0]
			slotNumber := parts[1]

			response := fetchStorcliOutputDisks(strconv.Itoa(controllerNumber), enclosureNumber, slotNumber)
			ch <- prometheus.MustNewConstMetric(
				e.driveTemperature, prometheus.GaugeValue, response.driveTemperature,
				strconv.Itoa(controllerNumber), enclosureNumber, slotNumber,
			)
			ch <- prometheus.MustNewConstMetric(
				e.shieldCounter, prometheus.GaugeValue, response.shieldCounter,
				strconv.Itoa(controllerNumber), enclosureNumber, slotNumber,
			)
			ch <- prometheus.MustNewConstMetric(
				e.mediaErrorCount, prometheus.GaugeValue, response.mediaErrorCount,
				strconv.Itoa(controllerNumber), enclosureNumber, slotNumber,
			)
			ch <- prometheus.MustNewConstMetric(
				e.otherErrorCount, prometheus.GaugeValue, response.otherErrorCount,
				strconv.Itoa(controllerNumber), enclosureNumber, slotNumber,
			)
			ch <- prometheus.MustNewConstMetric(
				e.predictiveFailureCount, prometheus.GaugeValue,
				response.predictiveFailureCount,
				strconv.Itoa(controllerNumber), enclosureNumber, slotNumber,
			)

		}
		for _, topology := range controller.ResponseData.TOPOLOGYLIST {
			ch <- prometheus.MustNewConstMetric(
				e.topologyStatus, prometheus.GaugeValue, 1.0,
				strconv.Itoa(controllerNumber), topology.Position, strconv.Itoa(topology.DiskGroup), fmt.Sprint(topology.Array),
				fmt.Sprint(topology.Row), fmt.Sprint(topology.Device), topology.State, topology.Type, topology.Size,
			)
		}
	}
}

func main() {
	flag.Parse()

	registry := prometheus.NewRegistry()
	registry.MustRegister(NewExporter())

	http.Handle(*argMetricsPath, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, err := w.Write([]byte(`<html>
    <head><title>StorCLI Exporter</title></head>
    <body>
    <h1>StorCLI Exporter</h1>
    <p><a href='` + *argMetricsPath + `'>Metrics</a></p>
    </html>`))
		if err != nil {
			log.Printf("Failed to write to HTTP client: %s", err)
		}
	})
	http.HandleFunc("/livez", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(`ok`))
		if err != nil {
			log.Printf("Failed to write to HTTP client: %s", err)
		}
	})
	http.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		_, err := registry.Gather()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`error occured while trying to get storcli data`))
			log.Printf("Failed to get storcli data: %s", err)
			return
		}
		_, err = w.Write([]byte(`ok`))
		if err != nil {
			log.Printf("Failed to write to HTTP client: %s", err)
		}
	})

	server := &http.Server{
		Addr:         *argListenAddress,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	log.Printf("StorCLI exporter started and listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
