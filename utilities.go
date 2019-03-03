package main

import (
	"errors"
	"strings"

	pb "github.com/IHI-Energy-Storage/sparkpluggw/Sparkplug"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/golang/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

func sendMQTTMsg(c mqtt.Client, pbMsg *pb.Payload,
	topic string) {

	msg, err := proto.Marshal(pbMsg)

	if err != nil {
		log.Warnf("Failed to Marshall: %s\n", err)
	} else {
		token := c.Publish(topic, 0, false, msg)
		token.Wait()

		log.Infof("Sending NCMD message to topic: %s\n", topic)
	}
}

func prepareLabelsAndValues(topic string) ([]string, prometheus.Labels, bool) {
	t := strings.TrimPrefix(topic, *prefix)
	t = strings.TrimPrefix(t, "/")
	parts := strings.Split(t, "/")

	// 6.1.3 covers 9 message types, only process device data
	// Sparkplug puts 5 key namespacing elements in the topic name
	// these are being parsed and will be added as metric labels

	if (parts[2] == "DDATA") || (parts[2] == "DBIRTH") {
		if len(parts) != 5 {
			log.Debugf("Ignoring topic %s, does not comply with Sparkspec\n", t)
			return nil, nil, false
		}
	} else {
		log.Debugf("Ignoring non-device metric data: %s\n", parts[2])
		return nil, nil, false
	}

	/* See the sparkplug definition for the topic construction */
	/** Set the Prometheus labels to their corresponding topic part **/

	var labels = []string{"sp_namespace", "sp_group_id", "sp_edge_node_id", "sp_device_id"}

	labelValues := prometheus.Labels{}

	// Labels are created from the topic parsing above and compared against
	// the set of labels for this metric.   If this is a unique set then it will
	// be stored and the metric will be treated as unique and new.   If the
	// metric and label set is not new, it will be updated.
	//
	// The logic for this is that the same metric name could used across
	// topics (same metric posted for different devices)

	labelValues["sp_namespace"] = parts[0]
	labelValues["sp_group_id"] = parts[1]
	labelValues["sp_edge_node_id"] = parts[3]
	labelValues["sp_device_id"] = parts[4]

	return labels, labelValues, true
}

func convertMetricToFloat(metric *pb.Payload_Metric) (float64, error) {
	var errUnexpectedType = errors.New("Non-numeric type could not be converted to float")

	const (
		PBInt8   uint32 = 1
		PBInt16  uint32 = 2
		PBInt32  uint32 = 3
		PBInt64  uint32 = 4
		PBUInt8  uint32 = 5
		PBUInt16 uint32 = 6
		PBUInt32 uint32 = 7
		PBUInt64 uint32 = 8
		PBFloat  uint32 = 9
		PBDouble uint32 = 10
	)

	switch metric.GetDatatype() {
	case PBInt8:
		tmpLong := metric.GetIntValue()
		tmpSigned := int8(tmpLong)
		return float64(tmpSigned), nil
	case PBInt16:
		tmpLong := metric.GetIntValue()
		tmpSigned := int16(tmpLong)
		return float64(tmpSigned), nil
	case PBInt32:
		tmpLong := metric.GetIntValue()
		tmpSigned := int32(tmpLong)
		return float64(tmpSigned), nil
	case PBUInt8:
		return float64(metric.GetIntValue()), nil
	case PBUInt16:
		return float64(metric.GetIntValue()), nil
	case PBUInt32:
		return float64(metric.GetIntValue()), nil
	case PBInt64:
		// This exists because there is an unsigned consersion that
		// occurs, so moving it to an int64 allows for the sign to work properly
		tmpLong := metric.GetLongValue()
		tmpSigned := int64(tmpLong)
		return float64(tmpSigned), nil
	case PBUInt64:
		return float64(metric.GetLongValue()), nil
	case PBFloat:
		return float64(metric.GetFloatValue()), nil
	case PBDouble:
		return float64(metric.GetDoubleValue()), nil
	default:
		return float64(0), errUnexpectedType
	}
}