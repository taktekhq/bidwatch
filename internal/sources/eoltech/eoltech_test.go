package eoltech

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/taktekhq/bidwatch/internal/model"
	"github.com/taktekhq/bidwatch/internal/sources"
)

func TestFetch_ParsesHitsIntoLeadsWithEvidenceAndFirstSeen(t *testing.T) {
	fixture, err := os.ReadFile("testdata/search_page.json")
	if err != nil {
		t.Fatal(err)
	}
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("query")
		w.Write(fixture)
	}))
	defer srv.Close()

	orig := apiURL
	apiURL = srv.URL
	defer func() { apiURL = orig }()

	origNow := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC) }
	defer func() { nowFunc = origNow }()

	listings, err := New("test-key", 25)(context.Background(), srv.Client(), "test-ua")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listings) != 2 {
		t.Fatalf("expected 2 listings, got %d: %+v", len(listings), listings)
	}
	if gotQuery != `"Drupal 7" site:org` {
		t.Errorf("query sent = %q", gotQuery)
	}

	first := listings[0]
	if first.Kind != model.KindLead {
		t.Errorf("Kind = %q, want %q", first.Kind, model.KindLead)
	}
	if first.Title != "sofworld.org" {
		t.Errorf("Title = %q", first.Title)
	}
	if first.ID != "sofworld.org" {
		t.Errorf("ID should be the bare domain so a domain posts once ever: %q", first.ID)
	}
	if !strings.Contains(first.Detail, "Drupal 7") {
		t.Errorf("Detail missing evidence: %q", first.Detail)
	}
	if !strings.Contains(first.Detail, "12 Oct 2026") {
		t.Errorf("Detail missing first-seen date: %q", first.Detail)
	}
	if first.URL != "https://www.sofworld.org/" {
		t.Errorf("URL = %q", first.URL)
	}

	second := listings[1]
	if !strings.Contains(second.Detail, `matched PublicWWW query`) {
		t.Errorf("expected a fallback evidence line with no snippet, got %q", second.Detail)
	}
}

func TestFetch_NoAPIKeyIsAnErrorNotABlock(t *testing.T) {
	_, err := New("", 25)(context.Background(), &http.Client{}, "test-ua")
	if err == nil {
		t.Fatal("expected an error with no API key")
	}
	if _, ok := err.(*sources.BlockedError); ok {
		t.Fatal("missing config is not the same as the source blocking us")
	}
}

func TestFetch_UnauthorizedKeyIsBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_token"}`))
	}))
	defer srv.Close()

	orig := apiURL
	apiURL = srv.URL
	defer func() { apiURL = orig }()

	_, err := New("bad-key", 25)(context.Background(), srv.Client(), "test-ua")
	if err == nil {
		t.Fatal("expected an error")
	}
	if _, ok := err.(*sources.BlockedError); !ok {
		t.Fatalf("expected *sources.BlockedError, got %T: %v", err, err)
	}
}
