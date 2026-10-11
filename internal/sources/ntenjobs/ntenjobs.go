// Package ntenjobs adapts NTEN's job board (nten.org/jobs), filtered to the
// postings NTEN itself tags as RFPs rather than employment.
//
// The page is server-rendered Next.js; the full job list, already tagged
// with an RFP/employment "listing type" per job, ships inline in a
// <script id="__NEXT_DATA__"> block. This adapter reads that block instead
// of guessing at visible markup, the same way the craft page recommends
// reading a page's own JSON-LD or embedded data over scraping rendered
// HTML. Checked 12 Oct 2026: 23 jobs on the page, 3 tagged "rfp".
package ntenjobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/taktekhq/bidwatch/internal/model"
	"github.com/taktekhq/bidwatch/internal/sources"
)

const Name = "nten-jobs"

// var, not const, so tests can point it at a local fixture server.
var pageURL = "https://www.nten.org/jobs/"

type nextData struct {
	Props struct {
		PageProps struct {
			Jobs []job `json:"jobs"`
		} `json:"pageProps"`
	} `json:"props"`
}

type job struct {
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Date      string `json:"date"`
	JobFields struct {
		Organization string `json:"organization"`
	} `json:"jobFields"`
	JobLocations struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"jobLocations"`
	ListingTypes struct {
		Nodes []struct {
			Slug string `json:"slug"`
		} `json:"nodes"`
	} `json:"listingTypes"`
}

func (j job) isRFP() bool {
	for _, n := range j.ListingTypes.Nodes {
		if n.Slug == "rfp" {
			return true
		}
	}
	return false
}

func (j job) country() string {
	for _, n := range j.JobLocations.Nodes {
		// Skip the "-Hybrid"/"-Remote" markers NTEN mixes into the same list.
		if !strings.HasPrefix(n.Name, "-") {
			return n.Name
		}
	}
	return ""
}

func Fetch(ctx context.Context, client *http.Client, ua string) ([]model.Listing, error) {
	resp, err := sources.Get(ctx, client, ua, pageURL)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &sources.BlockedError{Reason: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("%s: parse html: %w", Name, err)
	}

	raw := doc.Find("script#__NEXT_DATA__").First().Text()
	if raw == "" {
		return nil, &sources.BlockedError{Reason: "no __NEXT_DATA__ block in the page; the site's template changed"}
	}

	var data nextData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("%s: parse __NEXT_DATA__: %w", Name, err)
	}

	var out []model.Listing
	for _, j := range data.Props.PageProps.Jobs {
		if !j.isRFP() {
			continue
		}
		url := pageURL + j.Slug
		out = append(out, model.Listing{
			Source:  Name,
			ID:      url,
			Title:   strings.TrimSpace(j.Title),
			Buyer:   strings.TrimSpace(j.JobFields.Organization),
			Country: j.country(),
			// NTEN postings don't carry a deadline; it stays nil and passes
			// the deadline filter as "unknown" rather than being guessed.
			Deadline: nil,
			URL:      url,
		})
	}
	return out, nil
}
