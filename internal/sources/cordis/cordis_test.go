package cordis

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/taktekhq/bidwatch/internal/model"
)

// fixedNow is the reference clock every test's startDate is computed
// relative to, so window-day math never depends on when the test suite
// actually runs.
var fixedNow = time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)

func testConfig() Config {
	return Config{
		WindowDays:    45,
		SkipCountries: []string{"DE"},
		MaxPerRun:     5,
	}
}

func daysAgo(n int) string {
	return fixedNow.AddDate(0, 0, -n).Format("2006-01-02")
}

// csvBytes writes rows (header first) as CORDIS's own ";"-delimited CSV,
// letting encoding/csv handle quoting so the fixture round-trips through
// this package's own reader exactly like the real export would.
func csvBytes(t *testing.T, rows [][]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Comma = ';'
	if err := w.WriteAll(rows); err != nil {
		t.Fatalf("write csv fixture: %v", err)
	}
	return buf.Bytes()
}

// buildZip packs files (name -> content) into a zip the way CORDIS's own
// export does, so the adapter's zip-opening code is exercised too, not
// just its CSV parsing.
func buildZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s in fixture zip: %v", name, err)
		}
		if _, err := f.Write(content); err != nil {
			t.Fatalf("write %s in fixture zip: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close fixture zip: %v", err)
	}
	return buf.Bytes()
}

func projectHeader() []string {
	return []string{"id", "acronym", "status", "title", "startDate", "ecMaxContribution", "fundingScheme", "legalBasis", "masterCall", "subCall"}
}

func orgHeader() []string {
	return []string{"projectID", "role", "name", "country"}
}

func webLinkHeader() []string {
	return []string{"projectID", "type", "represents", "physUrl"}
}

// setFixture points dataURL at a test server serving the given zip and
// restores it after the test. It also fixes nowFunc to fixedNow so every
// WindowDays comparison is deterministic.
func setFixture(t *testing.T, zipBody []byte) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(zipBody)
	}))
	t.Cleanup(srv.Close)

	origURL, origNow := dataURL, nowFunc
	dataURL = srv.URL
	nowFunc = func() time.Time { return fixedNow }
	t.Cleanup(func() { dataURL, nowFunc = origURL, origNow })
}

func TestFetch_NoWebsiteListedIsALead(t *testing.T) {
	zipBody := buildZip(t, map[string][]byte{
		"project.csv": csvBytes(t, [][]string{
			projectHeader(),
			{"P1", "ACME", "SIGNED", "A project about something", daysAgo(10), "100000,50", "HORIZON-RIA", "HORIZON.2.5", "HORIZON-CL5-2021", "HORIZON-CL5-2021-D2-01"},
		}),
		"organization.csv": csvBytes(t, [][]string{
			orgHeader(),
			{"P1", "coordinator", "UNIVERSITY OF EXAMPLE", "FR"},
		}),
		"webLink.csv": csvBytes(t, [][]string{webLinkHeader()}),
	})
	setFixture(t, zipBody)

	listings, err := New(testConfig())(context.Background(), http.DefaultClient, "test-ua")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listings) != 1 {
		t.Fatalf("expected 1 listing, got %d: %+v", len(listings), listings)
	}
	l := listings[0]
	if l.Kind != model.KindLead {
		t.Errorf("Kind = %q, want %q", l.Kind, model.KindLead)
	}
	if l.Title != "UNIVERSITY OF EXAMPLE" {
		t.Errorf("Title = %q, want the coordinator name", l.Title)
	}
	if l.Country != "FR" {
		t.Errorf("Country = %q, want FR", l.Country)
	}
	if l.URL != "https://cordis.europa.eu/project/id/P1" || l.ID != l.URL {
		t.Errorf("URL/ID = %q/%q, want the CORDIS project link on both", l.URL, l.ID)
	}
	if !strings.Contains(l.Detail, "no website listed") {
		t.Errorf("Detail = %q, want it to say no website listed", l.Detail)
	}
	if !strings.Contains(l.Detail, "ACME") {
		t.Errorf("Detail = %q, want the acronym", l.Detail)
	}
}

