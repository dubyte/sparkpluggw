# Spark Plug Gateway for Prometheus

A project that subscribes to MQTT queues, reads / parses Spark Plug messages and pushes them as Prometheus metrics.

A project that subscribes to MQTT queues and published prometheus metrics.

```
usage: sparkpluggw [<flags>]

Flags:
  -h, --help                     Show context-sensitive help (also try --help-long and --help-man).
      --web.listen-address=":9337"  
                                 Address on which to expose metrics and web interface
      --web.telemetry-path="/metrics"  
                                 Path under which to expose metrics
      --web.disable-telemetry    if this flag is passed the app won't server the metrics endpoint
      --remote-write.enabled     if passed exporter will send metrics to a remote prometheus
      --loki.enabled             if passed some messages could be sent to loki instead of store them as metrics using a decision tree
      --remote-write.endpoint=http://localhost:9090/api/v1/write  
                                 the endpoint where prometheus is listen for remote write requests
      --remote-write.user-agent="sparkpluggw exporter"  
                                 the user agent that will be be used to identify the exporter in prometheus
      --remote-write.extra-label=REMOTE-WRITE.EXTRA-LABEL ...  
                                 extra label to add ( example --remote-write.extra-label=labelName=labelValue)
      --remote-write.replace-label=REMOTE-WRITE.REPLACE-LABEL ...  
                                 Allows to rename the default labels for remote write (for example --remote-write.replace-label=sp_namespace=ns will replace sp_namespace for ns)
      --remote-write.drop-label=REMOTE-WRITE.DROP-LABEL ...  
                                 remove to not send a label for remote write (repeat to remove more than one field)
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
      --mqtt.client-id=""        MQTT client identifier (limit to 23 characters)
      --mqtt.username=""         User to connect MQTT server
      --mqtt.password=""         Password to connect MQTT server
      --mqtt.ca.crt.file=""      the path of the extra CA certificate to be used to connect to mqtt
      --mqtt.crt.file=""         the path of the tls certificate used to connect to mqtt
      --mqtt.key.file=""         the path of the tls key used to connect to mqtt
      --mqtt.insecure-skip-verify  
                                 if passed mqtt client will verify the certificate with the ca
      --mqtt.debug="false"       Enable MQTT debugging
      --mqtt.conn.retry="false"  When enabled it will try to connect every 10 seconds if the first time was not possible
      --job="sparkpluggw"        the job value used for prometheus remote write and loki
      --loki.push-URL="http://localhost:3100/loki/api/v1/push"  
                                 the url of the loki push endpoint to send the logs
      --loki.extra-label=LOKI.EXTRA-LABEL ...  
                                 Label to send to loki (example --loki.extra-label=datacenter=us-west)
      --loki.batch-wait=5s       Maximum amount of time to wait before sending a batch, even if that batch isn't full.
      --loki.replace-field=LOKI.REPLACE-FIELD ...  
                                 Allows to rename the default fields for loki for example --loki.replace-field=sp_namespace=ns will replace sp_namespace for ns
      --decision-tree.file=DECISION-TREE.FILE  
                                 path to file with a decision tree defined in json
      --loki.drop-field=LOKI.DROP-FIELD ...  
                                 field will not be send to loki. (example --loki.drop-field=event_name --loki.drop-field=event_type)
      --edge-node.prefix=""      if given it removes the prefix from the edge_node_id
      --log.level="info"         Only log messages with the given severity or above. Valid levels: [debug, info, warn, error, fatal]
      --log.format="logger:stderr"  
                                 Set the log target and format. Example: "logger:syslog?appname=bob&local=7" or "logger:stdout?json=true"
```

## Installation

Requires go > 1.18

```
go get -u github.com/IHI-Energy-Storage/sparkpluggw
```

## How does it work?

sparkpluggw will connect to the MQTT broker at `--mqtt.broker-address` and
listen to the topics specified by `--mqtt.topic`.

By default, it will listen to `prometheus/#`.

The format for the topics is as follows:

[Link to 2.1AB specification](https://s3.amazonaws.com/cirrus-link-com/Sparkplug+Topic+Namespace+and+State+ManagementV2.1+Apendix++Payload+B+format.pdf)

'namespace/group_id/message_type/edge_node_id/[device_id]'

The following sections are parsed into the following labels and attached to metrics:

- sp_topic
- sp_namespace
- sp_group_id
- sp_msgtype
- sp_edge_node_id
- sp_device_id

Currently, only numeric metrics are supported.

In addition to published metrics, sparkpluggw will also publish two additional metrics per topic where messages have been received.

- sp_total_metrics_pushed  - Total metrics processed for that topic
- sp_last_pushed_timestamp - Last timestamp of a message received for that topic

## Security

This project does not support authentication yet but that is planned.

## A note about the prometheus config

If you use `job` and `instance` labels, please refer to the [pushgateway
exporter
documentation](https://github.com/prometheus/pushgateway#about-the-job-and-instance-labels).

TL;DR: you should set `honor_labels: true` in the scrape config.
