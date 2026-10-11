// Package slackpost posts one message to a Slack channel using the Web
// API's chat.postMessage, the same bearer-token call taktekbot/bin/slack
// uses. The token is passed in by the caller and is never printed or
// logged here.
package slackpost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

// Post sends text to channel (a Slack channel ID, e.g. "C0123456789") as
// the bot identified by token.
func Post(ctx context.Context, client *http.Client, token, channel, text string) error {
	form := url.Values{"channel": {channel}, "text": {text}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/chat.postMessage", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var r response
	if err := json.Unmarshal(body, &r); err != nil {
		return fmt.Errorf("parse slack response: %w", err)
	}
	if !r.OK {
		return fmt.Errorf("slack chat.postMessage: %s", r.Error)
	}
	return nil
}
