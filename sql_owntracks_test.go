package main

import (
	"encoding/json"
	"testing"
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
