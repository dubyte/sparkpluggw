//Package remotewrite provides helper functions to work a the prometheus remote write from the client perspective
package remotewrite

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
)

type Writer struct {
	Client      remote.WriteClient
	Logger      log.Logger
	Gatherer    prometheus.Gatherer
	ExtraLabels []prompb.Label
}

//Write get the metrics from the gatherer using the current time it creates a remote write request
// after marshall it and compress it using snappy it use the client to send the request.
func (w Writer) Write() {
	timestamp := time.Now().UnixNano() / int64(time.Millisecond)

	req, err := WriteRequest(w.Gatherer, timestamp, w.ExtraLabels...)
	if err != nil {
		level.Error(w.Logger).Log("msg", fmt.Sprintf("error while building remote write request: %s", err))
		return
	}

	data, err := proto.Marshal(&req)
	if err != nil {
		level.Error(w.Logger).Log("msg", fmt.Sprintf("error while marshalling write request: %s", err))
		return
	}

	compressed := snappy.Encode(nil, data)

	// Store adds headers like the contentType, encoding.
	err = w.Client.Store(context.Background(), compressed)
	if err != nil {
		level.Error(w.Logger).Log("msg", fmt.Sprintf("error while storing Time series: %s", err))
	}
}

//Client builds a config base on the parameter sent and return a remote.client using that config.
func Client(rawurl string, timeout time.Duration, userAgent string, retryOnRateLimit bool) (remote.WriteClient, error) {
	u, err := url.Parse(rawurl)
	if err != nil {
		return nil, fmt.Errorf("error while parsing url '%s': %w", rawurl, err)
	}
	clientConfig := remote.ClientConfig{
		URL:              &config.URL{URL: u},
		Timeout:          model.Duration(timeout),
		HTTPClientConfig: config.HTTPClientConfig{},
		SigV4Config:      nil,
		Headers:          nil,
		RetryOnRateLimit: retryOnRateLimit,
	}

	c, err := ClientFromConfig(clientConfig, userAgent)
	if err != nil {
		return nil, fmt.Errorf("error while creating client: %w", err)
	}

	return c, nil
}

// ClientFromConfig receives a remote.ClientConfig and an user agent use this functions when you need
// more control about the client like auth for example.
func ClientFromConfig(cfg remote.ClientConfig, userAgent string) (remote.WriteClient, error) {
	if userAgent != "" {
		remote.UserAgent = userAgent
	}

	c, err := remote.NewWriteClient("sparkplug exporter", &cfg)
	if err != nil {
		return nil, fmt.Errorf("error while creating client: %w", err)
	}
	return c, nil
}

