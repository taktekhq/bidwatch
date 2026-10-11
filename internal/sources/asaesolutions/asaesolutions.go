// Package asaesolutions adapts the ASAE Solutions Hub RFP board (US trade
// and professional associations).
//
// Checked 12 Oct 2026: every path tried (the RFP list, the site root, a
// guessed /api/rfps) answers HTTP 403 from an AWS load balancer, with no
// Cloudflare challenge page to solve and no cookie to add; it is a plain
// block on non-browser clients. Per the rule never to route around a
// block, this adapter reports it as blocked rather than guessing at a
// workaround.
package asaesolutions

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/taktekhq/bidwatch/internal/model"
	"github.com/taktekhq/bidwatch/internal/sources"
)

const Name = "asae-solutions-hub"

var url = "https://solutionshq.asaecenter.org/viewrfp"

func Fetch(ctx context.Context, client *http.Client, ua string) ([]model.Listing, error) {
	resp, err := sources.Get(ctx, client, ua, url)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", Name, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode == http.StatusForbidden {
		return nil, &sources.BlockedError{Reason: "HTTP 403 from the site's load balancer on every path tried"}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &sources.BlockedError{Reason: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	return nil, fmt.Errorf("%s: got HTTP 200 but no parser exists yet for a real page; check by hand before trusting this", Name)
}
