package firecrawl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestScrape_SendsProxyBasicOnlyAndReturnsMarkdownPlusCredits(t *testing.T) {
	var gotBody map[string]any
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Write([]byte(`{"success":true,"data":{"markdown":"# Page","metadata":{"statusCode":200,"creditsUsed":1}}}`))
	}))
	defer srv.Close()

	orig := Endpoint
	Endpoint = srv.URL
	defer func() { Endpoint = orig }()

	result, err := Scrape(context.Background(), srv.Client(), "test-key", "https://example.org/page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Markdown != "# Page" {
		t.Errorf("Markdown = %q", result.Markdown)
	}
	if result.CreditsUsed != 1 {
		t.Errorf("CreditsUsed = %d, want 1", result.CreditsUsed)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q", gotAuth)
	}
	if gotBody["proxy"] != "basic" {
		t.Fatalf("proxy = %v, want exactly \"basic\" (never auto/stealth/enhanced)", gotBody["proxy"])
	}
}

func TestScrape_NoAPIKeyIsRefusedBeforeAnyRequest(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	orig := Endpoint
	Endpoint = srv.URL
	defer func() { Endpoint = orig }()

	if _, err := Scrape(context.Background(), srv.Client(), "", "https://example.org/page"); err == nil {
		t.Fatal("expected an error with no API key")
	}
	if called {
		t.Fatal("expected no request to be sent with no API key")
	}
}

func TestScrape_FirecrawlErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		w.Write([]byte(`{"success":false,"error":"Insufficient credits"}`))
	}))
	defer srv.Close()

	orig := Endpoint
	Endpoint = srv.URL
	defer func() { Endpoint = orig }()

	_, err := Scrape(context.Background(), srv.Client(), "test-key", "https://example.org/page")
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); got == "" {
		t.Fatal("expected a non-empty error message")
	}
}
