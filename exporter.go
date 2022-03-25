package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	oslog "log"
	"os"
	"strings"
	"sync"
	"time"

	pb "github.com/IHI-Energy-Storage/sparkpluggw/Sparkplug"
	"github.com/IHI-Energy-Storage/sparkpluggw/log"
	"github.com/IHI-Energy-Storage/sparkpluggw/message"
	"github.com/dubyte/promtail-client/promtail"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/golang/protobuf/proto" //nolint
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tkanos/go-dtree"
)

var mutex sync.RWMutex
var edgeNodeList map[string]bool

// constants for various SP labels and metric names
//nolint
const (
	SPPushTotalMetric      string = "sp_total_metrics_pushed"
	SPLastTimePushedMetric string = "sp_last_pushed_timestamp"
	SPConnectionCount      string = "sp_connection_established_count"
	SPDisconnectionCount   string = "sp_connection_lost_count"
	SPPushInvalidMetric    string = "sp_invalid_metric_name_received"

	SPReincarnationAttempts string = "sp_reincarnation_attempt_count"
	SPReincarnationFailures string = "sp_reincarnation_failure_count"
	SPReincarnationSuccess  string = "sp_reincarnation_success_count"
	SPReincarnationDelay    string = "sp_reincarnation_delayed_count"

	NewMetricString string = "Creating new SP metric %s"

	SPReincarnateTimer  uint32 = 900
	SPReincarnateRetry  uint32 = 60
	SPReconnectionTimer uint32 = 300
	PBInt8              uint32 = 1
	PBInt16             uint32 = 2
	PBInt32             uint32 = 3
	PBInt64             uint32 = 4
	PBUInt8             uint32 = 5
	PBUInt16            uint32 = 6
	PBUInt32            uint32 = 7
	PBUInt64            uint32 = 8
	PBFloat             uint32 = 9
	PBDouble            uint32 = 10
	PBBoolean           uint32 = 11
	PBString            uint32 = 12
	PBDateTime          uint32 = 13
	PBText              uint32 = 14
	PBUUID              uint32 = 15
	PBDataSet           uint32 = 16
	PBBytes             uint32 = 17
	PBFile              uint32 = 18
	PBTemplate          uint32 = 19
	PBPropertySet       uint32 = 20
	PBPropertySetList   uint32 = 21
)

type prometheusmetric struct {
	prommetric *prometheus.GaugeVec
	promlabel  []string
}

type spplugExporter struct {
	client      mqtt.Client
	versionDesc *prometheus.Desc
	connectDesc *prometheus.Desc

	// Holds the metrics collected
	metrics        map[string][]prometheusmetric
	counterMetrics map[string]*prometheus.CounterVec

	// holds a decision tree to know if a message should be a metric or an event (send on exception)
	decisionTree *dtree.Tree

	// client to send msg to event
	lokiClient promtail.Client
}

