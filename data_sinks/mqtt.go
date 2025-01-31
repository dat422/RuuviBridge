package data_sinks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Scrin/RuuviBridge/common/limiter"
	"github.com/Scrin/RuuviBridge/config"
	"github.com/Scrin/RuuviBridge/parser"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
)

func MQTT(conf config.MQTTPublisher) chan<- parser.Measurement {
	address := conf.BrokerAddress
	if address == "" {
		address = "localhost"
	}
	port := conf.BrokerPort
	if port == 0 {
		port = 1883
	}
	server := conf.BrokerUrl
	if server == "" {
		server = fmt.Sprintf("tcp://%s:%d", address, port)
	}
	log.WithFields(log.Fields{
		"target":           server,
		"topic_prefix":     conf.TopicPrefix,
		"minimum_interval": conf.MinimumInterval,
	}).Info("Starting MQTT sink")

	clientID := conf.ClientID
	if clientID == "" {
		clientID = "RuuviBridgePublisher"
	}
	opts := mqtt.NewClientOptions()
	opts.SetCleanSession(false)
	opts.AddBroker(server)
	opts.SetClientID(clientID)
	opts.SetUsername(conf.Username)
	opts.SetPassword(conf.Password)
	opts.SetKeepAlive(10 * time.Second)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(10 * time.Second)
	if conf.LWTTopic != "" {
		payload := conf.LWTOfflinePayload
		if payload == "" {
			payload = "{\"state\":\"offline\"}"
		}
		opts.SetWill(conf.LWTTopic, payload, 0, true)
	}
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.WithFields(log.Fields{
			"target":           server,
			"topic_prefix":     conf.TopicPrefix,
			"minimum_interval": conf.MinimumInterval,
		}).WithError(token.Error()).Error("Failed to connect to MQTT")
	}
	if conf.LWTTopic != "" {
		payload := conf.LWTOnlinePayload
		if payload == "" {
			payload = "{\"state\":\"online\"}"
		}
		client.Publish(conf.LWTTopic, 0, true, payload)
	}

	limiter := limiter.New(conf.MinimumInterval)
	measurements := make(chan parser.Measurement, 1024)
	go func() {
		for measurement := range measurements {
			if !limiter.Check(measurement) {
				log.WithFields(log.Fields{"mac": measurement.Mac}).Trace("Skipping MQTT publish due to interval limit")
				continue
			}
			data, err := json.Marshal(measurement)
			if err != nil {
				log.WithError(err).Error("Failed to serialize measurement")
			} else {
				client.Publish(conf.TopicPrefix+"/"+measurement.Mac, 0, false, string(data))
				if conf.HomeassistantDiscoveryPrefix != "" {
					publishHomeAssistantDiscoveries(client, conf, measurement)
				}
			}
		}
	}()
	return measurements
}
