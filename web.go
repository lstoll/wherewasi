package main

import (
	"net/http"
	"sync"
)

// the world wide web
type web struct {
	monce sync.Once
	mux   *http.ServeMux
}

func (w *web) index(rw http.ResponseWriter, r *http.Request) {

}

func (w *web) init() {
	w.monce.Do(func() {
		w.mux = http.NewServeMux()

		w.mux.HandleFunc("/", w.index)
	})
}

func (w *web) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	w.init()
	w.mux.ServeHTTP(rw, r)
}
