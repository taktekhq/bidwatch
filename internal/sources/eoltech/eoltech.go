// Package eoltech adapts PublicWWW's source-code search API for live
// Drupal 7 sites. The Drupal project stopped shipping security patches
// for Drupal 7 in January 2025, so a live Drupal 7 site is a lead (a
// candidate for a quiet, no-strings "your site is end-of-life" note),
// never an RFP: nobody has asked for anything yet.
//
// Checked 12 Oct 2026: a plain, unauthenticated GET at publicwww.com gets
// PublicWWW's own bot-challenge page, and PublicWWW's documented API
// (api.publicwww.com) requires a key issued on a paid plan ("Getting a
// key... A paid plan is required", publicwww.com/docs/api/). Per the
// rule never to route around a block, this adapter makes the one
// documented API call when a key is configured, and main.go leaves this
// source out of a run entirely when it isn't, rather than this adapter
// trying anything unauthenticated.
package eoltech

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/taktekhq/bidwatch/internal/model"
	"github.com/taktekhq/bidwatch/internal/sources"
)

const Name = "eol-tech-drupal7"

// var, not const, so tests can point it at a local fixture server.
var apiURL = "https://api.publicwww.com/v1/search"

// query asks PublicWWW's own syntax for the Drupal 7 meta-generator tag,
// restricted server-side to .org with site:org: the task's own filter
// ("filtered to .org and nonprofit/association-looking domains where
// possible"), applied as cheaply as possible, in the query itself rather
// than by fetching more rows than needed and discarding most of them.
const query = `"Drupal 7" site:org`

// nowFunc is overridden in tests so "first seen" is deterministic.
var nowFunc = time.Now

type row struct {
	Domain   string `json:"domain"`
	URL      string `json:"url"`
	Snippets []struct {
		Text  string `json:"text"`
		Match string `json:"match"`
	} `json:"snippets"`
}

type response struct {
	Results []row `json:"results"`
}

// New returns a Fetcher that calls PublicWWW's API with apiKey and
// perPage. An empty apiKey is handled by main.go, which leaves this
// source out of a run rather than calling Fetch with nothing to
// authenticate a paid call with.
func New(apiKey string, perPage int) sources.Fetcher {
	if perPage <= 0 {
		perPage = 25
	}
	return func(ctx context.Context, client *http.Client, ua string) ([]model.Listing, error) {
		if apiKey == "" {
			return nil, fmt.Errorf("%s: no PublicWWW API key configured", Name)
		}

		q := url.Values{}
		q.Set("query", query)
		q.Set("per_page", fmt.Sprintf("%d", perPage))
		q.Set("snippets", "1")

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"?"+q.Encode(), nil)
		if err != nil {
			return nil, fmt.Errorf("%s: build request: %w", Name, err)
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("User-Agent", ua)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", Name, err)
		}
		defer resp.Body.Close()

		raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return nil, fmt.Errorf("%s: read response: %w", Name, err)
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, &sources.BlockedError{Reason: fmt.Sprintf("PublicWWW rejected the API key (HTTP %d)", resp.StatusCode)}
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: HTTP %d from the PublicWWW API", Name, resp.StatusCode)
		}

		var parsed response
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, fmt.Errorf("%s: parse response: %w", Name, err)
		}

		seenDate := nowFunc().UTC().Format("2 Jan 2006")
		var out []model.Listing
		for _, r := range parsed.Results {
			domain := strings.ToLower(strings.TrimSpace(r.Domain))
			if domain == "" || !strings.HasSuffix(domain, ".org") {
				// Defensive: site:org already restricts the query
				// server-side, but a result shaped unexpectedly is
				// skipped rather than guessed at.
				continue
			}
			siteURL := strings.TrimSpace(r.URL)
			if siteURL == "" {
				siteURL = "https://" + domain
			}

			evidence := `Drupal 7 (matched PublicWWW query "Drupal 7" site:org)`
			if len(r.Snippets) > 0 && strings.TrimSpace(r.Snippets[0].Text) != "" {
				evidence = fmt.Sprintf("Drupal 7, found in page source: %s", strings.TrimSpace(r.Snippets[0].Text))
			}

			out = append(out, model.Listing{
				Source: Name,
				ID:     domain,
				Kind:   model.KindLead,
				Title:  domain,
				Detail: fmt.Sprintf("%s, first seen %s", evidence, seenDate),
				URL:    siteURL,
			})
		}
		return out, nil
	}
}
