package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ancientlore/go-tripit"
)

func (s *Storage) UpsertTripitTrip(ctx context.Context, trip *tripit.Trip, raw []byte) error {
	if trip.Id == "" {
		return fmt.Errorf("trip has no id")
	}
	tripID := newDBID()

	// technically these are the same format, but roundtrip anyway for
	// consistency
	startTime, err := time.Parse(tripitDateFormat, trip.StartDate)
	if err != nil {
		return fmt.Errorf("parsing start date %s: %v", trip.StartDate, err)
	}
	endTime, err := time.Parse(tripitDateFormat, trip.EndDate)
	if err != nil {
		return fmt.Errorf("parsing end date %s: %v", trip.EndDate, err)
	}

	_, err = s.db.ExecContext(ctx, `
insert into trips(id, tripit_id, tripit_raw, name, start_date, end_date, primary_location, description) values (?, ?, ?, ?, ?, ?, ?, ?)
on conflict(tripit_id) do update
  set tripit_raw = ?, name = ?, start_date = ?, end_date = ?, primary_location = ?, description = ?
where tripit_id=?`,
		tripID, trip.Id, raw, trip.DisplayName, startTime, endTime, trip.PrimaryLocation, trip.Description, // insert
		raw, trip.DisplayName, startTime, endTime, trip.PrimaryLocation, trip.Description, // update
		trip.Id) // where
	if err != nil {
		return err
	}

	return nil
}

func (s *Storage) LatestTripitID(ctx context.Context) (string, error) {
	var (
		tripitID string
	)
	if err := s.db.QueryRowContext(ctx, `select tripit_id from trips where tripit_id is not null order by end_date desc`).Scan(&tripitID); err != nil {
		if err != sql.ErrNoRows {
			return "", fmt.Errorf("finding latest tripit ID: %v", err)
		}
		// no record, create a new ID
		return "", nil
	}
	return tripitID, nil
}