// Initialize
func initSparkPlugExporter(e **spplugExporter, lokiClient promtail.Client) {

	if *mqttDebug == "true" {
		mqtt.ERROR = oslog.New(os.Stdout, "MQTT ERROR    ", oslog.Ltime)
		mqtt.CRITICAL = oslog.New(os.Stdout, "MQTT CRITICAL ", oslog.Ltime)
		mqtt.WARN = oslog.New(os.Stdout, "MQTT WARNING  ", oslog.Ltime)
		mqtt.DEBUG = oslog.New(os.Stdout, "MQTT DEBUG    ", oslog.Ltime)
	}

	// create a MQTT client
	options := mqtt.NewClientOptions()

	// Set broker and client options
	options.AddBroker(*brokerAddress)
	options.SetClientID(*clientID)

	if *mqttUsername != "" {
		options.SetUsername(*mqttUsername)
	}

	if *mqttPassword != "" {
		options.SetPassword(*mqttPassword)
	}

	rootCAs, _ := x509.SystemCertPool()
	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}

	if *mqttCACrtFile != "" {
		certs, _ := ioutil.ReadFile(*mqttCACrtFile)
		if ok := rootCAs.AppendCertsFromPEM(certs); !ok {
			log.Info("No certs appended, using system certs only")
		}
	}

	var certs []tls.Certificate
	if *mqttCrtFile != "" && *mqttKeyFile != "" {
		cer, err := tls.LoadX509KeyPair(*mqttCrtFile, *mqttKeyFile)
		if err != nil {
			log.Errorf("fail to load tls cert: %s", err)
		}
		certs = append(certs, cer)
	}

	config := &tls.Config{
		RootCAs:            rootCAs,
		ClientCAs:          rootCAs,
		Certificates:       certs,
		InsecureSkipVerify: *mqttInsecureSkipVerify,
	}
	options.SetTLSConfig(config)

	// Set client timeouts and intervals
	options.SetWriteTimeout(5 * time.Second)
	options.SetPingTimeout(1 * time.Second)
	options.SetMaxReconnectInterval(time.Duration(SPReconnectionTimer))

	// Set handler functions
	options.SetOnConnectHandler(connectHandler)
	options.SetConnectionLostHandler(disconnectHandler)

	// Set capabilities
	options.SetAutoReconnect(true)

	// create an exporter
	*e = &spplugExporter{
		versionDesc: prometheus.NewDesc(
			prometheus.BuildFQName(progname, "build", "info"),
			"Build info of this instance", nil,
			prometheus.Labels{"version": version}),
		connectDesc: prometheus.NewDesc(
			prometheus.BuildFQName(progname, "mqtt", "connected"),
			"Is the exporter connected to mqtt broker", nil, nil),
		lokiClient: lokiClient,
	}

	(*e).client = mqtt.NewClient(options)

	if lokiClient != nil {
		log.Debug("Initializing Exporter decision tree")

		dt, err := loadDecisionTree(*decisionTreePath)
		if err != nil {
			log.Infof("decision tree load failed: %s", err)
			log.Info("all messages are going to be handled by metrics handler.")
		} else {
			(*e).decisionTree = dt
		}
	}
	log.Debug("Initializing Exporter Metrics and Data")
	(*e).initializeMetricsAndData()

	// Moved to main after the metrics exists.
	//if err := connectMQTT((*e).client); err != nil {
	//	log.Error("%s", err)
	//	os.Exit(1)
	//}

	// TODO: is it needed?
	//(*e).client.Subscribe(*topic, 2, (*e).receiveMessage)
}

// connectWithRetry try to connect to mqtt and
func connectWithRetry(c mqtt.Client, connFunc func(c mqtt.Client) error) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		err := connFunc(c)
		if err != nil {
			<-ticker.C
			continue
		}
		break
	}
}

func connectMQTT(c mqtt.Client) error {
	token := c.Connect()
	token.Wait()
	if err := token.Error(); err != nil {
		return err
	}
	return nil
}

func (e *spplugExporter) Describe(ch chan<- *prometheus.Desc) {
	mutex.RLock()
	defer mutex.RUnlock()
	ch <- e.versionDesc
	ch <- e.connectDesc
	for _, m := range e.counterMetrics {
		m.Describe(ch)
	}
	for k := range e.metrics {
		for _, ma := range e.metrics[k] {
			ma.prommetric.Describe(ch)
		}

	}
}

func (e *spplugExporter) Collect(ch chan<- prometheus.Metric) {
	mutex.RLock()
	defer mutex.RUnlock()
	ch <- prometheus.MustNewConstMetric(
		e.versionDesc,
		prometheus.GaugeValue,
		1,
	)

	connected := 0.
	if e.client.IsConnectionOpen() {
		connected = 1.
	}

	ch <- prometheus.MustNewConstMetric(
		e.connectDesc,
		prometheus.GaugeValue,
		connected,
	)

	for _, m := range e.counterMetrics {
		m.Collect(ch)
	}

	for k := range e.metrics {
		for _, ma := range e.metrics[k] {
			ma.prommetric.Collect(ch)
		}

	}
}

func (e *spplugExporter) receiveMessage(client mqtt.Client, m mqtt.Message) {
	var pbMsg pb.Payload

	// Unmarshal MQTT message into Google Protocol Buffer
	if err := proto.Unmarshal(m.Payload(), &pbMsg); err != nil {
		log.Errorf("Error decoding GPB, message: %v", err)
		return
	}
	topic := m.Topic()
	topic = strings.Replace(topic, *edgNodePrefix, "", 1)

	log.Debugf("Received message: %s", topic)
	log.Debug(pbMsg.String())

	if e.decisionTree != nil {
		attr := message.Attributes(topic, pbMsg)
		node, err := e.decisionTree.Resolve(attr)
		if err != nil {
			log.Errorf("messageRouter error: %v", err)
			return
		}

		if node.Name == "metric" {
			e.handleMetric(client, topic, pbMsg)
		} else {
			e.handleEvent(topic, pbMsg)
		}
		return
	}

	e.handleMetric(client, topic, pbMsg)
}

