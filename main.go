package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/mattn/go-sqlite3"
	oidcm "github.com/pardot/oidc/middleware"
	"golang.org/x/crypto/hkdf"
)

func init() {
	registerSpatiaLite()
}

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

		ws := &web{}

		ah := &oidcm.Handler{}

		var (
			listen          string
			disableAuth     bool
			secureKeyFlag   string
			basicAuth       bool
			otUsername      string
			otPassword      string
			requireSubject  string
			disable4sqSync  bool
			fsqSyncInterval time.Duration
		)

		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		base.AddFlags(fs)
		fs.StringVar(&listen, "listen", getEnvDefault("LISTEN", "localhost:8080"), "Address to listen on")
		fs.StringVar(&secureKeyFlag, "secure-key", getEnvDefault("SECURE_KEY", ""), "Key used to encrypt/verify information like cookies")

		fs.StringVar(&otUsername, "ot-username", getEnvDefault("OT_PUBLISH_USERNAME", ""), "Username for the owntracks publish endpoint (required)")
		fs.StringVar(&otPassword, "ot-password", getEnvDefault("OT_PUBLISH_PASSWORD", ""), "Password for the owntracks publish endpoint (required)")

		fs.StringVar(&ah.Issuer, "auth-issuer", getEnvDefault("AUTH_ISSUER", ""), "OIDC Issuer (required unless auth disabled)")
		fs.StringVar(&ah.ClientID, "auth-client-id", getEnvDefault("AUTH_CLIENT_ID", ""), "OIDC Client ID (required unless auth disabled)")
		fs.StringVar(&ah.ClientSecret, "auth-client-secret", getEnvDefault("AUTH_CLIENT_SECRET", ""), "OIDC Client Secret (required unless auth disabled)")
		fs.StringVar(&ah.BaseURL, "auth-base-url", getEnvDefault("AUTH_BASE_URL", ""), "Base URL this service runs on (required unless auth disabled)")
		fs.StringVar(&ah.RedirectURL, "auth-redirect-url", getEnvDefault("AUTH_REDIRECT_URL", ""), "OIDC Redirect URL (required unless auth disabled)")
		fs.StringVar(&requireSubject, "auth-require-subject", getEnvDefault("AUTH_REQUIRE_SUBJECT", ""), "If set, require this subject to grant access")
		fs.BoolVar(&basicAuth, "i-am-basic", false, "If enabled, basic auth will be used for the web UI using the owntracks endpoint creds")

		fs.BoolVar(&disableAuth, "auth-disabled", false, "Disable auth altogether")

		fs.BoolVar(&disable4sqSync, "4sq-sync-disabled", false, "Disable background foursquare sync")
		fs.DurationVar(&fsqSyncInterval, "4sq-sync-interval", 1*time.Hour, "How often we should sync foursquare in the background")
		fssync.AddFlags(fs)

		if err := fs.Parse(os.Args[parseIdx:]); err != nil {
			l.Fatal(err.Error())
		}
		base.Parse(ctx, l)

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
			if ah.Issuer == "" {
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

		krdr := hkdf.New(sha256.New, []byte(secureKeyFlag), nil, nil)
		scHashKey := make([]byte, 64)
		scEncryptKey := make([]byte, 32)
		if _, err := io.ReadFull(krdr, scHashKey); err != nil {
			log.Fatal(err)
		}
		if _, err := io.ReadFull(krdr, scEncryptKey); err != nil {
			log.Fatal(err)
		}
		ah.SessionAuthenticationKey = scHashKey
		ah.SessionEncryptionKey = scEncryptKey

		ots.store = base.storage

		mux := http.NewServeMux()

		mux.Handle("/pub", wrapBasicAuth(otUsername, otPassword, http.HandlerFunc(ots.HandlePublish)))
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

		if !disable4sqSync {
			go func() {
				fssync.storage = base.storage

				sync := func() {
					l.Print("Running foursquare sync")
					if err := fssync.run(ctx); err != nil {
						// for now, bombing out is an easy way to get attention
						l.Fatalf("error running foursquare sync: %v", err)
					}
				}
				sync()
				ticker := time.NewTicker(1 * time.Hour)
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						sync()
					}
				}
			}()
		}

		l.Printf("Listing on %s", listen)
		if err := http.ListenAndServe(listen, mux); err != nil {
			l.Fatalf("Error serving: %v", err)
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

		if err := cmd.Validate(); err != nil {
			l.Printf("validation error: %v", err)
			fs.Usage()
			os.Exit(2)
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

	dbPath string

	fs *flag.FlagSet
}

func (b *baseCommand) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&b.dbPath, "db", "db/wherewasi.db", "path to database")
	b.fs = fs
}

// Parse is called after the flags are parsed, to set things up
func (b *baseCommand) Parse(ctx context.Context, logger logger) {
	var errs []string

	if b.dbPath == "" {
		errs = append(errs, "db required")
	}

	if len(errs) > 0 {
		fmt.Printf("%s\n", strings.Join(errs, ", "))
		b.fs.Usage()
		os.Exit(1)
	}

	st, err := newStorage(ctx, logger, fmt.Sprintf("file:%s?cache=shared&_foreign_keys=on", b.dbPath))
	if err != nil {
		logger.Fatalf("creating storage: %v", err)
	}
	b.storage = st
}

func registerSpatiaLite() {
	exts := map[string]string{}

	if runtime.GOOS == "linux" {
		exts["libspatialite.so.7"] = "spatialite_init_ex"
	} else if runtime.GOOS == "darwin" {
		exts["mod_spatialite"] = "sqlite3_modspatialite_init"
	}

	sql.Register("spatialite", &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			if len(exts) > 0 {
				for l, e := range exts {
					if err := conn.LoadExtension(l, e); err == nil {
						return nil
					}
				}
				return fmt.Errorf("loading spatialite failed. make sure libraries are installed")
			}
			return nil
		},
	})
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
