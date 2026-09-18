package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/usewhale/whale/internal/defaults"
	"github.com/usewhale/whale/internal/securefs"
)

var deepSeekAPIKeyPattern = regexp.MustCompile(`^sk-[A-Za-z0-9_-]{16,}$`)

// Credentials holds one key per provider.
//
// Keys is the general store; the two named fields predate it and are kept so
// an existing credentials.json keeps working. Everything reads through
// KeyFor/WithKey rather than touching the fields, so the legacy pair stays an
// on-disk detail.
type Credentials struct {
	DeepSeekAPIKey     string            `json:"deepseek_api_key,omitempty"`
	GitHubCopilotToken string            `json:"github_copilot_token,omitempty"`
	Keys               map[string]string `json:"keys,omitempty"`
}

func (c Credentials) KeyFor(provider string) string {
	switch defaults.NormalizeProviderID(provider) {
	case ProviderDeepSeek:
		if v := strings.TrimSpace(c.DeepSeekAPIKey); v != "" {
			return v
		}
	case ProviderGitHubCopilot:
		if v := strings.TrimSpace(c.GitHubCopilotToken); v != "" {
			return v
		}
	}
	return strings.TrimSpace(c.Keys[defaults.NormalizeProviderID(provider)])
}

// WithKey returns a copy carrying key for provider, writing through to the
// legacy field when there is one so older builds still find it.
func (c Credentials) WithKey(provider, key string) Credentials {
	id := defaults.NormalizeProviderID(provider)
	key = strings.TrimSpace(key)
	out := c
	out.Keys = make(map[string]string, len(c.Keys)+1)
	for k, v := range c.Keys {
		out.Keys[k] = v
	}
	if key == "" {
		delete(out.Keys, id)
	} else {
		out.Keys[id] = key
	}
	switch id {
	case ProviderDeepSeek:
		out.DeepSeekAPIKey = key
	case ProviderGitHubCopilot:
		out.GitHubCopilotToken = key
	}
	return out
}

// ConfiguredProviders lists providers that already have a usable key, from the
// environment or from disk.
func (c Credentials) ConfiguredProviders() []string {
	var out []string
	for _, p := range defaults.Providers() {
		if !p.NeedsKey() {
			continue
		}
		if strings.TrimSpace(os.Getenv(p.KeyEnv)) != "" || c.KeyFor(p.ID) != "" {
			out = append(out, p.ID)
		}
	}
	return out
}

func LoadProviderAPIKey(dataDir, provider string) (string, error) {
	if key := providerAPIKey(provider); key != "" {
		return key, nil
	}
	creds, err := LoadCredentials(dataDir)
	if err != nil {
		return "", err
	}
	return creds.KeyFor(provider), nil
}

func credentialsPath(dataDir string) string {
	return filepath.Join(dataDir, "credentials.json")
}

func LoadCredentials(dataDir string) (Credentials, error) {
	path := credentialsPath(dataDir)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Credentials{}, nil
		}
		return Credentials{}, fmt.Errorf("read credentials: %w", err)
	}
	var creds Credentials
	if err := json.Unmarshal(b, &creds); err != nil {
		return Credentials{}, fmt.Errorf("unmarshal credentials: %w", err)
	}
	return creds, nil
}

func SaveCredentials(dataDir string, creds Credentials) error {
	b, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	path := credentialsPath(dataDir)
	if err := securefs.WritePrivateFile(path, b); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	return nil
}

// ValidateProviderAPIKey checks a key as far as the provider's format allows.
// Only DeepSeek publishes a stable shape; for everyone else an empty key is
// the only thing we can be sure is wrong, and guessing a pattern would reject
// keys that work.
func ValidateProviderAPIKey(provider, key string) error {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return fmt.Errorf("empty key")
	}
	if defaults.NormalizeProviderID(provider) == ProviderDeepSeek && !deepSeekAPIKeyPattern.MatchString(trimmed) {
		return fmt.Errorf("invalid DeepSeek API key format")
	}
	return nil
}

func ValidateDeepSeekAPIKey(key string) error {
	return ValidateProviderAPIKey(ProviderDeepSeek, key)
}

func LoadDeepSeekAPIKey(dataDir string) (string, error) {
	return LoadProviderAPIKey(dataDir, ProviderDeepSeek)
}