func (e *spplugExporter) handleMetric(c mqtt.Client, topic string, pbMsg pb.Payload) {
	mutex.Lock()
	defer mutex.Unlock()

	var eventString string

	// Get the labels and value for the labels from the topic and constants
	siteLabels, siteLabelValues, processMetric := prepareLabelsAndValues(topic)

	if !processMetric {
		return
	}

	// Process this edge node, if it is unique start the re-birth process
	e.evaluateEdgeNode(c, siteLabelValues[SPNamespace],
		siteLabelValues[SPGroupID],
		siteLabelValues[SPEdgeNodeID])

	metricList := pbMsg.GetMetrics()
	log.Debugf("Received message in processMetric: %s", metricList)

	for _, metric := range metricList {

		var newMetric prometheusmetric
		metricLabels := siteLabels
		metricLabelValues := cloneLabelSet(siteLabelValues)

		newLabelName, metricName, err := getMetricName(metric)
		if err != nil {
			if metricName != "Device Control/Rebirth" && metricName != "Scan Rate ms" {
				log.Errorf("Error: %s %s %v", siteLabelValues["sp_edge_node_id"], metricName, err)
				e.counterMetrics[SPPushInvalidMetric].With(siteLabelValues).Inc()
			}

			continue
		}

		if newLabelName != nil {
			for list := 0; list < len(newLabelName); list++ {
				parts := strings.Split(newLabelName[list], ":")

				metricLabels = append(metricLabels, parts[0])
				metricLabelValues[parts[0]] = string(parts[1])
			}
		}

		// TODO: check why this code fails with ("Device Control/Scan Rate ms") And why the err check was after `if newLabelName != nil {` block
		// if err != nil {
		// 	if metricName != "Device Control/Rebirth" && metricName != "Scan Rate ms" {
		// 		log.Errorf("Error: %s %s %v  \n", siteLabelValues["sp_edge_node_id"], metricName, err)
		// 		e.counterMetrics[SPPushInvalidMetric].With(siteLabelValues).Inc()
		// 	}

		// 	continue
		// }

		// if metricName is not within the e.metrics OR
		// if metricLabels (note you will need a function to compare maps) is not within e.metrics[metricName], then you need to create a new metric
		_, metricNameExists := e.metrics[metricName]
		var labelIndex int
		var labelSetExists bool

		// If the metric name exists in the map, that means that we
		// have seen this metric before.   However due to the customized
		// label support, it's possible that we can use an existing metric
		// or need to create a new one.

		if metricNameExists {
			// Each entry under a metricName contains a set of label names
			// and a pointer to the metric.   This is what makes a time
			// series unique, so if the label names we received are not
			// contained in the list, we need to create a new entry

			labelSetExists, labelIndex = compareLabelSet(e.metrics[metricName],
				metricLabels)

			// If the labels are not there, we create a new metric.
			// If they are we update an existing metric
			if !labelSetExists {

				eventString = "Creating new time series for existing metric"
				newMetric.promlabel = append(newMetric.promlabel,
					metricLabels...)

				newMetric.prommetric = createNewMetric(metricName,
					metricLabels)

				labelIndex = len(e.metrics[metricName])

				e.metrics[metricName] = append(e.metrics[metricName],
					newMetric)
			} else {
				eventString = "Updating metric"
				e.metrics[metricName][labelIndex].promlabel = metricLabels
			}
		} else {
			eventString = "Creating metric"
			newMetric.promlabel = append(newMetric.promlabel,
				metricLabels...)
			newMetric.prommetric = createNewMetric(metricName,
				metricLabels)
			labelIndex = 0
			e.metrics[metricName] = append(e.metrics[metricName],
				newMetric)
		}
		if metricVal, err := convertMetricToFloat(metric); err != nil {
			log.Debugf("Error %v converting data type for metric %s", err, metricName)
		} else {
			log.Infof("%s: name (%s) value (%g) labels: (%s)", eventString, metricName, metricVal, metricLabelValues)
			log.Debugf("metriclabels: (%s) siteLabelValues: (%s)", metricLabels, siteLabelValues)

			e.metrics[metricName][labelIndex].prommetric.With(metricLabelValues).Set(metricVal)
			e.metrics[SPLastTimePushedMetric][0].prommetric.With(siteLabelValues).SetToCurrentTime()
			e.counterMetrics[SPPushTotalMetric].With(siteLabelValues).Inc()
		}
	}
}

