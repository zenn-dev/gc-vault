package gcp

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const fakeBootstrapKey = `{
  "type": "service_account",
  "project_id": "my-project",
  "private_key_id": "abc123",
  "private_key": "-----BEGIN PRIVATE KEY-----\nFAKE\n-----END PRIVATE KEY-----\n",
  "client_email": "bootstrap@my-project.iam.gserviceaccount.com",
  "client_id": "1234"
}`

func TestWriteImpersonatedADC_Content(t *testing.T) {
	targetSA := "readonly@my-project.iam.gserviceaccount.com"
	path, cleanup, err := WriteImpersonatedADC([]byte(fakeBootstrapKey), targetSA)
	if err != nil {
		t.Fatalf("WriteImpersonatedADC: %v", err)
	}
	defer cleanup()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read temp file: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse ADC JSON: %v", err)
	}

	if got["type"] != "impersonated_service_account" {
		t.Errorf("type: got %v, want impersonated_service_account", got["type"])
	}

	url, _ := got["service_account_impersonation_url"].(string)
	if !strings.Contains(url, targetSA) {
		t.Errorf("impersonation URL should contain target SA %q, got %q", targetSA, url)
	}
	if !strings.HasSuffix(url, ":generateAccessToken") {
		t.Errorf("impersonation URL should end with :generateAccessToken, got %q", url)
	}

	delegates, ok := got["delegates"].([]any)
	if !ok {
		t.Fatalf("delegates: missing or wrong type, got %T", got["delegates"])
	}
	if len(delegates) != 0 {
		t.Errorf("delegates should be empty, got %v", delegates)
	}

	src, ok := got["source_credentials"].(map[string]any)
	if !ok {
		t.Fatal("source_credentials missing or wrong type")
	}
	if src["type"] != "service_account" {
		t.Errorf("source.type: got %v", src["type"])
	}
	if src["client_email"] != "bootstrap@my-project.iam.gserviceaccount.com" {
		t.Errorf("source.client_email: got %v", src["client_email"])
	}
	if src["private_key_id"] != "abc123" {
		t.Errorf("source.private_key_id: got %v", src["private_key_id"])
	}
}

func TestWriteImpersonatedADC_FileMode(t *testing.T) {
	path, cleanup, err := WriteImpersonatedADC([]byte(fakeBootstrapKey), "x@test.com")
	if err != nil {
		t.Fatalf("WriteImpersonatedADC: %v", err)
	}
	defer cleanup()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("file mode: got %o, want 0600", mode)
	}
}

func TestWriteImpersonatedADC_Cleanup(t *testing.T) {
	path, cleanup, err := WriteImpersonatedADC([]byte(fakeBootstrapKey), "x@test.com")
	if err != nil {
		t.Fatalf("WriteImpersonatedADC: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist before cleanup: %v", err)
	}

	cleanup()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file removed after cleanup, stat err: %v", err)
	}
}

func TestWriteImpersonatedADC_CleanupIdempotent(t *testing.T) {
	_, cleanup, err := WriteImpersonatedADC([]byte(fakeBootstrapKey), "x@test.com")
	if err != nil {
		t.Fatalf("WriteImpersonatedADC: %v", err)
	}
	cleanup()
	cleanup() // second call must not panic
}

func TestWriteImpersonatedADC_InvalidJSON(t *testing.T) {
	if _, _, err := WriteImpersonatedADC([]byte(`not json`), "x@test.com"); err == nil {
		t.Error("expected error for invalid bootstrap JSON")
	}
}
