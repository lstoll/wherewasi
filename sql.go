package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type migration struct {
	// Idx is a unique identifier for this migration. It should be sortable in
	// the desired order of execution. Datestamp is a good idea
	Idx int
	// SQL to execute as part of this migration
	SQL string
	// AfterFunc is run inside the migration transaction, if not nil. Runs
	// _after_ the associated SQL is executed. This should be self-contained
	// code, that has no dependencies on application structure to make sure it
	// passes the test of time. It should not commit or rollback the TX, the
	// migration framework will handle that
	AfterFunc func(context.Context, *sql.Tx) error
}

var migrations = []migration{
	{
		Idx: 202006141339,
		SQL: `
			create table checkins (
				id text primary key,
				fsq_raw text,
				fsq_id text unique,
				created_at datetime default (datetime('now'))
			);

			create table people (
				id text primary key,
				firstname text,
				lastname text,
				fsq_id text unique,
				email text, -- unique would be nice, but imports don't have it
				created_at datetime default (datetime('now'))
			);

			-- venue represents a visitable place/business
			create table venues (
				id text primary key,
				name text,
				fsq_id text unique,
				created_at datetime default (datetime('now'))
			);

			-- location represent a physical place
			create table locations (
				id text primary key,
				name text,
				fsq_id text unique,
				created_at datetime default (datetime('now'))
			);
		`,
	},
	{
		Idx: 202006192128,
		SQL: `
			alter table checkins add checkin_time datetime; -- UTC time of the checkin
			alter table checkins add checkin_time_offset integer; -- Offset (in mins) to local time of checkin
		`,
		AfterFunc: func(ctx context.Context, tx *sql.Tx) error {
			rows, err := tx.QueryContext(ctx,
				`select id, fsq_raw from checkins where fsq_id is not null`)
			if err != nil {
				return fmt.Errorf("getting checkins: %v", err)
			}
			defer rows.Close()
			for rows.Next() {
				var (
					id     string
					cijson string
				)
				if err := rows.Scan(&id, &cijson); err != nil {
					return fmt.Errorf("scanning row: %v", err)
				}
				fsq := fsqCheckin{}
				if err := json.Unmarshal([]byte(cijson), &fsq); err != nil {
					return fmt.Errorf("unmarshaling checkin: %v", err)
				}
				// the foursquare created at time is UTC
				citime := time.Unix(int64(fsq.CreatedAt), 0)

				log.Printf("inserting %s into %s", citime.String(), id)

				res, err := tx.ExecContext(ctx, `update checkins set checkin_time=$1, checkin_time_offset=$s where id=$3`,
					citime, fsq.TimeZoneOffset, id)
				if err != nil {
					return fmt.Errorf("setting checkedin_at: %v", err)
				}
				ra, err := res.RowsAffected()
				if err != nil {
					return fmt.Errorf("checking rows affected: %v", err)
				}
				if ra != 1 {
					return fmt.Errorf("wanted update row %s to affect 1 row, got: %d", id, ra)
				}
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("rows err: %v", err)
			}
			return nil
		},
	},
	{
		Idx: 202006200953,
		SQL: `
		create table checkin_with (
			checkin_id text,
			user_id text,
			unique(checkin_id, user_id) -- doesn't make sense to have more than one
		);

		-- sqlite can't drop column, so create new table

		create table people_new (
			id text primary key,
			name text,
			fsq_id text unique,
			email text, -- unique would be nice, but imports don't have it
			created_at datetime default (datetime('now'))
		);

		insert into people_new select id, firstname || ' ' || lastname as name, fsq_id, email, created_at from people;

		drop table if exists people;
		alter table people_new rename to people;
		`,
	},
	{
		Idx: 202006201039,
		SQL: `
		create table checkin_people (
			checkin_id text,
			person_id text,
			unique(checkin_id, person_id) -- doesn't make sense to have more than one
		);

		insert into checkin_people select checkin_id, user_id as person_id from checkin_with;

		drop table if exists checkin_with;
		`,
	},
	{
		Idx: 2020062021325,
		SQL: `
		create table venues_new (
			id text primary key,
			name text,
			fsq_id text unique,
			lat integer,
			long integer,
			category text,
			friendly_address text,
			created_at datetime default (datetime('now'))
		);

		insert into venues_new(id, fsq_id, name) select id, fsq_id, name from venues;

		drop table if exists venues;
		alter table venues_new rename to venues;

		create table checkins_new (
			id text primary key,
			fsq_raw text,
			fsq_id text unique,
			created_at datetime default (datetime('now')),
			checkin_time datetime,
			checkin_time_offset integer,
			venue_id text,
			foreign key(venue_id) references venues(id)
		);
		insert into checkins_new (id, fsq_raw, fsq_id, created_at, checkin_time, checkin_time_offset) select id, fsq_raw, fsq_id, created_at, checkin_time, checkin_time_offset from checkins;

		drop table if exists checkins;
		alter table checkins_new rename to checkins;
		`,
	},
	{
		Idx: 2020062021404,
		SQL: `
		create table venues_new (
			id text primary key,
			name text,
			fsq_id text unique,
			lat integer,
			lng integer,
			category text,
			street_address text,
			city text,
			state text,
			postal_code text,
			country text,
			country_code text,
			created_at datetime default (datetime('now'))
		);

		insert into venues_new(id, name, fsq_id, lat, lng, category, created_at)
			select id, name, fsq_id, lat, long, category, created_at from venues;

		drop table if exists venues;
		alter table venues_new rename to venues;
		`,
	},
	{
		Idx: 2020062021736,
		SQL: `
		create table device_locations (
			accuracy integer,
			altitude integer,
			batt integer,
			battery_status integer,
			course_over_ground integer,
			lat float,
			lng float,
			region_radius float,
			trigger text,
			tracker_id text,
			timestamp datetime,
			vertical_accuracy integer,
			velocity integer,
			barometric_pressure float64,
			connection_status string,
			topic string,
			in_regions string, -- json array of regions
			raw_message text,
			created_at datetime default (datetime('now'))
		);
		`,
	},
}

