package rfpdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func serveFile(t *testing.T, path string) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
}

func TestFetchCategory_ParsesMicrodataRows(t *testing.T) {
	srv := serveFile(t, "testdata/category_with_results.html")
	defer srv.Close()

	listings, err := fetchCategory(context.Background(), srv.Client(), "test-ua", srv.URL)
	if err != nil {
		t.Fatalf("fetchCategory error: %v", err)
	}
	if len(listings) != 2 {
		t.Fatalf("expected 2 listings, got %d", len(listings))
	}

	first := listings[0]
	if first.Title != "RFP #: P-2027-2 SS4A Safety Action Plan" {
		t.Errorf("Title = %q", first.Title)
	}
	if first.URL != "https://www.rfpdb.com/view/document/name/RFP-%23%3A-P_2027_2-SS4A-Safety-Action-Plan" {
		t.Errorf("URL = %q", first.URL)
	}
	if first.Country != "Washington, United States" {
		t.Errorf("Country = %q", first.Country)
	}
	wantDeadline := time.Date(2026, 11, 4, 23, 59, 59, 0, time.UTC)
	if first.Deadline == nil || !first.Deadline.Equal(wantDeadline) {
		t.Errorf("Deadline = %v, want %v", first.Deadline, wantDeadline)
	}
}

func TestFetchCategory_NoResultsIsZeroNotAnError(t *testing.T) {
	srv := serveFile(t, "testdata/category_empty.html")
	defer srv.Close()

	listings, err := fetchCategory(context.Background(), srv.Client(), "test-ua", srv.URL)
	if err != nil {
		t.Fatalf("an empty category should not be an error, got: %v", err)
	}
	if len(listings) != 0 {
		t.Fatalf("expected 0 listings on an empty category page, got %d", len(listings))
	}
}

func TestFetch_AllCategoriesBlockedIsReportedAsBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	origURLs := categoryURLs
	categoryURLs = []string{srv.URL + "/a", srv.URL + "/b"}
	defer func() { categoryURLs = origURLs }()

	_, err := Fetch(context.Background(), srv.Client(), "test-ua")
	if err == nil {
		t.Fatal("expected a blocked error when every category page 403s")
	}
}
