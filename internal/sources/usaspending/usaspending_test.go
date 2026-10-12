package usaspending

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/taktekhq/bidwatch/internal/model"
)

func testConfig() Config {
	return Config{
		WindowDays: 7,
		MinAmount:  25000,
		MaxAmount:  500000,
		NAICSCodes: []string{"541511", "541512", "541519", "518210"},
	}
}

func TestFetch_ParsesAwardsIntoLeads(t *testing.T) {
	fixture, err := os.ReadFile("testdata/awards_page.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	}))
	defer srv.Close()

	orig := apiURL
	apiURL = srv.URL
	defer func() { apiURL = orig }()

	listings, err := New(testConfig())(context.Background(), srv.Client(), "test-ua")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The third fixture award has no recipient name and must be skipped.
	if len(listings) != 2 {
		t.Fatalf("expected 2 listings, got %d: %+v", len(listings), listings)
	}

	first := listings[0]
	if first.Kind != model.KindLead {
		t.Errorf("Kind = %q, want %q", first.Kind, model.KindLead)
	}
	if first.Title != "COMPTECH COMPUTER TECHNOLOGIES, INC" {
		t.Errorf("Title = %q", first.Title)
	}
	if first.Buyer != "Environmental Protection Agency" {
		t.Errorf("Buyer = %q", first.Buyer)
	}
	if !strings.Contains(first.Detail, "$127,914") {
		t.Errorf("Detail missing formatted amount: %q", first.Detail)
	}
	if !strings.Contains(first.Detail, "Environmental Protection Agency") {
		t.Errorf("Detail missing agency: %q", first.Detail)
	}
	if first.URL != "https://www.usaspending.gov/award/CONT_AWD_68HE0827F0001_6800_47QTCA19D00BT_4732" {
		t.Errorf("URL = %q", first.URL)
	}
	if first.ID != first.URL {
		t.Errorf("ID should be the award link so a dedupe is forever: ID=%q URL=%q", first.ID, first.URL)
	}

	long := listings[1]
	if !strings.HasSuffix(long.Detail, "...") {
		t.Errorf("expected the long description to be shortened with a trailing ellipsis, got %q", long.Detail)
	}
}

func TestFetch_NonOKStatusIsAPlainErrorNotABlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	orig := apiURL
	apiURL = srv.URL
	defer func() { apiURL = orig }()

	_, err := New(testConfig())(context.Background(), srv.Client(), "test-ua")
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestFormatAmount(t *testing.T) {
	cases := map[float64]string{
		500:        "$500",
		127913.76:  "$127,914",
		1234567.89: "$1,234,568",
	}
	for in, want := range cases {
		if got := formatAmount(in); got != want {
			t.Errorf("formatAmount(%v) = %q, want %q", in, got, want)
		}
	}
}
