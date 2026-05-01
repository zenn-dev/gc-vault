package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseOpRef(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantVault string
		wantItem  string
		wantErr   bool
	}{
		{"basic", "op://Private/my-bootstrap", "Private", "my-bootstrap", false},
		{"with field segment", "op://Private/my-bootstrap/field", "Private", "my-bootstrap", false},
		{"vault with spaces", "op://Shared Team/my-bootstrap", "Shared Team", "my-bootstrap", false},
		{"missing prefix", "Private/my-bootstrap", "", "", true},
		{"empty vault", "op:///my-bootstrap", "", "", true},
		{"empty item", "op://Private/", "", "", true},
		{"vault only", "op://Private", "", "", true},
		{"empty", "", "", "", true},
		{"only prefix", "op://", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, i, err := parseOpRef(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for %q, got vault=%q item=%q", tc.input, v, i)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v != tc.wantVault {
				t.Errorf("vault: got %q, want %q", v, tc.wantVault)
			}
			if i != tc.wantItem {
				t.Errorf("item: got %q, want %q", i, tc.wantItem)
			}
		})
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFrom_Valid(t *testing.T) {
	path := writeConfig(t, `
op_account = "test.1password.com"

[profiles.dev]
bootstrap_op_ref = "op://Private/dev-bootstrap"
target_sa = "readonly@dev.iam.gserviceaccount.com"
project = "dev"
lifetime = 1800

[profiles.stg]
bootstrap_op_ref = "op://Private/stg-bootstrap"
target_sa = "readonly@stg.iam.gserviceaccount.com"
project = "stg"
lifetime = 7200
`)

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.OpAccount != "test.1password.com" {
		t.Errorf("op_account: got %q", cfg.OpAccount)
	}
	if len(cfg.Profiles) != 2 {
		t.Errorf("profiles count: got %d, want 2", len(cfg.Profiles))
	}
	dev := cfg.Profiles["dev"]
	if dev.Project != "dev" || dev.Lifetime != 1800 {
		t.Errorf("dev profile: got %+v", dev)
	}
	stg := cfg.Profiles["stg"]
	if stg.Lifetime != 7200 {
		t.Errorf("stg lifetime: got %d, want 7200", stg.Lifetime)
	}
}

func TestLoadFrom_DefaultLifetime(t *testing.T) {
	path := writeConfig(t, `
op_account = "test.1password.com"

[profiles.dev]
bootstrap_op_ref = "op://Private/dev-bootstrap"
target_sa = "readonly@dev.iam.gserviceaccount.com"
project = "dev"
`)
	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if got := cfg.Profiles["dev"].Lifetime; got != defaultLifetime {
		t.Errorf("default lifetime: got %d, want %d", got, defaultLifetime)
	}
}

func TestLoadFrom_ZeroLifetimeDefaulted(t *testing.T) {
	path := writeConfig(t, `
op_account = "test.1password.com"

[profiles.dev]
bootstrap_op_ref = "op://Private/dev-bootstrap"
target_sa = "readonly@dev.iam.gserviceaccount.com"
project = "dev"
lifetime = 0
`)
	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if got := cfg.Profiles["dev"].Lifetime; got != defaultLifetime {
		t.Errorf("zero lifetime should default: got %d, want %d", got, defaultLifetime)
	}
}

func TestLoadFrom_Invalid(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"empty file", ``},
		{"no profiles", `# only comments`},
		{"missing op_account", `
[profiles.dev]
bootstrap_op_ref = "op://Private/i"
target_sa = "x@y.com"
project = "dev"`},
		{"missing bootstrap_op_ref", `
op_account = "test.1password.com"
[profiles.dev]
target_sa = "x@y.com"
project = "dev"`},
		{"invalid op ref scheme", `
op_account = "test.1password.com"
[profiles.dev]
bootstrap_op_ref = "not-a-ref"
target_sa = "x@y.com"
project = "dev"`},
		{"op ref without item", `
op_account = "test.1password.com"
[profiles.dev]
bootstrap_op_ref = "op://Private"
target_sa = "x@y.com"
project = "dev"`},
		{"missing target_sa", `
op_account = "test.1password.com"
[profiles.dev]
bootstrap_op_ref = "op://Private/i"
project = "dev"`},
		{"missing project", `
op_account = "test.1password.com"
[profiles.dev]
bootstrap_op_ref = "op://Private/i"
target_sa = "x@y.com"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeConfig(t, tc.content)
			if _, err := LoadFrom(path); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestLoadFrom_NotExist(t *testing.T) {
	if _, err := LoadFrom(filepath.Join(t.TempDir(), "missing.toml")); err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestProfile_ParseOpRef(t *testing.T) {
	p := Profile{BootstrapOpRef: "op://Private/my-item"}
	v, i, err := p.ParseOpRef()
	if err != nil {
		t.Fatalf("ParseOpRef: %v", err)
	}
	if v != "Private" || i != "my-item" {
		t.Errorf("got (%q, %q), want (Private, my-item)", v, i)
	}
}
