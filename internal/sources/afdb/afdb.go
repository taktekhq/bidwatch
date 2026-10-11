// Package afdb adapts the African Development Bank's corporate procurement
// RSS feed.
//
// Checked 12 Oct 2026: the feed URL itself (not just the HTML page) answers
// a Cloudflare challenge page (HTTP 403, Cf-Mitigated: challenge) to a
// plain GET with an honest User-Agent. Per the rule never to route around
// a block, this adapter reports it as blocked.
package afdb

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/taktekhq/bidwatch/internal/model"
	"github.com/taktekhq/bidwatch/internal/sources"
)

const Name = "afdb"

var feedURL = "https://www.afdb.org/en/about-us/corporate-procurement/procurement-notices/current-solicitations.xml"

func Fetch(ctx context.Context, client *http.Client, ua string) ([]model.Listing, error) {
	resp, err := sources.Get(ctx, client, ua, feedURL)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", Name, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if sources.CloudflareChallenge(resp, body) {
		return nil, &sources.BlockedError{Reason: "Cloudflare challenge on the RSS feed itself"}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &sources.BlockedError{Reason: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	return nil, fmt.Errorf("%s: got HTTP 200 but no parser exists yet for a real feed; check by hand before trusting this", Name)
}
