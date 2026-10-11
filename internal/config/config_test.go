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
