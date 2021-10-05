# Spark Plug Gateway for Prometheus

A project that subscribes to MQTT queues, reads / parses Spark Plug messages and pushes them as Prometheus metrics.

A project that subscribes to MQTT queues and published prometheus metrics.

```bash
usage: sparkpluggw [<flags>]

Flags:
  -h, --help                   Show context-sensitive help (also try --help-long and --help-man).
      --web.listen-address=":9337"  
                               Address on which to expose metrics and web interface
      --web.telemetry-path="/metrics"  
                               Path under which to expose metrics
      --web.disable-telemetry  if this flag is passed the app won't server the metrics endpoint
      --remote-write.enabled   if passed exporter will send metrics to a remote prometheus
      --remote-write.endpoint=http://localhost:9090/api/v1/write  
                               the endpoint where prometheus is listen for remote write requests
      --remote-write.user-agent="sparkpluggw exporter"  
                               the user agent that will be be used to identify the exporter in prometheus
      --remote-write.extra-label=REMOTE-WRITE.EXTRA-LABEL ...  
                               extra label to add
      --remote-write.replace-label=REMOTE-WRITE.REPLACE-LABEL ...  
                               Allows to rename the default labels for remote write
      --remote-write.timeout=30s  
                               Sets the timeout when sending request to prometheus remote write
      --remote-write.send-every=30s  
                               Sets how ofter the exporter will sent metrics to prometheus
      --mqtt.broker-address="tcp://localhost:1883"  
                               Address of the MQTT broker
      --mqtt.topic="prometheus/#"  
                               MQTT topic to subscribe to
      --mqtt.prefix="prometheus"  
                               MQTT topic prefix to remove when creating metrics
      --mqtt.client-id=""      MQTT client identifier (limit to 23 characters)
      --mqtt.debug="false"     Enable MQTT debugging
      --job="sparkpluggw"      the job value used for prometheus remote write
      --log.level="info"       Only log messages with the given severity or above. Valid levels: [debug, info, warn, error, fatal]
      --log.format="logger:stderr"  
                               Set the log target and format. Example: "logger:syslog?appname=bob&local=7" or "logger:stdout?json=true"
```

## Installation

Requires go > 1.17

```bash
go get -u github.com/IHI-Energy-Storage/sparkpluggw
```

## How does it work?

sparkpluggw will connect to the MQTT broker at `--mqtt.broker-address` and
listen to the topics specified by `--mqtt.topic`.

By default, it will listen to `prometheus/#`.

The format for the topics is as follow:

[Link to 2.1AB specification](https://s3.amazonaws.com/cirrus-link-com/Sparkplug+Topic+Namespace+and+State+ManagementV2.1+Apendix++Payload+B+format.pdf)

'namespace/group_id/message_type/edge_node_id/[device_id]'

The following sections are parsed into the following labels and attached to metrics:

- sp_topic
- sp_namespace
- sp_group_id
- sp_msgtype
- sp_edge_node_id
- sp_device_id

Currently only numeric metrics are supported.

In addition to published metrics, sparkpluggw will also publish two additional metrics per topic where messages have been received.

- sp_total_metrics_pushed  - Total metrics processed for that topic
- sp_last_pushed_timestamp - Last timestamp of a message received for that topic

## config file
There is an example config.args which has some limitations explained in the first three lines of the file
so for example you could:
```bash
cp config.arg-example config.args

# sparkpluggw will read your config form the file.
sparkpluggw @config.args
```

## prometheus remote write
When you have a version of prometheus that suports --enable-feature=remote-write-receiver you could enable remote write
by passing --remote-write.enabled.

In cmd package remotewritelistener could help you to visualize more or less what is being sent.

## Security

This project does not support authentication yet but that is planned.

## A note about the prometheus config

If you use `job` and `instance` labels, please refer to the [pushgateway
exporter
documentation](https://github.com/prometheus/pushgateway#about-the-job-and-instance-labels).

TL;DR: you should set `honor_labels: true` in the scrape config.
