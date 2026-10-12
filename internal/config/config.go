// Package config loads bidwatch's TOML config: which sources are on, the
// keyword/exclude lists, and how to reach Slack and any metered API a
// source needs.
package config

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	UserAgent           string `toml:"user_agent"`
	DelayBetweenSources int    `toml:"delay_between_sources_seconds"`
	RequestTimeout      int    `toml:"request_timeout_seconds"`

	Slack struct {
		TokenCommand string `toml:"token_command"`
		Channel      string `toml:"channel"`
	} `toml:"slack"`

	Match struct {
		Keywords []string `toml:"keywords"`
		Excludes []string `toml:"excludes"`
	} `toml:"match"`

	// Firecrawl resolves the API key daleel_madani uses to read a
	// Cloudflare-blocked page. Empty (no env var, no command) disables
	// that source entirely rather than running it unauthenticated.
	Firecrawl struct {
		APIKeyCommand string `toml:"api_key_command"`
	} `toml:"firecrawl"`

	// USAspending has no key (the API is public); these are the filters
	// applied to its one query.
	USAspending struct {
		WindowDays          int      `toml:"window_days"`
		MinAmount           float64  `toml:"min_amount"`
		MaxAmount           float64  `toml:"max_amount"`
		NAICSCodes          []string `toml:"naics_codes"`
		IncludeWords        []string `toml:"include_words"`
		ExcludeWords        []string `toml:"exclude_words"`
		PreferSmallBusiness bool     `toml:"prefer_small_business"`
	} `toml:"usaspending"`

	// Ignore holds substrings matched, case-insensitively, against a
	// listing's title or URL, for anything Nizar already decided
	// against: it never posts again even if a source lists it under a
	// new URL (an RFP repost) or it would otherwise pass every filter
	// (a lead). Applies to every source, not just one.
	Ignore struct {
		Substrings []string `toml:"substrings"`
	} `toml:"ignore"`

	// EOLTech resolves the PublicWWW API key eol_tech_drupal7 uses.
	// PublicWWW's API needs a key issued on a paid plan; empty disables
	// the source entirely rather than running it unauthenticated.
	EOLTech struct {
		PublicWWWAPIKeyCommand string `toml:"publicwww_api_key_command"`
		PerPage                int    `toml:"per_page"`
	} `toml:"eol_tech"`

	// Leads caps how many of the leads sources' new finds (awards,
	// end-of-life sites) one run posts, so a daily run never floods the
	// channel; the rest carry over and can post on a later run.
	Leads struct {
		MaxPerRun int `toml:"max_per_run"`
	} `toml:"leads"`

	Sources struct {
		DaleelMadani      bool `toml:"daleel_madani"`
		NTENJobs          bool `toml:"nten_jobs"`
		ASAESolutionsHub  bool `toml:"asae_solutions_hub"`
		RFPdb             bool `toml:"rfpdb"`
		UNGM              bool `toml:"ungm"`
		AfDB              bool `toml:"afdb"`
		SaveTheChildren   bool `toml:"save_the_children"`
		ProsperGlobal     bool `toml:"prosper_global"`
		SBASubNet         bool `toml:"sba_subnet"`
		USAspendingAwards bool `toml:"usaspending_awards"`
		EOLTechDrupal7    bool `toml:"eol_tech_drupal7"`
	} `toml:"sources"`
}

