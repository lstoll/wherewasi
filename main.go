package main

import (
	"flag"
	"log"
	"os"

	"github.com/sirupsen/logrus"
)

func main() {
	l := logrus.New()

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
		cmd := fsqImportCommand{}

		fs := flag.NewFlagSet("4sqimport", flag.ExitOnError)
		fs.StringVar(&cmd.oauth2token, "api-key", getEnvDefault("FOURSQUARE_API_KEY", ""), "Token to authenticate to foursquare API with. https://your-foursquare-oauth-token.glitch.me")
		if err := fs.Parse(os.Args[parseIdx:]); err != nil {
			l.WithError(err).Fatal()
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
