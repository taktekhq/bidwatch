// Package savethechildren adapts Save the Children International's public
// tenders page.
//
// Checked 12 Oct 2026: a plain GET with an honest User-Agent gets a
// Cloudflare challenge page (HTTP 403, Cf-Mitigated: challenge), not the
// listing. Per the rule never to route around a block, this adapter
// reports it as blocked.
package savethechildren

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/taktekhq/bidwatch/internal/model"
	"github.com/taktekhq/bidwatch/internal/sources"
)

const Name = "save-the-children"

var url = "https://www.savethechildren.net/index.php/tenders"

func Fetch(ctx context.Context, client *http.Client, ua string) ([]model.Listing, error) {
	resp, err := sources.Get(ctx, client, ua, url)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", Name, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if sources.CloudflareChallenge(resp, body) {
		return nil, &sources.BlockedError{Reason: "Cloudflare challenge on the tenders page"}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &sources.BlockedError{Reason: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	return nil, fmt.Errorf("%s: got HTTP 200 but no parser exists yet for a real page; check by hand before trusting this", Name)
}
