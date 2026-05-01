package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	OpAccount string             `toml:"op_account"`
	Profiles  map[string]Profile `toml:"profiles"`
}

type Profile struct {
	BootstrapOpRef string `toml:"bootstrap_op_ref"`
	TargetSA       string `toml:"target_sa"`
	Project        string `toml:"project"`
	Lifetime       int    `toml:"lifetime"`
}

const defaultLifetime = 3600

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".config", "gc-vault", "config.toml"), nil
}

func Load() (*Config, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return LoadFrom(path)
}

func LoadFrom(path string) (*Config, error) {
	var c Config
	if _, err := toml.DecodeFile(path, &c); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %s\nCreate one based on examples/config.toml", path)
		}
		return nil, fmt.Errorf("loading config %s: %w", path, err)
	}
	if err := c.normalize(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) normalize() error {
	if len(c.Profiles) == 0 {
		return fmt.Errorf("no profiles defined")
	}
	if c.OpAccount == "" {
		return fmt.Errorf("op_account is required at the top level (e.g. op_account = \"my.1password.com\")")
	}
	for name, p := range c.Profiles {
		if p.BootstrapOpRef == "" {
			return fmt.Errorf("profile %q: bootstrap_op_ref is required", name)
		}
		if !strings.HasPrefix(p.BootstrapOpRef, "op://") {
			return fmt.Errorf("profile %q: bootstrap_op_ref must start with op:// (got %q)", name, p.BootstrapOpRef)
		}
		if _, _, err := parseOpRef(p.BootstrapOpRef); err != nil {
			return fmt.Errorf("profile %q: %w", name, err)
		}
		if p.TargetSA == "" {
			return fmt.Errorf("profile %q: target_sa is required", name)
		}
		if p.Project == "" {
			return fmt.Errorf("profile %q: project is required", name)
		}
		if p.Lifetime <= 0 {
			p.Lifetime = defaultLifetime
		}
		c.Profiles[name] = p
	}
	return nil
}

func (p *Profile) ParseOpRef() (vault, item string, err error) {
	return parseOpRef(p.BootstrapOpRef)
}

func parseOpRef(ref string) (vault, item string, err error) {
	if !strings.HasPrefix(ref, "op://") {
		return "", "", fmt.Errorf("op ref must start with op:// (got %q)", ref)
	}
	rest := strings.TrimPrefix(ref, "op://")
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("op ref must be op://VAULT/ITEM (got %q)", ref)
	}
	return parts[0], parts[1], nil
}
