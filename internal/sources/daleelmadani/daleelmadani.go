// Package daleelmadani adapts Daleel Madani's "Calls for Proposals" board
// (Lebanese NGOs and INGOs) through the Firecrawl API, since a plain HTTP
// GET gets a Cloudflare challenge page every time (checked 12 Oct 2026).
//
// Firecrawl is asked for proxy "basic" only, never "auto", "stealth" or
// "enhanced": "auto" can escalate into "stealth", which is ban evasion,
// and bidwatch's rule is never to route around a source's block. If
// "basic" still gets challenged, this adapter reports the source as
// blocked rather than asking Firecrawl to try harder.
//
// One Firecrawl call per run, about 1 to 5 credits depending on the
// page's size; the credits actually spent are reported back to the
// runner (sources.ReportCredits) so a run's Slack message and runs.log
// both show them.
package daleelmadani

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/taktekhq/bidwatch/internal/firecrawl"
	"github.com/taktekhq/bidwatch/internal/model"
	"github.com/taktekhq/bidwatch/internal/sources"
)

const Name = "daleel-madani"

// var, not const, so tests can point it at a local fixture.
var pageURL = "https://daleel-madani.org/calls-for-proposal"

// scrapeFn is overridden in tests so no real Firecrawl call is made.
var scrapeFn = firecrawl.Scrape

// entryPattern reads one call's markdown block as Firecrawl renders the
// page (checked 12 Oct 2026):
//
//	#### [title](url)
//
//	country
//
//	call type
//
//	Deadline:
//
//	Weekday, D Month YYYY
//
//	[org name](org url)
var entryPattern = regexp.MustCompile(`(?m)^#### \[(.+?)\]\(([^)]+)\)\n\n(.+?)\n\n(.+?)\n\nDeadline:\n\n(.+?)\n\n\[(.+?)\]\([^)]+\)`)

// New returns a Fetcher that reads the board through Firecrawl using
// apiKey. An empty apiKey is handled by main.go, which leaves this source
// out of a run entirely rather than calling Fetch with nothing to
// authenticate a paid API call with.
func New(apiKey string) sources.Fetcher {
	return func(ctx context.Context, client *http.Client, ua string) ([]model.Listing, error) {
		if apiKey == "" {
			return nil, fmt.Errorf("%s: no Firecrawl API key configured", Name)
		}

		result, err := scrapeFn(ctx, client, apiKey, pageURL)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", Name, err)
		}
		sources.ReportCredits(ctx, result.CreditsUsed)

		if looksBlocked(result.Markdown) {
			return nil, &sources.BlockedError{Reason: "Cloudflare challenge even through Firecrawl's proxy basic"}
		}

		matches := entryPattern.FindAllStringSubmatch(result.Markdown, -1)
		if len(matches) == 0 {
			return nil, fmt.Errorf("%s: Firecrawl returned a page but no call matched the known layout; the site's template may have changed", Name)
		}

		var out []model.Listing
		for _, m := range matches {
			title := strings.TrimSpace(m[1])
			url := strings.TrimSpace(m[2])
			country := strings.TrimSpace(m[3])
			deadlineText := strings.TrimSpace(m[5])
			buyer := strings.TrimSpace(m[6])

			var deadline *time.Time
			if t, err := time.Parse("Monday, 2 January 2006", deadlineText); err == nil {
				deadline = &t
			}

			out = append(out, model.Listing{
				Source:   Name,
				ID:       url,
				Title:    title,
				Buyer:    buyer,
				Country:  country,
				Deadline: deadline,
				URL:      url,
			})
		}
		return out, nil
	}
}

// looksBlocked catches the shape of a bot-challenge page that still comes
// back as a Firecrawl "success" (a 200 full of challenge JS rather than
// an error), the same way a human eyeballing the page would recognize it,
// rather than trusting an empty parse as "no calls open right now."
func looksBlocked(markdown string) bool {
	if strings.TrimSpace(markdown) == "" {
		return true
	}
	lower := strings.ToLower(markdown)
	return strings.Contains(lower, "checking your browser") ||
		strings.Contains(lower, "just a moment") ||
		strings.Contains(lower, "enable javascript and cookies")
}
