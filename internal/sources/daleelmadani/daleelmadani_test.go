package daleelmadani

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/taktekhq/bidwatch/internal/firecrawl"
	"github.com/taktekhq/bidwatch/internal/sources"
)

func fakeScrape(markdown string, credits int, err error) func(context.Context, *http.Client, string, string) (firecrawl.Result, error) {
	return func(ctx context.Context, client *http.Client, apiKey, pageURL string) (firecrawl.Result, error) {
		if err != nil {
			return firecrawl.Result{}, err
		}
		return firecrawl.Result{Markdown: markdown, CreditsUsed: credits}, nil
	}
}

func TestFetch_ParsesARealTrimmedPage(t *testing.T) {
	md, err := os.ReadFile("testdata/calls_page.md")
	if err != nil {
		t.Fatal(err)
	}

	orig := scrapeFn
	scrapeFn = fakeScrape(string(md), 1, nil)
	defer func() { scrapeFn = orig }()

	var gotCredits int
	ctx := sources.WithCredits(context.Background(), &gotCredits)

	listings, err := New("test-key")(ctx, &http.Client{}, "test-ua")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listings) != 3 {
		t.Fatalf("expected 3 listings, got %d: %+v", len(listings), listings)
	}

	first := listings[0]
	if first.Title != "Consultancy services for the design and supervision of the construction and installation of a solar PV system for Abou Halqa pumping station - North Lebanon." {
		t.Errorf("Title = %q", first.Title)
	}
	if first.Buyer != "Lebrelief" {
		t.Errorf("Buyer = %q", first.Buyer)
	}
	if first.Country != "Lebanon" {
		t.Errorf("Country = %q", first.Country)
	}
	if first.Deadline == nil {
		t.Fatal("expected a parsed deadline")
	}
	if got := first.Deadline.Format("2006-01-02"); got != "2026-10-23" {
		t.Errorf("Deadline = %q", got)
	}
	if first.ID != first.URL {
		t.Errorf("ID should be the listing URL: ID=%q URL=%q", first.ID, first.URL)
	}
	if first.Kind != "" {
		t.Errorf("Kind = %q, want \"\" (an RFP, not a lead)", first.Kind)
	}

	if gotCredits != 1 {
		t.Errorf("credits reported = %d, want 1", gotCredits)
	}
}

func TestFetch_ChallengePageIsBlockedNotEmpty(t *testing.T) {
	orig := scrapeFn
	scrapeFn = fakeScrape("Just a moment... Checking your browser before continuing.", 1, nil)
	defer func() { scrapeFn = orig }()

	_, err := New("test-key")(context.Background(), &http.Client{}, "test-ua")
	if err == nil {
		t.Fatal("expected a blocked error")
	}
	if _, ok := err.(*sources.BlockedError); !ok {
		t.Fatalf("expected *sources.BlockedError, got %T: %v", err, err)
	}
}

func TestFetch_NoAPIKeyIsAnErrorNotABlock(t *testing.T) {
	_, err := New("")(context.Background(), &http.Client{}, "test-ua")
	if err == nil {
		t.Fatal("expected an error with no API key")
	}
	if _, ok := err.(*sources.BlockedError); ok {
		t.Fatal("missing config is not the same as the source blocking us")
	}
}
