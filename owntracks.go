package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	log   logger
	store owntracksStore
}

func (o *owntracksServer) HandlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "bad method", http.StatusMethodNotAllowed)
		return
	}

	if r.ContentLength == 0 {
		// in some cases (deleting friends) a 0 length message can be
		// intentionally sent. (ref:
		// https://github.com/owntracks/ios/issues/580#issuecomment-495276821).
		// If we get one of these simply do nothing, as there is nothing to
		// deserialize and no known action we can take
		o.log.Print("skipping publish empty body")
		return
	}

	// parse message
	msg := owntracksMessage{}
	rawMsg, err := io.ReadAll(r.Body)
	if err != nil {
		metricOTSubmitErrorCount.Inc()
		o.log.Printf("read owntracks message: %v", err)
		http.Error(w, fmt.Sprintf("read owntracks message: %v", err), http.StatusInternalServerError)
		return
	}
	if err := json.Unmarshal(rawMsg, &msg); err != nil {
		metricOTSubmitErrorCount.Inc()
		o.log.Printf("decoding owntracks message (%s): %v", string(rawMsg), err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// ignore if not location
	if !msg.IsLocation() {
		o.log.Printf("ignoring payload type %s", msg.Type)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
		return
	}

	if err := o.store.AddOTLocation(r.Context(), msg); err != nil {
		metricOTSubmitErrorCount.Inc()
		o.log.Printf("persisting device location: %v", err)
		http.Error(w, fmt.Sprintf("error: %s", err), http.StatusInternalServerError)
		return
	}

	// return empty json array. Eventually we should support reading/publishing
	// from a MQTT endpoint
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `[]`)
	metricOTSubmitSuccessCount.Inc()
}
