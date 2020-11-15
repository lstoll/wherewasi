package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

var _ owntracksStore = (*Storage)(nil)

type DeviceLocation struct {
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	Accuracy int     `json:"accuracy"`
}

func (s *Storage) AddOTLocation(ctx context.Context, msg owntracksMessage) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

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

	_, err = s.db.ExecContext(ctx, `insert into device_locations (accuracy, altitude, batt, battery_status, course_over_ground, lat, lng, region_radius, trigger, tracker_id, timestamp, vertical_accuracy, velocity, barometric_pressure, connection_status, topic, in_regions, raw_owntracks_message) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		loc.Accuracy, loc.Altitude, loc.Batt, loc.BatteryStatus, loc.CourseOverGround, loc.Latitude, loc.Longitude, loc.RegionRadius, loc.Trigger, loc.TrackerID, loc.Timestamp(), loc.VerticalAccuracy, loc.Velocity, loc.BarometricPressure, loc.ConnectionStatus, loc.Topic, regions, string(msg.Data),
	)
	if err != nil {
		return fmt.Errorf("inserting location: %v", err)
	}

	return nil
}

func (s *Storage) AddGoogleTakeoutLocations(ctx context.Context, locs []takeoutLocation) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	err := s.execTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		for _, loc := range locs {
			if loc.Raw == nil || len(loc.Raw) < 1 {
				return fmt.Errorf("location missing raw data")
			}
			// check for https://support.google.com/maps/thread/4595364?hl=en
			if e7ToNormal(loc.LatitudeE7) > 90 || e7ToNormal(loc.LatitudeE7) < -90 ||
				e7ToNormal(loc.LongitudeE7) > 180 || e7ToNormal(loc.LongitudeE7) < -180 {
				return fmt.Errorf("location has invalid lat %f (e7 %d) or long %f (e7 %d)",
					e7ToNormal(loc.LatitudeE7), loc.LatitudeE7, e7ToNormal(loc.LongitudeE7), loc.LongitudeE7)
			}

			tsms, err := strconv.ParseInt(loc.TimestampMS, 10, 64)
			if err != nil {
				return fmt.Errorf("parsing timestamp %s to int64: %v", loc.TimestampMS, err)
			}

			ts := time.Unix(0, tsms*int64(1000000))

			var velkmh *int
			if loc.Velocity != nil {
				v := msToKmh(*loc.Velocity)
				velkmh = &v
			}

			_, err = tx.ExecContext(ctx, `insert into device_locations (accuracy, altitude, course_over_ground, lat, lng, timestamp, vertical_accuracy, velocity, raw_google_location) values (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				loc.Accuracy, loc.Altitude, loc.Heading, e7ToNormal(loc.LatitudeE7), e7ToNormal(loc.LongitudeE7), ts, loc.VerticalAccuracy, velkmh, string(loc.Raw),
			)
			if err != nil {
				return fmt.Errorf("inserting location: %v", err)
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("running tx: %v", err)
	}

	return nil
}

func (s *Storage) RecentLocations(ctx context.Context, count int) ([]DeviceLocation, error) {
	rows, err := s.db.QueryContext(ctx,
		`select lat, lng, accuracy from device_locations order by timestamp desc limit ?`, count)
	if err != nil {
		return nil, fmt.Errorf("getting locations: %v", err)
	}
	defer rows.Close()

	ret := []DeviceLocation{}

	for rows.Next() {
		var loc DeviceLocation
		if err := rows.Scan(&loc.Lat, &loc.Lng, &loc.Accuracy); err != nil {
			return nil, fmt.Errorf("scanning row: %v", err)
		}

		ret = append(ret, loc)
	}

	return ret, nil
}

// LatestLocationTimestamp returns the time at which the latest location was
// recorded
func (s *Storage) LatestLocationTimestamp(ctx context.Context) (time.Time, error) {
	var timestamp time.Time

	if err := s.db.QueryRowContext(ctx, `select timestamp from device_locations order by timestamp desc limit 1`).Scan(&timestamp); err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("finding lastest device_locations timestamp: %v", err)
	}

	return timestamp, nil
}
