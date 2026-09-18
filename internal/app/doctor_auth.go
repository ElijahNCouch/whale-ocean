package app

import (
	"fmt"
	"strings"
)

func doctorCheckAPIKey(dataDir string, provider string) (DoctorCheck, apiKeySource, string) {
	key, source, err := resolveProviderAPIKey(dataDir, provider)
	if err != nil {
		return DoctorCheck{
			Label:  "api key",
			Level:  DoctorFail,
			Detail: err.Error(),
		}, apiKeySourceMissing, ""
	}
	if strings.TrimSpace(key) == "" {
		// A provider that serves models from this machine has no key to check,
		// so reporting a missing one would be a failure nobody can fix.
		if !ProviderNeedsKey(normalizeProvider(provider)) {
			return DoctorCheck{
				Label:  "api key",
				Level:  DoctorOK,
				Detail: "not required by " + normalizeProvider(provider),
			}, apiKeySourceMissing, ""
		}
		return DoctorCheck{
			Label:  "api key",
			Level:  DoctorFail,
			Detail: "not configured — run `whale setup`, or set " + providerEnvName(provider),
		}, apiKeySourceMissing, ""
	}
	switch source {
	case apiKeySourceEnv:
		return DoctorCheck{
			Label:  "api key",
			Level:  DoctorOK,
			Detail: fmt.Sprintf("set via env %s (%s)", providerEnvName(provider), tailKey(key)),
		}, source, key
	case apiKeySourceCredentials:
		return DoctorCheck{
			Label:  "api key",
			Level:  DoctorOK,
			Detail: fmt.Sprintf("from %s (%s)", credentialsPath(dataDir), tailKey(key)),
		}, source, key
	default:
		return DoctorCheck{
			Label:  "api key",
			Level:  DoctorFail,
			Detail: "not configured — set " + providerEnvName(provider),
		}, apiKeySourceMissing, ""
	}
}

func doctorCheckCredentials(dataDir string) DoctorCheck {
	st := readCredentialsState(dataDir)
	switch {
	case st.Err != nil:
		return DoctorCheck{
			Label:  "credentials",
			Level:  DoctorFail,
			Detail: fmt.Sprintf("%s unreadable — %v", st.Path, st.Err),
		}
	case !st.Present:
		return DoctorCheck{
			Label:  "credentials",
			Level:  DoctorWarn,
			Detail: fmt.Sprintf("%s missing — `whale setup` writes one", st.Path),
		}
	default:
		return DoctorCheck{
			Label:  "credentials",
			Level:  DoctorOK,
			Detail: st.Path,
		}
	}
}
