# gc-vault

[![CI](https://github.com/cm-igarashi-ryosuke/gc-vault/actions/workflows/ci.yml/badge.svg)](https://github.com/cm-igarashi-ryosuke/gc-vault/actions/workflows/ci.yml)

Google Cloud のサービスアカウント権限借用 (Service Account Impersonation) を、AWS の `aws-vault` ライクな体験でローカル開発に提供する CLI ツールです。

> **Status: WIP (Pre-alpha)**
> 仕様策定および MVP 実装中。

## なぜ作るか

- ローカルに `gcloud auth login` / `gcloud auth application-default login` で保存される credentials は、リフレッシュトークンを含むため永続的なアクセスが可能で、漏洩時のリスクが高い。
- Google Cloud には `--impersonate-service-account` 等の素材は揃っているが、`aws-vault` 相当のラッパーが存在しない。
- 1Password で長期認証情報を保管し、必要時にだけ短命トークンを発行する仕組みをローカルで「強制」したい。

## アーキテクチャ概要

```
1Password (Private vault)
  └── bootstrap-{user}@{project} の SA キー JSON
        │ op read (Touch ID)
        ▼
gc-vault exec <profile> -- <cmd>
  1. 1Password から bootstrap SA キーを取得し一時ファイルへ (umask 0600)
  2. CLI 用:  iamcredentials.generateAccessToken で短命トークン (1h) を取得
             → CLOUDSDK_AUTH_ACCESS_TOKEN にセット
  3. SDK 用:  impersonated_service_account 形式の ADC JSON を生成
             → GOOGLE_APPLICATION_CREDENTIALS にセット
             (SDK 側がトークンを自動更新)
  4. cmd を exec
  5. 終了時に一時ファイル削除
```

ローカルディスクには **長期認証情報を一切残さない** のが本ツールの核となる設計目標です。

## 対象スコープ

`gc-vault` は **gcloud 単体ツールではなく、GCP エコシステム全般** に対する認証ラッパーです。以下の 2 つの環境変数を経由するすべてのツールに対して、自動的に借用クレデンシャルが供給されます。

### `CLOUDSDK_AUTH_ACCESS_TOKEN` 経由（gcloud ファミリー CLI）

| ツール | 用途 |
|---|---|
| `gcloud` | Google Cloud 操作全般 |
| `gsutil` / `gcloud storage` | Cloud Storage |
| `bq` | BigQuery |
| `gcloud alpha` / `gcloud beta` | プレビュー機能 |

### `GOOGLE_APPLICATION_CREDENTIALS` 経由（ADC を読むすべてのもの）

`impersonated_service_account` 形式の ADC JSON を一時生成して渡すため、**SDK 側がトークンを自動更新します**（長時間プロセスでもトークン期限切れを意識する必要なし）。

| カテゴリ | 例 |
|---|---|
| Google Cloud Client Libraries | Ruby / Python / Go / Node / Java / .NET |
| IaC ツール | Terraform `google` provider, Pulumi `gcp` provider |
| アプリケーション | Rails アプリ内の GCS / Cloud Tasks / Pub/Sub クライアントなど |
| 補助ツール | `cloud-sql-proxy` ほか |

### 既知の対象外

独自の認証経路を持つツールは個別対応が必要です：

| ツール | 状況 |
|---|---|
| `kubectl` (GKE) | `gke-gcloud-auth-plugin` が独自フロー（ADC 尊重モードあり） |
| `firebase` CLI | `firebase login` 独自認証 |
| `~/.config/gcloud/credentials.db` を直読みする一部ツール | 動作しない |

## 想定する SA 構成

各 GCP プロジェクトに、ユーザーごとに以下の 2 つの SA を持ちます：

| SA | 役割 | 権限 |
|---|---|---|
| `bootstrap-{user}@{project}` | 借用元（1Password に鍵を保管） | `roles/iam.serviceAccountTokenCreator` を target SA に対してのみ |
| `readonly-{user}@{project}` | 借用先（実際にリソースを操作） | `roles/viewer` 等、必要最小限 |

`bootstrap` は target を借用する以外の権限を持たないため、漏洩時の被害は target の権限範囲に限定されます。

## 前提

- macOS (arm64 / amd64)
- [`gcloud` CLI](https://cloud.google.com/sdk/docs/install)
- [`op` CLI (1Password)](https://developer.1password.com/docs/cli/get-started/)
- (開発時のみ) Go 1.23+

## インストール

```bash
git clone https://github.com/cm-igarashi-ryosuke/gc-vault.git
cd gc-vault
make install   # $GOPATH/bin にインストール
```

ローカルディレクトリにビルドだけしたい場合：

```bash
make build     # bin/gc-vault に出力
```

## 開発

```bash
make help        # 使用可能なターゲット一覧
make build       # バイナリビルド
make test        # テスト実行
make test-cover  # カバレッジ付きテスト
make lint        # go vet + gofmt チェック
make fmt         # gofmt -w .
make clean       # bin/ 削除
```

## セットアップ

1. **IAM セットアップ**: [docs/runbook-iam-setup.md](./docs/runbook-iam-setup.md) に従い、bootstrap SA / target SA を作成し、bootstrap キーを 1Password に保管します。
2. **設定ファイル**: [examples/config.toml](./examples/config.toml) を `~/.config/gc-vault/config.toml` にコピーし、自分の値に編集します。
3. **動作確認**: `gc-vault doctor` で前提条件をチェックします。

## 使い方

### `gc-vault list`

設定済みプロファイルの一覧を表示します。

```bash
$ gc-vault list
PROFILE             PROJECT             TARGET SA                                      LIFETIME
my-app-dev          my-app-dev          readonly-alice@my-app-dev.iam.gservice...      3600s
```

### `gc-vault exec PROFILE -- COMMAND`

借用クレデンシャルでコマンドを 1 回実行します。

```bash
$ gc-vault exec my-app-dev -- gcloud projects describe my-app-dev
$ gc-vault exec my-app-dev -- terraform plan
$ gc-vault exec my-app-dev -- bin/rails console
```

### `gc-vault shell PROFILE`

借用クレデンシャルがセットされた subshell を起動します。`exit` で抜けると環境変数も自動的に消えます。

```bash
$ gc-vault shell my-app-dev
gc-vault: starting subshell with profile "my-app-dev" (exit to leave)
$ gcloud sql instances list
$ gcloud run services list
$ exit
```

#### プロンプトに profile 名を表示する（任意）

`shell` 中は `GCP_VAULT_ACTIVE_PROFILE` 環境変数がセットされています。これを使ってプロンプトを装飾できます。

**重要**: oh-my-zsh などのテーマフレームワークが `PROMPT` / `PS1` を上書きするため、**rc ファイルの末尾**（テーマ読み込みより後ろ）に配置してください。

**zsh** (`~/.zshrc` の末尾):
```zsh
if [[ -n "$GCP_VAULT_ACTIVE_PROFILE" ]]; then
  PROMPT="(gcp:$GCP_VAULT_ACTIVE_PROFILE) $PROMPT"
fi
```

**bash** (`~/.bashrc` の末尾):
```bash
if [ -n "$GCP_VAULT_ACTIVE_PROFILE" ]; then
  PS1="(gcp:$GCP_VAULT_ACTIVE_PROFILE) $PS1"
fi
```

**配置順を気にしたくない場合 (zsh, precmd hook 利用)**:
```zsh
__gc_vault_prompt() {
  if [[ -n "$GCP_VAULT_ACTIVE_PROFILE" && "$PROMPT" != "(gcp:$GCP_VAULT_ACTIVE_PROFILE) "* ]]; then
    PROMPT="(gcp:$GCP_VAULT_ACTIVE_PROFILE) $PROMPT"
  fi
}
precmd_functions+=(__gc_vault_prompt)
```

### `gc-vault doctor`

ローカル環境の健全性を診断します。

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

### 裸 gcloud credentials の削除（手動）

`gc-vault` への移行が完了したら、ローカルに残っている裸の gcloud credentials を以下で削除します（`gc-vault` 自身は gcloud の state に手を出さないため、純正の手順で行います）：

```bash
gcloud auth revoke --all
gcloud auth application-default revoke

# 念のためファイルが残っていないか確認
ls -la ~/.config/gcloud/credentials.db \
       ~/.config/gcloud/application_default_credentials.json 2>/dev/null
```

`gc-vault doctor` がこれらのファイルの残存を WARN で警告します。

### bootstrap SA キーのローテーション（手動）

bootstrap SA キーは 90 日ごとのローテーションを推奨します。bootstrap SA は `roles/iam.serviceAccountTokenCreator` のみを持ち、漏洩時の影響範囲が target SA の権限内に閉じるため、ローテーションは `gc-vault` 内のコマンドではなく **`gcloud` + `op` の手動操作** で行います（[Runbook の 8 章](./docs/runbook-iam-setup.md#8-ローテーション-90-日ごと推奨) 参照）。

## ドキュメント

- [docs/runbook-iam-setup.md](./docs/runbook-iam-setup.md) — GCP / IAM 側のセットアップ手順