// Load reads and parses the TOML file at path, then fills in defaults for
// anything left at zero value.
func Load(path string) (*Config, error) {
	var c Config
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	c.applyDefaults()
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.UserAgent == "" {
		c.UserAgent = "bidwatch/1.0 (+https://github.com/taktekhq/bidwatch)"
	}
	if c.DelayBetweenSources <= 0 {
		c.DelayBetweenSources = 3
	}
	if c.RequestTimeout <= 0 {
		c.RequestTimeout = 30
	}
	if c.USAspending.WindowDays <= 0 {
		c.USAspending.WindowDays = 7
	}
	if c.USAspending.MinAmount <= 0 {
		c.USAspending.MinAmount = 25000
	}
	if c.USAspending.MaxAmount <= 0 {
		c.USAspending.MaxAmount = 500000
	}
	if len(c.USAspending.NAICSCodes) == 0 {
		c.USAspending.NAICSCodes = []string{"541511", "541512", "541519", "518210"}
	}
	if len(c.USAspending.IncludeWords) == 0 {
		c.USAspending.IncludeWords = []string{
			"develop", "development", "design", "website", "web", "portal",
			"application", "app", "platform", "modernization", "modernisation",
			"migration", "integration", "dashboard", "ux", "accessibility",
			"508", "data pipeline", "automation",
		}
	}
	if len(c.USAspending.ExcludeWords) == 0 {
		c.USAspending.ExcludeWords = []string{
			"license", "licenses", "licence", "licences", "renewal",
			"renewals", "subscription", "subscriptions", "maintenance",
			"support", "hardware", "equipment", "radio", "medical", "lab",
			"seats", "seat",
		}
	}
	if c.EOLTech.PerPage <= 0 {
		c.EOLTech.PerPage = 25
	}
	if c.Leads.MaxPerRun <= 0 {
		c.Leads.MaxPerRun = 10
	}
}

// SlackToken resolves the Slack bot token: the SLACK_BOT_TOKEN environment
// variable first, then token_command, run as a subprocess. The token is
// returned to the caller and never printed or logged by this package.
// Unlike the optional API keys below, a missing Slack token is always an
// error: there is no "post nowhere" fallback.
func (c *Config) SlackToken() (string, error) {
	if t := os.Getenv("SLACK_BOT_TOKEN"); t != "" {
		return t, nil
	}
	if c.Slack.TokenCommand == "" {
		return "", fmt.Errorf("no SLACK_BOT_TOKEN and no slack.token_command in config")
	}
	tok, err := runCommand(c.Slack.TokenCommand)
	if err != nil {
		return "", fmt.Errorf("run token_command: %w", err)
	}
	return tok, nil
}

// FirecrawlAPIKey resolves daleel_madani's Firecrawl key: the
// FIRECRAWL_API_KEY environment variable first, then
// firecrawl.api_key_command. Neither set is not an error, just "" and
// nil: main.go reads that as "leave daleel_madani out of this run." A
// command that is set but fails is reported, so a broken setup is
// visible instead of silently looking the same as "no key at all."
func (c *Config) FirecrawlAPIKey() (string, error) {
	return resolveOptionalSecret("FIRECRAWL_API_KEY", c.Firecrawl.APIKeyCommand)
}

// PublicWWWAPIKey resolves eol_tech_drupal7's PublicWWW key the same way
// FirecrawlAPIKey resolves daleel_madani's: the PUBLICWWW_API_KEY
// environment variable first, then eol_tech.publicwww_api_key_command.
func (c *Config) PublicWWWAPIKey() (string, error) {
	return resolveOptionalSecret("PUBLICWWW_API_KEY", c.EOLTech.PublicWWWAPIKeyCommand)
}

// resolveOptionalSecret is SlackToken's env-var-then-command pattern,
// reused for a key whose absence disables one source rather than halting
// every run.
func resolveOptionalSecret(envVar, command string) (string, error) {
	if v := os.Getenv(envVar); v != "" {
		return v, nil
	}
	if command == "" {
		return "", nil
	}
	v, err := runCommand(command)
	if err != nil {
		return "", err
	}
	return v, nil
}

// runCommand runs command through the shell (sh -c) rather than a naive
// whitespace split, so a command that pipes bin/get's output through a
// second program to pull one field out of a JSON-wrapped secret (as
// Taktek's own Firecrawl key is stored) works the same as a single plain
// command. Its stdout is returned to the caller and never printed or
// logged here.
func runCommand(command string) (string, error) {
	out, err := exec.Command("sh", "-c", command).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