const (
	positionEdgeNodeID = 3
)

func (e *spplugExporter) handleEvent(topic string, pbMsg pb.Payload) {
	if len(pbMsg.GetMetrics()) == 0 {
		log.Info("skipping event without content")
		return
	}

	topic = trimTopicPrefix(topic, *prefix)

	valueType := pbMsg.GetMetrics()[0].GetDatatype()
	var value interface{}
	switch dataTypeName[valueType] {
	case "Int8", "Int16", "Int32", "Int64", "UInt8", "UInt16", "UInt32", "UInt64":
		value = pbMsg.GetMetrics()[0].GetIntValue()
	case "Float":
		value = pbMsg.GetMetrics()[0].GetFloatValue()
	case "Double":
		value = pbMsg.GetMetrics()[0].GetDoubleValue()
	case "Boolean":
		value = pbMsg.GetMetrics()[0].GetBooleanValue()
	case "String":
		value = pbMsg.GetMetrics()[0].GetStringValue()
	default:
		log.Warnf("event type not supported: %s", dataTypeName[valueType])
	}

	eventValue := fmt.Sprintf("%v", value)
	topicParts := strings.Split(topic, "/")

	var edgeNodeId string
	if len(topicParts) > positionEdgeNodeID {
		edgeNodeId = topicParts[positionEdgeNodeID]
	}

	event := lokiEvent{
		Time:       int(pbMsg.GetTimestamp()),
		Topic:      topic,
		EventName:  pbMsg.GetMetrics()[0].GetName(),
		EventValue: eventValue,
		EventType:  dataTypeName[valueType],
		EdgeNode:   edgeNodeId,
		dropFields: dropFields,
	}

	l := fmt.Sprintf("%s", event)
	for k, v := range *lokiFieldSubstitutions {
		l = strings.Replace(l, k, v, 1)
	}

	e.lokiClient.Infof(l)
}

type lokiEvent struct {
	Time       int
	Topic      string
	EventName  string
	EventValue string
	EventType  string
	EdgeNode   string
	dropFields map[string]struct{}
}

func (l lokiEvent) String() string {
	kv := make([]string, 0, 6)

	if _, ok := l.dropFields["time"]; !ok {
		kv = append(kv, fmt.Sprintf("time=%d", l.Time))
	}
	if _, ok := l.dropFields["topic"]; !ok {
		kv = append(kv, ConditionalQuote("topic=%s", l.Topic))
	}
	if _, ok := l.dropFields["event_name"]; !ok {
		kv = append(kv, ConditionalQuote("event_name=%s", l.EventName))
	}
	if _, ok := l.dropFields["event_value"]; !ok {
		kv = append(kv, ConditionalQuote("event_value=%s", l.EventValue))
	}
	if _, ok := l.dropFields["event_type"]; !ok {
		kv = append(kv, ConditionalQuote("event_type=%s", l.EventType))
	}
	if _, ok := l.dropFields["edge_node"]; !ok {
		kv = append(kv, ConditionalQuote("edge_node=%s", l.EdgeNode))
	}

	return strings.Join(kv, " ")
}

func ConditionalQuote(format, value string) string {
	if strings.Contains(value, " ") {
		format = strings.Replace(format, "%s", "%q", 1)
	}
	return fmt.Sprintf(format, value)
}

// If the edge node is unique (this is the first time seeing it), then
// issue an NCMD and start the rebirth process so we get a fresh set of all
// the metrics / tags

func (e *spplugExporter) evaluateEdgeNode(c mqtt.Client, namespace string,
	group string, nodeID string) {

	edgeNode := group + "/" + nodeID

	if _, exists := edgeNodeList[edgeNode]; !exists {
		edgeNodeList[edgeNode] = true
		e.reincarnate(namespace, group, nodeID)
	} else {
		log.Debugf("Known edge node: %s\n", edgeNode)
	}
}

