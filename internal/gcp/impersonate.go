package gcp

import (
	"context"
	"fmt"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/iamcredentials/v1"
	"google.golang.org/api/option"
)

const cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

func GenerateAccessToken(ctx context.Context, bootstrapKey []byte, targetSA string, lifetimeSec int) (string, error) {
	creds, err := google.CredentialsFromJSON(ctx, bootstrapKey, cloudPlatformScope)
	if err != nil {
		return "", fmt.Errorf("loading bootstrap credentials: %w", err)
	}

	svc, err := iamcredentials.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return "", fmt.Errorf("creating iamcredentials service: %w", err)
	}

	name := "projects/-/serviceAccounts/" + targetSA
	req := &iamcredentials.GenerateAccessTokenRequest{
		Scope:    []string{cloudPlatformScope},
		Lifetime: fmt.Sprintf("%ds", lifetimeSec),
	}

	resp, err := svc.Projects.ServiceAccounts.GenerateAccessToken(name, req).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("GenerateAccessToken on %s: %w", targetSA, err)
	}
	return resp.AccessToken, nil
}
