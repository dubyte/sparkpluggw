package main

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("\n@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@")
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
		fmt.Printf("\n#############################################\n")
		fmt.Printf("Labels: ")
		for _, l := range ts.GetLabels() {
			fmt.Printf(" %s:%s ", l.GetName(), l.GetValue())
		}
		fmt.Printf("\nSamples: ")
		for _, s := range ts.GetSamples() {
			fmt.Printf("timestamp:%d value:%f ", s.T(), s.V())
		}
		fmt.Println()
	}

	w.WriteHeader(http.StatusBadRequest)
}

func main() {
	log.Print("helloworld: starting server...")

	http.HandleFunc("/", handler)

	port := "9090"

	log.Printf("start listening on port %s", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
}
