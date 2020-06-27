package main

import (
	"fmt"
	"net/http"
	"sync"

	"golang.org/x/oauth2"
)

// the world wide web
type web struct {
	monce sync.Once
	mux   *http.ServeMux

	smgr *secretsManager

	fsqOauthConfig oauth2.Config
}

func (w *web) index(rw http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(rw, "Not Found", http.StatusNotFound)
		return
	}
}

func (w *web) connect(rw http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(rw, `<a href="%s">Connect Foursquare</a>`, w.fsqOauthConfig.AuthCodeURL("STATE"))
}

func (w *web) fsqcallback(rw http.ResponseWriter, r *http.Request) {
	tok, err := w.fsqOauthConfig.Exchange(r.Context(), r.FormValue("code"))
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	w.smgr.secrets.FourquareAPIKey = tok.AccessToken

	if err := w.smgr.Save(); err != nil {
		http.Error(rw, "saving token: "+err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(rw, "saved")
}

func (w *web) init() {
	w.monce.Do(func() {
		w.mux = http.NewServeMux()

		w.mux.HandleFunc("/", w.index)
		w.mux.HandleFunc("/connect", w.connect)
		w.mux.HandleFunc("/connect/fsqcallback", w.fsqcallback)

	})
}

func (w *web) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	w.init()
	w.mux.ServeHTTP(rw, r)
}
