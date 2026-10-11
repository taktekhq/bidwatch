// Package runner ties the pieces together: fetch every configured source,
// apply the match rules, work out what's new against state, and build the
// one Slack message a run posts. It takes the source list as a parameter
// so it never imports a specific adapter itself, which keeps it testable
// with fakes.
package runner

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/taktekhq/bidwatch/internal/config"
	"github.com/taktekhq/bidwatch/internal/match"
	"github.com/taktekhq/bidwatch/internal/model"
	"github.com/taktekhq/bidwatch/internal/sources"
)

// SourceCount is what the run found at one source, for the per-source
// visibility line every message carries.
type SourceCount struct {
	Name    string
	Fetched int
	Matched int
	New     int
	Blocked string // non-empty if the source was blocked; Fetched/Matched/New are all 0 then
	Err     string // non-empty on an unexpected error other than a block
}

// Summary is what one run produced.
type Summary struct {
	Counts  []SourceCount
	New     []model.Listing // matched listings not already in state, across all sources
	Message string
}

// Seen is the subset of state.State a run needs; defined here so runner
// doesn't need to know about the on-disk format.
type Seen interface {
	IsNew(id string) bool
}

// Run fetches every source in srcs, in order, with cfg.DelayBetweenSources
// seconds of polite pause between each one. now is passed in rather than
// read from time.Now so tests are deterministic.
func Run(ctx context.Context, cfg *config.Config, seen Seen, srcs []sources.Source, client *http.Client, now time.Time) Summary {
	rules := match.Rules{Keywords: cfg.Match.Keywords, Excludes: cfg.Match.Excludes}

	var counts []SourceCount
	var newListings []model.Listing

	for i, src := range srcs {
		if i > 0 {
			time.Sleep(time.Duration(cfg.DelayBetweenSources) * time.Second)
		}

		sc := SourceCount{Name: src.Name}
		listings, err := src.Fetch(ctx, client, cfg.UserAgent)
		if err != nil {
			var be *sources.BlockedError
			if errors.As(err, &be) {
				sc.Blocked = be.Reason
			} else {
				sc.Err = err.Error()
			}
			counts = append(counts, sc)
			continue
		}

		sc.Fetched = len(listings)
		for _, l := range listings {
			res := match.Check(l, "", rules, now)
			if !res.Matched {
				continue
			}
			sc.Matched++
			if seen.IsNew(l.ID) {
				sc.New++
				newListings = append(newListings, l)
			}
		}
		counts = append(counts, sc)
	}

	sort.Slice(newListings, func(i, j int) bool {
		di, dj := newListings[i].Deadline, newListings[j].Deadline
		if di == nil && dj == nil {
			return newListings[i].Title < newListings[j].Title
		}
		if di == nil {
			return false
		}
		if dj == nil {
			return true
		}
		return di.Before(*dj)
	})

	return Summary{
		Counts:  counts,
		New:     newListings,
		Message: formatMessage(newListings, counts),
	}
}

func formatMessage(newListings []model.Listing, counts []SourceCount) string {
	var b strings.Builder
	if len(newListings) == 0 {
		b.WriteString("bidwatch: no new matches this week\n")
	} else {
		fmt.Fprintf(&b, "bidwatch: %d new match(es) this week\n", len(newListings))
		for _, l := range newListings {
			deadline := "no stated deadline"
			if l.Deadline != nil {
				deadline = l.Deadline.Format("2 Jan 2006")
			}
			buyer := l.Buyer
			if buyer == "" {
				buyer = "buyer not stated"
			}
			fmt.Fprintf(&b, "- %s, %s, deadline %s, %s\n", l.Title, buyer, deadline, l.URL)
		}
	}

	b.WriteString("\nSources: ")
	parts := make([]string, 0, len(counts))
	for _, c := range counts {
		switch {
		case c.Blocked != "":
			parts = append(parts, fmt.Sprintf("%s blocked (%s)", c.Name, c.Blocked))
		case c.Err != "":
			parts = append(parts, fmt.Sprintf("%s error (%s)", c.Name, c.Err))
		default:
			parts = append(parts, fmt.Sprintf("%s %d fetched, %d matched, %d new", c.Name, c.Fetched, c.Matched, c.New))
		}
	}
	b.WriteString(strings.Join(parts, "; "))
	return b.String()
}
