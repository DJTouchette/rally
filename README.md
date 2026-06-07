# Rally

Your tickets, as local markdown.

Rally syncs your assigned work items from Jira and Linear into plain markdown files in your repo. Your AI coding agent and you both get to work from a local, git-trackable backlog instead of poking at external APIs mid-task — and the credentials for those APIs never touch disk.

Rally is the project-management layer of the [Rivet](https://github.com/djtouchette/rivet) ecosystem. It's a CLI that keeps your backlog local, and it leans on [Vaulty](https://github.com/djtouchette/vaulty) so OAuth tokens are brokered, never stored in plaintext.

## What It Does

- **Sync assigned tickets** from connected providers into `.rally/tickets/*.md`
- **Normalizes** provider-specific statuses and priorities into one consistent vocabulary
- **Local-first** — tickets are markdown, committable, readable, diff-able
- **Pin tickets** to surface them as working context for your agent
- **Push status back** — `start` and `done` move the ticket in the source system too
- **Secrets stay in Vaulty** — tokens are injected at runtime, never written to `.rally/`

## Quick Start

```bash
# Install
go install github.com/djtouchette/rally/cmd/rally@latest

# Connect a provider (OAuth flow; tokens go into Vaulty, not disk)
rally connect jira

# Pull your assigned tickets into .rally/tickets/
rally sync

# What should I work on?
rally next

# Start it (marks in-progress, pins it, pushes status upstream)
rally start PROJ-123

# ...do the work...

rally done PROJ-123
```

## Setting up a provider (one-time)

You can authenticate either with a **personal API key** (simplest — no app, no
browser) or with **OAuth**.

### Option A — API key (recommended for personal use)

Create a personal token in the provider's settings, then connect with
`--api-key`. Rally **prompts for the key** (hidden), verifies it, and stores it
in Vaulty for you — no OAuth app, no callback URL, no manual `vaulty set`.

```bash
# Linear — Settings → API → Personal API keys (key starts with lin_api_)
rally connect linear --api-key

# Jira — id.atlassian.com → Security → API tokens (needs your email + site)
rally connect jira --api-key --email you@your-co.com --site your-co.atlassian.net
```

You'll be prompted to paste the key; it's verified against the API and saved to
Vaulty (never to disk). The key can also be piped (`echo $KEY | rally connect
linear --api-key`) or supplied via `vaulty exec --secrets RALLY_LINEAR_TOKEN`.

### Option B — OAuth (for shared/multi-user apps)

Rally authenticates via **OAuth**, so you first create an OAuth app on the
provider to get a client ID and secret. Both providers require the callback URL
to match exactly, so rally listens on a **fixed** port — register this redirect
URI in your app:

```
http://localhost:8412/callback
```

(Override the port with `RALLY_OAUTH_PORT` if 8412 is taken — register whatever
you choose.)

**Jira** — create an OAuth 2.0 (3LO) app at <https://developer.atlassian.com/console/myapps/>,
add the *Jira API* permission, and set the callback URL above.

**Linear** — create an OAuth application under Settings → API → OAuth
applications, with `read,write` scope and the callback URL above. (Linear access
tokens expire after 24h; rally stores the refresh token and renews automatically.)

Then store the credentials in Vaulty and connect:

```bash
vaulty set RALLY_LINEAR_CLIENT_ID --value <client-id> --domains linear.app
vaulty set RALLY_LINEAR_CLIENT_SECRET --value <client-secret> --domains linear.app
vaulty exec --secrets RALLY_LINEAR_CLIENT_ID,RALLY_LINEAR_CLIENT_SECRET -- rally connect linear
```

The OAuth tokens that come back are stored in Vaulty too — never on disk.

## Commands

| Command | What it does |
|---------|-------------|
| `rally connect <provider>` | OAuth authorize a provider; store tokens in Vaulty |
| `rally sync` | Pull assigned tickets from all connected providers to `.rally/tickets/` |
| `rally list` | List synced tickets (`--status`, `--priority`, `--provider`, `--json`) |
| `rally next` | Show the highest-priority actionable ticket |
| `rally start <id>` | Mark in-progress, pin it, push status upstream |
| `rally done [id]` | Mark done and unpin (infers the id if only one is in progress) |
| `rally status` | Connection health, secret status, last sync, ticket counts |
| `rally pin <id>` | Pin a ticket as working context (`--note`) |
| `rally unpin <id>` | Remove a pin |
| `rally pinned` | List pinned tickets (`--json`) |

## Local Layout

Everything Rally writes lives under `.rally/` and is safe to commit:

```
.rally/
  config.yaml      ← connected providers + secret declarations (no values)
  state.json       ← per-ticket sync hashes for change detection
  pins.json        ← pinned ticket list
  tickets/         ← one markdown file per ticket, e.g. jira-PROJ-123.md
```

Each ticket is normalized markdown with title, status, priority, labels, and a description (Jira's Atlassian Document Format is flattened to readable text). Statuses collapse to `backlog | todo | in_progress | in_review | done | cancelled`; priorities to `urgent | high | medium | low`.

## Secrets via Vaulty

Rally never stores credentials in `.rally/`. On `connect`, the OAuth tokens are handed to Vaulty; on `sync` and friends, Vaulty injects them into the environment at runtime. `config.yaml` only declares *which* secrets a connection needs (with domain hints for Vaulty's policy engine) — never the secret values themselves. That keeps the whole `.rally/` directory committable.

## Providers

| Provider | Status |
|----------|--------|
| **Jira** (Cloud) | ✅ Full support — OAuth, pagination, token refresh, ADF descriptions, status transitions, multi-site |
| **Linear** | ✅ Full support — OAuth, GraphQL assigned-issue sync (paginated), state/priority normalization, status push-back |

## Building

```bash
make build       # build ./rally
make test        # run tests
make vet         # static analysis
make install     # go install
```

Requires Go 1.25+.

## License

MIT
