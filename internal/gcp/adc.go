package gcp

import (
	"encoding/json"
	"fmt"
	"os"
)

type adcImpersonated struct {
	Delegates                      []string       `json:"delegates"`
	ServiceAccountImpersonationURL string         `json:"service_account_impersonation_url"`
	SourceCredentials              map[string]any `json:"source_credentials"`
	Type                           string         `json:"type"`
}

func WriteImpersonatedADC(bootstrapKey []byte, targetSA string) (path string, cleanup func(), err error) {
	var source map[string]any
	if err := json.Unmarshal(bootstrapKey, &source); err != nil {
		return "", nil, fmt.Errorf("parsing bootstrap key JSON: %w", err)
	}

	adc := adcImpersonated{
		Delegates: []string{},
		ServiceAccountImpersonationURL: fmt.Sprintf(
			"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateAccessToken",
			targetSA,
		),
		SourceCredentials: source,
		Type:              "impersonated_service_account",
	}

	data, err := json.Marshal(adc)
	if err != nil {
		return "", nil, fmt.Errorf("marshaling ADC JSON: %w", err)
	}

	f, err := os.CreateTemp("", "gc-vault-adc-*.json")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp file: %w", err)
	}
	name := f.Name()
	cleanupFn := func() { _ = os.Remove(name) }

	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		cleanupFn()
		return "", nil, fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		cleanupFn()
		return "", nil, fmt.Errorf("writing ADC JSON: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanupFn()
		return "", nil, fmt.Errorf("closing temp file: %w", err)
	}

	return name, cleanupFn, nil
}
