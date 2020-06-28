package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/ancientlore/go-tripit"
	"golang.org/x/oauth2"
)

// the world wide web
type web struct {
	monce sync.Once
	mux   *http.ServeMux

	baseURL string

	smgr *secretsManager

	fsqOauthConfig oauth2.Config

	tripitAPIKey    string
	tripitAPISecret string
}

func (w *web) index(rw http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(rw, "Not Found", http.StatusNotFound)
		return
	}
}

func (w *web) connect(rw http.ResponseWriter, r *http.Request) {
	var links []string
	if w.fsqOauthConfig.ClientID != "" {
		links = append(links, fmt.Sprintf(`<a href="%s">Connect Foursquare</a><br>`, w.fsqOauthConfig.AuthCodeURL("STATE")))
	}
	if w.tripitAPIKey != "" {
		cred := tripit.NewOAuthRequestCredential(w.tripitAPIKey, w.tripitAPISecret)
		t := tripit.New(tripit.ApiUrl, tripit.ApiVersion, http.DefaultClient, cred)
		log.Print("start call")
		m, err := t.GetRequestToken()
		if err != nil {
			http.Error(rw, "Getting tripit request token: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.smgr.secrets.TripitOAuthToken = m["oauth_token"]
		w.smgr.secrets.TripitOAuthSecret = m["oauth_token_secret"]

		aurl := w.baseURL + "/connect/tripitcallback"

		links = append(links, fmt.Sprintf(`<a href="%s">Connect TripIt</a><br>`, fmt.Sprintf(tripit.UrlObtainUserAuthorization, url.QueryEscape(m["oauth_token"]), url.QueryEscape(aurl))))
	}
	fmt.Fprint(rw, strings.Join(links, "<br>"))
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

func (w *web) tripitCallback(rw http.ResponseWriter, r *http.Request) {
	cred := tripit.NewOAuth3LeggedCredential(w.tripitAPIKey, w.tripitAPISecret, w.smgr.secrets.TripitOAuthToken, w.smgr.secrets.TripitOAuthSecret)
	t := tripit.New(tripit.ApiUrl, tripit.ApiVersion, http.DefaultClient, cred)
	m, err := t.GetAccessToken()
	if err != nil {
		http.Error(rw, "Getting tripit access token: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.smgr.secrets.TripitOAuthToken = m["oauth_token"]
	w.smgr.secrets.TripitOAuthSecret = m["oauth_token_secret"]

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
		w.mux.HandleFunc("/connect/tripitcallback", w.tripitCallback)
	})
}

func (w *web) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	w.init()
	w.mux.ServeHTTP(rw, r)
}
