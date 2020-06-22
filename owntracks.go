package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type owntracksStore interface {
	// AddOTLocation persists the message, of type location
	AddOTLocation(context.Context, owntracksMessage) error
}

// owntracksServer can handle receiving and persisting owntracks events from a
// phone
//
// https://owntracks.org/booklet/tech/http/
type owntracksServer struct {
	log logger

	// basic auth for the endpoint
	username string
	password string

	store owntracksStore

	brokerURL      string
	brokerUsername string
	brokerPassword string
	brokerTopic    string
}

func (o *owntracksServer) HandlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "bad method", http.StatusMethodNotAllowed)
		return
	}

	u, p, ok := r.BasicAuth()
	if !(ok && u == o.username && p == o.password) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// parse message
	msg := owntracksMessage{}
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		o.log.Printf("decoding owntracks message: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// ignore if not location
	if msg.IsLocation() {
		if err := o.store.AddOTLocation(r.Context(), msg); err != nil {
			o.log.Printf("saving owntracks location: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Publish to MQTT, get response info
	opts := mqtt.NewClientOptions()
	opts.AddBroker(o.brokerURL)
	opts.SetClientID("wherewasi")
	opts.SetUsername(o.brokerUsername)
	opts.SetPassword(o.brokerPassword)

	choke := make(chan [2]string)

	opts.SetDefaultPublishHandler(func(_ mqtt.Client, msg mqtt.Message) {
		choke <- [2]string{msg.Topic(), string(msg.Payload())}
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		o.log.Printf("connecting to broker: %v", token.Error())
		http.Error(w, token.Error().Error(), http.StatusInternalServerError)
		return
	}

	if token := client.Subscribe(o.brokerTopic, byte(0), nil); token.Wait() && token.Error() != nil {
		o.log.Printf("subscribing: %v", token.Error())
		http.Error(w, token.Error().Error(), http.StatusInternalServerError)
		return
	}

	to := time.NewTimer(5 * time.Second)

outer:
	for {
		select {
		case <-to.C:
			break outer
		case msg := <-choke:
			fmt.Printf("RECEIVED TOPIC: %s MESSAGE: %s\n", msg[0], msg[1])
		}
	}

	// return empty json array. Eventually we should support reading/publishing
	// from a MQTT endpoint
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `[]`)
}
