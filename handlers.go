package main

import (
	"fmt"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/go-kit/log/level"
)

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	level.Info(logger).Log("msg", fmt.Sprint("Connected to MQTT"))

	exporter.client.Subscribe(*topic, 2, exporter.receiveMessage())

	_, labelValues := getServiceLabelSetandValues()
	exporter.counterMetrics[SPConnectionCount].With(labelValues).Inc()
}

var disconnectHandler mqtt.ConnectionLostHandler = func(_ mqtt.Client,
	err error) {
	level.Info(logger).Log("msg", fmt.Sprintf("Disconnected from MQTT (%s)", err.Error()))
	_, labelValues := getServiceLabelSetandValues()
	exporter.counterMetrics[SPDisconnectionCount].With(labelValues).Inc()
}
