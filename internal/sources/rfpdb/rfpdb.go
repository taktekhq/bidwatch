// Package rfpdb adapts three categories of the RFP Database
// (rfpdb.com): web development, software, and information technology.
//
// Each category page is plain server-rendered HTML. A listing, when one
// exists, carries schema.org CreativeWork/RequestForProposal microdata
// (itemprop="name", "url", "description", "endDate"), the same "read the
// page's own structured markup" pattern the craft page recommends for
// JSON-LD, just expressed as itemprop attributes instead of a JSON-LD
// block. An empty category renders "No results" with zero such elements,
// which this adapter reads as zero listings, not a block. Checked 12 Oct
// 2026: all three categories were empty; the microdata shape above was
// confirmed against /view/all, which does carry live listings in the same
// template.
package rfpdb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/taktekhq/bidwatch/internal/model"
	"github.com/taktekhq/bidwatch/internal/sources"
)

const Name = "rfpdb"

// One request per category page, as the brief asks: web design, software, IT.
var categoryURLs = []string{
	"https://www.rfpdb.com/view/category/name/web-development",
	"https://www.rfpdb.com/view/category/name/software",
	"https://www.rfpdb.com/view/category/name/information-technology",
}

func Fetch(ctx context.Context, client *http.Client, ua string) ([]model.Listing, error) {
	var out []model.Listing
	var blocked []string
	for _, url := range categoryURLs {
		listings, err := fetchCategory(ctx, client, ua, url)
		if err != nil {
			var be *sources.BlockedError
			if errors.As(err, &be) {
				blocked = append(blocked, fmt.Sprintf("%s (%s)", url, be.Reason))
				continue
			}
			return out, fmt.Errorf("%s: %w", Name, err)
		}
		out = append(out, listings...)
		time.Sleep(2 * time.Second) // polite pause between the three category pages
	}
	if len(blocked) == len(categoryURLs) {
		return nil, &sources.BlockedError{Reason: strings.Join(blocked, "; ")}
	}
	return out, nil
}

func fetchCategory(ctx context.Context, client *http.Client, ua, url string) ([]model.Listing, error) {
	resp, err := sources.Get(ctx, client, ua, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &sources.BlockedError{Reason: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	var out []model.Listing
	doc.Find(`li[itemtype*="RequestForProposal"]`).Each(func(_ int, li *goquery.Selection) {
		title := strings.TrimSpace(li.Find(`[itemprop="name"]`).First().Text())
		href, _ := li.Find(`[itemprop="url"]`).First().Attr("href")
		if title == "" || href == "" {
			return
		}
		fullURL := href
		if strings.HasPrefix(href, "/") {
			fullURL = "https://www.rfpdb.com" + href
		}

		country := strings.TrimSpace(li.Find(`[itemprop="addressRegion"]`).First().Text())
		if c := strings.TrimSpace(li.Find(`[itemprop="addressCountry"]`).First().Text()); c != "" {
			if country != "" {
				country = country + ", " + c
			} else {
				country = c
			}
		}

		var deadline *time.Time
		if dt, ok := li.Find(`[itemprop="endDate"]`).First().Attr("datetime"); ok && dt != "" {
			if t, err := time.Parse(time.RFC3339, dt); err == nil {
				deadline = &t
			}
		}

		out = append(out, model.Listing{
			Source:   Name,
			ID:       fullURL,
			Title:    title,
			Buyer:    "", // RFPdb's microdata doesn't name the buyer separately from the title
			Country:  country,
			Deadline: deadline,
			URL:      fullURL,
		})
	})
	return out, nil
}
