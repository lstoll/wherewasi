package main

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

func TestAddOTLocation(t *testing.T) {
	ctx, s := setupDB(t)

	om := owntracksMessage{}
	if err := json.Unmarshal([]byte(egOwntracksLocation), &om); err != nil {
		t.Fatal(err)
	}

	if err := s.AddOTLocation(ctx, om); err != nil {
		t.Fatal(err)
	}

	var count int

	if err := s.db.QueryRowContext(ctx, `select count(*) from device_locations`).Scan(&count); err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Errorf("want one row, got: %d", count)
	}
}

func TestAddTakeoutLocation(t *testing.T) {
	ctx, s := setupDB(t)

	locs := []takeoutLocation{
		{LatitudeE7: 10 * 1e7, LongitudeE7: -10 * 10e7, TimestampMS: strconv.Itoa(int(time.Now().Unix() * 1000)), Accuracy: 100, Raw: json.RawMessage([]byte(`{}`))},
		{LatitudeE7: 10 * 1e7, LongitudeE7: -10 * 10e7, TimestampMS: strconv.Itoa(int(time.Now().Unix() * 1000)), Accuracy: 100, Raw: json.RawMessage([]byte(`{}`))},
	}

	if err := s.AddGoogleTakeoutLocations(ctx, locs); err != nil {
		t.Fatal(err)
	}

	var count int

	if err := s.db.QueryRowContext(ctx, `select count(*) from device_locations`).Scan(&count); err != nil {
		t.Fatal(err)
	}

	if count != 2 {
		t.Errorf("want 2 rows, got: %d", count)
	}
}
