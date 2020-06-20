package main

import (
	"encoding/json"
	"fmt"
	"time"
)

// https://owntracks.org/booklet/tech/json/

// parent type for any owntracks message
type owntracksMessage struct {
	Type string `json:"_type"`
	Data json.RawMessage
}

func (o *owntracksMessage) UnmarshalJSON(b []byte) error {
	var into struct {
		Type string `json:"_type"`
	}
	if err := json.Unmarshal(b, &into); err != nil {
		return err
	}
	o.Type = into.Type
	o.Data = b
	return nil
}

func (o *owntracksMessage) IsLocation() bool {
	return o.Type == "location"
}

func (o *owntracksMessage) AsLocation() (otLocation, error) {
	if !o.IsLocation() {
		return otLocation{}, fmt.Errorf("message type %s is not location", o.Type)
	}
	r := otLocation{}
	if err := json.Unmarshal(o.Data, &r); err != nil {
		return otLocation{}, err
	}
	return r, nil
}

// otLocation represents a device location
// https://owntracks.org/booklet/tech/json/#_typelocation
type otLocation struct {
	// Accuracy of the reported location in meters without unit
	// (iOS,Android/integer/meters/optional)
	Accuracy *int `json:"acc,omitempty"`
	// Altitude measured above sea level (iOS,Android/integer/meters/optional)
	Altitude *int `json:"alt,omitempty"`
	// Device battery level (iOS,Android/integer/percent/optional)
	Batt *int `json:"batt,omitempty"`
	// Battery Status 0=unknown, 1=unplugged, 2=charging, 3=full (iOS)
	BatteryStatus *int `json:"bs,omitempty"`
	// Course over ground (iOS/integer/degree/optional
	CourseOverGround *int `json:"cog,omitempty"`
	// latitude (iOS,Android/float/degree/required)
	Latitude float64 `json:"lat"`
	// longitude (iOS,Android/float/degree/required)
	Longitude float64 `json:"lon"`
	// radius around the region when entering/leaving
	// (iOS/integer/meters/optional)
	RegionRadius *float64 `json:"rad,omitempty"`
	// trigger for the location report (iOS,Android/string/optional)
	//   * p ping issued randomly by background task (iOS,Android)
	//   * c circular region enter/leave event (iOS,Android)
	//   * b beacon region enter/leave event (iOS)
	//   * r response to a reportLocation cmd message (iOS,Android)
	//   * u manual publish requested by the user (iOS,Android)
	//   * t timer based publish in move move (iOS)
	//   * v updated by Settings/Privacy/Locations Services/System Services/Frequent Locations monitoring (iOS)
	Trigger *string `json:"t,omitempty"`
	// Tracker ID used to display the initials of a user
	// (iOS,Android/string/optional) required for http mode
	TrackerID *string `json:"tid,omitempty"`
	// UNIX epoch timestamp in seconds of the location fix
	// (iOS,Android/integer/epoch/required)
	TimestampUnix int `json:"tst"`
	// vertical accuracy of the alt element (iOS/integer/meters/optional)
	VerticalAccuracy *int `json:"vac,omitempty"`
	// velocity (iOS,Android/integer/kmh/optional)
	Velocity *int `json:"vel,omitempty"`
	// barometric pressure (iOS/float/kPa/optional/extended data)
	BarometricPressure *float64 `json:"p,omitempty"`
	// Internet connectivity status (route to host) when the message is created
	// (iOS,Android/string/optional/extended data)
	//   * w phone is connected to a WiFi connection (iOS,Android)
	//   * o phone is offline (iOS,Android)
	//   * m mobile data (iOS,Android)
	ConnectionStatus *string `json:"conn,omitempty"`
	// (only in HTTP payloads) contains the original publish topic (e.g.
	// owntracks/jane/phone). (iOS)
	Topic *string `json:"topic,omitempty"`
	// contains a list of regions the device is currently in (e.g.
	// ["Home","Garage"]). Might be empty. (iOS,Android/list of
	// strings/optional)
	InRegions []string `json:"inregions,omitempty"`
}

func (l *otLocation) Timestamp() time.Time {
	return time.Unix(int64(l.TimestampUnix), 0)
}
