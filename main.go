package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/sessions"
	"github.com/oklog/run"
	oidcm "github.com/pardot/oidc/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/oauth2"
)

const (
	mainDBFile  = "wherewasi.db"
	secretsFile = "secrets.json"
)

func main() {
	ctx := context.Background()
	l := log.New(os.Stderr, "", log.LstdFlags)

	command := "serve"
	parseIdx := 1
	if len(os.Args) > 1 {
		command = os.Args[1]
		parseIdx = 2
	}

	base := &baseCommand{}

	switch command {
	case "serve":
		ots := &owntracksServer{
			log: l,
		}

		fssync := &fsqSyncCommand{
			log: l,
		}

		ws := &web{
			log: l,
		}

		ah := &oidcm.Handler{}

		tpsync := &tripitSyncCommand{
			log: l,
		}

		var (
			listen            string
			promListen        string
			baseURL           string
			disableAuth       bool
			secureKeyFlag     string
			basicAuth         bool
			otListen          string
			otUsername        string
			otPassword        string
			requireSubject    string
			disable4sqSync    bool
			disableTripitSync bool
			fsqSyncInterval   time.Duration
			tpSyncInterval    time.Duration
		)

		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		base.AddFlags(fs)
		fs.StringVar(&listen, "listen", getEnvDefault("LISTEN", "localhost:8080"), "Address to listen on")
		fs.StringVar(&promListen, "metrics-listen", getEnvDefault("METRICS_LISTEN", ""), "Address to serve metrics on (prom at /metrics), if set")
		fs.StringVar(&secureKeyFlag, "secure-key", "", "Key used to encrypt/verify information like cookies")
		fs.StringVar(&baseURL, "base-url", getEnvDefault("BASE_URL", "http://localhost:8080"), "Base URL this service runs on")

		fs.StringVar(&otListen, "ot-listen", getEnvDefault("OT_LISTEN", ""), "Optional address to listen on for the owntracks publish endpoint.")
		fs.StringVar(&otUsername, "ot-username", getEnvDefault("OT_PUBLISH_USERNAME", ""), "Username for the owntracks publish endpoint (required)")
		fs.StringVar(&otPassword, "ot-password", "", "Password for the owntracks publish endpoint (required)")

		fs.StringVar(&ah.Issuer, "auth-issuer", getEnvDefault("AUTH_ISSUER", ""), "OIDC Issuer (required unless auth disabled)")
		fs.StringVar(&ah.ClientID, "auth-client-id", getEnvDefault("AUTH_CLIENT_ID", ""), "OIDC Client ID (required unless auth disabled)")
		fs.StringVar(&ah.ClientSecret, "auth-client-secret", "", "OIDC Client Secret (required unless auth disabled)")
		fs.StringVar(&ah.RedirectURL, "auth-redirect-url", getEnvDefault("AUTH_REDIRECT_URL", ""), "OIDC Redirect URL (required unless auth disabled)")
		fs.StringVar(&requireSubject, "auth-require-subject", getEnvDefault("AUTH_REQUIRE_SUBJECT", ""), "If set, require this subject to grant access")
		fs.BoolVar(&basicAuth, "i-am-basic", false, "If enabled, basic auth will be used for the web UI using the owntracks endpoint creds")

		fs.BoolVar(&disableAuth, "auth-disabled", false, "Disable auth altogether")

		fs.BoolVar(&disable4sqSync, "4sq-sync-disabled", false, "Disable background foursquare sync")
		fs.DurationVar(&fsqSyncInterval, "4sq-sync-interval", 1*time.Hour, "How often we should sync foursquare in the background")
		fssync.AddFlags(fs)
		// https://foursquare.com/developers/apps
		// redirect to https://<host>/connect/fsqcallback
		fs.StringVar(&ws.fsqOauthConfig.ClientID, "fsq-client-id", getEnvDefault("FSQ_CLIENT_ID", ""), "Oauth2 Client ID")
		fs.StringVar(&ws.fsqOauthConfig.ClientSecret, "fsq-client-secret", "", "Oauth2 Client Secret")

		// https://www.tripit.com/developer
		fs.BoolVar(&disableTripitSync, "tripit-sync-disabled", false, "Disable background tripit sync")
		fs.DurationVar(&tpSyncInterval, "tripit-sync-interval", 6*time.Hour, "How often we should sync tripit in the background")
		tpsync.AddFlags(fs)

		if err := fs.Parse(os.Args[parseIdx:]); err != nil {
			l.Fatal(err.Error())
		}
		base.Parse(ctx, l)

		ws.smgr = base.smgr
		ws.store = base.storage

		if v := os.Getenv("SECURE_KEY"); v != "" && secureKeyFlag == "" {
			secureKeyFlag = v
		}
		if v := os.Getenv("OT_PUBLISH_PASSWORD"); v != "" && otPassword == "" {
			otPassword = v
		}
		if v := os.Getenv("AUTH_CLIENT_SECRET"); v != "" && ah.ClientSecret == "" {
			ah.ClientSecret = v
		}
		if v := os.Getenv("FSQ_CLIENT_SECRET"); v != "" && ws.fsqOauthConfig.ClientSecret == "" {
			ws.fsqOauthConfig.ClientSecret = v
		}

		if v, ok := os.LookupEnv("CREDENTIALS_DIRECTORY"); ok {
			l.Printf("loading credentials from files in directory %s", v)

			if s, err := os.ReadFile(filepath.Join(v, "secure-key")); err == nil {
				secureKeyFlag = strings.TrimSpace(string(s))
			}
			if s, err := os.ReadFile(filepath.Join(v, "ot-publish-password")); err == nil {
				otPassword = strings.TrimSpace(string(s))
			}
			if s, err := os.ReadFile(filepath.Join(v, "auth-client-secret")); err == nil {
				ah.ClientSecret = strings.TrimSpace(string(s))
			}
			if s, err := os.ReadFile(filepath.Join(v, "fsq-client-id")); err == nil {
				ws.fsqOauthConfig.ClientID = strings.TrimSpace(string(s))
			}
			if s, err := os.ReadFile(filepath.Join(v, "fsq-client-secret")); err == nil {
				ws.fsqOauthConfig.ClientSecret = strings.TrimSpace(string(s))
			}
			if s, err := os.ReadFile(filepath.Join(v, "tripit-api-key")); err == nil {
				ws.tripitAPIKey = strings.TrimSpace(string(s))
				tpsync.oauthAPIKey = strings.TrimSpace(string(s))
			}
			if s, err := os.ReadFile(filepath.Join(v, "tripit-api-secret")); err == nil {
				ws.tripitAPISecret = strings.TrimSpace(string(s))
				tpsync.oauthAPISecret = strings.TrimSpace(string(s))
			}
		}

		var errs []string

		if secureKeyFlag == "" {
			errs = append(errs, "secure-key required")
		}

		if otUsername == "" {
			errs = append(errs, "ot-username required")
		}

		if otPassword == "" {
			errs = append(errs, "ot-password required")
		}

		if !disableAuth && !basicAuth {
			if ah.Issuer == "" {
				errs = append(errs, "auth-issuer required")
			}
			if ah.Issuer == "" {
				errs = append(errs, "auth-client-id required")
			}
			if ah.ClientSecret == "" {
				errs = append(errs, "auth-client-secret required")
			}
			if ah.Issuer == "" {
				errs = append(errs, "auth-base-url required")
			}
			if ah.Issuer == "" {
				errs = append(errs, "auth-redirect-url required")
			}
		}

		if len(errs) > 0 {
			fmt.Printf("%s\n", strings.Join(errs, ", "))
			fs.Usage()
			os.Exit(1)
		}

		ws.fsqOauthConfig.RedirectURL = baseURL + "/connect/fsqcallback"
		ws.fsqOauthConfig.Endpoint = oauth2.Endpoint{
			AuthURL:  "https://foursquare.com/oauth2/authenticate",
			TokenURL: "https://foursquare.com/oauth2/access_token",
		}

		krdr := hkdf.New(sha256.New, []byte(secureKeyFlag), nil, nil)
		scHashKey := make([]byte, 64)
		scEncryptKey := make([]byte, 32)
		if _, err := io.ReadFull(krdr, scHashKey); err != nil {
			log.Fatal(err)
		}
		if _, err := io.ReadFull(krdr, scEncryptKey); err != nil {
			log.Fatal(err)
		}
		ah.SessionStore = sessions.NewCookieStore(scHashKey, scEncryptKey)

		// Copy fields around as needed
		ah.BaseURL = baseURL
		ws.baseURL = baseURL
		ws.tripitAPIKey = tpsync.oauthAPIKey
		ws.tripitAPISecret = tpsync.oauthAPISecret

		ots.store = base.storage

		mux := http.NewServeMux()

		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			if _, err := base.storage.db.ExecContext(r.Context(), `SELECT 1`); err != nil {
				slog.ErrorContext(r.Context(), "error communicating with db in healthz", "err", err)
				http.Error(w, "Internal Error", http.StatusInternalServerError)
				return
			}
			_, _ = fmt.Fprint(w, "OK")
		})

		if disableAuth {
			mux.Handle("/", ws)
		} else if basicAuth {
			mux.Handle("/", wrapBasicAuth(otUsername, otPassword, ws))
		} else {
			mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ah.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					cl := oidcm.ClaimsFromContext(r.Context())
					if requireSubject != "" && (cl == nil || cl.Subject != requireSubject) {
						var emsg string
						if cl != nil {
							emsg = fmt.Sprintf("Subject %s not permitted", cl.Subject)
						} else {
							emsg = "Subject claim not found"
						}
						http.Error(w, emsg, http.StatusForbidden)
						return
					}
					ws.ServeHTTP(w, r)
				})).ServeHTTP(w, r)
			}))
		}

		var g run.Group

		if otListen == "" {
			mux.Handle("/pub", wrapBasicAuth(otUsername, otPassword, http.HandlerFunc(ots.HandlePublish)))
		} else {
			otmux := http.NewServeMux()
			otmux.Handle("/pub", wrapBasicAuth(otUsername, otPassword, http.HandlerFunc(ots.HandlePublish)))
			srv := http.Server{Addr: otListen, Handler: otmux}

			g.Add(func() error {
				l.Printf("owntracks publisher listing on %s", listen)
				return srv.ListenAndServe()
			}, func(error) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := srv.Shutdown(ctx); err != nil {
					l.Printf("shutting down owntracks publisher: %v", err)
				}
			})
		}

		if !disable4sqSync {
			if ws.fsqOauthConfig.ClientID == "" || ws.fsqOauthConfig.ClientSecret == "" {
				l.Fatal("foursquare oauth2 config not set")
			}

			fssync.storage = base.storage
			fssync.smgr = base.smgr

			if err := fssync.Validate(); err != nil {
				l.Fatalf("validating foursquare sync command: %v", err)
			}

			fsqSyncDone := make(chan struct{}, 1)
			g.Add(func() error {
				for {
					if base.smgr.secrets.FourquareAPIKey == "" {
						metric4sqSyncErrorCount.Inc()
						l.Printf("no foursquare API key saved, not running")
					} else {
						l.Print("Running foursquare sync")
						if err := fssync.run(ctx); err != nil {
							metric4sqSyncErrorCount.Inc()
							l.Printf("error running foursquare sync: %v", err)
						} else {
							metric4sqSyncSuccessCount.Inc()
						}
					}

					select {
					case <-fsqSyncDone:
						return nil
					case <-time.After(fsqSyncInterval):
						continue
					}
				}
			}, func(error) {
				fsqSyncDone <- struct{}{}
				log.Print("returning fsq shutdown")
			})
		}

		if !disableTripitSync {
			if v := os.Getenv("TRIPIT_API_KEY"); v != "" && ws.tripitAPIKey == "" {
				ws.tripitAPIKey = v
			}
			if v := os.Getenv("TRIPIT_API_SECRET"); v != "" && ws.tripitAPISecret == "" {
				ws.tripitAPISecret = v
			}

			if ws.tripitAPIKey == "" || ws.tripitAPISecret == "" {
				l.Fatal("tripit oauth1 config not set on ws")
			}

			tpsync.storage = base.storage
			tpsync.smgr = base.smgr

			if err := tpsync.Validate(); err != nil {
				l.Fatalf("validating tripit sync command: %v", err)
			}

			tripitSyncDone := make(chan struct{}, 1)
			g.Add(func() error {
				for {
					if tpsync.smgr.secrets.TripitOAuthToken == "" || tpsync.smgr.secrets.TripitOAuthSecret == "" {
						metricTripitSyncErrorCount.Inc()
						l.Print("No tripit API keys saved, not running")
					} else {
						l.Print("Running tripit sync")
						if err := tpsync.run(ctx); err != nil {
							metricTripitSyncErrorCount.Inc()
							l.Printf("error running tripit sync: %v", err)
						} else {
							metricTripitSyncSuccessCount.Inc()
						}
					}

					select {
					case <-tripitSyncDone:
						return nil
					case <-time.After(tpSyncInterval):
						continue
					}
				}
			}, func(error) {
				tripitSyncDone <- struct{}{}
				log.Print("returning tripit shutdown")
			})

		}

		mainSrv := &http.Server{
			Addr:    listen,
			Handler: mux,
		}

		g.Add(func() error {
			l.Printf("Listing on %s", listen)
			if err := mainSrv.ListenAndServe(); err != nil {
				return fmt.Errorf("serving http: %v", err)
			}
			return nil
		}, func(error) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := mainSrv.Shutdown(ctx); err != nil {
				l.Printf("shutting down main http server: %v", err)
			}
			log.Print("returning http shutdown")
		})

		if promListen != "" {
			ph := promhttp.InstrumentMetricHandler(
				prometheus.DefaultRegisterer, promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{
					ErrorLog: l,
				}),
			)

			pm := http.NewServeMux()
			pm.Handle("/metrics", ph)

			metricsSrv := &http.Server{
				Addr:    promListen,
				Handler: pm,
			}

			prometheus.MustRegister(newMetricsCollector(l, base.storage))

			g.Add(func() error {
				l.Printf("Listing for metrics on %s", promListen)
				if err := metricsSrv.ListenAndServe(); err != nil {
					return fmt.Errorf("serving metrics: %v", err)
				}
				return nil
			}, func(error) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := metricsSrv.Shutdown(ctx); err != nil {
					log.Printf("shutting down metrics server: %v", err)
				}
				log.Print("returning metrics shutdown")
			})
		}

		g.Add(run.SignalHandler(ctx, os.Interrupt))

		if err := g.Run(); err != nil {
			var se run.SignalError
			if errors.As(err, &se) {
				log.Printf("Received signal %s, terminating", se.Signal.String())
				os.Exit(0)
			}
			l.Fatalf("group error: %v", err)
		}
	case "4sqsync":
		cmd := fsqSyncCommand{
			log: l,
		}

		fs := flag.NewFlagSet("4sqsync", flag.ExitOnError)
		base.AddFlags(fs)
		cmd.AddFlags(fs)

		if err := fs.Parse(os.Args[parseIdx:]); err != nil {
			l.Fatal(err.Error())
		}
		base.Parse(ctx, l)

		cmd.storage = base.storage
		cmd.smgr = base.smgr

		if err := cmd.Validate(); err != nil {
			l.Printf("validation error: %v", err)
			fs.Usage()
			os.Exit(2)
		}

		if cmd.smgr.secrets.FourquareAPIKey == "" {
			l.Fatalf("secrets does not have foursquare creds. http://<server>/connect")
		}

		if err := cmd.run(ctx); err != nil {
			l.Fatal(err.Error())
		}
	case "tripitsync":
		cmd := tripitSyncCommand{
			log: l,
		}

		fs := flag.NewFlagSet("tripitsync", flag.ExitOnError)
		base.AddFlags(fs)
		cmd.AddFlags(fs)
		fs.BoolVar(&cmd.fetchAll, "fetch-all", false, "fetch all trips, not just ones not already fetched")

		if err := fs.Parse(os.Args[parseIdx:]); err != nil {
			l.Fatal(err.Error())
		}
		base.Parse(ctx, l)

		cmd.storage = base.storage
		cmd.smgr = base.smgr

		if err := cmd.Validate(); err != nil {
			l.Printf("validation error: %v", err)
			fs.Usage()
			os.Exit(2)
		}
		if cmd.smgr.secrets.TripitOAuthToken == "" || cmd.smgr.secrets.TripitOAuthSecret == "" {
			l.Fatalf("secrets does not have tripit creds. http://<server>/connect")
		}

		if err := cmd.run(ctx); err != nil {
			l.Fatal(err.Error())
		}
	case "takeoutimport":
		cmd := takeoutimportCommand{
			log: l,
		}

		fs := flag.NewFlagSet("takeoutimport", flag.ExitOnError)
		base.AddFlags(fs)
		fs.StringVar(&cmd.filePath, "path", "", "Path to google takeout locatiom history file (required)")

		if err := fs.Parse(os.Args[parseIdx:]); err != nil {
			l.Fatal(err.Error())
		}
		base.Parse(ctx, l)

		var errs []string

		if cmd.filePath == "" {
			errs = append(errs, "path required")
		}

		if len(errs) > 0 {
			fmt.Printf("%s\n", strings.Join(errs, ", "))
			fs.Usage()
			os.Exit(1)
		}

		cmd.store = base.storage

		if err := cmd.run(ctx); err != nil {
			l.Fatal(err.Error())
		}
	default:
		log.Fatal("invalid command")
	}
}

