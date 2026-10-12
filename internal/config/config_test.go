package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTOML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_FillsDefaults(t *testing.T) {
	path := writeTOML(t, `
[slack]
channel = "C0C8CUJVCT0"
`)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.UserAgent == "" {
		t.Error("expected a default user agent")
	}
	if c.DelayBetweenSources != 3 {
		t.Errorf("DelayBetweenSources = %d, want default 3", c.DelayBetweenSources)
	}
	if c.RequestTimeout != 30 {
		t.Errorf("RequestTimeout = %d, want default 30", c.RequestTimeout)
	}
}

func TestLoad_ReadsSourceToggles(t *testing.T) {
	path := writeTOML(t, `
[sources]
nten_jobs = true
daleel_madani = false
`)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Sources.NTENJobs {
		t.Error("expected nten_jobs on")
	}
	if c.Sources.DaleelMadani {
		t.Error("expected daleel_madani off")
	}
}

func TestSlackToken_EnvVarWins(t *testing.T) {
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test-token")
	c := &Config{}
	c.Slack.TokenCommand = "echo should-not-run"
	tok, err := c.SlackToken()
	if err != nil {
		t.Fatal(err)
	}
	if tok != "xoxb-test-token" {
		t.Errorf("token = %q", tok)
	}
}

func TestSlackToken_RunsCommandWhenNoEnvVar(t *testing.T) {
	c := &Config{}
	c.Slack.TokenCommand = "echo xoxb-from-command"
	tok, err := c.SlackToken()
	if err != nil {
		t.Fatal(err)
	}
	if tok != "xoxb-from-command" {
		t.Errorf("token = %q", tok)
	}
}

func TestSlackToken_NoSourceIsAnError(t *testing.T) {
	c := &Config{}
	if _, err := c.SlackToken(); err == nil {
		t.Fatal("expected an error with no env var and no token_command")
	}
}

func TestSlackToken_CommandRunsThroughAShellSoPipesWork(t *testing.T) {
	c := &Config{}
	c.Slack.TokenCommand = `echo '{"token":"xoxb-wrapped"}' | python3 -c 'import json,sys;print(json.load(sys.stdin)["token"])'`
	tok, err := c.SlackToken()
	if err != nil {
		t.Fatal(err)
	}
	if tok != "xoxb-wrapped" {
		t.Errorf("token = %q", tok)
	}
}

func TestFirecrawlAPIKey_EnvVarWins(t *testing.T) {
	t.Setenv("FIRECRAWL_API_KEY", "fc-test-key")
	c := &Config{}
	c.Firecrawl.APIKeyCommand = "echo should-not-run"
	key, err := c.FirecrawlAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if key != "fc-test-key" {
		t.Errorf("key = %q", key)
	}
}

func TestFirecrawlAPIKey_EmptyWithNoSourceIsNotAnError(t *testing.T) {
	c := &Config{}
	key, err := c.FirecrawlAPIKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "" {
		t.Errorf("key = %q, want empty (daleel_madani disabled, not fatal)", key)
	}
}

func TestFirecrawlAPIKey_FailingCommandIsAnError(t *testing.T) {
	c := &Config{}
	c.Firecrawl.APIKeyCommand = "false"
	if _, err := c.FirecrawlAPIKey(); err == nil {
		t.Fatal("expected an error when the configured command fails, so a broken setup is visible")
	}
}

func TestPublicWWWAPIKey_EmptyWithNoSourceIsNotAnError(t *testing.T) {
	c := &Config{}
	key, err := c.PublicWWWAPIKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "" {
		t.Errorf("key = %q, want empty (eol_tech_drupal7 disabled, not fatal)", key)
	}
}

func TestLoad_FillsLeadsAndUSAspendingDefaults(t *testing.T) {
	path := writeTOML(t, `
[slack]
channel = "C0C8CUJVCT0"
`)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Leads.MaxPerRun != 10 {
		t.Errorf("Leads.MaxPerRun = %d, want default 10", c.Leads.MaxPerRun)
	}
	if c.USAspending.WindowDays != 7 {
		t.Errorf("USAspending.WindowDays = %d, want default 7", c.USAspending.WindowDays)
	}
	if c.USAspending.MinAmount != 25000 {
		t.Errorf("USAspending.MinAmount = %v, want default 25000", c.USAspending.MinAmount)
	}
	if c.USAspending.MaxAmount != 500000 {
		t.Errorf("USAspending.MaxAmount = %v, want default 500000", c.USAspending.MaxAmount)
	}
	if len(c.USAspending.NAICSCodes) != 4 {
		t.Errorf("USAspending.NAICSCodes = %v, want 4 defaults", c.USAspending.NAICSCodes)
	}
	if c.EOLTech.PerPage != 25 {
		t.Errorf("EOLTech.PerPage = %d, want default 25", c.EOLTech.PerPage)
	}
	if len(c.USAspending.IncludeWords) == 0 {
		t.Error("expected default USAspending.IncludeWords to be filled in")
	}
	if len(c.USAspending.ExcludeWords) == 0 {
		t.Error("expected default USAspending.ExcludeWords to be filled in")
	}
}

func TestLoad_ReadsIgnoreList(t *testing.T) {
	path := writeTOML(t, `
[ignore]
substrings = ["call-proposals-social-media-agency", "some-other-decided-against"]
`)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Ignore.Substrings) != 2 {
		t.Fatalf("Ignore.Substrings = %v, want 2 entries", c.Ignore.Substrings)
	}
}

func TestLoad_ReadsUSAspendingFilterOverrides(t *testing.T) {
	path := writeTOML(t, `
[usaspending]
include_words = ["website"]
exclude_words = ["license"]
prefer_small_business = true
`)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.USAspending.IncludeWords) != 1 || c.USAspending.IncludeWords[0] != "website" {
		t.Errorf("IncludeWords = %v, want [\"website\"] (not the default list)", c.USAspending.IncludeWords)
	}
	if !c.USAspending.PreferSmallBusiness {
		t.Error("expected PreferSmallBusiness true")
	}
}

func TestLoad_ReadsUSAspendingAndEOLTechToggles(t *testing.T) {
	path := writeTOML(t, `
[sources]
usaspending_awards = true
eol_tech_drupal7 = false
`)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Sources.USAspendingAwards {
		t.Error("expected usaspending_awards on")
	}
	if c.Sources.EOLTechDrupal7 {
		t.Error("expected eol_tech_drupal7 off")
	}
}