func TestFetch_ListedWebsiteThatResolvesIsNotALead(t *testing.T) {
	var zipData []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/export.zip", func(w http.ResponseWriter, r *http.Request) { w.Write(zipData) })
	mux.HandleFunc("/project-site", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	zipData = buildZip(t, map[string][]byte{
		"project.csv": csvBytes(t, [][]string{
			projectHeader(),
			{"P1", "ACME", "SIGNED", "A project about something", daysAgo(10), "100000", "HORIZON-RIA", "", "", ""},
		}),
		"organization.csv": csvBytes(t, [][]string{
			orgHeader(),
			{"P1", "coordinator", "UNIVERSITY OF EXAMPLE", "FR"},
		}),
		"webLink.csv": csvBytes(t, [][]string{
			webLinkHeader(),
			{"P1", "relatedWebsite", "project", srv.URL + "/project-site"},
		}),
	})

	origURL, origNow := dataURL, nowFunc
	dataURL = srv.URL + "/export.zip"
	nowFunc = func() time.Time { return fixedNow }
	defer func() { dataURL, nowFunc = origURL, origNow }()

	listings, err := New(testConfig())(context.Background(), srv.Client(), "test-ua")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listings) != 0 {
		t.Fatalf("expected 0 listings (the listed site resolves), got %d: %+v", len(listings), listings)
	}
}

func TestFetch_ListedWebsiteThatIsDownIsALead(t *testing.T) {
	var zipData []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/export.zip", func(w http.ResponseWriter, r *http.Request) { w.Write(zipData) })
	mux.HandleFunc("/dead-site", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	deadURL := srv.URL + "/dead-site"
	zipData = buildZip(t, map[string][]byte{
		"project.csv": csvBytes(t, [][]string{
			projectHeader(),
			{"P1", "ACME", "SIGNED", "A project about something", daysAgo(10), "100000", "HORIZON-RIA", "", "", ""},
		}),
		"organization.csv": csvBytes(t, [][]string{
			orgHeader(),
			{"P1", "coordinator", "UNIVERSITY OF EXAMPLE", "FR"},
		}),
		"webLink.csv": csvBytes(t, [][]string{
			webLinkHeader(),
			{"P1", "relatedWebsite", "project", deadURL},
		}),
	})

	origURL, origNow := dataURL, nowFunc
	dataURL = srv.URL + "/export.zip"
	nowFunc = func() time.Time { return fixedNow }
	defer func() { dataURL, nowFunc = origURL, origNow }()

	listings, err := New(testConfig())(context.Background(), srv.Client(), "test-ua")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listings) != 1 {
		t.Fatalf("expected 1 listing (the listed site is down), got %d: %+v", len(listings), listings)
	}
	if !strings.Contains(listings[0].Detail, "site down: "+deadURL) {
		t.Errorf("Detail = %q, want it to name the dead URL", listings[0].Detail)
	}
}

func TestFetch_SkipsConfiguredCountry(t *testing.T) {
	zipBody := buildZip(t, map[string][]byte{
		"project.csv": csvBytes(t, [][]string{
			projectHeader(),
			{"P1", "DEPROJECT", "SIGNED", "A German-coordinated project", daysAgo(10), "100000", "HORIZON-RIA", "", "", ""},
			{"P2", "FRPROJECT", "SIGNED", "A French-coordinated project", daysAgo(10), "100000", "HORIZON-RIA", "", "", ""},
		}),
		"organization.csv": csvBytes(t, [][]string{
			orgHeader(),
			{"P1", "coordinator", "GERMAN UNI", "DE"},
			{"P2", "coordinator", "FRENCH UNI", "FR"},
		}),
		"webLink.csv": csvBytes(t, [][]string{webLinkHeader()}),
	})
	setFixture(t, zipBody)

	listings, err := New(testConfig())(context.Background(), http.DefaultClient, "test-ua")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listings) != 1 {
		t.Fatalf("expected 1 listing (DE skipped), got %d: %+v", len(listings), listings)
	}
	if listings[0].Title != "FRENCH UNI" {
		t.Errorf("Title = %q, want the French coordinator, not the German one", listings[0].Title)
	}
}

