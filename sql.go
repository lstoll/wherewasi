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
}

type Storage struct {
	db *sql.DB

	log logger
}

func newStorage(ctx context.Context, logger logger, connStr string) (*Storage, error) {
	db, err := sql.Open("sqlite3", connStr)
	if err != nil {
		return nil, err
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
