# bidwatch

Checks a short list of public contract and RFP boards once a week, matches
new postings against your own keyword list, and posts anything new to
Slack. Plain HTTP requests and plain rules: no browser, no account, no AI
or model call anywhere in it.

bidwatch never applies to anything and never bids. It tells you what showed
up; you decide what to do with it.

## What it reads

Each source is a small, separate adapter, so one going down never breaks
the others. Checked 12 Oct 2026:

| Source | What it's for | Status |
|---|---|---|
| NTEN jobs (nten.org/jobs), filtered to postings NTEN itself tags "RFP" | US nonprofits and tech-for-good orgs | Working |
| The RFP Database (rfpdb.com), web development / software / information technology categories | General RFP board, any buyer | Working |
| Mercy Corps / Prosper Global tenders (prosperglobal.org/tenders) | Mercy Corps programs worldwide | Working |
| SBA SUBNet (legacy.sba.gov) | US federal prime contractors' subcontracting posts | Working (mostly construction/trades right now; the keyword filter just finds nothing some weeks) |
| Daleel Madani calls for proposals (daleel-madani.org) | Lebanese NGOs and INGOs | Blocked: Cloudflare challenge on every request |
| ASAE Solutions Hub (solutionshq.asaecenter.org) | US trade and professional associations | Blocked: HTTP 403 on every path tried |
| UNGM public notice search (ungm.org) | UN agencies | Blocked: the result list only loads through a JavaScript call this adapter doesn't reproduce; a plain GET always returns the empty search template |
| AfDB corporate procurement RSS feed | African Development Bank | Blocked: Cloudflare challenge, even on the feed URL itself |
| Save the Children tenders (savethechildren.net) | Save the Children country offices | Blocked: Cloudflare challenge on every request |

A blocked source is never routed around: no browser, no cookie jar, no
different user agent pretending to be something else. Each blocked
adapter still runs its one request every time, in case the block lifts,
and the weekly Slack message always says which sources are blocked, so a
dead source stays visible instead of silently dropping out.

## Install

Go 1.27 or later.

```
go install github.com/taktekhq/bidwatch/cmd/bidwatch@latest
```

Or clone the repo and run `go build -o bidwatch ./cmd/bidwatch`.

## Set up a data folder

bidwatch keeps everything outside the repo, in a folder you choose with
`--data` or the `BIDWATCH_DATA` environment variable. Nothing in this
folder is meant to be shared or committed anywhere.

```
mkdir -p ~/bidwatch-data
cp config.example.toml ~/bidwatch-data/config.toml
```

Edit `config.toml`: your Slack channel, how to get a bot token, your
keyword and exclude lists, and which sources to turn on. The comments in
`config.example.toml` explain each setting.

### Get a Slack bot token

Create a Slack app (or reuse one) with the `chat:write` scope, install it
to your workspace, and invite the bot to the channel you want it posting
in. bidwatch reads the token one of two ways:

- Set the `SLACK_BOT_TOKEN` environment variable, or
- Set `token_command` in `config.toml` to a shell command whose output is
  the token, for example a password manager CLI. bidwatch runs that
  command itself and never prints or logs the token.

## Run it

```
export BIDWATCH_DATA=~/bidwatch-data
bidwatch
```

That fetches every source turned on in `config.toml`, applies the match
rules, drops anything already posted on an earlier run, and posts one
Slack message: either the list of new matches (title, buyer, deadline,
link) or a one-line "no new matches", always followed by a per-source
count line so a dead source is visible.

```
bidwatch -dry-run   # print the message, post nothing, change no state
```

`state.json` and `runs.log` always live in `-data`. The config file is
`<data>/config.toml` by default; pass `-config /path/to/your.toml` to keep
the config somewhere else (for example, in a repo under a name other than
`config.toml`) while state still lives in `-data`.

## What the message looks like

```
bidwatch: 2 new match(es) this week
- Website Redesign and Development (RFP), CAMWS, deadline 30 Oct 2026, https://www.nten.org/jobs/website-redesign-and-development-rfp
- RFP #: P-2027-2 SS4A Safety Action Plan, buyer not stated, deadline 4 Nov 2026, https://www.rfpdb.com/view/document/name/RFP-...

Sources: nten-jobs 23 fetched, 1 matched, 1 new; rfpdb 0 fetched, 0 matched, 0 new; prosper-global 4 fetched, 0 matched, 0 new; sba-subnet 10 fetched, 0 matched, 0 new; daleel-madani blocked (Cloudflare challenge on the calls-for-proposal page); asae-solutions-hub blocked (HTTP 403 from the site's load balancer on every path tried); ungm blocked (notices load through a JavaScript POST this adapter does not reproduce; the plain GET always returns the empty search template); afdb blocked (Cloudflare challenge on the RSS feed itself); save-the-children blocked (Cloudflare challenge on the tenders page)
```

`state.json` in the data folder remembers every listing's URL once it's
been posted, so the same opportunity never shows up twice. `runs.log`
gets one line per run with a timestamp and the new-match count.

## Matching rules

A listing matches when its title (plus a short description, where a
source gives one) contains at least one of your `keywords`, contains none
of your `excludes`, and its deadline, if the source states one, is still
in the future. A listing with no stated deadline passes through rather
than being guessed at. All of this is plain string matching, case
insensitive; there's no model call anywhere in bidwatch, so it costs
nothing to run and never "forgets" a rule.

## Running it for someone else

bidwatch has no idea who "you" are beyond `config.toml`. Give a second
person their own data folder and their own config, and point `--data` at
it:

```
bidwatch -data ~/bidwatch-data-someone-else
```

Nothing about the first person's data folder is read or touched.

## Schedule it

- **macOS (launchd):** `deploy/com.example.bidwatch.plist` is a template
  for a weekly Monday-morning run. Copy it into
  `~/Library/LaunchAgents/`, edit the three paths inside, and
  `launchctl bootstrap` it. The comments in the file walk through it.
- **Linux (systemd):** `deploy/bidwatch.service` and `deploy/bidwatch.timer`
  do the same thing as a systemd user timer. See the comments in
  `bidwatch.service` for the install steps.
- **Anything else (cron):** a plain weekly cron line works too, since
  bidwatch is a single run with no server to keep alive:
  `47 8 * * 1 BIDWATCH_DATA=$HOME/bidwatch-data /path/to/bidwatch`.

Each source is one polite HTTP request (a few seconds apart from the
others, per `delay_between_sources_seconds`), so a weekly run finishes in
well under a minute; there's nothing here that needs a lock against
itself unless you're worried about two schedulers overlapping, in which
case any flock-style wrapper in front of the binary works.

## Testing

```
go test ./...
go vet ./...
```

The match and state logic has its own unit tests. Every source adapter is
tested against a small, trimmed fixture taken from a real response (saved
under each package's `testdata/`), including the blocked ones, which are
tested against a mock server that returns the same Cloudflare-challenge
or HTTP-403 shape the real site returned when this was written. Nothing in
the test suite makes a real network request.

## License

MIT. See `LICENSE`.