func TestFetch_OutsideWindowIsExcluded(t *testing.T) {
	zipBody := buildZip(t, map[string][]byte{
		"project.csv": csvBytes(t, [][]string{
			projectHeader(),
			{"P1", "OLD", "SIGNED", "An old project", daysAgo(200), "100000", "HORIZON-RIA", "", "", ""},
			{"P2", "NEW", "SIGNED", "A new project", daysAgo(10), "100000", "HORIZON-RIA", "", "", ""},
		}),
		"organization.csv": csvBytes(t, [][]string{
			orgHeader(),
			{"P1", "coordinator", "OLD UNI", "FR"},
			{"P2", "coordinator", "NEW UNI", "FR"},
		}),
		"webLink.csv": csvBytes(t, [][]string{webLinkHeader()}),
	})
	setFixture(t, zipBody)

	listings, err := New(testConfig())(context.Background(), http.DefaultClient, "test-ua")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listings) != 1 || listings[0].Title != "NEW UNI" {
		t.Fatalf("expected only the project started within the window, got %+v", listings)
	}
}

func TestFetch_ExcludesSoloFellowshipAndERCSchemes(t *testing.T) {
	zipBody := buildZip(t, map[string][]byte{
		"project.csv": csvBytes(t, [][]string{
			projectHeader(),
			{"P1", "FELLOW", "SIGNED", "A solo fellowship", daysAgo(10), "200000", "HORIZON-TMA-MSCA-PF-EF", "", "", ""},
			{"P2", "ERCGRANT", "SIGNED", "A solo ERC grant", daysAgo(10), "1500000", "HORIZON-ERC", "", "", ""},
			{"P3", "CONSORTIUM", "SIGNED", "A real consortium project", daysAgo(10), "2000000", "HORIZON-RIA", "", "", ""},
		}),
		"organization.csv": csvBytes(t, [][]string{
			orgHeader(),
			{"P1", "coordinator", "FELLOW HOST UNI", "FR"},
			{"P2", "coordinator", "ERC HOST UNI", "FR"},
			{"P3", "coordinator", "CONSORTIUM LEAD UNI", "FR"},
		}),
		"webLink.csv": csvBytes(t, [][]string{webLinkHeader()}),
	})
	setFixture(t, zipBody)

	listings, err := New(testConfig())(context.Background(), http.DefaultClient, "test-ua")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listings) != 1 || listings[0].Title != "CONSORTIUM LEAD UNI" {
		t.Fatalf("expected only the RIA/IA/CSA consortium project to survive, got %+v", listings)
	}
}

func TestFetch_OneLeadPerCoordinatorPerRunKeepsTheNewest(t *testing.T) {
	zipBody := buildZip(t, map[string][]byte{
		"project.csv": csvBytes(t, [][]string{
			projectHeader(),
			{"P1", "OLDER", "SIGNED", "An older project, same coordinator", daysAgo(40), "100000", "HORIZON-RIA", "", "", ""},
			{"P2", "NEWER", "SIGNED", "A newer project, same coordinator", daysAgo(5), "100000", "HORIZON-RIA", "", "", ""},
		}),
		"organization.csv": csvBytes(t, [][]string{
			orgHeader(),
			{"P1", "coordinator", "SAME UNI", "FR"},
			{"P2", "coordinator", "SAME UNI", "FR"},
		}),
		"webLink.csv": csvBytes(t, [][]string{webLinkHeader()}),
	})
	setFixture(t, zipBody)

	listings, err := New(testConfig())(context.Background(), http.DefaultClient, "test-ua")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listings) != 1 {
		t.Fatalf("expected exactly 1 lead for a coordinator with 2 qualifying projects, got %d: %+v", len(listings), listings)
	}
	if !strings.Contains(listings[0].Detail, "NEWER") {
		t.Errorf("Detail = %q, want the newer (P2/NEWER) project kept, not the older one", listings[0].Detail)
	}
}

func TestFetch_ProgrammeFilter(t *testing.T) {
	zipBody := buildZip(t, map[string][]byte{
		"project.csv": csvBytes(t, [][]string{
			projectHeader(),
			{"P1", "CLIMATE", "SIGNED", "Climate cluster project", daysAgo(10), "100000", "HORIZON-RIA", "HORIZON.2.5", "HORIZON-CL5-2021-D2-01", ""},
			{"P2", "HEALTH", "SIGNED", "Health cluster project", daysAgo(10), "100000", "HORIZON-RIA", "HORIZON.2.1", "HORIZON-HLTH-2021-D2-01", ""},
		}),
		"organization.csv": csvBytes(t, [][]string{
			orgHeader(),
			{"P1", "coordinator", "CLIMATE UNI", "FR"},
			{"P2", "coordinator", "HEALTH UNI", "FR"},
		}),
		"webLink.csv": csvBytes(t, [][]string{webLinkHeader()}),
	})
	setFixture(t, zipBody)

	cfg := testConfig()
	cfg.Programmes = []string{"HORIZON-CL5"}
	listings, err := New(cfg)(context.Background(), http.DefaultClient, "test-ua")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listings) != 1 || listings[0].Title != "CLIMATE UNI" {
		t.Fatalf("expected only the HORIZON-CL5 project to match the programme filter, got %+v", listings)
	}
}

