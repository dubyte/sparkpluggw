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

	// TODO: be more specific and include the version
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

	//TODO: should we enable loki or should we enable events at all
	//lokiPushEnabled = kingpin.Flag("loki.enabled", "Enable push events to loki").
	//		Bool()

	jobName = kingpin.Flag("job", "the job value used for prometheus remote write and loki").
		Default("sparkpluggw").String()

	pushURL = kingpin.Flag("loki.push-URL",
		"the url of the loki push endpoint to send the logs").
		Default("http://localhost:3100/loki/api/v1/push").String()

	// it allows passing labels like: --loki.extra-label env=production --loki.extra-label datacenter=us-west
	lokiExtraLabels = kingpin.Flag("loki.extra-label", "Label to send to loki").
			StringMap()

	lokiBatchWait = kingpin.Flag("loki.batch-wait",
		"Maximum amount of time to wait before sending a batch, even if that batch isn't full.").
		Default("5s").Duration()

	decisionTree = kingpin.Flag("decision-tree.file", "path to file with a decision tree defined in json").
			Default("./decision-tree.json").String()

	progname = "sparkpluggw"
	exporter *spplugExporter
	logger   log.Logger
)

func main() {
	var promlogConfig promlog.Config
	flag.AddFlags(kingpin.CommandLine, &promlogConfig)
	kingpin.Parse()

	logger = promlog.New(&promlogConfig)

	conf := promtail.ClientConfig{
		PushURL:            *pushURL,
		Labels:             buildLokiLabels(*lokiExtraLabels),
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

	initSparkPlugExporter(&exporter, lokiClient, *decisionTree)
	prometheus.MustRegister(exporter)

	var wg sync.WaitGroup

	if *remoteWriteEnabled {
		rawURL := fmt.Sprintf("%s", *remoteWriteEndpoint)
		c, err := remotewrite.Client(rawURL, *remoteWriteTimeout, *remoteWriteUserAgent, true)
		if err != nil {
			level.Error(logger).Log("msg", err)
			os.Exit(1)
		}

		w := remotewrite.Writer{Client: c, Logger: logger, Gatherer: prometheus.DefaultGatherer,
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

	webInterfaceEnabled := !*disableWebInterface

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
