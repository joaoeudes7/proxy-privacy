<p align="center">
  <h1 align="center">Proxy Privacy</h1>
  <p align="center"><strong>OpenAI-compatible privacy proxy for AI agents</strong></p>
  <p align="center">
    <a href="https://github.com/joaoeudes7/proxy-privacy/actions/workflows/build.yml"><img src="https://github.com/joaoeudes7/proxy-privacy/actions/workflows/build.yml/badge.svg" alt="CI"></a>
    <a href="https://github.com/joaoeudes7/proxy-privacy/releases"><img src="https://img.shields.io/github/v/release/joaoeudes7/proxy-privacy" alt="Release"></a>
    <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.26+-00ADD8" alt="Go"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-yellow" alt="License"></a>
    <a href="https://github.com/sponsors/joaoeudes7"><img src="https://img.shields.io/badge/Sponsor-%23EA4AAA?style=flat&logo=github&logoColor=white" alt="Sponsor"></a>
  </p>
</p>
</p>

> Maybe you want Privacy sometimes

**Proxy Privacy** is a lightweight, single-binary privacy proxy that sits between your AI agents and any OpenAI-compatible API provider. It intercepts requests to inject privacy params (`data_collection`, `zdr`) — the proxy never sends a request without them.

Works as a **drop-in proxy** for Claude Code, Codex CLI, OpenCode, Cline, Cursor, Copilot, and any OpenAI-compatible client.

## AI Agent Prompt

Paste this into any AI agent (Claude Code, OpenCode, Codex, Pi, etc.):

> Install proxy-privacy, then load the skill at `skills/proxy-provider-setup/SKILL.md` and follow its instructions to configure providers for me.

The agent will:

1. Download and install the binary
2. Load the provider-setup skill
3. Ask which providers you want (OpenAI, OpenRouter, Anthropic, etc.)
4. Auto-resolve their API base URLs
5. Write `~/.proxy-privacy/configs.json` with empty API keys
6. Tell you to fill in the keys manually

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/joaoeudes7/proxy-privacy/main/install.sh | sh
```

Or build from source:

```bash
git clone https://github.com/joaoeudes7/proxy-privacy.git
cd proxy-privacy
make install
```

## Quick Start

Create `~/.proxy-privacy/configs.json` with your providers:

```json
{
  "privacy_mode": "standard",
  "redact_secrets": true,
  "providers": [
    {
      "id": "openrouter",
      "name": "OpenRouter",
      "base_url": "https://openrouter.ai/api/v1",
      "api_key": "sk-or-...",
      "provider_prefs": {
        "order": ["novita", "deepinfra"],
        "sort": "throughput"
      }
    },
    {
      "id": "openai",
      "name": "OpenAI",
      "base_url": "https://api.openai.com/v1",
      "api_key": "sk-..."
    }
  ]
}
```

The first provider in the array is used by default. Select a different one with `--provider`:

```bash
proxy-privacy --provider openai
```

### Launch mode (proxy + agent)

Start proxy and agent in one command — env vars are set automatically:

```bash
proxy-privacy launch claude --provider openai
proxy-privacy launch codex
proxy-privacy launch opencode -m qwen3.5:8b -p strict
proxy-privacy launch pi --provider openrouter
```

Supported agents: `claude`, `codex`, `opencode`, `pi`, `cline`, `hermes`, `droid`, `copilot`, `codex-app`, `cursor`, `generic`

### Manual agent config

Run the proxy separately and point your agent to `http://localhost:8000/v1` with API key `sk-proxy-dev`:

| Agent | Environment variables |
|-------|----------------------|
| Claude Code | `ANTHROPIC_BASE_URL=http://localhost:8000` + `ANTHROPIC_API_KEY=sk-proxy-dev` |
| Codex CLI | `OPENAI_BASE_URL=http://localhost:8000/v1` + `OPENAI_API_KEY=sk-proxy-dev` |
| OpenCode | `OPENAI_BASE_URL=http://localhost:8000/v1` + `OPENAI_API_KEY=sk-proxy-dev` |
| Cline / Cursor / Copilot | Set `base_url` to `http://localhost:8000/v1`, key `sk-proxy-dev` |

Or use with any OpenAI SDK:

```python
from openai import OpenAI
client = OpenAI(api_key="sk-proxy-dev", base_url="http://localhost:8000/v1")
```

## Privacy

### Modes

| Mode | Flag | Behavior |
|------|------|----------|
| **Standard** (default) | `-p standard` | Tries strict first (`zdr` + no fallbacks). If rejected, falls back to `data_collection: "deny"` + allow fallbacks. Model is cached in memory. |
| **Strict** | `-p strict` | Always enforces `zdr: true` + no fallbacks. If rejected, request fails. |

### Params injected per request

| Where | Param | Standard | Strict |
|-------|-------|----------|--------|
| **Body** | `provider.data_collection` | `"deny"` | `"deny"` |
| **Body** | `provider.zdr` | — | `true` |
| **Body** | `provider.allow_fallbacks` | `true` | `false` |
| **Header** | `X-Title` | Project URL | Project URL |
| **Body** | `messages[].content` | Secrets redacted* | Secrets redacted* |
| **Body** | `messages[].content` | PII redacted† | PII redacted† |

\* API keys and tokens (`sk-...`, `AKIA...`) — on by default, toggle via `--redact-secrets`  
† Email addresses (`user@example.com`) — opt-in via `--redact-pii`

## Configuration

All config lives in a single file: `~/.proxy-privacy/configs.json`

### Priority

**CLI flag > env var > configs.json > default**

