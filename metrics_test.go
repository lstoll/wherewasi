package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetrics(t *testing.T) {
	ctx := context.Background()
	logger := log.New(os.Stderr, "", log.LstdFlags)
	db := fmt.Sprintf("file:%s?cache=shared&mode=memory&_foreign_keys=on", filepath.Join(t.TempDir(), "metrics.db"))

	st, err := newStorage(ctx, logger, db)
	if err != nil {
		t.Fatalf("creating storage: %v", err)
	}
	c := newMetricsCollector(logger, st)

	// Test with empty database first (i.e. handles sql.ErrNoRows correctly)
	if want, got := 2, testutil.CollectAndCount(c); got != want {
		t.Fatalf("want %d metrics, got %d", want, got)
	}

	om := owntracksMessage{}
	if err := json.Unmarshal([]byte(egOwntracksLocation), &om); err != nil {
		t.Fatal(err)
	}
	if err := st.AddOTLocation(ctx, om); err != nil {
		t.Fatalf("AddOTLocation: %v", err)
	}
	if _, err := st.Upsert4sqCheckin(ctx, fsqCheckin{ID: "abcdef", CreatedAt: int(time.Now().Unix())}); err != nil {
		t.Fatalf("Upsert4sqCheckin: %v", err)
	}

	if want, got := 2, testutil.CollectAndCount(c); got != want {
		t.Fatalf("want %d metrics, got %d", want, got)
	}

	lint, err := testutil.CollectAndLint(c)
	if err != nil {
		t.Fatalf("CollectAndLint: %v", err)
	}
	for _, prob := range lint {
		t.Errorf("lint: %s: %s", prob.Metric, prob.Text)
	}
	if len(lint) > 0 {
		t.Fatal("lint problems detected")
	}
}
