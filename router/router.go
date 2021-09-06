//Package router provides the business logic to decide if a message will be handled as metric or as a event
package router

import (
	pb "github.com/IHI-Energy-Storage/sparkpluggw/Sparkplug"
	"github.com/tkanos/go-dtree"
)

type DecisionTree struct {
	tree *dtree.Tree
}

func New(input string) (DecisionTree, error) {
	var result DecisionTree
	nodes := []dtree.Tree{
		{ID: 1, Name: "root"},
		// xMetricIs
		{ID: 2, ParentID: 1, Value: "Device Control/Scan Rate ms", Operator: "eq", Key: "firstMetricIs"},
		{ID: 3, ParentID: 1, Value: "Device Control/Scan Rate ms", Operator: "ne", Key: "firstMetricIs"},
		{ID: 4, Name: "metric", ParentID: 2},
		{ID: 5, Name: "event", ParentID: 3},
	}
	// data, err := os.ReadFile(input)
	// if err != nil {
	// 	return result, err
	// }

	// tree, err = dtree.LoadTree(data)
	// if err != nil {
	// 	return result, err
	// }

	result = DecisionTree{dtree.CreateTree(nodes)}
	return result, nil
}

func (d DecisionTree) Resolve(payload pb.Payload, topic string) (string, error) {
	msg := make(map[string]interface{})
	msg["firstMetricIs"] = firstMetric(payload)
	node, err := d.tree.Resolve(msg)
	if err != nil {
		return "", err
	}

	return node.Name, nil
}

func firstMetric(payload pb.Payload) string {
	var firstMetric string
	if len(payload.Metrics) > 0 {
		firstMetric = *payload.GetMetrics()[0].Name
	}
	return firstMetric
}
