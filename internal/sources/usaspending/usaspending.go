// Package usaspending adapts USAspending.gov's public award-search API
// (api.usaspending.gov, no auth, no key) to find small, recent federal
// software/web/IT contract awards. These are leads to approach the
// winning company as a subcontractor, not RFPs: usaspending shows what
// already got awarded, so there's nothing open to bid on by the time it
// shows up here.
package usaspending

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/taktekhq/bidwatch/internal/model"
	"github.com/taktekhq/bidwatch/internal/sources"
)

const Name = "usaspending-awards"

// var, not const, so tests can point it at a local fixture server.
var apiURL = "https://api.usaspending.gov/api/v2/search/spending_by_award/"

// Config is the small set of filters this adapter applies; usaspending
// has no keyword list of its own, so config.go fills these in from
// bidwatch's config file rather than from [match].
type Config struct {
	WindowDays int      // new awards with an action date in the trailing N days
	MinAmount  float64  // award amount floor, inclusive
	MaxAmount  float64  // award amount ceiling, inclusive
	NAICSCodes []string // e.g. 541511, 541512, 541519, 518210
}

type award struct {
	AwardID    string  `json:"Award ID"`
	Recipient  string  `json:"Recipient Name"`
	Amount     float64 `json:"Award Amount"`
	Agency     string  `json:"Awarding Agency"`
	Desc       string  `json:"Description"`
	InternalID string  `json:"generated_internal_id"`
}

type searchResponse struct {
	Results []award `json:"results"`
}

// New returns a Fetcher that posts one query to USAspending for new
// awards in the trailing cfg.WindowDays, under cfg.NAICSCodes, with an
// amount between cfg.MinAmount and cfg.MaxAmount.
func New(cfg Config) sources.Fetcher {
	return func(ctx context.Context, client *http.Client, ua string) ([]model.Listing, error) {
		now := time.Now().UTC()
		start := now.AddDate(0, 0, -cfg.WindowDays)

		reqBody, err := json.Marshal(map[string]any{
			"filters": map[string]any{
				"award_type_codes": []string{"B", "C", "D"},
				"time_period": []map[string]string{{
					"start_date": start.Format("2006-01-02"),
					"end_date":   now.Format("2006-01-02"),
					"date_type":  "action_date",
				}},
				"naics_codes": cfg.NAICSCodes,
				"award_amounts": []map[string]float64{{
					"lower_bound": cfg.MinAmount,
					"upper_bound": cfg.MaxAmount,
				}},
			},
			"fields": []string{
				"Award ID", "Recipient Name", "Award Amount",
				"Awarding Agency", "Description", "generated_internal_id",
			},
			"page":  1,
			"limit": 100,
			"sort":  "Award Amount",
			"order": "desc",
		})
		if err != nil {
			return nil, fmt.Errorf("%s: build request: %w", Name, err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(reqBody))
		if err != nil {
			return nil, fmt.Errorf("%s: build request: %w", Name, err)
		}
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", Name, err)
		}
		defer resp.Body.Close()

		raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if err != nil {
			return nil, fmt.Errorf("%s: read response: %w", Name, err)
		}
		if resp.StatusCode != http.StatusOK {
			// A public API answering with an error is not the same as a
			// source refusing a request it recognized as a bot; this is
			// a plain error, not sources.BlockedError.
			return nil, fmt.Errorf("%s: HTTP %d from the award search API", Name, resp.StatusCode)
		}

		var parsed searchResponse
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, fmt.Errorf("%s: parse response: %w", Name, err)
		}

		var out []model.Listing
		for _, a := range parsed.Results {
			if a.Recipient == "" || a.InternalID == "" {
				continue
			}
			link := "https://www.usaspending.gov/award/" + a.InternalID
			out = append(out, model.Listing{
				Source: Name,
				ID:     link,
				Kind:   model.KindLead,
				Title:  strings.TrimSpace(a.Recipient),
				Buyer:  strings.TrimSpace(a.Agency),
				Detail: fmt.Sprintf("%s award from %s: %s", formatAmount(a.Amount), strings.TrimSpace(a.Agency), shorten(a.Desc, 160)),
				URL:    link,
			})
		}
		return out, nil
	}
}

func formatAmount(amount float64) string {
	whole := int64(amount + 0.5)
	s := fmt.Sprintf("%d", whole)
	// Comma-group the digits by hand: no fmt verb does this, and pulling
	// in a formatting dependency for one line isn't worth it.
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var grouped []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			grouped = append(grouped, ',')
		}
		grouped = append(grouped, c)
	}
	out := "$" + string(grouped)
	if neg {
		out = "-" + out
	}
	return out
}

func shorten(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max]) + "..."
}
