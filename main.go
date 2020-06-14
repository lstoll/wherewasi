package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
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

	switch command {
	case "serve":
		l.Fatal("todo")
	case "4sqimport":
		cmd := fsqImportCommand{
			log: l,
		}

		fs := flag.NewFlagSet("4sqimport", flag.ExitOnError)
		fs.StringVar(&cmd.oauth2token, "api-key", getEnvDefault("FOURSQUARE_API_KEY", ""), "Token to authenticate to foursquare API with. https://your-foursquare-oauth-token.glitch.me")
		if err := fs.Parse(os.Args[parseIdx:]); err != nil {
			l.Fatal(err.Error())
		}

		var errs []string

		if cmd.oauth2token == "" {
			errs = append(errs, "api-key required")
		}

		if len(errs) > 0 {
			fmt.Printf("%s\n", strings.Join(errs, ", "))
			fs.Usage()
			os.Exit(1)
		}

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
