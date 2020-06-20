package main

import (
	"encoding/json"
	"testing"
)

const egOwntracksLocation = `{"tst":1592691368,"acc":3000,"_type":"location","alt":51,"lon":86.7816,"vac":29,"vel":-1,"lat":36.1627,"cog":-1,"tid":"NE","batt":55}`

func TestOTMessage(t *testing.T) {

	om := owntracksMessage{}
	if err := json.Unmarshal([]byte(egOwntracksLocation), &om); err != nil {
		t.Fatal(err)
	}

	if !om.IsLocation() {
		t.Error("should be location")
	}

	loc, err := om.AsLocation()
	if err != nil {
		t.Fatalf("turning in to location: %v", err)
	}

	if loc.Timestamp().IsZero() {
		t.Error("should be non-zero timestamp")
	}
}
