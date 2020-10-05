package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
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

	store owntracksStore

	// If this is set, proxy the location.
	recorderURL *url.URL
}

func (o *owntracksServer) HandlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "bad method", http.StatusMethodNotAllowed)
		return
	}

	// parse message
	msg := owntracksMessage{}
	rawMsg, err := ioutil.ReadAll(r.Body)
	if err != nil {
		metricOTSubmitErrorCount.Inc()
		o.log.Printf("read owntracks message: %v", err)
		http.Error(w, fmt.Sprintf("read owntracks message: %v", err), http.StatusInternalServerError)
		return
	}
	if err := json.Unmarshal(rawMsg, &msg); err != nil {
		metricOTSubmitErrorCount.Inc()
		o.log.Printf("decoding owntracks message: %v", err)
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

	var errs []string

	if err := o.store.AddOTLocation(r.Context(), msg); err != nil {
		errs = append(errs, fmt.Sprintf("saving owntracks location: %v", err))
	}

	if o.recorderURL != nil {
		if err := o.proxyLocation(r, rawMsg); err != nil {
			errs = append(errs, fmt.Sprintf("proxy owntracks location: %v", err))
		}
	}

	// Failed to save location in local database and/or proxy it to recorder.
	if len(errs) != 0 {
		metricOTSubmitErrorCount.Inc()
		o.log.Printf("persisting device location: %s", strings.Join(errs, ", "))
		http.Error(w, fmt.Sprintf("error: %s", strings.Join(errs, ", ")), http.StatusInternalServerError)
		return
	}

	// return empty json array. Eventually we should support reading/publishing
	// from a MQTT endpoint
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `[]`)
	metricOTSubmitSuccessCount.Inc()
}

func (o *owntracksServer) proxyLocation(req *http.Request, msg []byte) error {
	// Set user and device params:
	// https://owntracks.org/booklet/tech/http/
	// Order of precedence: recorderURL params, request headers, request params.
	q := o.recorderURL.Query()
	user, device := q.Get("u"), q.Get("d")
	if user == "" {
		user = req.Header.Get("X-Limit-U")
	}
	if device == "" {
		device = req.Header.Get("X-Limit-D")
	}
	if user == "" {
		user = req.URL.Query().Get("u")
	}
	if device == "" {
		device = req.URL.Query().Get("d")
	}

	proxyURL := *o.recorderURL
	pq := proxyURL.Query()
	pq.Set("u", user)
	pq.Set("d", device)
	proxyURL.RawQuery = pq.Encode()

	resp, err := http.Post(proxyURL.String(), "application/json", bytes.NewReader(msg))
	if err != nil {
		return fmt.Errorf("http post: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		o.log.Printf("successfully proxied owntracks location for user %s, device %s", user, device)
		return nil
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("upstream status %d, body %v", resp.StatusCode, err)
	}
	return fmt.Errorf("upstream status %d, body %s", resp.StatusCode, string(body))
}
