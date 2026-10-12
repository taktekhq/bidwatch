# bidwatch

Checks a short list of public contract and RFP boards, matches new
postings against your own keyword list, and posts anything new to Slack.
A couple of extra sources go further upstream, before any RFP exists:
companies just awarded a federal contract (worth approaching as a
subcontractor) and live end-of-life tech sites (worth a quiet, free
"your site needs attention" note). Plain HTTP requests and plain rules
otherwise: no browser, no account, no AI or model call anywhere in it.

bidwatch never applies to anything and never bids. It tells you what
showed up; you decide what to do with it.

## What it reads

Each source is a small, separate adapter, so one going down never breaks
the others. Checked 12 Oct 2026:

| Source | What it's for | Status |
|---|---|---|
| NTEN jobs (nten.org/jobs), filtered to postings NTEN itself tags "RFP" | US nonprofits and tech-for-good orgs | Working |
| The RFP Database (rfpdb.com), web development / software / information technology categories | General RFP board, any buyer | Working |
| Mercy Corps / Prosper Global tenders (prosperglobal.org/tenders) | Mercy Corps programs worldwide | Working |
| SBA SUBNet (legacy.sba.gov) | US federal prime contractors' subcontracting posts | Working (mostly construction/trades right now; the keyword filter just finds nothing some weeks) |
| Daleel Madani calls for proposals (daleel-madani.org), read through Firecrawl | Lebanese NGOs and INGOs | Working (needs a Firecrawl API key; see below) |
| ASAE Solutions Hub (solutionshq.asaecenter.org) | US trade and professional associations | Blocked: HTTP 403 on every path tried |
| UNGM public notice search (ungm.org) | UN agencies | Blocked: the result list only loads through a JavaScript call this adapter doesn't reproduce; a plain GET always returns the empty search template |
| AfDB corporate procurement RSS feed | African Development Bank | Blocked: Cloudflare challenge, even on the feed URL itself |
| Save the Children tenders (savethechildren.net) | Save the Children country offices | Blocked: Cloudflare challenge on every request |
| USAspending.gov award search (api.usaspending.gov, public, no key) | Small new federal software/web/IT contract awards, as subcontracting leads | Working |
| PublicWWW source-code search (publicwww.com), for live Drupal 7 sites | End-of-life tech sites, as outreach leads | Needs a paid PublicWWW API key; off by default (see below) |
| CORDIS bulk open-data export (cordis.europa.eu/data), for Horizon Europe projects with no dissemination website listed | Recently started EU-funded research projects, as outreach leads | Built and tested, but off by default: the website-listed field doesn't support the signal (see below) |

A blocked source is never routed around: no browser, no cookie jar, no
different user agent pretending to be something else. Each blocked
adapter still runs its one request every time, in case the block lifts,
and the Slack message always says which sources are blocked, so a dead
source stays visible instead of silently dropping out.

### Daleel Madani, through Firecrawl

