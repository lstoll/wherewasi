package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	_ "github.com/mattn/go-sqlite3"
)

func TestMigrations(t *testing.T) {
	ctx, s := setup(t)

	var seen []int
	for _, m := range migrations {
		for _, s := range seen {
			if m.Idx == s {
				t.Errorf("migration index %d is duplicated", s)
			}
		}
		seen = append(seen, m.Idx)
	}

	for i := 0; i < 3; i++ {
		if err := s.migrate(ctx); err != nil {
			t.Errorf("unexpected error when repeat migrating database: %v", err)
		}
	}

	var numMigs int
	if err := s.db.QueryRowContext(ctx, `select count(idx) from migrations`).Scan(&numMigs); err != nil {
		t.Fatalf("error checking migration count: %v", err)
	}

	if numMigs != len(migrations) {
		t.Errorf("want %d migrations, found %d in db", len(migrations), numMigs)
	}

	unsorted := []migration{
		{
			Idx: 5,
		},
		{
			Idx: 10,
		},
		{
			Idx: 1,
		},
	}

	sortMigrations(unsorted)

	sorted := []migration{
		{
			Idx: 1,
		},
		{
			Idx: 5,
		},
		{
			Idx: 10,
		},
	}

	if diff := cmp.Diff(unsorted, sorted); diff != "" {
		t.Errorf("sorting failed: %s", diff)
	}
}

func setup(t *testing.T) (ctx context.Context, s *Storage) {
	ctx = context.Background()

	db, err := sql.Open("sqlite3", "file:test.db?cache=shared&mode=memory")
	if err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{} {
		if _, err := db.Exec(`drop table if exists ` + table); err != nil {
			t.Fatal(err)
		}
	}

	s, err = New(ctx, log.New(os.Stderr, "", log.LstdFlags), db)
	if err != nil {
		t.Fatal(err)
	}

	return ctx, s
}
