package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ancientlore/go-tripit"
)

const tripitDateFormat = "2006-01-02"

type tripitSyncCommand struct {
	log logger

	storage *Storage
	smgr    *secretsManager

	fetchAll bool

	oauthAPIKey    string
	oauthAPISecret string
}

func (t *tripitSyncCommand) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&t.oauthAPIKey, "tripit-api-key", "", "Oauth1 API Key for Tripit")
	fs.StringVar(&t.oauthAPISecret, "tripit-api-secret", "", "Oauth1 API Secret for Tripit")
}

func (t *tripitSyncCommand) Validate() error {
	var errs []string

	if t.storage == nil {
		errs = append(errs, "storage is required")
	}

	if t.smgr == nil {
		errs = append(errs, "secrets manager is required")
	}

	if t.oauthAPIKey == "" {
		errs = append(errs, "tripit-api-key is required")
	}

	if t.oauthAPISecret == "" {
		errs = append(errs, "tripit-api-secret is required")
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, ", "))
	}
	return nil
}

func (t *tripitSyncCommand) run(ctx context.Context) error {
	t.log.Print("Starting tripit trip export")

	if t.smgr.secrets.TripitOAuthToken == "" || t.smgr.secrets.TripitOAuthSecret == "" {
		return fmt.Errorf("secrets manager has no tripit api keys")
	}

	latestTripID, err := t.storage.LatestTripitID(ctx)
	if err != nil {
		return fmt.Errorf("finding latest tripit trip: %v", err)
	}

	cred := tripit.NewOAuth3LeggedCredential(t.oauthAPIKey, t.oauthAPISecret, t.smgr.secrets.TripitOAuthToken, t.smgr.secrets.TripitOAuthSecret)
	tcl := tripit.New(tripit.ApiUrl, tripit.ApiVersion, http.DefaultClient, cred)

	pageSize := 5 // be easy on the old API
	var fetchPage int64

	// TODO - consider if we want to fetch everything all the time, or bail when
	// we hit a known ID?
	for {
		var stopFetch bool

		t.log.Print("Fetching page from TripIt")
		// only get past trips we traveled on. Future trips are less concrete
		filters := map[string]string{
			tripit.FilterTraveler: "true",
			tripit.FilterPast:     "true",
			tripit.FilterPageSize: strconv.Itoa(pageSize),
		}
		if fetchPage > 0 {
			filters[tripit.FilterPageNum] = strconv.Itoa(int(fetchPage))
		}
		resp, err := tcl.List(tripit.ObjectTypeTrip, filters)
		if err != nil {
			return fmt.Errorf("getting trips: %v", err)
		}
		if resp.Error != nil && len(resp.Error) > 0 {
			return fmt.Errorf("error from tripit api: %v", resp.Error)
		}
		if resp.Warning != nil && len(resp.Warning) > 0 {
			return fmt.Errorf("warning from tripit api: %v", resp.Warning)
		}

		maxPages, err := resp.MaxPage.Int64()
		if err != nil {
			return fmt.Errorf("format error for max_page %s: %v", resp.MaxPage, err)
		}

		apiPage, err := resp.PageNumber.Int64()
		if err != nil {
			return fmt.Errorf("format error for page_num %s: %v", resp.PageNumber, err)
		}

		t.log.Printf("Fetched page %d of %d", apiPage, maxPages)

		for _, tr := range resp.Trip {
			// re-marshal to get raw, we don't have access to underlying data as
			// easy
			trJSON, err := json.Marshal(tr)
			if err != nil {
				return fmt.Errorf("marshaling trip: %v", err)
			}

			if !t.fetchAll && latestTripID != "" && tr.Id == latestTripID {
				// we're not fetching all, and we've hit an ID we already know.
				// Stop fetching
				stopFetch = true
			}

			// TODO - at some point do we want to deal with invitees? Would be
			// useful, but need some thought around how we link them properly to the
			// contacts table/fix bad matches
			if err := t.storage.UpsertTripitTrip(ctx, tr, trJSON); err != nil {
				return fmt.Errorf("upserting %s: %v", tr.Id, err)
			}
		}

		fetchPage = apiPage + 1
		if int64(fetchPage) > maxPages || stopFetch {
			t.log.Print("Tripit sync complete")
			// we're done
			break
		}
	}

	return nil
}
