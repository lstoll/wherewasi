package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/mattn/go-sqlite3"
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

		var (
			listen string
		)

		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		base.AddFlags(fs)
		fs.StringVar(&listen, "listen", getEnvDefault("LISTEN", "localhost:8080"), "Address to listen on")
		fs.StringVar(&ots.username, "ot-username", getEnvDefault("OT_PUBLISH_USERNAME", ""), "Username for the owntracks publish endpoint (required)")
		fs.StringVar(&ots.password, "ot-password", getEnvDefault("OT_PUBLISH_PASSWORD", ""), "Password for the owntracks publish endpoint (required)")

		if err := fs.Parse(os.Args[parseIdx:]); err != nil {
			l.Fatal(err.Error())
		}
		base.Parse(ctx, l)

		var errs []string

		if ots.username == "" {
			errs = append(errs, "ot-username required")
		}

		if ots.password == "" {
			errs = append(errs, "ot-password required")
		}

		if len(errs) > 0 {
			fmt.Printf("%s\n", strings.Join(errs, ", "))
			fs.Usage()
			os.Exit(1)
		}

		ots.store = base.storage

		mux := http.NewServeMux()

		mux.HandleFunc("/pub", ots.HandlePublish)

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
		fs.StringVar(&cmd.oauth2token, "api-key", getEnvDefault("FOURSQUARE_API_KEY", ""), "Token to authenticate to foursquare API with. https://your-foursquare-oauth-token.glitch.me")

		if err := fs.Parse(os.Args[parseIdx:]); err != nil {
			l.Fatal(err.Error())
		}
		base.Parse(ctx, l)

		var errs []string

		if cmd.oauth2token == "" {
			errs = append(errs, "api-key required")
		}

		if len(errs) > 0 {
			fmt.Printf("%s\n", strings.Join(errs, ", "))
			fs.Usage()
			os.Exit(1)
		}

		cmd.storage = base.storage

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
