// Package config loads bidwatch's TOML config: which sources are on, the
// keyword/exclude lists, and how to reach Slack.
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

	Sources struct {
		DaleelMadani     bool `toml:"daleel_madani"`
		NTENJobs         bool `toml:"nten_jobs"`
		ASAESolutionsHub bool `toml:"asae_solutions_hub"`
		RFPdb            bool `toml:"rfpdb"`
		UNGM             bool `toml:"ungm"`
		AfDB             bool `toml:"afdb"`
		SaveTheChildren  bool `toml:"save_the_children"`
		ProsperGlobal    bool `toml:"prosper_global"`
		SBASubNet        bool `toml:"sba_subnet"`
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
}

// SlackToken resolves the Slack bot token: the SLACK_BOT_TOKEN environment
// variable first, then token_command, run as a subprocess. The token is
// returned to the caller and never printed or logged by this package.
func (c *Config) SlackToken() (string, error) {
	if t := os.Getenv("SLACK_BOT_TOKEN"); t != "" {
		return t, nil
	}
	if c.Slack.TokenCommand == "" {
		return "", fmt.Errorf("no SLACK_BOT_TOKEN and no slack.token_command in config")
	}
	fields := strings.Fields(c.Slack.TokenCommand)
	cmd := exec.Command(fields[0], fields[1:]...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("run token_command: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
