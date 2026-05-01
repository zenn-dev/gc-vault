package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/cm-igarashi-ryosuke/gc-vault/internal/config"
)

func Run() error {
	var problems int

	if _, err := exec.LookPath("gcloud"); err != nil {
		fmt.Println("FAIL  gcloud CLI not found in PATH")
		problems++
	} else {
		fmt.Println("OK    gcloud CLI found")
	}

	cfgPath, _ := config.DefaultPath()
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("FAIL  config: %v\n", err)
		problems++
	} else {
		fmt.Printf("OK    config: %s (%d profile(s), 1Password account=%q)\n",
			cfgPath, len(cfg.Profiles), cfg.OpAccount)
		names := make([]string, 0, len(cfg.Profiles))
		for n := range cfg.Profiles {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Printf("        - %s\n", n)
		}
	}

	home, _ := os.UserHomeDir()
	bareFiles := []string{
		filepath.Join(home, ".config", "gcloud", "credentials.db"),
		filepath.Join(home, ".config", "gcloud", "application_default_credentials.json"),
		filepath.Join(home, ".config", "gcloud", "access_tokens.db"),
	}
	var bareFound []string
	for _, p := range bareFiles {
		if _, err := os.Stat(p); err == nil {
			bareFound = append(bareFound, p)
		}
	}
	if len(bareFound) > 0 {
		fmt.Println("WARN  bare gcloud credentials detected:")
		for _, p := range bareFound {
			fmt.Printf("        - %s\n", p)
		}
		fmt.Println("        run: gcloud auth revoke --all && gcloud auth application-default revoke")
	} else {
		fmt.Println("OK    no bare gcloud credentials")
	}

	if problems > 0 {
		return fmt.Errorf("%d problem(s) found", problems)
	}
	return nil
}
