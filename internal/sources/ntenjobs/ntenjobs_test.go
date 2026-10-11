package ntenjobs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestFetch_FiltersToRFPListings(t *testing.T) {
	fixture, err := os.ReadFile("testdata/jobs_page.html")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	}))
	defer srv.Close()

	orig := pageURL
	pageURL = srv.URL + "/"
	defer func() { pageURL = orig }()

	listings, err := Fetch(context.Background(), srv.Client(), "test-ua")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if len(listings) != 1 {
		t.Fatalf("expected 1 RFP-tagged listing (the fixture has one RFP and one employment job), got %d", len(listings))
	}
	got := listings[0]
	if got.Title != "Contract Salesforce Administrator" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Buyer != "Black Clergy Collaborative of Memphis" {
		t.Errorf("Buyer = %q", got.Buyer)
	}
	if got.Deadline != nil {
		t.Errorf("Deadline should be nil (NTEN doesn't state one), got %v", got.Deadline)
	}
	if got.URL != srv.URL+"/contract-salesforce-administrator" {
		t.Errorf("URL = %q", got.URL)
	}
}

func TestFetch_BlockedOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	orig := pageURL
	pageURL = srv.URL + "/"
	defer func() { pageURL = orig }()

	_, err := Fetch(context.Background(), srv.Client(), "test-ua")
	if err == nil {
		t.Fatal("expected an error for HTTP 403")
	}
}
