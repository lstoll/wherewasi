package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v4"
)

type fsqSyncCommand struct {
	log logger

	oauth2token string

	storage *Storage
}

func (f *fsqSyncCommand) run(ctx context.Context) error {
	f.log.Print("Starting foursquare checkout export")

	bf := func(batch []fsqCheckin) error {
		for _, ci := range batch {
			id, err := f.storage.Upsert4sqCheckin(ctx, ci)
			if err != nil {
				return err
			}
			f.log.Printf("Upserted %s", id)
		}
		return nil
	}

	var err error
	if err = f.fetchCheckins(ctx, time.Time{}, bf); err != nil {
		return err
	}

	return nil
}

func (f *fsqSyncCommand) fetchCheckins(ctx context.Context, since time.Time, batchHandler func([]fsqCheckin) error) error {
	hcl := http.DefaultClient

	// https://developer.foursquare.com/docs/api-reference/users/checkins/#parameters
	q := url.Values{}
	q.Set("v", "20190101")
	q.Set("oauth_token", f.oauth2token)
	q.Set("sort", "newestfirst")
	q.Set("limit", "50") // 250 seems to be max, but bigger than 250 gets lots of 500 errors
	if !since.IsZero() {
		q.Set("afterTimestamp", strconv.FormatInt(since.UTC().Unix(), 10))
	}

	var fetched int

	for {
		q.Set("offset", strconv.Itoa(fetched))

		f.log.Printf("Fetching items after %s from offset %d", since.String(), fetched)

		req, err := http.NewRequest("GET", fmt.Sprintf("https://api.foursquare.com/v2/users/self/checkins?%s", q.Encode()), nil)
		if err != nil {
			return err
		}

		fsResp := fsqAPICheckinsResponse{}

		apiCall := func() error {
			resp, err := hcl.Do(req)
			if err != nil {
				return err
			}

			b, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			if err := json.Unmarshal(b, &fsResp); err != nil {
				return &backoff.PermanentError{Err: err}
			}

			if fsResp.Meta.Code != 200 {
				return fmt.Errorf("unexpected code from fsq api: %d\n\n%s", fsResp.Meta.Code, string(b))
			}

			return nil
		}
		bo := backoff.NewExponentialBackOff()
		// bo.MaxElapsedTime = 15 * time.Minute
		bo.MaxElapsedTime = 0
		bo.InitialInterval = 30 * time.Second
		nf := func(err error, d time.Duration) {
			f.log.Printf("Backing off for %s, received error %v", d.String(), err)
		}
		if err := backoff.RetryNotify(apiCall, bo, nf); err != nil {
			// Handle error.
			return err
		}

		fetchCount := len(fsResp.Response.Checkins.Items)
		if fetchCount == 0 {
			if fetched != fsResp.Response.Checkins.Count {
				f.log.Printf("WARN consistency error: fetched %d of %d records", fetched, fsResp.Response.Checkins.Count)
			}
			return nil
		}

		f.log.Printf("Fetched %d out of %d total ", fetched+fetchCount, fsResp.Response.Checkins.Count)

		var batch []fsqCheckin
		for _, i := range fsResp.Response.Checkins.Items {
			c := fsqCheckin{}
			if err := json.Unmarshal(i, &c); err != nil {
				return err
			}
			c.raw = i
			batch = append(batch, c)
		}

		if err := batchHandler(batch); err != nil {
			return fmt.Errorf("calling batch handler: %v", err)
		}

		fetched = fetched + fetchCount
	}
}

type fsqAPICheckinsResponse struct {
	Meta struct {
		Code      int    `json:"code"`
		RequestID string `json:"requestId"`
	} `json:"meta"`
	Notifications []struct {
		Type string `json:"type"`
		Item struct {
			UnreadCount int `json:"unreadCount"`
		} `json:"item"`
	} `json:"notifications"`
	Response struct {
		Checkins struct {
			Count int `json:"count"`
			// Items are set into a RawMessage, so we can stick the original data in the DB too for
			// later analysis/backup
			Items []json.RawMessage `json:"items"`
		} `json:"checkins"`
	} `json:"response"`
}

// https://developer.foursquare.com/docs/api-reference/users/checkins/#response-fields
type fsqCheckin struct {
	// 	A unique identifier for this checkin.
	ID string `json:"id"`
	// Seconds since epoch when this checkin was created.
	CreatedAt int `json:"createdAt"`
	// One of checkin, shout, or venueless
	Type string `json:"type"`
	// undocumented
	Entities []fsqEntities `json:"entities"`
	// Message from check-in, if present and visible to the acting user.
	Shout string `json:"shout"`
	// The offset in minutes between when this check-in occurred and the same time in UTC. For example, a check-in that happened at -0500 UTC will have a timeZoneOffset of -300.
	TimeZoneOffset int `json:"timeZoneOffset"`
	// undocumented
	With []fsqWith `json:"with"`
	// If the venue is not clear from context, and this checkin was at a venue, then a compact venue is present.
	Venue fsqVenue `json:"venue"`
	// The count of users who have liked this checkin, and groups containing any friends and others who have liked it. The groups included are subject to change.
	Likes fsqLikes `json:"likes"`
	// undocumented
	Like bool `json:"like"`
	// undocumented
	Sticker fsqSticker `json:"sticker"`
	IsMayor bool       `json:"isMayor"`
	// count and items for photos on this checkin. All items may not be present.
	Photos fsqPhotos `json:"photos"`
	// undocumented
	Posts fsqPosts `json:"posts"`
	// If present, it indicates the checkin was marked as private and not sent to friends. It is only being returned because the owner of the checkin is viewing this data.
	Private bool `json:"private"`
	// count and items for comments on this checkin. All items may not be present.
	Comments fsqComments `json:"comments"`
	// If present, the name and url for the application that created this checkin.
	Source fsqSource `json:"source"`

	// Following not present in sample. Try and find some, and dump data

	// If the user is not clear from context, then a compact user is present.
	User interface{} `json:"user"`
	// If the type of this checkin is shout or venueless, this field may be present and may contains a lat, lng pair and/or a name, providing unstructured information about the user's current location.
	Location interface{} `json:"location"`
	// If the user checked into an event, this field will be present, containing the id and name of the event
	Event interface{} `json:"event"`
	// count and items of checkins from friends checked into the same venue around the same time.
	Overlaps interface{} `json:"overlaps"`
	// total and scores for this checkin
	Score interface{} `json:"score"`

	// Internal use. Track raw data
	raw json.RawMessage
}

