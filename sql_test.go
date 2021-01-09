package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ancientlore/go-tripit"
	_ "github.com/mattn/go-sqlite3"
)

func TestMigrations(t *testing.T) {
	ctx, s := setupDB(t)

	var seen []int64
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

func TestSQLiteConcurreny(t *testing.T) {
	ctx, s := setupDB(t)

	var wg sync.WaitGroup

	var errs []error

	otLoc := otLocation{
		Accuracy:      iptr(5), // TODO - should this allow nulls in the DB? OT has it as optional
		TimestampUnix: int(time.Now().Unix()),
	}
	otLocb, err := json.Marshal(&otLoc)
	if err != nil {
		t.Fatal(err)
	}
	otMsg := owntracksMessage{
		Type: "location",
		Data: otLocb,
	}

	for i := 0; i < 10; i++ {
		wg.Add(3)

		go func() {
			defer wg.Done()

			for i := 0; i < 10; i++ {
				if err := s.AddOTLocation(ctx, otMsg); err != nil {
					errs = append(errs, err)
				}
				if _, err := s.RecentLocations(ctx, time.Now().Add(-1*time.Minute), time.Now().Add(1*time.Minute)); err != nil {
					errs = append(errs, err)
				}
			}
		}()
		go func() {
			defer wg.Done()

			for i := 0; i < 10; i++ {
				if err := s.UpsertTripitTrip(ctx, &tripit.Trip{
					Id:        randString(10),
					StartDate: "2006-01-02",
					EndDate:   "2006-01-02",
				}, []byte(`{}`)); err != nil {
					errs = append(errs, err)
				}
				if _, err := s.LatestTripitID(ctx); err != nil {
					errs = append(errs, err)
				}
			}
		}()
		go func() {
			defer wg.Done()

			for i := 0; i < 10; i++ {
				if _, err := s.Upsert4sqCheckin(ctx, fsqCheckin{
					ID:        randString(10),
					CreatedAt: int(time.Now().Unix()),
					Score:     s,
					raw:       []byte(`{}`),
				}); err != nil {
					errs = append(errs, err)
				}
				if _, err := s.Last4sqCheckinTime(ctx); err != nil {
					errs = append(errs, err)
				}
			}
		}()
	}

	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("wanted 0 errors, found: %d\n\n%v", len(errs), errs)
	}

}

func iptr(i int) *int {
	return &i
}

func setupDB(t *testing.T) (ctx context.Context, s *Storage) {
	ctx = context.Background()

	tr := rand.New(rand.NewSource(time.Now().UnixNano())).Int63()

	connStr := fmt.Sprintf("file:%s/test-%d.db?cache=shared&_foreign_keys=on", t.TempDir(), tr)

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

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}
