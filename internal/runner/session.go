package runner

import (
	"context"
	"fmt"
	"os"

	"github.com/zenn-dev/gc-vault/internal/config"
	"github.com/zenn-dev/gc-vault/internal/gcp"
	"github.com/zenn-dev/gc-vault/internal/onepassword"
)

type session struct {
	profileName string
	profile     config.Profile
	env         []string
	cleanup     func()
}

func newSession(profileName string) (*session, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	profile, ok := cfg.Profiles[profileName]
	if !ok {
		return nil, fmt.Errorf("profile not found: %s", profileName)
	}

	vault, item, err := profile.ParseOpRef()
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "gc-vault: fetching bootstrap key from 1Password (%s/%s)\n", vault, item)
	bootstrapKey, err := onepassword.GetDocument(vault, item)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "gc-vault: impersonating %s\n", profile.TargetSA)
	ctx := context.Background()
	token, err := gcp.GenerateAccessToken(ctx, bootstrapKey, profile.TargetSA, profile.Lifetime)
	if err != nil {
		return nil, err
	}

	adcPath, cleanup, err := gcp.WriteImpersonatedADC(bootstrapKey, profile.TargetSA)
	if err != nil {
		return nil, err
	}

	env := append(os.Environ(),
		"CLOUDSDK_AUTH_ACCESS_TOKEN="+token,
		"CLOUDSDK_CORE_PROJECT="+profile.Project,
		"GOOGLE_APPLICATION_CREDENTIALS="+adcPath,
		"GCP_VAULT_ACTIVE_PROFILE="+profileName,
	)

	return &session{
		profileName: profileName,
		profile:     profile,
		env:         env,
		cleanup:     cleanup,
	}, nil
}