| Setting | CLI | Env var | configs.json field |
|---------|-----|---------|-------------------|
| Provider selection | `--provider` | — | `providers[].id` / `providers[].name` |
| Upstream URL | `--upstream` | `UPSTREAM_BASE_URL` | `providers[].base_url` |
| Upstream key | `--upstream-key` | `OPENAI_API_KEY` | `providers[].api_key` |
| Default model | `-m` / `--model` | — | `providers[].default_model` |
| Privacy mode | `-p` / `--privacy` | — | `privacy_mode` |
| Redact secrets | `--redact-secrets` | `PROXY_REDACT_SECRETS` | `redact_secrets` |
| Redact emails | `--redact-pii` | `PROXY_REDACT_PII` | `redact_pii` |
| Provider sort | `-s` / `--sort` | — | `provider_prefs.sort` |
| Provider order | `--provider-order` | — | `provider_prefs.order` |
| Debug upstream | `--debug-upstream` | `PROXY_DEBUG_UPSTREAM` | — |
| Trace directory | `--trace-dir` | `PROXY_TRACE_DIR` | — |
| Proxy API key | `-k` / `--api-key` | `PROXY_API_KEY` | — |

### configs.json reference

```json
{
  "privacy_mode": "standard",
  "redact_pii": false,
  "redact_secrets": true,
  "allowed_models": null,
  "providers": [
    {
      "id": "openai",
      "name": "OpenAI",
      "base_url": "https://api.openai.com/v1",
      "api_key": "sk-...",
      "default_model": "gpt-4o"
    }
  ]
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `privacy_mode` | string | `"standard"` | `"standard"` or `"strict"` |
| `redact_pii` | bool | `false` | Redact email addresses from messages |
| `redact_secrets` | bool | `true` | Redact API keys from messages |
| `allowed_models` | string[] | `null` | Restrict to specific models (optional) |
| `providers` | array | `[]` | Upstream provider configurations |

Each provider object:

| Field | Required | Description |
|-------|----------|-------------|
| `id` | yes | Unique identifier, used with `--provider` |
| `name` | yes | Display name |
| `base_url` | yes | Upstream API base URL |
| `api_key` | yes | API key for the upstream provider |
| `default_model` | no | Model to use when none specified |
| `default` | no | If `true`, this provider is selected by default |
| `provider_prefs` | no | OpenRouter routing prefs: `order`, `sort`, `zdr`, etc. |
| `models` | no | Per-model params: `temperature`, `reasoning`, `max_tokens`, etc. |

### CLI reference

```
proxy-privacy [OPTIONS]

  -p, --privacy string     Privacy mode: standard or strict (default "standard")
  -m, --model string       Default model (auto-fetches from /v1/models if empty)
  --provider string        Provider ID from configs.json (default: first)
  --upstream string        Upstream base URL (overrides all other config)
  --upstream-key string    Upstream API key (overrides all other config)
  --port int               Listen port (default 8000)
  --host string            Bind address (default "127.0.0.1")
  -k, --api-key string     Proxy API key (env: PROXY_API_KEY, default "sk-proxy-dev")
  -s, --sort string        Provider sort: price, throughput, latency (OpenRouter)
  --provider-order string  Comma-separated provider slugs for routing (OpenRouter)
  --redact-pii             Redact email addresses (default: false)
  --redact-secrets         Redact API keys and secrets (default: true)
  --debug-upstream         Log upstream requests (env: PROXY_DEBUG_UPSTREAM)
  --trace-dir string       Directory for log/trace files (env: PROXY_TRACE_DIR)
```

```
proxy-privacy launch <agent> [OPTIONS]

  -p, --privacy string     Privacy mode (default "standard")
  -m, --model string       Model for the agent
  --provider string        Provider ID from configs.json (default: first)
  -k, --api-key string     Proxy API key (default "sk-proxy-dev")
  --upstream string        Upstream base URL (overrides provider/config)
  --upstream-key string    Upstream API key (overrides provider/config)
  --debug-upstream         Log upstream requests
  --trace-dir string       Directory for log/trace files
  -s, --sort string        Provider sort: price, throughput, latency (OpenRouter)
  --provider-order string  Comma-separated provider slugs for routing (OpenRouter)
  --config-only            Configure without launching
  --redact-pii             Redact email addresses (default: false)
  --redact-secrets bool    Redact API keys and secrets (default: true)
```

## API Endpoints

| Method | Route | Description |
|--------|-------|-------------|
| POST | `/v1/chat/completions` | OpenAI chat (streaming supported) |
| POST | `/v1/completions` | Legacy completions |
| POST | `/v1/messages` | Anthropic format (Claude Code) |
| GET | `/v1/models` | List upstream models |
| GET | `/health` | Health check |

## Supported Providers

Any OpenAI-compatible API: OpenRouter, OpenAI, Anthropic, Together, Groq, DeepSeek, Perplexity, xAI (Grok), Google (Gemini), and more.

## Architecture

- **Single Go binary**, zero external dependencies
- **No database** — fully ephemeral (Ctrl+C to stop)
- **Single config file** at `~/.proxy-privacy/configs.json` (global settings + providers)
- **Anthropic compatibility** — `/v1/messages` translated to OpenAI on the fly
- **Provider error messages** — upstream errors shown in a friendly format
- **Client identification** — `X-Title` header sempre definido como URL do projeto

## Development

```bash
make test     # run all tests
make build    # cross-compile for all platforms
make clean    # remove build artifacts
```

### Agent compatibility tests

The proxy adapts model responses for different agent clients:

| Agent | Model format | Detection |
|-------|-------------|-----------|
| Claude | Anthropic (`type`, `id`, `display_name`, `created_at`) | `anthropic-version` header or `Claude` User-Agent |
| Codex | Codex (`slug`, `display_name`, `shell_type`, `visibility`) | `client_version` query param or `Codex` User-Agent |
| OpenCode | Standard OpenAI | Default |
| Pi | Standard OpenAI | Default |
