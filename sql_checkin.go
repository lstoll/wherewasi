package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// type checkin struct {
// 	ID           string
// 	FoursquareID string
// 	CreatedAt    time.Time
// }

func (s *Storage) Upsert4sqCheckin(ctx context.Context, checkin fsqCheckin) (string, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

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

// Sync4sqUsers finds all foursquare checkins in the DB, and ensures there are
// up-to-date user entries for them. Also denormalizes the checkin with
// information in to the database record.
func (s *Storage) Sync4sqUsers(ctx context.Context) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	txErr := s.execTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// run against all checkins
		rows, err := tx.QueryContext(ctx,
			`select id, fsq_raw from checkins where fsq_id is not null`)
		if err != nil {
			return fmt.Errorf("getting checkins: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var (
				checkinID string
				cijson    string
			)
			if err := rows.Scan(&checkinID, &cijson); err != nil {
				return fmt.Errorf("scanning row: %v", err)
			}
			fsq := fsqCheckin{}
			if err := json.Unmarshal([]byte(cijson), &fsq); err != nil {
				return fmt.Errorf("unmarshaling checkin: %v", err)
			}

			for _, with := range fsq.With {
				fullName := fmt.Sprintf("%s %s", with.FirstName, with.LastName)

				// try and get the person, to see if we have an existing ID. If
				// we do use that for linkage, if we don't generate a new ID for
				// the upserted record
				var (
					personID string
				)
				if err := tx.QueryRowContext(ctx, `select id from people where fsq_id=$1`, with.ID).Scan(&personID); err != nil {
					if err != sql.ErrNoRows {
						return fmt.Errorf("checking for existing person ID: %v", err)
					}
					// no record, create a new ID
					personID = newDBID()
				}

				// upsert the person, to ensure their data is up to date regardless
				_, err := tx.ExecContext(ctx, `
					insert into people(id, fsq_id, name) values ($1, $2, $3)
					on conflict(fsq_id) do update set name = $4
					where fsq_id=$5`,
					personID, with.ID, fullName,
					fullName,
					with.ID,
				)
				if err != nil {
					return fmt.Errorf("upserting person %s: %v", fullName, err)
				}

				// upsert the linkage
				_, err = tx.ExecContext(ctx, `
				insert into checkin_people(checkin_id, person_id) values ($1, $2)
				on conflict do nothing`,
					checkinID, personID,
				)
				if err != nil {
					return fmt.Errorf("linking %s to checkin %s: %v", fullName, checkinID, err)
				}
			}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("rows err: %v", err)
		}
		return nil

	})
	return txErr
}

// Sync4sqVenues finds all foursquare checkins in the DB, and ensures there are
// up-to-date venue entries for them.
func (s *Storage) Sync4sqVenues(ctx context.Context) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	txErr := s.execTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// run against all checkins
		rows, err := tx.QueryContext(ctx,
			`select id, fsq_raw from checkins where fsq_id is not null`)
		if err != nil {
			return fmt.Errorf("getting checkins: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var (
				checkinID string
				cijson    string
			)
			if err := rows.Scan(&checkinID, &cijson); err != nil {
				return fmt.Errorf("scanning row: %v", err)
			}
			fsq := fsqCheckin{}
			if err := json.Unmarshal([]byte(cijson), &fsq); err != nil {
				return fmt.Errorf("unmarshaling checkin: %v", err)
			}

			fv := fsq.Venue
			var cgry fsqCategories
			for _, c := range fv.Categories {
				if c.Primary {
					cgry = c
				}
			}

			// get existing or new ID
			var venueID string

			if err := tx.QueryRowContext(ctx, `select id from venues where fsq_id=$1`, fv.ID).Scan(&venueID); err != nil {
				if err != sql.ErrNoRows {
					return fmt.Errorf("checking for existing venue ID: %v", err)
				}
				// no record, create a new ID
				venueID = newDBID()
			}

			_, err := tx.ExecContext(ctx, `
				insert into venues(id, fsq_id, name, lat, lng, category, street_address, city, state, postal_code, country, country_code) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				on conflict(fsq_id) do update set name = ?, lat = ?, lng = ?, category = ?, street_address = ?, city = ?, state = ?, postal_code = ?, country = ?, country_code = ?
				where fsq_id=?`,
				venueID, fv.ID, fv.Name, fv.Location.Lat, fv.Location.Lng, cgry.Name, fv.Location.Address, fv.Location.City, fv.Location.State, fv.Location.PostalCode, fv.Location.Country, fv.Location.Cc,
				fv.Name, fv.Location.Lat, fv.Location.Lng, cgry.Name, fv.Location.Address, fv.Location.City, fv.Location.State, fv.Location.PostalCode, fv.Location.Country, fv.Location.Cc,
				fv.ID,
			)
			if err != nil {
				return fmt.Errorf("upserting venue %s: %v", fv.Name, err)
			}

			_, err = tx.ExecContext(ctx, `
				update checkins set venue_id=? where id=?`,
				venueID, checkinID,
			)
			if err != nil {
				return fmt.Errorf("updating checkin venue ID %s: %v", fv.Name, err)
			}

		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("rows err: %v", err)
		}
		return nil

	})
	return txErr
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
