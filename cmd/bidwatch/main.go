// bidwatch checks a short list of public contract/RFP boards and a
// couple of lead sources (recent federal awards, end-of-life tech sites)
// against a keyword/exclude list or their own filters, and posts
// anything new to Slack. No AI or model call anywhere in it: plain
// rules, run on a schedule.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/taktekhq/bidwatch/internal/config"
	"github.com/taktekhq/bidwatch/internal/runner"
	"github.com/taktekhq/bidwatch/internal/slackpost"
	"github.com/taktekhq/bidwatch/internal/sources"
	"github.com/taktekhq/bidwatch/internal/sources/afdb"
	"github.com/taktekhq/bidwatch/internal/sources/asaesolutions"
	"github.com/taktekhq/bidwatch/internal/sources/daleelmadani"
	"github.com/taktekhq/bidwatch/internal/sources/eoltech"
	"github.com/taktekhq/bidwatch/internal/sources/ntenjobs"
	"github.com/taktekhq/bidwatch/internal/sources/prosperglobal"
	"github.com/taktekhq/bidwatch/internal/sources/rfpdb"
	"github.com/taktekhq/bidwatch/internal/sources/savethechildren"
	"github.com/taktekhq/bidwatch/internal/sources/sbasubnet"
	"github.com/taktekhq/bidwatch/internal/sources/ungm"
	"github.com/taktekhq/bidwatch/internal/sources/usaspending"
	"github.com/taktekhq/bidwatch/internal/state"
)

func main() {
	var (
		dataDir    = flag.String("data", envOr("BIDWATCH_DATA", "."), "folder holding state.json and runs.log (and config.toml, unless -config points elsewhere)")
		configPath = flag.String("config", "", "path to config.toml; defaults to <data>/config.toml")
		dryRun     = flag.Bool("dry-run", false, "fetch and match, print the message, but never post to Slack or update state")
	)
	flag.Parse()

	cfgPath := *configPath
	if cfgPath == "" {
		cfgPath = filepath.Join(*dataDir, "config.toml")
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("bidwatch: %v", err)
	}

	statePath := filepath.Join(*dataDir, "state.json")
	st, err := state.Load(statePath)
	if err != nil {
		log.Fatalf("bidwatch: load state: %v", err)
	}

	client := &http.Client{Timeout: time.Duration(cfg.RequestTimeout) * time.Second}
	ctx := context.Background()
	now := time.Now()

	firecrawlKey, err := cfg.FirecrawlAPIKey()
	if err != nil {
		log.Printf("bidwatch: %s disabled this run, could not resolve the Firecrawl API key: %v", daleelmadani.Name, err)
	}
	publicwwwKey, err := cfg.PublicWWWAPIKey()
	if err != nil {
		log.Printf("bidwatch: %s disabled this run, could not resolve the PublicWWW API key: %v", eoltech.Name, err)
	}

	srcs := enabledSources(cfg, firecrawlKey, publicwwwKey)
	summary := runner.Run(ctx, cfg, st, srcs, client, now)

	fmt.Println(summary.Message)

	if *dryRun {
		fmt.Println("\n(dry run: nothing posted, state not updated)")
		return
	}

	token, err := cfg.SlackToken()
	if err != nil {
		log.Fatalf("bidwatch: %v", err)
	}
	if err := slackpost.Post(ctx, client, token, cfg.Slack.Channel, summary.Message); err != nil {
		log.Fatalf("bidwatch: post to slack: %v", err)
	}

	for _, l := range summary.New {
		st.Mark(l.ID, now)
	}
	for _, l := range summary.PostedLeads {
		st.Mark(l.ID, now)
	}
	if err := st.Save(); err != nil {
		log.Fatalf("bidwatch: save state: %v", err)
	}

	appendRunLog(*dataDir, now, summary)
}

func enabledSources(cfg *config.Config, firecrawlKey, publicwwwKey string) []sources.Source {
	var out []sources.Source
	add := func(on bool, name string, fn sources.Fetcher) {
		if on {
			out = append(out, sources.Source{Name: name, Fetch: fn})
		}
	}

	// daleel_madani needs a Firecrawl key to run at all; no key means
	// this source is simply left out of the run, not attempted
	// unauthenticated.
	add(cfg.Sources.DaleelMadani && firecrawlKey != "", daleelmadani.Name, daleelmadani.New(firecrawlKey))
	add(cfg.Sources.NTENJobs, ntenjobs.Name, ntenjobs.Fetch)
	add(cfg.Sources.ASAESolutionsHub, asaesolutions.Name, asaesolutions.Fetch)
	add(cfg.Sources.RFPdb, rfpdb.Name, rfpdb.Fetch)
	add(cfg.Sources.UNGM, ungm.Name, ungm.Fetch)
	add(cfg.Sources.AfDB, afdb.Name, afdb.Fetch)
	add(cfg.Sources.SaveTheChildren, savethechildren.Name, savethechildren.Fetch)
	add(cfg.Sources.ProsperGlobal, prosperglobal.Name, prosperglobal.Fetch)
	add(cfg.Sources.SBASubNet, sbasubnet.Name, sbasubnet.Fetch)
	add(cfg.Sources.USAspendingAwards, usaspending.Name, usaspending.New(usaspending.Config{
		WindowDays:          cfg.USAspending.WindowDays,
		MinAmount:           cfg.USAspending.MinAmount,
		MaxAmount:           cfg.USAspending.MaxAmount,
		NAICSCodes:          cfg.USAspending.NAICSCodes,
		IncludeWords:        cfg.USAspending.IncludeWords,
		ExcludeWords:        cfg.USAspending.ExcludeWords,
		PreferSmallBusiness: cfg.USAspending.PreferSmallBusiness,
	}))
	// eol_tech_drupal7 needs a PublicWWW key issued on a paid plan; no
	// key means this source is left out of the run the same way
	// daleel_madani is.
	add(cfg.Sources.EOLTechDrupal7 && publicwwwKey != "", eoltech.Name, eoltech.New(publicwwwKey, cfg.EOLTech.PerPage))

	return out
}

func appendRunLog(dataDir string, now time.Time, summary runner.Summary) {
	f, err := os.OpenFile(filepath.Join(dataDir, "runs.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("bidwatch: could not write runs.log: %v", err)
		return
	}
	defer f.Close()

	credits := 0
	for _, c := range summary.Counts {
		credits += c.Credits
	}
	fmt.Fprintf(f, "%s new=%d leads_posted=%d leads_held=%d sources=%d credits=%d\n",
		now.Format(time.RFC3339), len(summary.New), len(summary.PostedLeads),
		len(summary.NewLeads)-len(summary.PostedLeads), len(summary.Counts), credits)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
