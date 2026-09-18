package defaults

import "strings"

// Provider describes one OpenAI-compatible endpoint Whale can talk to.
//
// Every provider here speaks the same chat-completions dialect, so adding one
// is a table entry rather than a client: what differs is the address, the name
// of the key, and where a person goes to get that key.
type Provider struct {
	ID    string
	Label string
	// BaseURL is the OpenAI-compatible root. Empty means the client's built-in
	// default, which is DeepSeek's.
	BaseURL string
	// KeyEnv is the environment variable holding the key. Empty means the
	// provider needs no credential at all.
	KeyEnv string
	// ConsoleURL is where a person creates a key. Setup offers to open it.
	ConsoleURL string
	// FreeTier, when set, is the one-line reason this provider costs nothing,
	// shown in the setup picker.
	FreeTier      string
	Models        []string
	ContextWindow int
	// OpenModels marks endpoints whose catalogue Whale cannot enumerate: a
	// local daemon serves whatever was pulled, a router serves hundreds, and
	// hosted families gain new variants constantly. For these, Models is a
	// shortlist for the picker and any model string is allowed through.
	OpenModels bool
}

// NeedsKey reports whether the provider requires a credential. A model served
// from the user's own machine does not.
func (p Provider) NeedsKey() bool { return p.KeyEnv != "" }

// DefaultModel is the first model listed for the provider.
func (p Provider) DefaultModel() string {
	if len(p.Models) == 0 {
		return ""
	}
	return p.Models[0]
}

const (
	ProviderGemini         = "gemini"
	ProviderOllama         = "ollama"
	ProviderGroq           = "groq"
	ProviderOpenRouter     = "openrouter"
	ProviderCerebras       = "cerebras"
	ProviderDeepSeek       = "deepseek"
	ProviderGitHubCopilot  = "github-copilot"
	ProviderOpenAICompat   = "openai-compatible"
	OllamaDefaultBaseURL   = "http://localhost:11434/v1"
	DefaultFallbackContext = 128_000
)