func TestFetch_MaxPerRunCaps(t *testing.T) {
	zipBody := buildZip(t, map[string][]byte{
		"project.csv": csvBytes(t, [][]string{
			projectHeader(),
			{"P1", "A1", "SIGNED", "Project one", daysAgo(10), "100000", "HORIZON-RIA", "", "", ""},
			{"P2", "A2", "SIGNED", "Project two", daysAgo(10), "100000", "HORIZON-RIA", "", "", ""},
			{"P3", "A3", "SIGNED", "Project three", daysAgo(10), "100000", "HORIZON-RIA", "", "", ""},
		}),
		"organization.csv": csvBytes(t, [][]string{
			orgHeader(),
			{"P1", "coordinator", "UNI ONE", "FR"},
			{"P2", "coordinator", "UNI TWO", "FR"},
			{"P3", "coordinator", "UNI THREE", "FR"},
		}),
		"webLink.csv": csvBytes(t, [][]string{webLinkHeader()}),
	})
	setFixture(t, zipBody)

	cfg := testConfig()
	cfg.MaxPerRun = 2
	listings, err := New(cfg)(context.Background(), http.DefaultClient, "test-ua")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listings) != 2 {
		t.Fatalf("expected the MaxPerRun cap to hold at 2, got %d: %+v", len(listings), listings)
	}
}

// TestFetch_ToleratesAMalformedRowWithExtraFields guards against a real
// quirk found in CORDIS's own export on 12 Oct 2026: a handful of rows
// carry a raw, unescaped ";" inside a free-text field (project.csv's
// "objective"), splitting one field into two and shifting every column
// after it. That one row should be skipped or read with whatever columns
// it ends up with, never fail parsing for the whole file.
func TestFetch_ToleratesAMalformedRowWithExtraFields(t *testing.T) {
	header := "id;acronym;status;title;startDate;ecMaxContribution;fundingScheme;legalBasis;masterCall;subCall\n"
	malformed := "P1;BAD;SIGNED;A title with a raw; semicolon inside it;2026-09-01;100000;HORIZON-RIA;;;\n"
	wellFormed := fmt.Sprintf("P2;GOOD;SIGNED;A normal title;%s;100000;HORIZON-RIA;;;\n", daysAgo(10))
	projectCSV := []byte(header + malformed + wellFormed)

	zipBody := buildZip(t, map[string][]byte{
		"project.csv": projectCSV,
		"organization.csv": csvBytes(t, [][]string{
			orgHeader(),
			{"P1", "coordinator", "BAD UNI", "FR"},
			{"P2", "coordinator", "GOOD UNI", "FR"},
		}),
		"webLink.csv": csvBytes(t, [][]string{webLinkHeader()}),
	})
	setFixture(t, zipBody)

	listings, err := New(testConfig())(context.Background(), http.DefaultClient, "test-ua")
	if err != nil {
		t.Fatalf("expected the malformed row not to fail the whole fetch, got: %v", err)
	}
	found := false
	for _, l := range listings {
		if l.Title == "GOOD UNI" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the well-formed row (P2/GOOD UNI) to still come through, got %+v", listings)
	}
}

func TestFetch_NonOKStatusIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	origURL := dataURL
	dataURL = srv.URL
	defer func() { dataURL = origURL }()

	_, err := New(testConfig())(context.Background(), srv.Client(), "test-ua")
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestParseAmount(t *testing.T) {
	cases := map[string]float64{
		"2073781,25":   2073781.25,
		"100000":       100000,
		"":             0,
		"not-a-number": 0,
	}
	for in, want := range cases {
		if got := parseAmount(in); got != want {
			t.Errorf("parseAmount(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestFormatAmount(t *testing.T) {
	cases := map[float64]string{
		500:        "€500",
		127913.76:  "€127,914",
		2073781.25: "€2,073,781",
	}
	for in, want := range cases {
		if got := formatAmount(in); got != want {
			t.Errorf("formatAmount(%v) = %q, want %q", in, got, want)
		}
	}
}
