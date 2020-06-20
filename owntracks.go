package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

	// return empty json array. Eventually we should support reading/publishing
	// from a MQTT endpoint
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `[]`)
}
