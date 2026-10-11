package sbasubnet

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestFetch_ParsesTableRows(t *testing.T) {
	data, err := os.ReadFile("testdata/subnet_page.html")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer srv.Close()

	origBase, origPage := baseURL, pageURL
	baseURL = srv.URL
	pageURL = srv.URL
	defer func() { baseURL, pageURL = origBase, origPage }()

	listings, err := Fetch(context.Background(), srv.Client(), "test-ua")
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(listings) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(listings))
	}

	first := listings[0]
	if first.Title != "Caltrans IFB 10A2923" {
		t.Errorf("Title = %q", first.Title)
	}
	if first.Buyer != "MP Nexlevel" {
		t.Errorf("Buyer = %q", first.Buyer)
	}
	if first.URL != srv.URL+"/opportunity/caltrans-ifb-10a2923" {
		t.Errorf("URL = %q", first.URL)
	}
	wantDeadline := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	if first.Deadline == nil || !first.Deadline.Equal(wantDeadline) {
		t.Errorf("Deadline = %v, want %v", first.Deadline, wantDeadline)
	}
}

func TestFetch_Non200IsBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	origBase, origPage := baseURL, pageURL
	baseURL = srv.URL
	pageURL = srv.URL
	defer func() { baseURL, pageURL = origBase, origPage }()

	_, err := Fetch(context.Background(), srv.Client(), "test-ua")
	if err == nil {
		t.Fatal("expected an error for HTTP 403")
	}
}
