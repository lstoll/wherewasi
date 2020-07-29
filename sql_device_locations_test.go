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

func TestLatestLocationTimestamp(t *testing.T) {
	ctx, s := setupDB(t)

	now := time.Now()
	earlier := now.Add(-5 * time.Minute)

	for _, msg := range []otLocation{
		{
			TimestampUnix: int(now.Unix()),
		},
		{
			TimestampUnix: int(earlier.Unix()),
		},
	} {
		jb, err := json.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		om := owntracksMessage{
			Type: "location",
			Data: jb,
		}
		if err := s.AddOTLocation(ctx, om); err != nil {
			t.Fatal(err)
		}
	}

	l, err := s.LatestLocationTimestamp(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// round off, db storage is unix time in seconds
	if !l.Equal(now.Truncate(1 * time.Second)) {
		t.Errorf("want latest time %s, got: %s", now.String(), l.String())
	}
}
