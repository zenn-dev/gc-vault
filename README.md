# gc-vault

**English** | [日本語](./README.ja.md)

[![CI](https://github.com/zenn-dev/gc-vault/actions/workflows/ci.yml/badge.svg)](https://github.com/zenn-dev/gc-vault/actions/workflows/ci.yml)

A CLI tool for working with GCP resources in local development without leaving long-lived credentials on disk. Bootstrap service-account keys live in 1Password, and short-lived impersonated tokens are handed only to commands invoked through `gc-vault exec`.

> **Status: WIP (Pre-alpha)**
> Spec and MVP implementation in progress.

## Motivation

- Credentials saved by `gcloud auth login` / `gcloud auth application-default login` include refresh tokens, giving persistent access — high risk if leaked.
- Google Cloud provides the primitives (`--impersonate-service-account`, etc.), but a thin wrapper that unifies where credentials are stored and how they are consumed has been missing.
- The goal is to enforce locally a workflow where long-lived credentials live only in 1Password and short-lived tokens are minted on demand.

## Architecture overview

```
1Password
  └── SA key JSON for bootstrap-{user}@{project}
        │ op read (1Password authentication)
        ▼
gc-vault exec <profile> -- <cmd>
  1. Fetch the bootstrap SA key from 1Password into a temp file
  2. For CLIs: obtain a short-lived token (1h) via iamcredentials.generateAccessToken
              → set as CLOUDSDK_AUTH_ACCESS_TOKEN
  3. For SDKs: generate an impersonated_service_account ADC JSON
              → set as GOOGLE_APPLICATION_CREDENTIALS
              (the SDK refreshes the token automatically)
  4. exec cmd
  5. Remove the temp file on exit
```

The core design goal is to keep **zero long-lived credentials on local disk**.

## Scope

`gc-vault` is **not a gcloud-only wrapper but an authentication wrapper for the broader GCP ecosystem**. Any tool that honors the two environment variables below automatically receives the impersonated credentials.

### Via `CLOUDSDK_AUTH_ACCESS_TOKEN` (gcloud-family CLIs)

| Tool | Purpose |
|---|---|
| `gcloud` | General Google Cloud operations (**legacy `gsutil` is unsupported** — see [Known incompatibilities](#known-incompatibilities)) |
| `bq` | BigQuery |

### Via `GOOGLE_APPLICATION_CREDENTIALS` (anything that reads ADC)

An `impersonated_service_account`-style ADC JSON is generated on the fly, so **the SDK refreshes tokens automatically** (no need to worry about token expiry in long-running processes).

| Category | Examples |
|---|---|
| Google Cloud client libraries | Ruby / Python / Go / Node / Java / .NET |
| IaC tools | Terraform `google` provider, Pulumi `gcp` provider |
| Applications | GCS / Cloud Tasks / Pub/Sub clients inside Rails apps, etc. |
| Auxiliary tools | `cloud-sql-proxy`, others |

### Known incompatibilities

Tools with their own authentication paths require separate handling:

| Tool | Status |
|---|---|
| `gsutil` | Reads neither `CLOUDSDK_AUTH_ACCESS_TOKEN` nor `GOOGLE_APPLICATION_CREDENTIALS`; it consults `~/.boto` or the gcloud credential store and fails with 401 Anonymous caller. Use `gcloud storage` instead. |
| `kubectl` (GKE) | `gke-gcloud-auth-plugin` uses its own flow (has an ADC-respecting mode). |
| `firebase` CLI | Uses its own `firebase login` authentication. |

## Expected SA layout

For each GCP project, create a per-user `bootstrap` SA and one or more `target` SAs:

| SA | Role | Permissions |
|---|---|---|
| `bootstrap-{user}@{project}` | Impersonation source (key stored in 1Password) | Only `roles/iam.serviceAccountTokenCreator` against the target SAs |
| `target-{role}-{user}@{project}` | Impersonation target (actually touches resources) | Least-privilege roles per use case |

Because the `bootstrap` SA has no permissions beyond impersonating targets, a leaked bootstrap key gives an attacker only what the corresponding target SA can do.

You can configure **multiple target SAs per operator role**. For example, split:

- `target-user-{user}@{project}` — what you use from your own terminal
- `target-claude-{user}@{project}` — what Claude Code uses, restricted to read-only roles

With this layout, Claude Code is limited to read operations, and a leaked Claude Code credential is bounded by the `target-claude` permissions.

Switch targets per profile in `gc-vault`, and grant `roles/iam.serviceAccountTokenCreator` from the `bootstrap` SA to each target individually. The naming above is illustrative — adapt it to your operational conventions.

## Prerequisites

- macOS (arm64 / amd64)
- [`gcloud` CLI](https://cloud.google.com/sdk/docs/install)
- [`op` CLI (1Password)](https://developer.1password.com/docs/cli/get-started/) — enable Settings → Developer → "Integrate with 1Password CLI"
- (Development only) Go 1.23+

> **When using from Claude Code**: Claude Code's sandbox blocks access to the 1Password desktop app, so `gc-vault exec` must be invoked with `dangerouslyDisableSandbox: true`. Because Claude Code's Bash tool spawns each call as an independent process, 1Password treats every invocation as a separate session and re-prompts for authentication each time (a two-layer defense: sandbox release plus 1Password re-authentication). For workflows with many sequential calls, you can launch via `gc-vault shell` instead. See [Using with Claude Code](#using-with-claude-code).

## Installation

### `go install`

```bash
go install github.com/zenn-dev/gc-vault/cmd/gc-vault@latest
```

### From source with `make install`

```bash
git clone https://github.com/zenn-dev/gc-vault.git
cd gc-vault
make install   # installs to $GOPATH/bin
```

To build only into the current directory:

```bash
make build     # writes to bin/gc-vault
```

### From the Releases page

Download the tar.gz for your OS / architecture from [Releases](https://github.com/zenn-dev/gc-vault/releases), extract it, and place the binary on your PATH.

### Making the binary callable from Claude Code

`~/go/bin/gc-vault`, where `go install` / `make install` places the binary, is not necessarily on the PATH Claude Code's Bash tool sees. To make sure Claude Code can find it, symlink the binary into `~/.local/bin`, which is on the PATH:

```bash
mkdir -p ~/.local/bin
ln -s ~/go/bin/gc-vault ~/.local/bin/gc-vault
```

Alternatively you can add `~/go/bin` to PATH in `~/.zshenv`, but Claude Code's Bash tool doesn't necessarily launch zsh, so the symlink is more reliable.

## Development

```bash
make help        # list available targets
make build       # build the binary
make test        # run tests
make test-cover  # run tests with coverage
make lint        # go vet + gofmt check
make fmt         # gofmt -w .
make clean       # remove bin/
```

## Release process

Tagging a release triggers GitHub Actions to run goreleaser and create a draft Release:

```bash
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

The draft Release receives the artifacts (`darwin_x86_64` / `darwin_arm64` tar.gz files plus `checksums.txt`). After reviewing, click Publish in the GitHub UI to make it public.

To enable a brew tap later, make the `gc-vault` repository public, create a separate `zenn-dev/homebrew-tap` repository, and add a `brews:` section to `.goreleaser.yaml`.

## Setup

1. **IAM setup**: Follow [docs/runbook-iam-setup.md](./docs/runbook-iam-setup.md) to create the bootstrap SA / target SAs and store the bootstrap key in 1Password. (Runbook is currently Japanese-only.)
2. **Config file**: Copy [examples/config.toml](./examples/config.toml) to `~/.config/gc-vault/config.toml` and edit it for your values.
3. **Sanity check**: Run `gc-vault doctor` to verify prerequisites.

## Usage

### `gc-vault list`

Lists configured profiles.

```bash
$ gc-vault list
PROFILE             PROJECT             TARGET SA                                      LIFETIME
my-app-dev          my-app-dev          readonly-alice@my-app-dev.iam.gservice...      3600s
```

### `gc-vault exec PROFILE -- COMMAND`

Runs a single command with impersonated credentials.

```bash
$ gc-vault exec my-app-dev -- gcloud projects describe my-app-dev
$ gc-vault exec my-app-dev -- terraform plan
$ gc-vault exec my-app-dev -- bin/rails console
```

### `gc-vault shell PROFILE`

Launches a subshell with impersonated credentials set. Exit the subshell with `exit`; the environment variables disappear with it.

```bash
$ gc-vault shell my-app-dev
gc-vault: starting subshell with profile "my-app-dev" (exit to leave)
$ gcloud sql instances list
$ gcloud run services list
$ exit
```

#### Showing the profile name in your prompt (optional)

Inside `shell`, the environment variable `GCP_VAULT_ACTIVE_PROFILE` is set. You can use it to decorate your prompt.

**Important**: Theme frameworks such as oh-my-zsh overwrite `PROMPT` / `PS1`, so place the snippet **at the end of your rc file** (after the theme is loaded).

**zsh** (end of `~/.zshrc`):
```zsh
if [[ -n "$GCP_VAULT_ACTIVE_PROFILE" ]]; then
  PROMPT="(gcp:$GCP_VAULT_ACTIVE_PROFILE) $PROMPT"
fi
```

**bash** (end of `~/.bashrc`):
```bash
if [ -n "$GCP_VAULT_ACTIVE_PROFILE" ]; then
  PS1="(gcp:$GCP_VAULT_ACTIVE_PROFILE) $PS1"
fi
```

**If you'd rather not worry about ordering** (zsh, via `precmd` hook):
```zsh
__gc_vault_prompt() {
  if [[ -n "$GCP_VAULT_ACTIVE_PROFILE" && "$PROMPT" != "(gcp:$GCP_VAULT_ACTIVE_PROFILE) "* ]]; then
    PROMPT="(gcp:$GCP_VAULT_ACTIVE_PROFILE) $PROMPT"
  fi
}
precmd_functions+=(__gc_vault_prompt)
```

### `gc-vault doctor`

Diagnoses the local environment.

```bash
$ gc-vault doctor
OK    gcloud CLI found
OK    1Password CLI signed in: my.1password.com
OK    config: /Users/alice/.config/gc-vault/config.toml (3 profile(s))
        - my-app-dev
        - my-app-stg
        - my-app-prod
OK    no bare gcloud credentials
```

### Removing gcloud credentials (manual)

Once your migration to `gc-vault` is complete, remove any remaining local gcloud credentials. `gc-vault` does not touch gcloud's own state, so use the official commands:

```bash
gcloud auth revoke --all
gcloud auth application-default revoke

# Verify nothing remains
ls -la ~/.config/gcloud/credentials.db \
       ~/.config/gcloud/application_default_credentials.json 2>/dev/null
```

`gc-vault doctor` raises a WARN if these files remain.

### Rotating the bootstrap SA key (manual)

Rotate the bootstrap SA key periodically per your organization's policy. Because the bootstrap SA only holds `roles/iam.serviceAccountTokenCreator` and the blast radius of a leak is bounded by the target SA's permissions, rotation is handled **manually with `gcloud` + `op`** rather than a `gc-vault` subcommand (see [Section 8 of the runbook](./docs/runbook-iam-setup.md#8-ローテーション)).

## Using with Claude Code

[Claude Code](https://claude.com/claude-code) runs a sandbox on macOS that restricts access to Unix domain sockets and similar IPC paths. This blocks the `op` CLI from talking to the 1Password desktop app, so a plain `gc-vault exec` call from Claude Code's Bash tool fails.

### Recommended: install the bundled Claude Code skill

This repository bundles a Claude Code skill at [`.claude/skills/gc-vault/`](./.claude/skills/gc-vault). When using `gc-vault` from Claude Code, install this skill first. It instructs Claude Code to assemble commands following the workflow described below (sandbox release + 1Password authentication, two layers of defense).

#### Global install (recommended when used across multiple projects)

```bash
mkdir -p ~/.claude/skills
cp -r .claude/skills/gc-vault ~/.claude/skills/
```

Or symlink so the skill tracks repository updates:

```bash
mkdir -p ~/.claude/skills
ln -s "$(pwd)/.claude/skills/gc-vault" ~/.claude/skills/gc-vault
```

#### Per-project install

For use in a single project, copy it under that project's `.claude/skills/`:

```bash
mkdir -p /path/to/your-project/.claude/skills
cp -r .claude/skills/gc-vault /path/to/your-project/.claude/skills/
```

With the skill in place, Claude Code automatically references it when `gc-vault` is needed and proposes invocations of `gc-vault exec` with `dangerouslyDisableSandbox: true`.

> **Note**: Each skill-driven invocation re-prompts for 1Password authentication. This behavior depends on Claude Code's and 1Password's process models and may change if either side changes. If 1Password ever stops prompting, you've lost one layer of defense — revisit your workflow.

### Alternative: launch Claude Code from `gc-vault shell`

If you make many consecutive GCP calls and approving 1Password every time is operationally heavy, you can enter a `gc-vault shell` **before** launching Claude Code.

```bash
gc-vault shell zenn-dev-develop   # 1Password authentication
claude                            # launches inheriting impersonated credentials
```

**Properties**:

- Inside Claude Code, the environment variables (`CLOUDSDK_AUTH_ACCESS_TOKEN`, `GOOGLE_APPLICATION_CREDENTIALS`, etc.) are inherited, so `gcloud` / `gcloud storage` / `bq` / `terraform` run **directly** (no `gc-vault exec` needed, sandbox stays in default mode).
- 1Password access happens **before** Claude Code launches, outside the agent's reach.
- 1Password authentication is required **only once**, at `gc-vault shell` startup.

**Trade-offs**:

- During the session, the credentials expanded into the shell are readable by the agent:
  - `CLOUDSDK_AUTH_ACCESS_TOKEN` (short-lived access token, 30–60 min)
  - The ADC file pointed to by `GOOGLE_APPLICATION_CREDENTIALS` (which references the bootstrap SA key)
- A prompt injection that reads these gives the attacker the target SA's permissions (read-only) and the remaining lifetime of the bootstrap key reference.
- However, **there is no new path to 1Password itself from inside Claude Code** — exposure stays bounded by the credentials already in hand.

When the impersonated token expires, exit Claude Code and re-launch it via `gc-vault shell`.

### Additional: baseline defenses

Either workflow benefits from these alongside:

- **On the GCP side**: Keep target SAs read-only (the default in this tool's design) and assume leaks — use IAM Conditions, VPC Service Controls, and audit alerts.
- **Rotation cadence**: Follow your organization's policy for bootstrap key rotation. More frequent rotation shortens the post-leak exposure window.

### Approach not recommended: blanket sandbox permissions

You might think you can add the necessary permissions to `~/.claude/settings.json`'s `sandbox` setting so that `gc-vault exec` runs even inside the sandbox. **In practice, the `op` CLI's path to the 1Password desktop app is not a single Unix socket**, so you would need to allow multiple Unix sockets and macOS-specific IPC paths — effectively a major sandbox relaxation.

Reasons not to recommend it:

- 1Password updates can change internal paths, breaking your setup or quietly routing through a new path — the configuration is **fragile**.
- Once allowed, a prompt injection inside Claude Code has a permanently open path to all of 1Password.
- No advantage over the recommended workflow: the 1Password prompt is a consequence of Claude Code's Bash process model and fires either way, while this approach loses the extra Bash approval prompt that comes with `dangerouslyDisableSandbox: true`.

## Documentation

- [docs/runbook-iam-setup.md](./docs/runbook-iam-setup.md) — GCP / IAM setup procedure (currently Japanese-only)
