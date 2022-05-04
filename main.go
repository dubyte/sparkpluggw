package main

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/IHI-Energy-Storage/sparkpluggw/log"
	"github.com/IHI-Energy-Storage/sparkpluggw/remotewrite"
	"github.com/dubyte/promtail-client/promtail"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

	lokiEnabled = kingpin.Flag("loki.enabled",
		"if passed some messages could be sent to loki instead of store them as metrics using a decision tree").
		Bool()

	remoteWriteEndpoint = kingpin.Flag("remote-write.endpoint",
		"the endpoint where prometheus is listen for remote write requests").
		Default("http://localhost:9090/api/v1/write").URL()

	remoteWriteUserAgent = kingpin.Flag("remote-write.user-agent",
		"the user agent that will be be used to identify the exporter in prometheus").
		Default("sparkpluggw exporter").String()

	// to pass extra labels --remote-write.extra-label=app=test --remote-write.extra-label=env=development
	remoteWriteExtraLabels = kingpin.Flag("remote-write.extra-label",
		"extra label to add ( example --remote-write.extra-label=labelName=labelValue)").StringMap()

	remoteWriteLabelSubstitutions = kingpin.Flag("remote-write.replace-label",
		"Allows to rename the default labels for remote write (for example --remote-write.replace-label=sp_namespace=ns will replace sp_namespace for ns)").StringMap()

	remoteWriteDropLabels = kingpin.Flag("remote-write.drop-label",
		"remove to not send a label for remote write (repeat to remove more than one field)").Strings()

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

	mqttUsername = kingpin.Flag("mqtt.username",
		"User to connect MQTT server").Default("").String()

	mqttPassword = kingpin.Flag("mqtt.password",
		"Password to connect MQTT server").Default("").String()

	mqttCACrtFile = kingpin.Flag("mqtt.ca.crt.file",
		"the path of the extra CA certificate to be used to connect to mqtt").Default("").String()

	mqttCrtFile = kingpin.Flag("mqtt.crt.file",
		"the path of the tls certificate used to connect to mqtt").Default("").String()

	mqttKeyFile = kingpin.Flag("mqtt.key.file",
		"the path of the tls key used to connect to mqtt").Default("").String()

	mqttInsecureSkipVerify = kingpin.Flag("mqtt.insecure-skip-verify",
		"if passed mqtt client will verify the certificate with the ca").
		Bool()

	mqttDebug = kingpin.Flag("mqtt.debug", "Enable MQTT debugging").
			Default("false").String()

	mqttConnectWithRetry = kingpin.Flag("mqtt.conn.retry",
		"When enabled it will try to connect every 10 seconds if the first time was not possible").
		Default("false").String()

	jobName = kingpin.Flag("job", "the job value used for prometheus remote write and loki").
		Default("sparkpluggw").String()

	pushURL = kingpin.Flag("loki.push-URL",
		"the url of the loki push endpoint to send the logs").
		Default("http://localhost:3100/loki/api/v1/push").String()

	// it allows passing labels like: --loki.extra-label env=production --loki.extra-label datacenter=us-west
	lokiExtraLabels = kingpin.Flag("loki.extra-label",
		"Label to send to loki (example --loki.extra-label=datacenter=us-west)").StringMap()

	lokiBatchWait = kingpin.Flag("loki.batch-wait",
		"Maximum amount of time to wait before sending a batch, even if that batch isn't full.").
		Default("5s").Duration()

	lokiFieldSubstitutions = kingpin.Flag("loki.replace-field",
		"Allows to rename the default fields for loki for example --loki.replace-field=sp_namespace=ns will replace sp_namespace for ns").StringMap()

	decisionTreePath = kingpin.Flag("decision-tree.file", "path to file with a decision tree defined in json").
				String()

	lokiDropFields = kingpin.Flag("loki.drop-field", "field will not be send to loki. (example --loki.drop-field=event_name --loki.drop-field=event_type)").
			Strings()

	edgNodePrefix = kingpin.Flag("edge-node.prefix", "if given it removes the prefix from the edge_node_id").
			Default("").String()

	progname = "sparkpluggw"
	exporter *spplugExporter
)

// lokiFields used to know valid fields that can be replaced for loki
var lokiFields = map[string]struct{}{"topic": {}, "event_name": {}, "event_value": {}, "event_type": {}, "edge_node": {}}

// dropFields used to know if we drop the field or not
var dropFields = make(map[string]struct{})

func main() {
	kingpin.CommandLine.HelpFlag.Short('h')
	log.AddFlags(kingpin.CommandLine)
	kingpin.Parse()

	var lokiClient promtail.Client
	if *lokiEnabled {
		for k := range *lokiFieldSubstitutions {
			if _, ok := lokiFields[k]; !ok {
				log.Warnf("'%s' is not a valid loki field to replace", k)
				delete(*lokiFieldSubstitutions, k)
			}
		}

		for _, v := range *lokiDropFields {
			dropFields[v] = struct{}{}
		}

		conf := promtail.ClientConfig{
			PushURL:            *pushURL,
			Labels:             buildLokiLabels(*lokiExtraLabels),
			BatchWait:          *lokiBatchWait,
			BatchEntriesNumber: 10000,
			SendLevel:          promtail.INFO,
			PrintLevel:         promtail.ERROR,
			PrefixFormat:       "level=%s ",
		}
		var err error
		lokiClient, err = promtail.NewClientProto(conf)
		if err != nil {
			log.Errorf("lokiClient init err: %s", err)
			os.Exit(1)
		}
		defer lokiClient.Shutdown()
	}

	initSparkPlugExporter(&exporter, lokiClient)
	prometheus.MustRegister(exporter)

	var wg sync.WaitGroup

	webInterfaceEnabled := !*disableWebInterface
	if webInterfaceEnabled && *remoteWriteEnabled {
		log.Error("remoteWrite can not be enabled when disableWebInterface is false.")
		os.Exit(1)
	}

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
		dropLabels := make(map[string]struct{})
		for _, l := range *remoteWriteDropLabels {
			dropLabels[l] = struct{}{}
		}
		w := remotewrite.Writer{Client: c,
			ExtraLabels: buildRemoteWriteLabels(*remoteWriteExtraLabels), LabelSubstitutions: *remoteWriteLabelSubstitutions,
			DropLabels: dropLabels,
		}

		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			for {
				w.Write()
				mutex.Lock()
				newRegistry := prometheus.NewRegistry()
				prometheus.DefaultGatherer = newRegistry
				prometheus.DefaultRegisterer = newRegistry
				prometheus.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
				prometheus.MustRegister(prometheus.NewGoCollector())
				exporter.initializeMetricsAndData()
				mutex.Unlock()
				prometheus.MustRegister(exporter)
				<-time.Tick(*remoteWriteEvery)
			}
		}(&wg)
	}

	log.Infof("Connecting to %v", *brokerAddress)

	if *mqttConnectWithRetry == "true" {
		connectWithRetry(exporter.client, connectMQTT)
	} else if err := connectMQTT(exporter.client); err != nil {
		log.Error(err)
		os.Exit(1)
	}

	wg.Wait()
}