//WriteRequest get the metric families from the gatherer using passed timestamp in unix miliseconds time and returns
// a remoteWrite request
func WriteRequest(g prometheus.Gatherer, timeStamp int64, extraLabels ...prompb.Label) (prompb.WriteRequest, error) {
	var req prompb.WriteRequest

	mfs, err := g.Gather()
	if err != nil {
		return req, fmt.Errorf("gatherer.Gather err: %w", err)
	}

	// metric families are the metrics with the same name and type but could have different labels.
	for _, mf := range mfs {
		for _, metric := range mf.GetMetric() {

			//TODO get the instance from config
			commonLabels := prometheusTSLabels(metric, extraLabels)

			switch mf.GetType() {

			case dto.MetricType_COUNTER:
				if metric.GetCounter() == nil {
					return req, fmt.Errorf("metric %s is %s but Get%s returned null", mf.GetName(), mf.GetType(), mf.GetType())
				}
				ts, err := prometheusTS(timeStamp, mf.GetName(), metric.GetCounter().GetValue(), commonLabels)
				if err != nil {
					return req, fmt.Errorf("prometheusTS err: %w", err)
				}
				req.Timeseries = append(req.Timeseries, ts)

			case dto.MetricType_GAUGE:
				if metric.GetGauge() == nil {
					return req, fmt.Errorf("metric %s is %s but Get%s returned null", mf.GetName(), mf.GetType(), mf.GetType())
				}
				ts, err := prometheusTS(timeStamp, mf.GetName(), metric.GetGauge().GetValue(), commonLabels)
				if err != nil {
					return req, fmt.Errorf("prometheusTS err: %w", err)
				}
				req.Timeseries = append(req.Timeseries, ts)

			case dto.MetricType_UNTYPED:
				if metric.GetUntyped() == nil {
					return req, fmt.Errorf("metric %s is %s but Get%s returned null", mf.GetName(), mf.GetType(), mf.GetType())
				}
				ts, err := prometheusTS(timeStamp, mf.GetName(), metric.GetUntyped().GetValue(), commonLabels)
				if err != nil {
					return req, fmt.Errorf("prometheusTS err: %w", err)
				}
				req.Timeseries = append(req.Timeseries, ts)

			case dto.MetricType_SUMMARY:
				if metric.GetSummary() == nil {
					return req, fmt.Errorf("metric %s is %s but Get%s returned null", mf.GetName(), mf.GetType(), mf.GetType())
				}

				for _, q := range metric.GetSummary().GetQuantile() {
					labels := append(commonLabels, prompb.Label{Name: "quantile", Value: fmt.Sprintf("%f", q.GetQuantile())})
					ts, err := prometheusTS(timeStamp, fmt.Sprintf("%s_sum", mf.GetName()), q.GetValue(), labels)
					if err != nil {
						return req, fmt.Errorf("prometheusTS err: %w", err)
					}
					req.Timeseries = append(req.Timeseries, ts)
				}

				// add summary sum
				{
					ts, err := prometheusTS(timeStamp, fmt.Sprintf("%s_sum", mf.GetName()), metric.GetSummary().GetSampleSum(), commonLabels)
					if err != nil {
						return req, fmt.Errorf("prometheusTS err: %w", err)
					}
					req.Timeseries = append(req.Timeseries, ts)
				}

				// add summary count
				{
					ts, err := prometheusTS(timeStamp, fmt.Sprintf("%s_count", mf.GetName()), float64(metric.GetSummary().GetSampleCount()), commonLabels)
					if err != nil {
						return req, fmt.Errorf("prometheusTS err: %w", err)
					}
					req.Timeseries = append(req.Timeseries, ts)
				}
			case dto.MetricType_HISTOGRAM:
				if metric.GetHistogram() == nil {
					return req, fmt.Errorf("metric %s is %s but Get%s returned null", mf.GetName(), mf.GetType(), mf.GetType())
				}

				for _, bucket := range metric.GetHistogram().GetBucket() {
					labels := append(commonLabels, prompb.Label{Name: "le", Value: fmt.Sprintf("%f", bucket.GetUpperBound())})
					ts, err := prometheusTS(timeStamp, fmt.Sprintf("%s_sum", mf.GetName()), float64(bucket.GetCumulativeCount()), labels)
					if err != nil {
						return req, fmt.Errorf("prometheusTS err: %w", err)
					}
					req.Timeseries = append(req.Timeseries, ts)
				}

				// add histogram sum
				{
					ts, err := prometheusTS(timeStamp, fmt.Sprintf("%s_sum", mf.GetName()), metric.GetHistogram().GetSampleSum(), commonLabels)
					if err != nil {
						return req, fmt.Errorf("prometheusTS err: %w", err)
					}
					req.Timeseries = append(req.Timeseries, ts)
				}

				// add histogram count
				{
					ts, err := prometheusTS(timeStamp, fmt.Sprintf("%s_count", mf.GetName()), float64(metric.GetHistogram().GetSampleCount()), commonLabels)
					if err != nil {
						return req, fmt.Errorf("prometheusTS err: %w", err)
					}
					req.Timeseries = append(req.Timeseries, ts)
				}

			default:
				return req, fmt.Errorf("%s has an unknown metric type '%s'", mf.GetName(), mf.GetType())
			}
		}
	}

	return req, nil
}

// prometheusTS builds a prometheus time series (as needed for a remote write request) from the parameters received.
func prometheusTS(timeStamp int64, metricName string, value float64, labels []prompb.Label) (prompb.TimeSeries, error) {
	ts := prompb.TimeSeries{
		Samples: []prompb.Sample{{Value: value, Timestamp: timeStamp}},
		Labels:  labels,
	}
	ts.Labels = append(ts.Labels, prompb.Label{Name: "__name__", Value: metricName})
	sort.Slice(ts.Labels, func(i int, j int) bool {
		return ts.Labels[i].Name < ts.Labels[j].Name
	})

	return ts, nil
}

// prometheusTSLabels converts the labels from a metric family for one that can be use in the remote request.
func prometheusTSLabels(m *dto.Metric, extra []prompb.Label) []prompb.Label {
	var labels []prompb.Label

	for _, mLabel := range m.GetLabel() {
		var l prompb.Label
		l.Name = mLabel.GetName()
		l.Value = mLabel.GetValue()
		labels = append(labels, l)
	}

	for _, e := range extra {
		var l prompb.Label
		l.Name = e.GetName()
		l.Value = e.GetValue()
		labels = append(labels, l)
	}
	return labels
}
