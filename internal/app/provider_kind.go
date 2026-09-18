package app

import (
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/usewhale/whale/internal/defaults"
)

const (
	ProviderDeepSeek      = defaults.ProviderDeepSeek
	ProviderGitHubCopilot = defaults.ProviderGitHubCopilot
	ProviderGemini        = defaults.ProviderGemini
	ProviderOllama        = defaults.ProviderOllama
	GitHubCopilotAuthURL  = "https://github.com/settings/copilot"
)

func normalizeProvider(provider string) string {
	if id := defaults.NormalizeProviderID(provider); id != "" {
		return id
	}
	return ProviderDeepSeek
}

func NormalizeProvider(provider string) string {
	return normalizeProvider(provider)
}

// ProviderConsoleURL is where a person goes to create a key for a provider.
func ProviderConsoleURL(provider string) string {
	if p, ok := defaults.ProviderByID(provider); ok {
		return p.ConsoleURL
	}
	return ""
}

// OpenInBrowser shows a page without making the caller care which OS it is on.
// Setup uses it so "get a key" is a keypress rather than a copied URL.
func OpenInBrowser(url string) error {
	if strings.TrimSpace(url) == "" {
		return nil
	}
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{url}
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		name, args = "xdg-open", []string{url}
	}
	return exec.Command(name, args...).Start()
}

func OpenGitHubCopilotAuth() error {
	return OpenInBrowser(GitHubCopilotAuthURL)
}

// providerAPIKey reads the key a provider expects from the environment.
// Providers that serve models from the local machine have no key env and
// always return empty, which callers read as "nothing to ask for".
func providerAPIKey(provider string) string {
	env := defaults.ProviderKeyEnv(provider)
	if env == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(env))
}

// ProviderNeedsKey reports whether a provider must be given a credential
// before it can answer.
func ProviderNeedsKey(provider string) bool {
	p, ok := defaults.ProviderByID(provider)
	return ok && p.NeedsKey()
}

// ollamaProbeTimeout is deliberately short: this runs on the startup path, and
// a machine without Ollama should not pay for the check.
const ollamaProbeTimeout = 250 * time.Millisecond

// LocalModelRunning reports whether an Ollama daemon is listening. It is a
// plain TCP dial rather than an HTTP call so that a busy daemon still counts
// as present.
func LocalModelRunning() bool {
	return localModelRunningAt(defaults.OllamaDefaultBaseURL)
}

func localModelRunningAt(baseURL string) bool {
	host := strings.TrimPrefix(strings.TrimPrefix(baseURL, "http://"), "https://")
	if i := strings.Index(host, "/"); i >= 0 {
		host = host[:i]
	}
	if host == "" {
		return false
	}
	if !strings.Contains(host, ":") {
		host += ":80"
	}
	conn, err := net.DialTimeout("tcp", host, ollamaProbeTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// ProviderUsable reports whether a provider could answer right now: either it
// needs no credential and is reachable, or a key for it is on hand.
func ProviderUsable(provider string, credentials Credentials) bool {
	p, ok := defaults.ProviderByID(provider)
	if !ok {
		return false
	}
	if !p.NeedsKey() {
		return p.ID != ProviderOllama || LocalModelRunning()
	}
	return strings.TrimSpace(os.Getenv(p.KeyEnv)) != "" || credentials.KeyFor(p.ID) != ""
}

// ResolveProvider returns the provider Whale should actually talk to.
//
// The configured provider wins whenever it can answer. It is only overridden
// when it cannot — no key anywhere, nothing listening — because the
// alternative is a fresh install that is dead on arrival with a key prompt for
// a paid account. In that case Whale prefers a provider the user already has a
// key for, then a model running on their own machine, and finally the free
// default, which setup will walk them through.
func ResolveProvider(configured string, credentials Credentials) string {
	id := normalizeProvider(configured)
	if ProviderUsable(id, credentials) {
		return id
	}
	for _, p := range defaults.Providers() {
		if p.ID == id || !p.NeedsKey() {
			continue
		}
		if ProviderUsable(p.ID, credentials) {
			return p.ID
		}
	}
	if LocalModelRunning() {
		return ProviderOllama
	}
	return defaults.DefaultProvider
}
