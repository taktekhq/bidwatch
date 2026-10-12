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
		IncludeWords: []string{
			"develop", "development", "design", "website", "web", "portal",
			"application", "app", "platform", "modernization", "migration",
			"integration", "dashboard", "ux", "accessibility", "508",
			"data pipeline", "automation",
		},
		ExcludeWords: []string{
			"license", "licenses", "licence", "licences", "renewal",
			"renewals", "subscription", "subscriptions", "maintenance",
			"support", "hardware", "equipment", "radio", "medical", "lab",
			"seats", "seat",
		},
	}
}

func TestFetch_FiltersOutLicensesRenewalsSupportAndHardware(t *testing.T) {
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
	// Of the 7 fixture rows: a license-renewal (Govsmart), a bare
	// "support" row with no build word (Comptech), a no-recipient row,
	// a named-hardware purchase (Metgreen), and a second award to a
	// recipient already kept are all dropped. Only the genuine build
	// work survives: the first Good Build Works award and the long,
	// multi-build-word Treasury award.
	if len(listings) != 2 {
		t.Fatalf("expected 2 listings, got %d: %+v", len(listings), listings)
	}

	first := listings[0]
	if first.Kind != model.KindLead {
		t.Errorf("Kind = %q, want %q", first.Kind, model.KindLead)
	}
	if first.Title != "GOOD BUILD WORKS LLC" {
		t.Errorf("Title = %q", first.Title)
	}
	if first.URL != "https://www.usaspending.gov/award/CONT_AWD_GOOD_BUILD_WORKS" {
		t.Errorf("URL = %q, want the FIRST (newest) award to this recipient, not the second", first.URL)
	}
	if first.ID != first.URL {
		t.Errorf("ID should be the award link so a dedupe is forever: ID=%q URL=%q", first.ID, first.URL)
	}

	second := listings[1]
	if second.Title != "LONG DESCRIPTION DEVELOPERS LLC" {
		t.Errorf("Title = %q", second.Title)
	}
	if !strings.HasSuffix(second.Detail, "...") {
		t.Errorf("expected the long description to be shortened with a trailing ellipsis, got %q", second.Detail)
	}

	for _, l := range listings {
		if strings.Contains(strings.ToLower(l.Title), "govsmart") || strings.Contains(strings.ToLower(l.Title), "comptech") || strings.Contains(strings.ToLower(l.Title), "metgreen") {
			t.Errorf("noise recipient leaked through the filter: %+v", l)
		}
	}
}

func TestFetch_AtMostOneLeadPerRecipientPerRun(t *testing.T) {
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
		t.Fatal(err)
	}
	count := 0
	for _, l := range listings {
		if l.Title == "GOOD BUILD WORKS LLC" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 lead for a recipient with 2 qualifying awards, got %d", count)
	}
}

func TestFetch_PreferSmallBusinessSortsItFirstWithoutDroppingOthers(t *testing.T) {
	search := `{"results":[
		{"Award ID":"A1","Recipient Name":"BIG CO","Award Amount":100000,"Awarding Agency":"Agency A","Description":"NEW WEBSITE DEVELOPMENT","generated_internal_id":"CONT_AWD_BIGCO"},
		{"Award ID":"A2","Recipient Name":"SMALL CO","Award Amount":90000,"Awarding Agency":"Agency B","Description":"NEW WEB PLATFORM DEVELOPMENT","generated_internal_id":"CONT_AWD_SMALLCO"}
	]}`

	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(search))
	})
	mux.HandleFunc("/awards/CONT_AWD_BIGCO/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"recipient":{"business_categories":["Category Business"]}}`))
	})
	mux.HandleFunc("/awards/CONT_AWD_SMALLCO/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"recipient":{"business_categories":["Small Business"]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	origAPI, origDetail := apiURL, awardDetailBaseURL
	apiURL = srv.URL + "/search"
	awardDetailBaseURL = srv.URL + "/awards/"
	defer func() { apiURL, awardDetailBaseURL = origAPI, origDetail }()

	cfg := testConfig()
	cfg.PreferSmallBusiness = true
	listings, err := New(cfg)(context.Background(), srv.Client(), "test-ua")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listings) != 2 {
		t.Fatalf("expected both leads kept (prefer, not drop), got %d: %+v", len(listings), listings)
	}
	if listings[0].Title != "SMALL CO" {
		t.Errorf("expected the small-business recipient sorted first, got %q first", listings[0].Title)
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

func TestPassesDescriptionFilter(t *testing.T) {
	cfg := testConfig()
	cases := []struct {
		desc string
		want bool
	}{
		{"WEBSITE REDESIGN AND DEVELOPMENT", true},
		{"ACUSTAF LABOR MANAGEMENT SOFTWARE", false},             // no include word; "LABOR" must not false-match "lab"
		{"COLLIBRA SOFTWARE LICENSE MAINTENANCE.", false},        // license+maintenance, and no include word either
		{"DRUVA LICENSES RENEWAL", false},                        // plural "licenses" must still be caught by the "license" substring exclude
		{"WEB APPLICATION DEVELOPMENT, LICENSE INCLUDED", false}, // include word present, but an exclude word wins
	}
	for _, c := range cases {
		got := passesDescriptionFilter(c.desc, cfg.IncludeWords, cfg.ExcludeWords)
		if got != c.want {
			t.Errorf("passesDescriptionFilter(%q) = %v, want %v", c.desc, got, c.want)
		}
	}
}