type fsqEntities struct {
	Indices []int  `json:"indices"`
	Type    string `json:"type"`
	ID      string `json:"id"`
}

type fsqPhoto struct {
	Prefix string `json:"prefix"`
	Suffix string `json:"suffix"`
}

type fsqWith struct {
	ID           string   `json:"id"`
	FirstName    string   `json:"firstName"`
	LastName     string   `json:"lastName"`
	Gender       string   `json:"gender"`
	Relationship string   `json:"relationship"`
	Photo        fsqPhoto `json:"photo"`
}

type fsqContact struct {
}

type fsqLabeledLatLngs struct {
	Label string  `json:"label"`
	Lat   float64 `json:"lat"`
	Lng   float64 `json:"lng"`
}

type fsqLocation struct {
	Address          string              `json:"address"`
	Lat              float64             `json:"lat"`
	Lng              float64             `json:"lng"`
	LabeledLatLngs   []fsqLabeledLatLngs `json:"labeledLatLngs"`
	PostalCode       string              `json:"postalCode"`
	Cc               string              `json:"cc"`
	City             string              `json:"city"`
	State            string              `json:"state"`
	Country          string              `json:"country"`
	FormattedAddress []string            `json:"formattedAddress"`
}

type fsqIcon struct {
	Prefix string `json:"prefix"`
	Suffix string `json:"suffix"`
}

type fsqCategories struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	PluralName string  `json:"pluralName"`
	ShortName  string  `json:"shortName"`
	Icon       fsqIcon `json:"icon"`
	Primary    bool    `json:"primary"`
}

type fsqStats struct {
	TipCount      int `json:"tipCount"`
	UsersCount    int `json:"usersCount"`
	CheckinsCount int `json:"checkinsCount"`
	VisitsCount   int `json:"visitsCount"`
}

type fsqBeenHere struct {
	Count                int  `json:"count"`
	LastCheckinExpiredAt int  `json:"lastCheckinExpiredAt"`
	Marked               bool `json:"marked"`
	UnconfirmedCount     int  `json:"unconfirmedCount"`
}

type fsqVenue struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Contact    fsqContact      `json:"contact"`
	Location   fsqLocation     `json:"location"`
	Categories []fsqCategories `json:"categories"`
	Verified   bool            `json:"verified"`
	Stats      fsqStats        `json:"stats"`
	BeenHere   fsqBeenHere     `json:"beenHere"`
}

type fsqItems struct {
	ID           string   `json:"id"`
	FirstName    string   `json:"firstName"`
	LastName     string   `json:"lastName"`
	Gender       string   `json:"gender"`
	Relationship string   `json:"relationship"`
	Photo        fsqPhoto `json:"photo"`
}

type fsqGroups struct {
	Type  string     `json:"type"`
	Count int        `json:"count"`
	Items []fsqItems `json:"items"`
}

type fsqLikes struct {
	Count   int         `json:"count"`
	Groups  []fsqGroups `json:"groups"`
	Summary string      `json:"summary"`
}

type fsqImage struct {
	Prefix string `json:"prefix"`
	Sizes  []int  `json:"sizes"`
	Name   string `json:"name"`
}

type fsqGroup struct {
	Name  string `json:"name"`
	Index int    `json:"index"`
}

type fsqPickerPosition struct {
	Page  int `json:"page"`
	Index int `json:"index"`
}

type fsqSticker struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Image          fsqImage          `json:"image"`
	StickerType    string            `json:"stickerType"`
	Group          fsqGroup          `json:"group"`
	PickerPosition fsqPickerPosition `json:"pickerPosition"`
	TeaseText      string            `json:"teaseText"`
	UnlockText     string            `json:"unlockText"`
	BonusText      string            `json:"bonusText"`
	Points         int               `json:"points"`
	BonusStatus    string            `json:"bonusStatus"`
}

type fsqPhotos struct {
	Count int           `json:"count"`
	Items []interface{} `json:"items"`
}

type fsqPosts struct {
	Count     int `json:"count"`
	TextCount int `json:"textCount"`
}

type fsqComments struct {
	Count int `json:"count"`
}

type fsqSource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
