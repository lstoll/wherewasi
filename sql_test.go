package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestMigrations(t *testing.T) {
	ctx, s := setupDB(t)

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
}

func setupDB(t *testing.T) (ctx context.Context, s *Storage) {
	ctx = context.Background()

	connStr := "file:test.db?cache=shared&mode=memory&_foreign_keys=on"

	db, err := sql.Open("sqlite3", connStr)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s, err = newStorage(ctx, log.New(os.Stderr, "", log.LstdFlags), connStr)
	if err != nil {
		t.Fatalf("creating storage: %v", err)
	}

	return ctx, s
}
