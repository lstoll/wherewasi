package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
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
		l.Fatal("todo")
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
		// exts["libspatialite.so.7"] = "spatialite_init_ex"
	} else if runtime.GOOS == "darwin" {
		// Disabling for now, throws
		// [signal SIGFPE: floating-point exception code=0x7 addr=0x6d65426 pc=0x6d65426]

		// exts["mod_spatialite"] = "sqlite3_modspatialite_init"
		_ = struct{}{}
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
