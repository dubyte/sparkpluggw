package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

func handler(w http.ResponseWriter, r *http.Request) {
	compressed, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("error while reading body request: %s", err)
	}

	data, err := snappy.Decode(nil, compressed)
	if err != nil {
		log.Printf("error while decoding snappy compressed request: %s", err)
		return
	}

	var req prompb.WriteRequest

	if err := proto.Unmarshal(data, &req); err != nil {
		log.Printf("error while unmarshalling remote request: %s", err)
	}
	for _, ts := range req.Timeseries {
		var line string
		for _, l := range ts.GetLabels() {
			if l.GetName() == "__name__" {
				line += fmt.Sprintf(" %s{", l.GetValue())
			} else {
				line += fmt.Sprintf("%s=%s,", l.GetName(), l.GetValue())
			}
		}

		line = strings.TrimSuffix(line, ",")

		for _, s := range ts.GetSamples() {
			line += fmt.Sprintf("} %.02f", s.V())
		}

		fmt.Println(line)
	}
	fmt.Println("--")
	w.WriteHeader(http.StatusOK)
}

func main() {
	log.Print("remote write listener: starting server...")

	http.HandleFunc("/", handler)

	port := "9090"

	log.Printf("start listening on port %s", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
}
