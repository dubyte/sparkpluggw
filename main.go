package main

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/IHI-Energy-Storage/sparkpluggw/remotewrite"
	"github.com/afiskon/promtail-client/promtail"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/prometheus/prompb"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	listenAddress = kingpin.Flag("web.listen-address",
		"Address on which to expose metrics and web interface").
		Default(":9337").
		String()

	metricsPath = kingpin.Flag("web.telemetry-path",
		"Path under which to expose metrics").
		Default("/metrics").
		String()

	disableWebInterface = kingpin.Flag("web.disable-telemetry",
		"if this flag is passed the app won't server the metrics endpoint").
		Bool()

	remoteWriteEnabled = kingpin.Flag("remote-write.enabled",
		"if passed exporter will send metrics to a remote prometheus").
		Bool()

	prometheusAddr = kingpin.Flag("remote-write.addr",
		"the address of the prometheus server where the exporter will send the metrics").
		Default("http://localhost:9090").String()

	remoteWritePath = kingpin.Flag("remote-write.path",
		"the endpoint where prometheus is listen for remote write requests").
		Default("/api/v1/write").String()

	// TODO: be more specific and include the version
	remoteWriteUserAgent = kingpin.Flag("remote-write.user-agent",
		"the user agent that will be be used to identify the exporter in prometheus").
		Default("sparkpluggw exporter").String()

	// TODO: pass extra labels to be used for remotewrite.

	remoteWriteTimeout = kingpin.Flag("remote-write.timeout",
		"Sets the timeout when sending request to prometheus remote write").
		Default("30s").Duration()

	remoteWriteEvery = kingpin.Flag("remote-write.send-every",
		"Sets how ofter the exporter will sent metrics to prometheus").
		Default("30s").Duration()

	brokerAddress = kingpin.Flag("mqtt.broker-address",
		"Address of the MQTT broker").
		Default("tcp://localhost:1883").String()

	topic = kingpin.Flag("mqtt.topic",
		"MQTT topic to subscribe to").
		Default("prometheus/#").String()

	prefix = kingpin.Flag("mqtt.prefix",
		"MQTT topic prefix to remove when creating metrics").
		Default("prometheus").String()

	clientID = kingpin.Flag("mqtt.client-id",
		"MQTT client identifier (limit to 23 characters)").
		Default("").String()

	mqttDebug = kingpin.Flag("mqtt.debug", "Enable MQTT debugging").
			Default("false").String()

	pushURL = kingpin.Flag("loki.push-URL",
		"the url of the loki push endpoint to send the logs").
		Default("http://localhost:3100/loki/api/v1/push").String()

	lokiLabels = kingpin.Flag("loki.labels",
		"Labels to send to loki").
		Default(`{source="mqtt.sparkplugb",job="sparkpluggw"}`).String()

	lokiBatchWait = kingpin.Flag("loki.batch-wait",
		"Maximum amount of time to wait before sending a batch, even if that batch isn't full.").
		Default("5s").Duration()

	progname = "sparkpluggw"
	exporter *spplugExporter
	logger   log.Logger
)

const (
	sourceName = "Simulation"
)

func main() {
	var promlogConfig promlog.Config
	flag.AddFlags(kingpin.CommandLine, &promlogConfig)
	kingpin.Parse()
	logger = promlog.NewDynamic(&promlogConfig)

	conf := promtail.ClientConfig{
		PushURL:            *pushURL,
		Labels:             *lokiLabels,
		BatchWait:          *lokiBatchWait,
		BatchEntriesNumber: 10000,
		SendLevel:          promtail.INFO,
		PrintLevel:         promtail.ERROR,
	}

	lokiClient, err := promtail.NewClientProto(conf)
	if err != nil {
		level.Error(logger).Log("msg", err)
		os.Exit(1)
	}
	defer lokiClient.Shutdown()
	initSparkPlugExporter(&exporter, lokiClient)
	prometheus.MustRegister(exporter)
	webInterfaceEnabled := !*disableWebInterface

	var wg sync.WaitGroup

	if *remoteWriteEnabled {
		rawURL := fmt.Sprintf("%s%s", *prometheusAddr, *remoteWritePath)
		c, err := remotewrite.Client(rawURL, *remoteWriteTimeout, *remoteWriteUserAgent, true)
		if err != nil {
			level.Error(logger).Log("msg", err)
			os.Exit(1)
		}
		w := remotewrite.Writer{Client: c, Logger: logger, Gatherer: prometheus.DefaultGatherer,
			ExtraLabels: []prompb.Label{
				//{Name: "instance", Value: "localhost:9337"},
				{Name: "app", Value: "sparkpluggw"},
			},
		}

		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			for {
				w.Write()
				<-time.Tick(*remoteWriteEvery)
			}
		}(&wg)
	}

	if webInterfaceEnabled {
		http.Handle(*metricsPath, promhttp.Handler())
		level.Info(logger).Log("msg", fmt.Sprintf("Listening on %s", *listenAddress))
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			err := http.ListenAndServe(*listenAddress, nil)
			if err != nil {
				level.Error(logger).Log("msg", err)
				os.Exit(1)
			}
		}(&wg)
	}

	wg.Wait()
}
