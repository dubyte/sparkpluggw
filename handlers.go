package main

import (
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/common/log"
)

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	log.Infof("Connected to MQTT\n")

	exporter.client.Subscribe(*topic, 2, exporter.receiveMessage)

	_, labelValues := getServiceLabelSetandValues()
	exporter.counterMetrics[SPConnectionCount].With(labelValues).Inc()
}

var disconnectHandler mqtt.ConnectionLostHandler = func(_ mqtt.Client,
	err error) {
	log.Infof("Disconnected from MQTT (%s)\n", err.Error())
	_, labelValues := getServiceLabelSetandValues()
	exporter.counterMetrics[SPDisconnectionCount].With(labelValues).Inc()
}
