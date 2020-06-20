package main

import (
	"context"
	"encoding/json"
	"fmt"
)

var _ owntracksStore = (*Storage)(nil)

func (s *Storage) AddOTLocation(ctx context.Context, msg owntracksMessage) error {
	if !msg.IsLocation() {
		return fmt.Errorf("message needs to be location")
	}
	loc, err := msg.AsLocation()
	if err != nil {
		return err
	}

	var regions *string

	if len(loc.InRegions) > 0 {
		regb, err := json.Marshal(loc.InRegions)
		if err != nil {
			return fmt.Errorf("marshaling regions: %v", err)
		}
		s := string(regb)
		regions = &s
	}

	_, err = s.db.ExecContext(ctx, `insert into device_locations (accuracy, altitude, batt, battery_status, course_over_ground, lat, lng, region_radius, trigger, tracker_id, timestamp, vertical_accuracy, velocity, barometric_pressure, connection_status, topic, in_regions, raw_message) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		loc.Accuracy, loc.Altitude, loc.Batt, loc.BatteryStatus, loc.CourseOverGround, loc.Latitude, loc.Longitude, loc.RegionRadius, loc.Trigger, loc.TrackerID, loc.Timestamp(), loc.VerticalAccuracy, loc.Velocity, loc.BarometricPressure, loc.ConnectionStatus, loc.Topic, regions, string(msg.Data),
	)
	if err != nil {
		return fmt.Errorf("inserting location: %v", err)
	}

	return nil
}