func (e *spplugExporter) reincarnate(namespace string, group string,
	nodeID string) {
	go func() {
		var pbMsg pb.Payload
		var pbMetric pb.Payload_Metric
		var pbMetricList []*pb.Payload_Metric
		var pbValue pb.Payload_Metric_BooleanValue

		_, labelValues := getNodeLabelSetandValues(namespace, group, nodeID)

		metricName := "Node Control/Rebirth"
		dataType := PBBoolean

		pbValue.BooleanValue = true
		pbMetric.Name = &metricName
		pbMetric.Datatype = &dataType
		pbMetric.Value = &pbValue

		pbMetricList = append(pbMetricList, &pbMetric)
		pbMsg.Metrics = pbMetricList

		topic := namespace + "/" + group + "/NCMD/" + nodeID

		for {
			if e.client.IsConnectionOpen() {
				log.Infof("Reincarnate: %s\n", topic)

				e.counterMetrics[SPReincarnationAttempts].
					With(labelValues).Inc()

				timestamp := uint64(time.Now().UnixNano() / 1000000)
				pbMsg.Timestamp = &timestamp

				if sendMQTTMsg(e.client, &pbMsg, topic) {
					e.counterMetrics[SPReincarnationSuccess].
						With(labelValues).Inc()
				} else {
					e.counterMetrics[SPReincarnationFailures].
						With(labelValues).Inc()
				}

				time.Sleep(time.Duration(SPReincarnateTimer) * time.Second)
			} else {
				e.counterMetrics[SPReincarnationDelay].With(labelValues).Inc()
				time.Sleep(time.Duration(SPReincarnateRetry) * time.Second)
			}
		}
	}()
}

func (e *spplugExporter) initializeMetricsAndData() {

	e.metrics = make(map[string][]prometheusmetric)
	e.counterMetrics = make(map[string]*prometheus.CounterVec)

	edgeNodeList = make(map[string]bool)

	siteLabels := getLabelSet()
	serviceLabels, _ := getServiceLabelSetandValues()
	edgeNodeLabels := getNodeLabelSet()

	log.Debugf(NewMetricString, SPPushTotalMetric)

	e.counterMetrics[SPPushTotalMetric] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: SPPushTotalMetric,
			Help: "Number of messages published on a MQTT topic",
		},
		siteLabels,
	)

	log.Debugf(NewMetricString, SPLastTimePushedMetric)
	var test prometheusmetric
	test.prommetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: SPLastTimePushedMetric,
			Help: "Last time a metric was pushed to a MQTT topic",
		},
		siteLabels,
	)
	e.metrics[SPLastTimePushedMetric] = append(e.metrics[SPLastTimePushedMetric], test)

	log.Debugf(NewMetricString, SPPushInvalidMetric)

	e.counterMetrics[SPPushInvalidMetric] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: SPPushInvalidMetric,
			Help: "Total non-compliant metric names received",
		},
		siteLabels,
	)

	log.Debugf(NewMetricString, SPConnectionCount)

	e.counterMetrics[SPConnectionCount] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: SPConnectionCount,
			Help: "Total MQTT connections established",
		},
		serviceLabels,
	)

	log.Debugf(NewMetricString, SPDisconnectionCount)

	e.counterMetrics[SPDisconnectionCount] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: SPDisconnectionCount,
			Help: "Total MQTT disconnections",
		},
		serviceLabels,
	)

	log.Debugf(NewMetricString, SPReincarnationAttempts)

	e.counterMetrics[SPReincarnationAttempts] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: SPReincarnationAttempts,
			Help: "Total NCMD message attempts",
		},
		edgeNodeLabels,
	)

	log.Debugf(NewMetricString, SPReincarnationFailures)

	e.counterMetrics[SPReincarnationFailures] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: SPReincarnationFailures,
			Help: "Total NCMD message failures",
		},
		edgeNodeLabels,
	)

	log.Debugf(NewMetricString, SPReincarnationSuccess)

	e.counterMetrics[SPReincarnationSuccess] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: SPReincarnationSuccess,
			Help: "Total successful NCMD attempts",
		},
		edgeNodeLabels,
	)

	log.Debugf(NewMetricString, SPReincarnationDelay)

	e.counterMetrics[SPReincarnationDelay] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: SPReincarnationDelay,
			Help: "Total delayed NCMD attempts due to connection issues",
		},
		edgeNodeLabels,
	)
}
