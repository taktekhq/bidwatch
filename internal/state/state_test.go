package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_MissingFileIsEmptyState(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !s.IsNew("https://example.org/a") {
		t.Fatal("a listing never seen should be new")
	}
}

func TestMarkThenSaveThenLoad_RoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 12, 8, 47, 0, 0, time.UTC)
	s.Mark("https://example.org/a", now)
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.IsNew("https://example.org/a") {
		t.Fatal("a marked listing should not be new after reload")
	}
	if !reloaded.IsNew("https://example.org/b") {
		t.Fatal("a listing never marked should still read as new")
	}
}

func TestIsNew_UnmarkedIDStaysNew(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Mark("https://example.org/a", time.Now())
	if !s.IsNew("https://example.org/b") {
		t.Fatal("a different ID should still be new")
	}
}
