package onepassword

import (
	"context"
	"fmt"

	"github.com/1password/onepassword-sdk-go"
)

func GetDocument(ctx context.Context, account, vaultName, itemTitle string) ([]byte, error) {
	if account == "" {
		return nil, fmt.Errorf("op_account is required (set in config)")
	}

	client, err := onepassword.NewClient(ctx,
		onepassword.WithDesktopAppIntegration(account),
		onepassword.WithIntegrationInfo("gc-vault", "v0.0.1"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating 1Password client (account %q): %w", account, err)
	}

	vaultID, err := findVaultID(ctx, client, vaultName)
	if err != nil {
		return nil, err
	}

	itemID, err := findItemID(ctx, client, vaultID, itemTitle)
	if err != nil {
		return nil, err
	}

	item, err := client.Items().Get(ctx, vaultID, itemID)
	if err != nil {
		return nil, fmt.Errorf("getting item %q: %w", itemTitle, err)
	}
	if item.Document == nil {
		return nil, fmt.Errorf("item %q is not a Document type (category=%s)", itemTitle, item.Category)
	}

	data, err := client.Items().Files().Read(ctx, vaultID, itemID, *item.Document)
	if err != nil {
		return nil, fmt.Errorf("reading document %q: %w", itemTitle, err)
	}
	return data, nil
}

func findVaultID(ctx context.Context, client *onepassword.Client, vaultName string) (string, error) {
	vaults, err := client.Vaults().List(ctx)
	if err != nil {
		return "", fmt.Errorf("listing vaults: %w", err)
	}
	for _, v := range vaults {
		if v.Title == vaultName {
			return v.ID, nil
		}
	}
	return "", fmt.Errorf("vault %q not found", vaultName)
}

func findItemID(ctx context.Context, client *onepassword.Client, vaultID, itemTitle string) (string, error) {
	items, err := client.Items().List(ctx, vaultID)
	if err != nil {
		return "", fmt.Errorf("listing items in vault %s: %w", vaultID, err)
	}
	for _, it := range items {
		if it.Title == itemTitle {
			return it.ID, nil
		}
	}
	return "", fmt.Errorf("item %q not found in vault", itemTitle)
}
