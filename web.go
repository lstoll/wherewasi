package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ancientlore/go-tripit"
	"golang.org/x/oauth2"
)

var templates = template.Must(template.ParseGlob("*.html"))

// the world wide web
type web struct {
	log logger

	monce sync.Once
	mux   *http.ServeMux

	baseURL string

	smgr  *secretsManager
	store *Storage

	fsqOauthConfig oauth2.Config

	tripitAPIKey    string
	tripitAPISecret string

	mapboxAPIToken string
}

func (w *web) index(rw http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(rw, "Not Found", http.StatusNotFound)
		return
	}

	if err := templates.ExecuteTemplate(rw, "orion-index.html", map[string]interface{}{"MapboxAPIToken": w.mapboxAPIToken}); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
}

// orionLocation represents the data the orion webui uses. These appear to be
// pass through OT messages in the orion server, so use the same formats as
// OT here.
type orionLocation struct {
	Accuracy  int     `json:"accuracy,omitempty"`
	Timestamp int64   `json:"timestamp,omitempty"`
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
}

type orionLocationRequest struct {
	// user -- The username for which entries should be fetched
	User string `json:"user"`
	// device -- The corresponding device name for which entries should be fetched
	Device string `json:"device"`
	// offset -- An offset to apply to the selection (default 0)
	Offset *int `json:"offset"`
	// limit -- A limit of entries to return (default 10)
	Limit *int `json:"limit"`
	// timestamp_start -- The starting Unix timestamp for fetched entries (default a month ago)
	TimestampStartUnix *int64 `json:"timestamp_start"`
	// timestamp_end -- The ending Unix timestamp for fetched entries (default now)
	TimestampEndUnix *int64 `json:"timestamp_end"`
	// the fields to return
	Fields []string `json:"fields"`
}

// locations serves orion compatible data
// https://github.com/LINKIWI/orion-server/blob/4b58293/orion/handlers/locations_handler.py#L13-L39
// https://github.com/LINKIWI/orion-web/blob/8c18dce19a2199c4c53e1145881d362a05018cd5/src/app/redux/middleware/location.js#L38
// https://github.com/LINKIWI/orion-server/blob/1f3a45a84f7bcfe69d04d9c0221229932aedec6f/orion/models/location.py#L18-L30
func (w *web) locations(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "bad method", http.StatusMethodNotAllowed)
		return
	}

	var req orionLocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.httpErr(rw, err)
		return
	}

	var ( // defaults
		// Orion has these, but they are pretty limiting, Remove any defaults,
		// and just use the date range.
		// offset         = 0
		// limit          = 10
		timestampStart = time.Now().Add(-30 * 24 * time.Hour)
		timestampEnd   = time.Now()
	)

	if req.TimestampStartUnix != nil {
		timestampStart = time.Unix(*req.TimestampStartUnix, 0)
	}
	if req.TimestampEndUnix != nil {
		timestampEnd = time.Unix(*req.TimestampEndUnix, 0)
	}

	q := deviceLocationQuery{
		offset:         req.Offset,
		limit:          req.Limit,
		timestampStart: &timestampStart,
		timestampEnd:   &timestampEnd,
	}

	locs, err := w.store.QueryLocations(r.Context(), q)
	if err != nil {
		w.httpErr(rw, err)
		return
	}

	ols := []orionLocation{}

	for _, l := range locs {
		ols = append(ols, orionLocation{
			Accuracy:  l.Accuracy,
			Latitude:  l.Lat,
			Longitude: l.Lng,
			Timestamp: l.Timestamp.Unix(),
		})
	}

	rw.Header().Set("Content-Type", "application/json")

	data := map[string]interface{}{
		"data": ols,
	}

	w.log.Printf("returning %d locations", len(ols))

	if err := json.NewEncoder(rw).Encode(data); err != nil {
		w.httpErr(rw, err)
		return
	}
}

// https://github.com/LINKIWI/orion-server/blob/4b58293f8ea985f13957efc9f35dfc10d0dcc9a0/orion/handlers/users_handler.py#L8-L13
func (w *web) users(rw http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"data": []interface{}{
			map[string]interface{}{
				"user":    "User",
				"devices": []string{"Device"},
			},
		},
	}

	rw.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(rw).Encode(data); err != nil {
		w.httpErr(rw, err)
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
		w.mux.HandleFunc("/api/locations", w.locations)
		w.mux.HandleFunc("/api/users", w.users)
		w.mux.HandleFunc("/connect", w.connect)
		w.mux.HandleFunc("/connect/fsqcallback", w.fsqcallback)
		w.mux.HandleFunc("/connect/tripitcallback", w.tripitCallback)
	})
}

func (w *web) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	w.init()
	w.mux.ServeHTTP(rw, r)
}

func (w *web) httpErr(rw http.ResponseWriter, err error) {
	w.log.Printf("web: %v", err)
	http.Error(rw, "Internal error", http.StatusInternalServerError)
}