A plain HTTP GET against Daleel Madani's calls-for-proposals board gets
a Cloudflare challenge page on every request (checked 12 Oct 2026). This
adapter reads the page through the [Firecrawl](https://www.firecrawl.dev)
API instead, with proxy **"basic" only**: never "auto" (which can
escalate to "stealth", i.e. ban evasion), "stealth" or "enhanced". If
even "basic" gets challenged, the source reports blocked rather than
asking Firecrawl to try harder, the same never-route-around-a-block rule
every other adapter follows.

Set `firecrawl.api_key_command` in `config.toml` (or the
`FIRECRAWL_API_KEY` environment variable) to your own Firecrawl key.
With neither set, this source is left out of every run, not attempted
unauthenticated. One Firecrawl call per run, roughly 1 to 5 credits
depending on the page; credits actually spent show up in the Slack
message's per-source line and in `runs.log`.

### Leads: USAspending awards, end-of-life tech sites, and CORDIS

Sources that look upstream of any posted RFP post to a separate "Leads
(no RFP)" section of the same message, capped at `leads.max_per_run`
(default 10) so a daily run never floods the channel. Anything over the
cap just isn't marked seen yet, so it's still eligible to post on a
later run rather than being lost; a given award, domain or project
still posts at most once, ever, the same as every other listing.

- **USAspending.gov awards**: a public, no-key JSON API search for new
  contract awards in the trailing `usaspending.window_days` (default 7),
  in `usaspending.naics_codes` (default: software/web/IT), with an
  amount between `usaspending.min_amount` and `usaspending.max_amount`
  (default $25,000 to $500,000). The recipient is the lead: a company to
  approach as a subcontractor, not a buyer to bid against; by the time an
  award shows up here, there's nothing left to bid on.

  The query asks for `date_type: "new_awards_only"`, USAspending's own
  documented way to exclude a *modification* to an older award (one
  whose action date just moved, not a new award), and sorts newest
  first. A description still has to contain a whole word from
  `usaspending.include_words` (develop, website, platform, dashboard,
  508... the default list reads like build work) and none from
  `usaspending.exclude_words` (license, renewal, subscription,
  maintenance, support, hardware, radio, medical, lab... the default
  list reads like a purchase, not a project). At most one lead posts
  per recipient per run (the newest of that recipient's qualifying
  awards). When `usaspending.prefer_small_business` is on (default
  true), a recipient whose business categories (read from the award's
  own detail page; `spending_by_award` doesn't carry them) mention
  "small" sorts ahead of the rest, rather than dropping anyone: a
  lookup that fails just skips the boost for that one candidate.
- **End-of-life tech (Drupal 7)**: Drupal stopped shipping security
  patches for Drupal 7 in January 2025, so a live Drupal 7 site is a
  lead worth a free, no-strings note. Found through
  [PublicWWW](https://publicwww.com)'s source-code search API, filtered
  server-side to `.org` domains with a `site:org` query. PublicWWW's
  documented API requires a key issued on a **paid plan**
  (publicwww.com/docs/api/: "Getting a key... A paid plan is required");
  there's no free, unauthenticated way to get a real result count from
  it, so this source is **off by default**
  (`sources.eol_tech_drupal7 = false`). Set
  `eol_tech.publicwww_api_key_command` (or `PUBLICWWW_API_KEY`) and turn
  the source on if you have a PublicWWW plan.
- **CORDIS Horizon Europe leads**: built and tested, but **off by
  default** (`sources.cordis_leads = false`), and left off on purpose.
  The idea: a Horizon Europe grant agreement requires a dissemination
  plan, and most multi-partner projects (RIA/IA/CSA) set up a project
  website in their first few months, so a recently started project
  (`cordis.window_days`, default 45) with no project website listed is
  a lead to write to its coordinator. The source reads CORDIS's own
  free bulk CSV export (cordis.europa.eu/data, one GET, no key) rather
  than its search page, which only renders through JavaScript.

  Checked 12 Oct 2026: CORDIS's "project website" field
  (`webLink.csv`, type `relatedWebsite`) turned out to be an
  editorially curated field, not a systematic one. Counted by project
  start year across the whole HORIZON dataset, only 13% of 2021-start
  projects have one recorded, dropping to 1-2% for 2022-2025 and under
  0.2% for 2026-start projects, even though 2021-2023 projects are
  3-5 years old now and overwhelmingly have real sites. A direct
  spot-check of 3 "no website listed" 2022 consortium projects
  (SolDAC, Photo2Fuel, SSH CENTRE) found all 3 already running real,
  live project websites CORDIS had simply never recorded.

  That alone could be forgiven if the signal held for the narrow slice
  this adapter actually targets: a project within `window_days` of its
  own start date, where "too new to have a site yet" is plausible on
  its own. The one real run on 12 Oct 2026 tested that directly. The
  newest eligible cohort (started 2026-10-01, 11 days old: DigiWaves,
  FarmHeroes, PROCLAIM, EXCELY, RESSORT, the 5 pasted into the task
  report) held up on a live web search: no dedicated site for any of
  the 5. But the same run's much larger pool before the per-run cap,
  the 2026-09-01 cohort (41 days old, several hundred eligible
  projects, every one of them also lacking a recorded website), told a
  different story: a 6-project spot-check (ARION, PostQat, OceanNexus,
  WUNDERWORK, EOSC4ALL, SHIFT 5.0) found 3 of 6 already running real,
  live dissemination sites (openaire.eu/arion-101287335, postqat.com,
  oceannexus.eu) that CORDIS had not recorded: roughly a 50%
  false-positive rate at the older end of the very window this source
  uses by default. Narrowing the window would only shrink the pool
  back to a handful a month (the scale opposite.md's own research
  found), not fix the real problem: the field says almost nothing
  about whether a project has a site, only whether CORDIS wrote about
  it, so widening the window for useful volume just lets more
  already-built sites back in as false positives. Full writeup in
  `internal/sources/cordis`'s own doc comment; recheck the premise
  there before setting `cordis_leads = true`.

### The ignore list

`[ignore]` `substrings` is a flat list of case-insensitive substrings
matched against every listing's title and URL, from every source, RFP
or lead. It's for a standing "no": something already decided against
(the config comments are the place to say why), so it never posts
again even if a source lists it under a new URL. It's checked before
matching, so an ignored listing never reaches `[match]` or a lead's own
filter, and the per-source count line shows how many got dropped this
way (`"N ignored"`) so the rule's effect stays visible.

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
keyword and exclude lists, which sources to turn on, and, if you want
them, a Firecrawl key (Daleel Madani) and a PublicWWW key (end-of-life
tech leads). The comments in `config.example.toml` explain each setting.

### Get a Slack bot token

Create a Slack app (or reuse one) with the `chat:write` scope, install it
to your workspace, and invite the bot to the channel you want it posting
in. bidwatch reads the token one of two ways:

- Set the `SLACK_BOT_TOKEN` environment variable, or
- Set `token_command` in `config.toml` to a shell command whose output is
  the token, for example a password manager CLI. bidwatch runs that
  command itself (through `sh -c`, so a pipeline works too, for example
  piping a secret through a second program to pull one field out of a
  JSON-wrapped value) and never prints or logs the token.

Every other API key in `config.toml` (Firecrawl, PublicWWW) resolves the
same way: an environment variable first, then its own `*_command`
setting. Unlike the Slack token, a missing key for one of these just
leaves that one source out of the run; it's never fatal to the rest.

## Run it

```
export BIDWATCH_DATA=~/bidwatch-data
bidwatch
```

That fetches every source turned on in `config.toml`, applies the match
rules (or, for a lead source, its own filter), drops anything already
posted on an earlier run, and posts one Slack message: the list of new
RFP matches (or a one-line "no new matches"), a "Leads (no RFP)"
section, and always a per-source count line so a dead source is visible.

```
bidwatch -dry-run   # print the message, post nothing, change no state
```

`state.json` and `runs.log` always live in `-data`. The config file is
`<data>/config.toml` by default; pass `-config /path/to/your.toml` to keep
the config somewhere else (for example, in a repo under a name other than
`config.toml`) while state still lives in `-data`.

## What the message looks like

```
bidwatch: 2 new match(es) today
- Website Redesign and Development (RFP), CAMWS, deadline 30 Oct 2026, https://www.nten.org/jobs/website-redesign-and-development-rfp
- RFP #: P-2027-2 SS4A Safety Action Plan, buyer not stated, deadline 4 Nov 2026, https://www.rfpdb.com/view/document/name/RFP-...

Leads (no RFP): 2 new
- Good Build Works LLC, $142,000 award from Department of Commerce: WEBSITE REDESIGN AND MODERNIZATION OF AGENCY PUBLIC PORTAL, https://www.usaspending.gov/award/CONT_AWD_...
- sofworld.org, Drupal 7, found in page source: <meta name="Generator" content="Drupal 7 (http://drupal.org)" />, first seen 12 Oct 2026, https://www.sofworld.org/

Sources: nten-jobs 23 fetched, 1 matched, 1 new; rfpdb 0 fetched, 0 matched, 0 new; prosper-global 4 fetched, 0 matched, 0 new; sba-subnet 10 fetched, 0 matched, 0 new; daleel-madani 20 fetched, 0 matched, 0 new, 1 ignored, 1 credit(s); asae-solutions-hub blocked (HTTP 403 from the site's load balancer on every path tried); ungm blocked (notices load through a JavaScript POST this adapter does not reproduce; the plain GET always returns the empty search template); afdb blocked (Cloudflare challenge on the RSS feed itself); save-the-children blocked (Cloudflare challenge on the tenders page); usaspending-awards 34 fetched, 1 matched, 1 new
```

The `daleel-madani` line above shows "1 ignored": one of its 20 fetched calls matched an entry in `[ignore].substrings` and never reached matching at all. `usaspending-awards` shows how much `include_words`/`exclude_words`/the per-recipient cap narrow things down: 34 new awards fetched, 1 survives as a real build-work lead.

`state.json` in the data folder remembers every listing's ID once it's
been posted (an RFP's URL, an award's link, or a bare domain), so the
same opportunity, award or site never shows up twice. `runs.log` gets
one line per run with a timestamp, the new-match count, how many leads
posted versus were held back by the cap, and any metered API credits
spent.

## Matching rules

An RFP-board listing matches when its title (plus a short description,
where a source gives one) contains at least one of your `keywords`,
contains none of your `excludes`, and its deadline, if the source states
one, is still in the future. A listing with no stated deadline passes
through rather than being guessed at. All of this is plain string
matching, case insensitive; there's no model call anywhere in bidwatch,
so it costs nothing to run and never "forgets" a rule.

A lead (USAspending award, end-of-life tech site, CORDIS project) never
runs through `[match]` at all: its own adapter already applied a
source-specific filter (an amount band and NAICS codes, a Drupal 7
site-code match, or a start-date window and website check), so
`keywords`/`excludes` would either do nothing useful or wrongly drop a
real lead whose description doesn't happen to contain one of your RFP
keywords.

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
  for a daily 08:47 run. Copy it into `~/Library/LaunchAgents/`, edit the
  paths inside, and `launchctl bootstrap` it. The comments in the file
  walk through it.
- **Linux (systemd):** `deploy/bidwatch.service` and `deploy/bidwatch.timer`
  do the same thing as a systemd user timer. See the comments in
  `bidwatch.service` for the install steps.
- **Anything else (cron):** a plain daily cron line works too, since
  bidwatch is a single run with no server to keep alive:
  `47 8 * * * BIDWATCH_DATA=$HOME/bidwatch-data /path/to/bidwatch`.

Each source is one polite HTTP request (a few seconds apart from the
others, per `delay_between_sources_seconds`), so a run finishes in well
under a minute; there's nothing here that needs a lock against itself
unless you're worried about two schedulers overlapping, in which case any
flock-style wrapper in front of the binary works (`deploy/com.example.bidwatch.plist`
shows one such wrapper in its comments).

## Testing

```
go test ./...
go vet ./...
```

The match, state and runner logic has its own unit tests. Every source
adapter is tested against a small, trimmed fixture taken from a real
response (saved under each package's `testdata/`), including the
blocked ones, which are tested against a mock server that returns the
same Cloudflare-challenge, HTTP-403 or bot-challenge shape the real
source returned when this was written, and the Firecrawl- and
PublicWWW-backed adapters, which are tested against a mock server
standing in for Firecrawl or the PublicWWW API, not the real site.
CORDIS's tests build a small zip fixture in code (a few CSV rows, the
same ";"-delimited shape as the real export) rather than committing a
binary file, so the fixture itself stays readable in the test source.
Nothing in the test suite makes a real network request.

## License

MIT. See `LICENSE`.
