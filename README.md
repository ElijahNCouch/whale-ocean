# Whale

<p align="center">
  <img src="docs/logo.svg" alt="Whale — an AI coding agent for your terminal" width="640">
</p>

<p align="center">
  An AI coding agent for your terminal.<br>
  Free to start, runs on any model, and gets its competence from tools.
</p>

---

## Quick start

```bash
npm install -g @usewhale/whale     # or: brew install usewhale/tap/whale
whale setup                        # pick a provider — the free ones are listed first
whale                              # start a session
```

`whale setup` shows which providers cost nothing, marks the ones already usable
on this machine, and opens the page where a key is created. The default is
Google Gemini's free tier, which needs no card, on the lite model — free quotas
are counted in requests per day, and an agent spends several per turn. If an
Ollama server is already running locally, Whale uses that instead and asks for
nothing at all.

Nothing configured, no keys anywhere? Whale still starts, and tells you what it
needs.

## Free by default

| Provider | Key | Notes |
|---|---|---|
| **Google Gemini** | free tier | The default. No card. |
| **Ollama** | none | Local. Offline. Auto-detected. |
| **Groq** | free tier | Very fast. |
| **OpenRouter** | free tier | One key, many `:free` models. |
| **Cerebras** | free tier | Very fast. |

DeepSeek, GitHub Copilot and any other OpenAI-compatible endpoint work the same
way. Keys for several providers can be stored at once, and the one you
configured is never silently swapped. See [Providers](docs/providers.md).

## Tools instead of knowledge

A small free model is worse than a large paid one at *recalling* things: the
exact `kubectl` flags, which `git` incantation shows what you meant, where a
crash loop leaves its reason. It is much better at *choosing* from a list.

So Whale gives it a list. Alongside the usual file, search, shell and web
tools, it offers each operational enquiry as its own named tool:

| | |
|---|---|
| `env_info` | OS, architecture, and which CLIs exist here, with versions |
| `git_status` `git_log` `git_diff` | repository state, history, changes |
| `docker_ps` `docker_logs` | containers and their output |
| `k8s_get` `k8s_describe` `k8s_logs` `k8s_context` | cluster state, and why a pod will not start |
| `terraform_plan` | what would change, without changing it |
| `gh_run_list` | recent CI runs |
| `systemd_status` | unit state with recent logs |
| `port_check` `dns_lookup` | is it down, or unreachable, or just wrong? |

Three rules keep this honest:

- **Every one of them only reads.** No operational tool here can be the reason
  an incident got worse.
- **A tool only appears when the binary behind it exists.** The catalogue a
  model sees describes what this machine can actually do, rather than a wish
  list it will hallucinate its way through. `env_info` is the exception — it
  always exists, because it is what reports the rest.
- **Arguments are passed as argv, never through a shell.** A value containing a
  semicolon is a value.

A capable model does not need any of this and will reach for `shell_run`. A
weaker one stops guessing flags and starts picking tools, which is most of the
difference between the two.

## What else it does

| | | |
|---|---|---|
| **MCP servers** | Connect external tool servers | [docs/mcp.md](docs/mcp.md) |
| **Skills** | Load domain expertise on demand | [docs/skills.md](docs/skills.md) |
| **Subagents** | Focused child-agent roles | [docs/agents.md](docs/agents.md) |
| **Workflows** | Script multi-agent orchestration in JavaScript | [docs/workflows.md](docs/workflows.md) |
| **Plugins** | Extend the runtime | [docs/plugins.md](docs/plugins.md) |
| **Hooks** | Run scripts on lifecycle events | [docs/hooks.md](docs/hooks.md) |

Workflows are off by default; enable `Dynamic workflows` via `/config`, or add
`[workflows] enabled = true` to `.whale/config.local.toml`.

## Interfaces

| | |
|---|---|
| `whale` | Interactive terminal session |
| `whale exec "..."` | One-shot, scriptable, `--json` available |
| `whale doctor` | What is configured, and what is missing |

## Configuration

Config lives in `.whale/config.toml` (project) or `~/.whale/config.toml`
(global). Keys are stored separately in `credentials.json`, owner-readable
only. See [Configuration](docs/configuration.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Security issues: [SECURITY.md](SECURITY.md).

## Credits

Whale stands on the shoulders of giants:

- [Charmbracelet](https://charm.sh) — Bubble Tea, Lip Gloss, Glamour, and the entire TUI ecosystem
- [fastschema/qjs](https://github.com/fastschema/qjs) — QuickJS Go bindings for workflow scripting
- [spf13/cobra](https://github.com/spf13/cobra) — CLI framework
- [alecthomas/chroma](https://github.com/alecthomas/chroma) — Syntax highlighting
- [yuin/goldmark](https://github.com/yuin/goldmark) — Markdown parsing
- [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) — MCP protocol support
- [tetratelabs/wazero](https://github.com/tetratelabs/wazero) — Pure-Go WebAssembly runtime

And the many open-source libraries we depend on — thank you.

> Not affiliated with DeepSeek Inc., Google, or any other model provider.