// providers is ordered: the free, easiest-to-start options come first, because
// this order is what the setup picker and the auto-detection both walk.
var providers = []Provider{
	{
		ID:         ProviderGemini,
		Label:      "Google Gemini",
		BaseURL:    "https://generativelanguage.googleapis.com/v1beta/openai",
		KeyEnv:     "GEMINI_API_KEY",
		ConsoleURL: "https://aistudio.google.com/apikey",
		FreeTier:   "free tier, no card required",
		// Two things decide this list. The "-latest" aliases track whatever
		// Google currently serves, because pinned versions get retired for new
		// users without warning and turn a working default into a 404. And the
		// lite tier leads, because the free quota is counted in requests per
		// day and an agent spends several on a single turn: the flash alias
		// currently allows 20 a day, which one debugging session exhausts.
		Models:        []string{"gemini-flash-lite-latest", "gemini-flash-latest", "gemini-pro-latest"},
		ContextWindow: 1_000_000,
		OpenModels:    true,
	},
	{
		ID:            ProviderOllama,
		Label:         "Ollama (local)",
		BaseURL:       OllamaDefaultBaseURL,
		ConsoleURL:    "https://ollama.com/download",
		FreeTier:      "runs on your machine, no key, works offline",
		Models:        []string{"qwen2.5-coder:7b", "qwen2.5-coder:14b", "llama3.1:8b"},
		ContextWindow: 32_768,
		OpenModels:    true,
	},
	{
		ID:            ProviderGroq,
		Label:         "Groq",
		BaseURL:       "https://api.groq.com/openai/v1",
		KeyEnv:        "GROQ_API_KEY",
		ConsoleURL:    "https://console.groq.com/keys",
		FreeTier:      "free tier, very fast",
		Models:        []string{"llama-3.3-70b-versatile", "llama-3.1-8b-instant"},
		ContextWindow: 128_000,
		OpenModels:    true,
	},
	{
		ID:            ProviderOpenRouter,
		Label:         "OpenRouter",
		BaseURL:       "https://openrouter.ai/api/v1",
		KeyEnv:        "OPENROUTER_API_KEY",
		ConsoleURL:    "https://openrouter.ai/keys",
		FreeTier:      "one key, many models carrying a :free tag",
		Models:        []string{"deepseek/deepseek-chat-v3.1:free", "qwen/qwen3-coder:free"},
		ContextWindow: 128_000,
		OpenModels:    true,
	},
	{
		ID:            ProviderCerebras,
		Label:         "Cerebras",
		BaseURL:       "https://api.cerebras.ai/v1",
		KeyEnv:        "CEREBRAS_API_KEY",
		ConsoleURL:    "https://cloud.cerebras.ai",
		FreeTier:      "free tier, very fast",
		Models:        []string{"qwen-3-coder-480b", "llama-3.3-70b"},
		ContextWindow: 128_000,
		OpenModels:    true,
	},
	{
		ID:            ProviderDeepSeek,
		Label:         "DeepSeek",
		KeyEnv:        "DEEPSEEK_API_KEY",
		ConsoleURL:    "https://platform.deepseek.com/api_keys",
		Models:        []string{DefaultModel, ProModel},
		ContextWindow: DeepSeekV4ContextWindow,
	},
	{
		ID:            ProviderGitHubCopilot,
		Label:         "GitHub Copilot",
		BaseURL:       "https://api.githubcopilot.com",
		KeyEnv:        "GITHUB_COPILOT_TOKEN",
		ConsoleURL:    "https://github.com/settings/copilot",
		Models:        []string{"gpt-4o", "claude-3.7-sonnet", "gemini-2.5-pro"},
		ContextWindow: DefaultFallbackContext,
	},
	{
		ID:            ProviderOpenAICompat,
		Label:         "Any OpenAI-compatible endpoint",
		KeyEnv:        "OPENAI_API_KEY",
		Models:        nil,
		ContextWindow: DefaultFallbackContext,
		OpenModels:    true,
	},
}

// DefaultProvider is what a fresh install reaches for when nothing else is
// configured and no local model is running.
const DefaultProvider = ProviderGemini

func Providers() []Provider { return append([]Provider(nil), providers...) }

// FreeProviders are the ones that cost nothing to start with, in the order the
// setup picker should offer them.
func FreeProviders() []Provider {
	out := make([]Provider, 0, len(providers))
	for _, p := range providers {
		if p.FreeTier != "" {
			out = append(out, p)
		}
	}
	return out
}

func ProviderByID(id string) (Provider, bool) {
	id = NormalizeProviderID(id)
	if id == "" {
		return Provider{}, false
	}
	for _, p := range providers {
		if p.ID == id {
			return p, true
		}
	}
	return Provider{}, false
}

// NormalizeProviderID only canonicalizes the spelling. It deliberately leaves
// an empty id empty: "no provider named" and "the default provider" are
// different questions, and conflating them made an unknown model inherit the
// default provider's context window.
func NormalizeProviderID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

// ModelsForProvider lists the models offered in pickers. It is a convenience
// list, not a restriction: any model string the endpoint accepts will work.
func ModelsForProvider(id string) []string {
	if p, ok := ProviderByID(id); ok && len(p.Models) > 0 {
		return append([]string(nil), p.Models...)
	}
	return SupportedModels()
}

func DefaultModelForProvider(id string) string {
	if p, ok := ProviderByID(id); ok {
		if m := p.DefaultModel(); m != "" {
			return m
		}
	}
	return DefaultModel
}

// ProviderKeyEnv is the environment variable a provider reads its key from.
func ProviderKeyEnv(id string) string {
	if p, ok := ProviderByID(id); ok {
		return p.KeyEnv
	}
	return "DEEPSEEK_API_KEY"
}

// ModelsAreOpen reports whether any model string is acceptable for a provider.
func ModelsAreOpen(id string) bool {
	p, ok := ProviderByID(id)
	return ok && p.OpenModels
}