type Storage struct {
	db *sql.DB

	log logger
}

func newStorage(ctx context.Context, logger logger, connStr string) (*Storage, error) {
	db, err := sql.Open("spatialite", connStr)
	if err != nil {
		return nil, fmt.Errorf("opening DB: %v", err)
	}

	s := &Storage{
		db:  db,
		log: logger,
	}

	if err := s.migrate(ctx); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Storage) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(
		ctx,
		`create table if not exists migrations (
		idx integer primary key not null,
		at datetime not null
		);`,
	); err != nil {
		return err
	}

	if err := s.execTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		sortMigrations(migrations)

		for _, mig := range migrations {
			var idx int
			err := tx.QueryRowContext(ctx, `select idx from migrations where idx = $1;`, mig.Idx).Scan(&idx)
			if err == nil {
				// selected fine so we've already inserted migration, next
				// please.
				continue
			}
			if err != nil && err != sql.ErrNoRows {
				// genuine error
				return fmt.Errorf("checking for migration existence: %v", err)
			}

			if err := runMigration(ctx, tx, mig); err != nil {
				return err
			}

			if _, err := tx.ExecContext(ctx, `insert into migrations (idx, at) values ($1, datetime('now'));`, mig.Idx); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func runMigration(ctx context.Context, tx *sql.Tx, mig migration) error {
	if mig.SQL != "" {
		if _, err := tx.ExecContext(ctx, mig.SQL); err != nil {
			return err
		}
	}

	if mig.AfterFunc != nil {
		if err := mig.AfterFunc(ctx, tx); err != nil {
			return err
		}
	}

	return nil
}

func (s *Storage) execTx(ctx context.Context, f func(ctx context.Context, tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if err := f(ctx, tx); err != nil {
		// Not much we can do about an error here, but at least the database will
		// eventually cancel it on its own if it fails
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

func sortMigrations(in []migration) {
	sort.Slice(in, func(i, j int) bool { return in[i].Idx < in[j].Idx })
}

func newDBID() string {
	return uuid.New().String()
}
