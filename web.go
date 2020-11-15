package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ancientlore/go-tripit"
	geojson "github.com/paulmach/go.geojson"
	"golang.org/x/oauth2"
)

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
}

type indexData struct {
	DeviceLocations    template.JS
	DeviceLocationLine template.JS

	From string
	To   string

	Accuracy int

	Line bool
}

func (w *web) index(rw http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(rw, "Not Found", http.StatusNotFound)
		return
	}

	var (
		from     = time.Now().Add(-7 * 24 * time.Hour)
		to       = time.Now()
		accuracy = 100
		drawLine = false
	)

	if r.URL.Query().Get("from") != "" {
		f, err := time.Parse("2006-01-02", r.URL.Query().Get("from"))
		if err != nil {
			w.log.Printf("parsing from %v", err)
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		from = f
	}

	if r.URL.Query().Get("to") != "" {
		t, err := time.Parse("2006-01-02", r.URL.Query().Get("to"))
		if err != nil {
			w.log.Printf("parsing to %v", err)
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		to = t
	}

	if r.URL.Query().Get("acc") != "" {
		a, err := strconv.Atoi(r.URL.Query().Get("acc"))
		if err != nil {
			w.log.Printf("parsing acc %v", err)
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		accuracy = a
	}

	if r.URL.Query().Get("line") == "on" {
		drawLine = true
	}

	// make it to the end of the "to" day
	// TODO timezone awareness? Or move to EU where it's all closer to UTC
	// anyway
	rl, err := w.store.RecentLocations(r.Context(), from, to.Add(24*time.Hour-1*time.Second))
	if err != nil {
		w.log.Printf("getting recent locations: %v", err)
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	features := geojson.NewFeatureCollection()
	linePoints := [][]float64{}

	for _, l := range rl {
		if l.Accuracy <= accuracy {
			vel := 0
			if l.Velocity != nil {
				vel = *l.Velocity
			}
			features.AddFeature(&geojson.Feature{
				Geometry: geojson.NewPointGeometry([]float64{l.Lng, l.Lat}),
				Properties: map[string]interface{}{
					"accuracy":     l.Accuracy,
					"popupContent": fmt.Sprintf("At: %s<br>Velocity: %d km/h", l.Timestamp.String(), vel),
				},
			})
			if drawLine {
				linePoints = append(linePoints, []float64{l.Lng, l.Lat})
			}
		}
	}

	line := geojson.NewMultiLineStringFeature(linePoints)

	geoJSON, err := json.Marshal(features)
	if err != nil {
		w.log.Printf("marshaling geoJSON: %v", err)
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	lineJSON, err := json.Marshal(line)
	if err != nil {
		w.log.Printf("marshaling lineJSON: %v", err)
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	tmpData := indexData{
		DeviceLocations:    template.JS(geoJSON),
		DeviceLocationLine: template.JS(lineJSON),

		From: from.Format("2006-01-02"),
		To:   to.Format("2006-01-02"),

		Accuracy: accuracy,

		Line: drawLine,
	}

	t, err := template.ParseFiles("index.tmpl.html")
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := t.Execute(rw, tmpData); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
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

		// w.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

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
