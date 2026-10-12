// Package firecrawl is a minimal client for the one Firecrawl call
// bidwatch ever makes: a single-page scrape, proxy "basic" only.
//
// Firecrawl's proxy can escalate from "basic" through "stealth" to
// "enhanced" (at "auto", Firecrawl itself picks, which can mean stealth);
// stealth is ban evasion. bidwatch's own rule is never to route around a
// source's block, so proxy here is not a config option and is hard-coded
// to "basic": if a page is still blocked at that level, the caller reports
// the source as blocked rather than asking this package to try harder.
package firecrawl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Endpoint is Firecrawl's v2 scrape call. var, not const, so tests can
// point it at a local fixture server.
var Endpoint = "https://api.firecrawl.dev/v2/scrape"

type request struct {
	URL             string   `json:"url"`
	Formats         []string `json:"formats"`
	Proxy           string   `json:"proxy"`
	OnlyMainContent bool     `json:"onlyMainContent"`
}

type response struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Data    struct {
		Markdown string `json:"markdown"`
		Metadata struct {
			StatusCode  int `json:"statusCode"`
			CreditsUsed int `json:"creditsUsed"`
		} `json:"metadata"`
	} `json:"data"`
}

// Result is one page's main-content markdown plus how many Firecrawl
// credits the call spent, so a caller can log it.
type Result struct {
	Markdown    string
	CreditsUsed int
}

// Scrape fetches pageURL through Firecrawl with proxy "basic", never
// anything stronger. apiKey is required; an empty key is refused here
// rather than sent as an unauthenticated request. client's own timeout
// applies, same as every other adapter's requests.
func Scrape(ctx context.Context, client *http.Client, apiKey, pageURL string) (Result, error) {
	if apiKey == "" {
		return Result{}, fmt.Errorf("firecrawl: no API key configured")
	}

	body, err := json.Marshal(request{
		URL:             pageURL,
		Formats:         []string{"markdown"},
		Proxy:           "basic",
		OnlyMainContent: true,
	})
	if err != nil {
		return Result{}, fmt.Errorf("firecrawl: build request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, Endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("firecrawl: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("firecrawl: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return Result{}, fmt.Errorf("firecrawl: read response: %w", err)
	}

	var r response
	if err := json.Unmarshal(raw, &r); err != nil {
		return Result{}, fmt.Errorf("firecrawl: parse response (HTTP %d): %w", resp.StatusCode, err)
	}
	if !r.Success {
		msg := r.Error
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d with no error message", resp.StatusCode)
		}
		return Result{}, fmt.Errorf("firecrawl: %s", msg)
	}

	return Result{Markdown: r.Data.Markdown, CreditsUsed: r.Data.Metadata.CreditsUsed}, nil
}
