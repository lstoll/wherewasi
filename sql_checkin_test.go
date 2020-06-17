package main

import "testing"

func TestUpsert4sqCheckin(t *testing.T) {
	ctx, s := setupDB(t)

	ci := fsqCheckin{
		ID: "abcdef",
	}

	_, err := s.Upsert4sqCheckin(ctx, ci)
	if err != nil {
		t.Fatalf("initial insert: %v", err)
	}

	_, err = s.Upsert4sqCheckin(ctx, ci)
	if err != nil {
		t.Fatalf("subsequent insert: %v", err)
	}
}
