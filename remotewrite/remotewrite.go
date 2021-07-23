//Package remotewrite provides helper functions to work with remote write
package remotewrite

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
)

//TODO: accept a logger and send to /dev/null by default

func Writer(logger log.Logger) func() {
	return func() {
		timestamp := time.Now().UnixNano() / int64(time.Millisecond)

		req, err := BuildWriteRequest(prometheus.DefaultGatherer, timestamp)
		if err != nil {
			level.Error(logger).Log("msg", fmt.Sprintf("error while building remote write request: %s", err))
			return
		}

		data, err := proto.Marshal(&req)
		if err != nil {
			level.Error(logger).Log("msg", fmt.Sprintf("error while marshalling write request: %s", err))
			return
		}

		compressed := snappy.Encode(nil, data)
		u, err := url.Parse("http://localhost:9090/api/v1/write")
		if err != nil {
			level.Error(logger).Log("msg", fmt.Sprintf("error while parsing url: %s", err))
			return
		}

		clientConfig := remote.ClientConfig{
			URL:              &config.URL{URL: u},
			Timeout:          model.Duration(30 * time.Second),
			HTTPClientConfig: config.HTTPClientConfig{},
			SigV4Config:      nil,
			Headers:          nil,
			RetryOnRateLimit: false,
		}
		remote.UserAgent = "sparkplug exporter"
		c, err := remote.NewWriteClient("sparkplug exporter", &clientConfig)
		if err != nil {
			level.Error(logger).Log("msg", fmt.Sprintf("error while creating client: %s", err))
			return
		}

		err = c.Store(context.Background(), compressed)
		if err != nil {
			level.Error(logger).Log("msg", fmt.Sprintf("error while storing Time series: %s", err))
		}
	}
}

//BuildWriteRequest get the metrics from the gatherer using passed timestamp build remoteWrite request
func BuildWriteRequest(g prometheus.Gatherer, unixTimeStamp int64) (prompb.WriteRequest, error) {
	var req prompb.WriteRequest

	mfs, err := g.Gather()
	if err != nil {
		return req, fmt.Errorf("gatherer.Gather err: %w", err)
	}

	for _, mf := range mfs {
		for _, metric := range mf.GetMetric() {

			//TODO get the instance from config
			commonLabels := prometheusTSLabels("localhost:9337", metric)

			switch mf.GetType() {

			case dto.MetricType_COUNTER:
				if metric.GetCounter() == nil {
					return req, fmt.Errorf("metric %s is %s but Get%s returned null", mf.GetName(), mf.GetType(), mf.GetType())
				}
				ts, err := buildPrometheusTS(unixTimeStamp, mf.GetName(), metric.GetCounter().GetValue(), commonLabels)
				if err != nil {
					return req, fmt.Errorf("buildPrometheusTS err: %w", err)
				}
				req.Timeseries = append(req.Timeseries, ts)

			case dto.MetricType_GAUGE:
				if metric.GetGauge() == nil {
					return req, fmt.Errorf("metric %s is %s but Get%s returned null", mf.GetName(), mf.GetType(), mf.GetType())
				}
				ts, err := buildPrometheusTS(unixTimeStamp, mf.GetName(), metric.GetGauge().GetValue(), commonLabels)
				if err != nil {
					return req, fmt.Errorf("buildPrometheusTS err: %w", err)
				}
				req.Timeseries = append(req.Timeseries, ts)

			case dto.MetricType_UNTYPED:
				if metric.GetUntyped() == nil {
					return req, fmt.Errorf("metric %s is %s but Get%s returned null", mf.GetName(), mf.GetType(), mf.GetType())
				}
				ts, err := buildPrometheusTS(unixTimeStamp, mf.GetName(), metric.GetUntyped().GetValue(), commonLabels)
				if err != nil {
					return req, fmt.Errorf("buildPrometheusTS err: %w", err)
				}
				req.Timeseries = append(req.Timeseries, ts)

			case dto.MetricType_SUMMARY:
				if metric.GetSummary() == nil {
					return req, fmt.Errorf("metric %s is %s but Get%s returned null", mf.GetName(), mf.GetType(), mf.GetType())
				}

				for _, q := range metric.GetSummary().GetQuantile() {
					labels := append(commonLabels, prompb.Label{Name: "quantile", Value: fmt.Sprintf("%f", q.GetQuantile())})
					ts, err := buildPrometheusTS(unixTimeStamp, fmt.Sprintf("%s_sum", mf.GetName()), q.GetValue(), labels)
					if err != nil {
						return req, fmt.Errorf("buildPrometheusTS err: %w", err)
					}
					req.Timeseries = append(req.Timeseries, ts)
				}

				// summary sum
				{
					ts, err := buildPrometheusTS(unixTimeStamp, fmt.Sprintf("%s_sum", mf.GetName()), metric.GetSummary().GetSampleSum(), commonLabels)
					if err != nil {
						return req, fmt.Errorf("buildPrometheusTS err: %w", err)
					}
					req.Timeseries = append(req.Timeseries, ts)
				}

				// summary count
				{
					ts, err := buildPrometheusTS(unixTimeStamp, fmt.Sprintf("%s_count", mf.GetName()), float64(metric.GetSummary().GetSampleCount()), commonLabels)
					if err != nil {
						return req, fmt.Errorf("buildPrometheusTS err: %w", err)
					}
					req.Timeseries = append(req.Timeseries, ts)
				}
			case dto.MetricType_HISTOGRAM:
				if metric.GetHistogram() == nil {
					return req, fmt.Errorf("metric %s is %s but Get%s returned null", mf.GetName(), mf.GetType(), mf.GetType())
				}

				for _, bucket := range metric.GetHistogram().GetBucket() {
					labels := append(commonLabels, prompb.Label{Name: "le", Value: fmt.Sprintf("%f", bucket.GetUpperBound())})
					ts, err := buildPrometheusTS(unixTimeStamp, fmt.Sprintf("%s_sum", mf.GetName()), float64(bucket.GetCumulativeCount()), labels)
					if err != nil {
						return req, fmt.Errorf("buildPrometheusTS err: %w", err)
					}
					req.Timeseries = append(req.Timeseries, ts)
				}

				// histogram sum
				{
					ts, err := buildPrometheusTS(unixTimeStamp, fmt.Sprintf("%s_sum", mf.GetName()), metric.GetHistogram().GetSampleSum(), commonLabels)
					if err != nil {
						return req, fmt.Errorf("buildPrometheusTS err: %w", err)
					}
					req.Timeseries = append(req.Timeseries, ts)
				}

				// histogram count
				{
					ts, err := buildPrometheusTS(unixTimeStamp, fmt.Sprintf("%s_count", mf.GetName()), float64(metric.GetHistogram().GetSampleCount()), commonLabels)
					if err != nil {
						return req, fmt.Errorf("buildPrometheusTS err: %w", err)
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

func prometheusTSLabels(instance string, m *dto.Metric) []prompb.Label {
	var labels []prompb.Label
	for _, pair := range m.GetLabel() {
		var label prompb.Label
		label.Name = pair.GetName()
		label.Value = pair.GetValue()
		labels = append(labels, label)
	}
	labels = append(labels, prompb.Label{Name: "instance", Value: instance})
	return labels
}

func buildPrometheusTS(timeStamp int64, metricName string, value float64, labels []prompb.Label) (prompb.TimeSeries, error) {
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
