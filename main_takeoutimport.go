package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
)

type takeoutLocationStorage interface {
	AddGoogleTakeoutLocations(ctx context.Context, locs []takeoutLocation) error
}

var _ takeoutLocationStorage = (*Storage)(nil)

type takeoutLocation struct {
	// assuming metres
	Accuracy int                       `json:"accuracy"`
	Activity []takeoutLocationActivity `json:"activity"`
	// Seems like metres
	Altitude *int `json:"altitude"`
	// degrees
	Heading     *int   `json:"heading"`
	LatitudeE7  int    `json:"latitudeE7"`
	LongitudeE7 int    `json:"longitudeE7"`
	TimestampMS string `json:"timestampMs"`
	// Metres per second https://gis.stackexchange.com/a/294505
	Velocity *int `json:"velocity"`
	// assuming metres
	VerticalAccuracy *int `json:"verticalAccuracy"`

	Raw json.RawMessage `json:"-"`
}

type takeoutLocationActivity struct {
	TimestampMs string            `json:"timestampMs"`
	Activities  []takeoutActivity `json:"activity"`
}

type takeoutActivity struct {
	Type       string `json:"type"`
	Confidence int    `json:"confidence"`
}

type takeoutFile struct {
	// use a raw message, so we can parse each one individually and also capture
	// their raw value
	Locations []json.RawMessage `json:"locations"`
}

type takeoutimportCommand struct {
	log logger

	filePath string

	store takeoutLocationStorage
}

func (t *takeoutimportCommand) run(ctx context.Context) error {
	t.log.Printf("Importing google takeout location history file %s", t.filePath)

	tlocs, err := parseLocationFile(t.filePath)
	if err != nil {
		return err
	}
	_ = tlocs

	if err := t.store.AddGoogleTakeoutLocations(ctx, tlocs); err != nil {
		return fmt.Errorf("importing takeout locations: %v", err)
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	t.log.Printf("Done, Alloc %dMiB TotalAlloc %dMiB", m.Alloc/1024/1024, m.TotalAlloc/1024/1024)

	return nil
}

func parseLocationFile(path string) ([]takeoutLocation, error) {
	// this is pretty inefficent, but it's intended to be a one-time application
	// so w/e

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %v", path, err)
	}
	defer f.Close()

	tf := takeoutFile{}

	if err := json.NewDecoder(f).Decode(&tf); err != nil {
		return nil, fmt.Errorf("decoding takeout file %s: %v", path, err)
	}

	var ret []takeoutLocation

	for _, rl := range tf.Locations {
		tl := takeoutLocation{}
		if err := json.Unmarshal(rl, &tl); err != nil {
			return nil, fmt.Errorf("unmarshaling location %v", err)
		}
		tl.Raw = rl
		ret = append(ret, tl)
	}

	return ret, nil
}

func e7ToNormal(e7 int) float64 {
	return float64(e7) / float64(1e7)
}

func msToKmh(ms int) int {
	return int(float64(ms) * 3.6)
}
