package prosperglobal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestFetch_ParsesTenderRows(t *testing.T) {
	data, err := os.ReadFile("testdata/tenders_page.html")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer srv.Close()

	origPage, origOrigin := pageURL, siteOrigin
	pageURL = srv.URL
	siteOrigin = srv.URL
	defer func() { pageURL, siteOrigin = origPage, origOrigin }()

	listings, err := Fetch(context.Background(), srv.Client(), "test-ua")
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(listings) != 2 {
		t.Fatalf("expected 2 tenders, got %d", len(listings))
	}

	kenya := listings[0]
	if kenya.Country != "Kenya" {
		t.Errorf("Country = %q", kenya.Country)
	}
	if kenya.Buyer != "Mercy Corps (Prosper Global)" {
		t.Errorf("Buyer = %q", kenya.Buyer)
	}
	wantDeadline := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	if kenya.Deadline == nil || !kenya.Deadline.Equal(wantDeadline) {
		t.Errorf("Deadline = %v, want %v", kenya.Deadline, wantDeadline)
	}
	if kenya.URL != srv.URL+"/tenders/intent-bid-ref-mc-agtn-322026-digital-transformation-acceleration-consulting-services-kenya" {
		t.Errorf("URL = %q", kenya.URL)
	}
}

func TestFetch_Non200IsBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	origPage, origOrigin := pageURL, siteOrigin
	pageURL = srv.URL
	siteOrigin = srv.URL
	defer func() { pageURL, siteOrigin = origPage, origOrigin }()

	_, err := Fetch(context.Background(), srv.Client(), "test-ua")
	if err == nil {
		t.Fatal("expected an error for HTTP 403")
	}
}
