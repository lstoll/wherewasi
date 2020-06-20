package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"
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
insert into checkins(id, fsq_id, fsq_raw, checkin_time, checkin_time_offset) values ($1, $2, $3, $4, $5)
on conflict(fsq_id) do update
  set fsq_raw = $6, checkin_time = $7, checkin_time_offset = $8
where fsq_id=$9`,
		checkinID, checkin.ID, checkin.raw, time.Unix(int64(checkin.CreatedAt), 0), checkin.TimeZoneOffset, // insert
		checkin.raw, time.Unix(int64(checkin.CreatedAt), 0), checkin.TimeZoneOffset, // update
		checkin.ID) // where
	if err != nil {
		return "", err
	}

	return checkin.ID, nil
}

func (s *Storage) Last4sqCheckinTime(ctx context.Context) (time.Time, error) {
	var latestCheckin *time.Time

	err := s.db.QueryRowContext(ctx, `select checkin_time from checkins order by datetime(checkin_time) desc limit 1`).Scan(&latestCheckin)
	if err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("finding latest checkin: %v", err)
	}

	if latestCheckin != nil {
		return *latestCheckin, nil
	}

	return time.Time{}, nil
}
