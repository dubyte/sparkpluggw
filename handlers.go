package main

import (
	"github.com/IHI-Energy-Storage/sparkpluggw/log"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	log.Info("Connected to MQTT")

	exporter.client.Subscribe(*topic, 2, exporter.receiveMessage)
	exporter.client.IsConnected()

	_, labelValues := getServiceLabelSetandValues()
	exporter.counterMetrics[SPConnectionCount].With(labelValues).Inc()
}

var disconnectHandler mqtt.ConnectionLostHandler = func(_ mqtt.Client, err error) {
	log.Infof("Disconnected from MQTT (%s)", err)
	_, labelValues := getServiceLabelSetandValues()
	exporter.counterMetrics[SPDisconnectionCount].With(labelValues).Inc()
}
