//Package message provides the business logic to decide if a message will be handled as metric or as a event
package message

import (
	"fmt"

	pb "github.com/IHI-Energy-Storage/sparkpluggw/Sparkplug"
)

func Attributes(topic string, payload pb.Payload) map[string]interface{} {
	attr := make(map[string]interface{})
	attr["firstMetricIs"] = firstMetric(payload)
	attr["metricsLen"] = fmt.Sprintf("%d", len(payload.Metrics))
	return attr
}

func firstMetric(payload pb.Payload) string {
	var firstMetric string
	if len(payload.Metrics) > 0 {
		firstMetric = *payload.GetMetrics()[0].Name
	}
	return firstMetric
}
