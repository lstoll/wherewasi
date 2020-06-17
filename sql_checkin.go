package main

import (
	"context"
	"fmt"
)

// type checkin struct {
// 	ID           string
// 	FoursquareID string
// 	CreatedAt    time.Time
// }

func (s *Storage) Upsert4sqCheckin(ctx context.Context, checkin fsqCheckin) (string, error) {
	if checkin.ID == "" {
		return "", fmt.Errorf("checkin has no foursquare ID")
	}
	checkinID := newDBID()

	// check for the people

	// check for the venue

	_, err := s.db.ExecContext(ctx, `
insert into checkins(id, fsq_id, fsq_raw) values ($1, $2, $3)
on conflict(fsq_id) do update
  set fsq_raw = $4
where fsq_id=$5`,
		checkinID, checkin.ID, checkin.raw, // insert
		checkin.raw, // update
		checkin.ID)  // where
	if err != nil {
		return "", err
	}

	return checkin.ID, nil
}
