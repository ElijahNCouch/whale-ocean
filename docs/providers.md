# Providers

Whale talks to any OpenAI-compatible chat-completions endpoint. Choosing a
provider is choosing three things: an address, a key, and a model name.

```bash
whale setup
```

Setup lists the providers, marks the ones that cost nothing, shows which are
already usable on this machine, and offers to open the page where a key is
created. Everything below is what it does, written out.

## Starting for free

| Provider | Key needed | Notes |
|---|---|---|
| **Google Gemini** | `GEMINI_API_KEY` | Free tier, no card. The default. |
| **Ollama** | none | Runs on your machine. Works offline. |
| **Groq** | `GROQ_API_KEY` | Free tier, very fast. |
| **OpenRouter** | `OPENROUTER_API_KEY` | One key, many models tagged `:free`. |
| **Cerebras** | `CEREBRAS_API_KEY` | Free tier, very fast. |

Paid providers — DeepSeek, GitHub Copilot, and any other OpenAI-compatible
endpoint — are configured the same way.

### Google Gemini (default)

```bash
whale setup            # pick Google Gemini, paste the key
# or
GEMINI_API_KEY=... whale
```

The default model is `gemini-flash-lite-latest`. Two things decide that.

The `-latest` aliases are deliberate: pinned Gemini versions get retired for
new users without notice, and an alias keeps working. And the lite tier leads
because the free quota is counted in **requests per day**, not tokens — the
flash alias currently allows 20, which a single debugging session exhausts,
since one agent turn spends several requests. Lite answers less well but
answers all day, which is the trade this agent is built around.

`gemini-flash-latest` and `gemini-pro-latest` are also offered; pass either
with `--model` when you want the better answer and have the quota for it.

### Ollama (no key, offline)

```bash
# install from https://ollama.com/download, then
ollama pull qwen2.5-coder:7b
whale --provider ollama
```

Whale checks whether an Ollama server is listening on `localhost:11434`. If one
is, and nothing else is configured, it uses it without being asked.

A 7B model is weaker at choosing tools than a hosted model. That is what the
operational tools are for — see "Tools instead of knowledge" in the README.

## What Whale picks when you do not say

Auto-detection only applies to the `whale` command, and only when you have not
named a provider in config or on the command line. In order:

1. A provider whose key is already in the environment or in
   `credentials.json`.
2. A local Ollama server, if one is listening.
3. Google Gemini, which then asks you to run `whale setup`.

A provider you configured is never silently replaced. If it cannot answer, that
is reported rather than routed around, because the alternative hides the real
problem and sends your configured model to an endpoint that has never heard of
it.

## Configuring a provider explicitly

```toml
# .whale/config.toml or ~/.whale/config.toml
provider = "groq"
model    = "llama-3.3-70b-versatile"
```

```bash
whale --provider ollama --model qwen2.5-coder:14b
```

Model names are not validated against a fixed list for providers that serve
catalogues Whale cannot enumerate — a local daemon serves whatever you pulled,
a router serves hundreds, and hosted families gain variants constantly. Pass
any name the endpoint accepts.

## Keys

A key can come from the environment or from `credentials.json` in your data
directory, which `whale setup` writes with owner-only permissions. The
environment wins.

Each provider reads its own variable: `GEMINI_API_KEY`, `GROQ_API_KEY`,
`OPENROUTER_API_KEY`, `CEREBRAS_API_KEY`, `DEEPSEEK_API_KEY`,
`GITHUB_COPILOT_TOKEN`. Keys for several providers can be stored at once;
setup never overwrites the others.

`whale doctor` reports which provider is selected and whether its key is
present.

## Any other OpenAI-compatible endpoint

```toml
provider = "openai-compatible"
model    = "your-model"

[api]
base_url = "https://your-gateway.example/v1"
```

The key comes from `OPENAI_API_KEY`.

## Free tiers and rate limits

Free tiers return HTTP 429 and 503, and Whale retries with backoff, so a
request that seems to hang is usually a free tier throttling you.

Two different limits bite. Rate limits (requests per minute) clear in seconds.
Daily request quotas do not — `RESOURCE_EXHAUSTED` naming a
`free_tier_requests` metric means that model is done until the quota resets.
Each model has its own bucket, so `--model gemini-flash-lite-latest` or
`--provider groq` gets you working again immediately.

## DeepSeek and GitHub Copilot

DeepSeek is the only provider whose extensions Whale uses: the thinking block,
`reasoning_effort`, the Responses API and prefix completion. Those fields are
sent only to DeepSeek, because every other OpenAI-compatible endpoint rejects
them.

GitHub Copilot needs a GitHub OAuth token, or a fine-grained token with the
**Copilot Requests** permission, in `GITHUB_COPILOT_TOKEN`. Whale exchanges it
for the short-lived Copilot API token the endpoint requires; a classic PAT or a
normal GitHub CLI token is not enough.
