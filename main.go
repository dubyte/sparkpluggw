package main

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/IHI-Energy-Storage/sparkpluggw/remotewrite"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/log"
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

	remoteWriteEndpoint = kingpin.Flag("remote-write.endpoint",
		"the endpoint where prometheus is listen for remote write requests").
		Default("http://localhost:9090/api/v1/write").URL()

	remoteWriteUserAgent = kingpin.Flag("remote-write.user-agent",
		"the user agent that will be be used to identify the exporter in prometheus").
		Default("sparkpluggw exporter").String()

	// to pass extra labels --remote-write.extra-label=app=test --remote-write.extra-label=env=development
	remoteWriteExtraLabels = kingpin.Flag("remote-write.extra-label", "extra label to add").
				StringMap()

	remoteWriteLabelSubstitutions = kingpin.Flag("remote-write.replace-label",
		"Allows to rename the default labels for remote write").StringMap()

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

	jobName = kingpin.Flag("job", "the job value used for prometheus remote write").
		Default("sparkpluggw").String()

	progname = "sparkpluggw"
	exporter *spplugExporter
)

func main() {
	kingpin.CommandLine.HelpFlag.Short('h')
	log.AddFlags(kingpin.CommandLine)
	kingpin.Parse()

	initSparkPlugExporter(&exporter)
	prometheus.MustRegister(exporter)

	var wg sync.WaitGroup

	webInterfaceEnabled := !*disableWebInterface

	if webInterfaceEnabled {
		http.Handle(*metricsPath, promhttp.Handler())
		log.Infof("Listening on %s", *listenAddress)
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			err := http.ListenAndServe(*listenAddress, nil)
			if err != nil {
				log.Error(err)
				os.Exit(1)
			}
		}(&wg)
	}

	if *remoteWriteEnabled {
		rawURL := fmt.Sprintf("%s", *remoteWriteEndpoint)
		c, err := remotewrite.Client(rawURL, *remoteWriteTimeout, *remoteWriteUserAgent, true)
		if err != nil {
			log.Error(err)
			os.Exit(1)
		}

		w := remotewrite.Writer{Client: c, Gatherer: prometheus.DefaultGatherer,
			ExtraLabels: buildRemoteWriteLabels(*remoteWriteExtraLabels), LabelSubstitutions: *remoteWriteLabelSubstitutions,
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

	wg.Wait()
}
