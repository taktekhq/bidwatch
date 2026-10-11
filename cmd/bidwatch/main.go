// bidwatch checks a short list of public contract/RFP boards against a
// keyword and exclude list, and posts anything new to Slack. No AI or
// model call anywhere in it: plain rules, run on a schedule.
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
	"github.com/taktekhq/bidwatch/internal/sources/ntenjobs"
	"github.com/taktekhq/bidwatch/internal/sources/prosperglobal"
	"github.com/taktekhq/bidwatch/internal/sources/rfpdb"
	"github.com/taktekhq/bidwatch/internal/sources/savethechildren"
	"github.com/taktekhq/bidwatch/internal/sources/sbasubnet"
	"github.com/taktekhq/bidwatch/internal/sources/ungm"
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

	srcs := enabledSources(cfg)
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
	if err := st.Save(); err != nil {
		log.Fatalf("bidwatch: save state: %v", err)
	}

	appendRunLog(*dataDir, now, summary)
}

func enabledSources(cfg *config.Config) []sources.Source {
	all := []struct {
		on   bool
		name string
		fn   sources.Fetcher
	}{
		{cfg.Sources.DaleelMadani, daleelmadani.Name, daleelmadani.Fetch},
		{cfg.Sources.NTENJobs, ntenjobs.Name, ntenjobs.Fetch},
		{cfg.Sources.ASAESolutionsHub, asaesolutions.Name, asaesolutions.Fetch},
		{cfg.Sources.RFPdb, rfpdb.Name, rfpdb.Fetch},
		{cfg.Sources.UNGM, ungm.Name, ungm.Fetch},
		{cfg.Sources.AfDB, afdb.Name, afdb.Fetch},
		{cfg.Sources.SaveTheChildren, savethechildren.Name, savethechildren.Fetch},
		{cfg.Sources.ProsperGlobal, prosperglobal.Name, prosperglobal.Fetch},
		{cfg.Sources.SBASubNet, sbasubnet.Name, sbasubnet.Fetch},
	}
	var out []sources.Source
	for _, s := range all {
		if s.on {
			out = append(out, sources.Source{Name: s.name, Fetch: s.fn})
		}
	}
	return out
}

func appendRunLog(dataDir string, now time.Time, summary runner.Summary) {
	f, err := os.OpenFile(filepath.Join(dataDir, "runs.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("bidwatch: could not write runs.log: %v", err)
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s new=%d sources=%d\n", now.Format(time.RFC3339), len(summary.New), len(summary.Counts))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