func getEnvDefault(envar, defaultval string) string {
	ret := os.Getenv(envar)
	if ret == "" {
		ret = defaultval
	}
	return ret
}

type logger interface {
	Fatal(v ...interface{})
	Fatalf(format string, v ...interface{})
	Print(v ...interface{})
	Printf(format string, v ...interface{})
}

type baseCommand struct {
	storage *Storage
	smgr    *secretsManager

	dbPath     string
	disableWal bool

	fs *flag.FlagSet
}

func (b *baseCommand) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&b.dbPath, "db-path", "db", "directory for data storage")
	fs.BoolVar(&b.disableWal, "disable-wal", false, "disable WAL mode for sqlite")
	b.fs = fs
}

// Parse is called after the flags are parsed, to set things up
func (b *baseCommand) Parse(ctx context.Context, logger logger) {
	var errs []string

	if b.dbPath == "" {
		errs = append(errs, "db-path required")
	}

	if len(errs) > 0 {
		fmt.Printf("%s\n", strings.Join(errs, ", "))
		b.fs.Usage()
		os.Exit(1)
	}

	connStr := buildConnStr(filepath.Join(b.dbPath, mainDBFile), b.disableWal)

	st, err := newStorage(ctx, logger, connStr)
	if err != nil {
		logger.Fatalf("creating storage: %v", err)
	}
	b.storage = st

	b.smgr = &secretsManager{
		path: filepath.Join(b.dbPath, secretsFile),
	}
	if err := b.smgr.Load(); err != nil {
		logger.Fatalf("creating secrets manager: %v", err)
	}
}

func wrapBasicAuth(username, password string, wrap http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(username)) != 1 || subtle.ConstantTimeCompare([]byte(p), []byte(password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="wherewasi"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		wrap.ServeHTTP(w, r)
	})
}

type secrets struct {
	FourquareAPIKey   string `json:"foursquare_api_key,omitempty"`
	TripitOAuthToken  string `json:"tripit_oauth_token,omitempty"`
	TripitOAuthSecret string `json:"tripit_oauth_secret,omitempty"`
}

type secretsManager struct {
	path string

	secrets secrets
}

func (s *secretsManager) Load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			// no data yet, just return an empty set
			return nil
		}
		return fmt.Errorf("reading %s: %v", s.path, err)
	}
	if err := json.Unmarshal(b, &s.secrets); err != nil {
		return fmt.Errorf("unmarshaling %s: %v", s.path, err)
	}
	return nil
}

func (s *secretsManager) Save() error {
	b, err := json.Marshal(s.secrets)
	if err != nil {
		return fmt.Errorf("marshaling secrets: %s", err)
	}
	if err := os.WriteFile(s.path, b, 0o600); err != nil {
		return fmt.Errorf("writing secrets to %s: %v", s.path, err)
	}
	return nil
}
