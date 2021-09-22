package main

import (
	_ "embed"
	"fmt"
	"log"

	pb "github.com/IHI-Energy-Storage/sparkpluggw/Sparkplug"
	"github.com/IHI-Energy-Storage/sparkpluggw/message"
	"github.com/golang/protobuf/jsonpb"
	"github.com/tkanos/go-dtree"
)

//go:embed decision-tree-example.json
var data []byte

//go:embed metricPayload-example.json
var metricPayload string

//go:embed eventPayload-example.json
var eventPayload string

func main() {
	//edit: decision-tree-example.json to play with the decision tree
	t, err := dtree.LoadTree(data)
	if err != nil {
		log.Printf("loadTree err: %s", err)
		return
	}
	fmt.Printf("tree:\n%s", t)

	var p pb.Payload
	//choose between eventPayload or metricPayload
	err = jsonpb.UnmarshalString(metricPayload, &p)
	if err != nil {
		log.Printf("loadTree err: %s", err)
		return
	}

	m := jsonpb.Marshaler{Indent: "  "}
	j, err := m.MarshalToString(&p)
	topic := "spBv1.0/vehicles/DDATA/1234-1234/bus"
	fmt.Printf("topic: %s\npayload:\n%s", topic, j)

	attr := message.Attributes("spBv1.0/vehicles/DDATA/1234-1234/bus", p)

	node, err := t.Resolve(attr)
	if err != nil {
		log.Printf("resolve with: %+v err: %s", attr, err)
		return
	}

	fmt.Printf("\nresult: %s\nrepresentation:\n%v", node.Name, node)
}

func StringPtr(s string) *string {
	return &s
}

func DataTypeValue(s string) *uint32 {
	dataTypeValue := map[string]uint32{
		"Unknown":         0,
		"Int8":            1,
		"Int16":           2,
		"Int32":           3,
		"Int64":           4,
		"UInt8":           5,
		"UInt16":          6,
		"UInt32":          7,
		"UInt64":          8,
		"Float":           9,
		"Double":          10,
		"Boolean":         11,
		"String":          12,
		"DateTime":        13,
		"Text":            14,
		"UUID":            15,
		"DataSet":         16,
		"Bytes":           17,
		"File":            18,
		"Template":        19,
		"PropertySet":     20,
		"PropertySetList": 21,
		"Int8Array":       22,
		"Int16Array":      23,
		"Int32Array":      24,
		"Int64Array":      25,
		"UInt8Array":      26,
		"UInt16Array":     27,
		"UInt32Array":     28,
		"UInt64Array":     29,
		"FloatArray":      30,
		"DoubleArray":     31,
		"BooleanArray":    32,
		"StringArray":     33,
		"DateTimeArray":   34,
	}

	val := dataTypeValue[s]

	// if the key does not exist return unknown (address of "zero")
	return &val
}
