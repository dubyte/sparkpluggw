package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/IHI-Energy-Storage/sparkpluggw/remotewrite"
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

	progname = "sparkpluggw"
	exporter *spplugExporter
	logger   log.Logger
)

func main() {
	var promlogConfig promlog.Config
	flag.AddFlags(kingpin.CommandLine, &promlogConfig)
	kingpin.Parse()
	logger = promlog.NewDynamic(&promlogConfig)
	initSparkPlugExporter(&exporter)
	prometheus.MustRegister(exporter)

	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/test", test)
	level.Info(logger).Log("msg", fmt.Sprintf("Listening on %s", *listenAddress))
	write := remotewrite.Writer(logger)
	go func() {
		for {
			write()
			<-time.Tick(5 * time.Second)
		}

	}()
	err := http.ListenAndServe(*listenAddress, nil)
	if err != nil {
		level.Error(logger).Log("msg", err)
		os.Exit(1)
	}
}

// We could get the tests repo and add our exporter to check the compliance.
// https://github.com/prometheus/compliance/tree/main/remote_write
func test(w http.ResponseWriter, _ *http.Request) {
	write := remotewrite.Writer(logger)

	write()

	//timestamp := time.Now().Unix()
	//
	//wReq, err := remotewrite.BuildWriteRequest(prometheus.DefaultGatherer, timestamp)
	//if err != nil {
	//	level.Error(logger).Log("msg", fmt.Sprintf("error while building the write request: %s", err))
	//}
	//
	//data, err := proto.Marshal(&wReq)
	//if err != nil {
	//	level.Error(logger).Log("msg", fmt.Sprintf("error while marshalling write request: %s", err))
	//	return
	//}
	//
	//compressed := snappy.Encode(nil, data)
	//
	////////////////////// test only /////////////////////
	//reqBuf, err := snappy.Decode(nil, compressed)
	//if err != nil {
	//	level.Error(logger).Log("msg", fmt.Sprintf("error while decoding snappy compressed request: %s", err))
	//	return
	//}
	//
	//var req prompb.WriteRequest
	//
	//if err := proto.Unmarshal(reqBuf, &req); err != nil {
	//	level.Error(logger).Log("msg", fmt.Sprintf("error while unmarshalling remote request: %s", err))
	//}
	//level.Info(logger).Log("msg", fmt.Sprintf("request: %s", req))
	/////////////////////////////////////////////////////
	//
	//// To send metrics
	////https://github.com/prometheus/prometheus/blob/main/storage/remote/client.go
	////c := remote.NewClient()
	////
	////fmt.Println(c)
}
