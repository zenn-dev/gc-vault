# gc-vault

[![CI](https://github.com/zenn-dev/gc-vault/actions/workflows/ci.yml/badge.svg)](https://github.com/zenn-dev/gc-vault/actions/workflows/ci.yml)

ローカル開発で GCP リソースを操作する際、長期 credentials をディスクに残さずに済ませるための CLI ツールです。bootstrap SA キーを 1Password に保管し、`gc-vault exec` 経由で実行されるコマンドにだけ短命の借用トークン (Service Account Impersonation) を渡します。

> **Status: WIP (Pre-alpha)**
> 仕様策定および MVP 実装中。

## なぜ作るか

- ローカルに `gcloud auth login` / `gcloud auth application-default login` で保存される credentials は、リフレッシュトークンを含むため永続的なアクセスが可能で、漏洩時のリスクが高い。
- Google Cloud には `--impersonate-service-account` 等の素材は揃っているが、認証情報の保管場所と利用フローを統一する薄いラッパーが手元になかった。
- 1Password で長期認証情報を保管し、必要時にだけ短命トークンを発行する仕組みをローカルで「強制」したい。

## アーキテクチャ概要

```
1Password (Private vault)
  └── bootstrap-{user}@{project} の SA キー JSON
        │ op read (1Password 認証)
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
| `gcloud` | Google Cloud 操作全般（**旧 `gsutil` は対応外** — [既知の対象外](#既知の対象外) 参照） |
| `bq` | BigQuery |

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
| `gsutil` | `CLOUDSDK_AUTH_ACCESS_TOKEN` も `GOOGLE_APPLICATION_CREDENTIALS` も読まず、`~/.boto` か gcloud 認証ストアを参照するため動作しない（401 Anonymous caller）。代わりに `gcloud storage` を使う |
| `kubectl` (GKE) | `gke-gcloud-auth-plugin` が独自フロー（ADC 尊重モードあり） |
| `firebase` CLI | `firebase login` 独自認証 |
| `~/.config/gcloud/credentials.db` を直読みする一部ツール | 動作しない |

## 想定する SA 構成

各 GCP プロジェクトに、ユーザーごとに `bootstrap` SA と 1 つ以上の `target` SA を作成します：

| SA | 役割 | 権限 |
|---|---|---|
| `bootstrap-{user}@{project}` | 借用元（1Password に鍵を保管） | 借用先の target SA に対する `roles/iam.serviceAccountTokenCreator` のみ |
| `target-{role}-{user}@{project}` | 借用先（実際にリソースを操作） | ロールに応じた最小権限 |

`bootstrap` は target を借用する以外の権限を持たないため、漏洩時の被害は target の権限範囲に限定されます。

target SA は **操作する主体（ロール）ごとに複数設定できます**。例えば「自分が手元のターミナルから操作する用 (`target-user-{user}@{project}`)」と「Claude Code から操作させる用 (`target-claude-{user}@{project}`)」を分けて持ち、後者には readonly 権限のみを付与しておけば、Claude Code 経由での操作はリソース閲覧に限定でき、credentials が漏れた場合の被害範囲も `target-claude` の権限内に収まります。`gc-vault` のプロファイル単位で target を切り替え、`bootstrap` SA から各 target に対して個別に `roles/iam.serviceAccountTokenCreator` を付与してください。命名は例示にすぎないため、運用に合わせて調整可能です。

## 前提

- macOS (arm64 / amd64)
- [`gcloud` CLI](https://cloud.google.com/sdk/docs/install)
- [`op` CLI (1Password)](https://developer.1password.com/docs/cli/get-started/) — Settings → Developer → 「Integrate with 1Password CLI」を有効化
- (開発時のみ) Go 1.23+

> **Claude Code から使う場合**: Claude Code のサンドボックスが 1Password デスクトップアプリへのアクセスを遮断するため、`gc-vault exec` を呼ぶ際は `dangerouslyDisableSandbox: true` での実行が必要です。Claude Code の Bash ツールは各呼び出しを独立プロセスとして spawn するため、1Password が各実行を別セッション扱いし、結果的に呼び出しごとに 1Password の認証が要求されます（サンドボックス解除と 1Password の認証の二段階防御）。連続実行が多い場合は `gc-vault shell` 経由で起動する運用も可能。詳細は [Claude Code との併用](#claude-code-との併用) を参照。

## インストール

### `go install`

```bash
go install github.com/zenn-dev/gc-vault/cmd/gc-vault@latest
```

### ソースから `make install`

```bash
git clone https://github.com/zenn-dev/gc-vault.git
cd gc-vault
make install   # $GOPATH/bin にインストール
```

ローカルディレクトリにビルドだけしたい場合：

```bash
make build     # bin/gc-vault に出力
```

### Releases ページから

[Releases](https://github.com/zenn-dev/gc-vault/releases) から該当 OS / アーキテクチャの tar.gz をダウンロードし、解凍してパスの通った場所に配置。

### Claude Code から呼び出せるようにする

`go install` / `make install` で配置される `~/go/bin/gc-vault` は、Claude Code の Bash ツールの PATH に含まれていない場合があります。確実に Claude Code から呼べるよう、PATH に含まれている `~/.local/bin` にシンボリックリンクを張ります。

```bash
mkdir -p ~/.local/bin
ln -s ~/go/bin/gc-vault ~/.local/bin/gc-vault
```

代替として `~/.zshenv` で `~/go/bin` を PATH に追加する方法もありますが、Claude Code の Bash ツールが zsh を起動するとは限らないため、symlink のほうが汎用的です。

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

## リリース手順

タグを切るだけで GitHub Actions が goreleaser を起動し、Release（draft）を作成します：

```bash
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

成果物（`darwin_x86_64` / `darwin_arm64` の tar.gz と `checksums.txt`）が draft Release に添付されます。確認後、GitHub UI で Publish すれば公開リリースになります。

将来 brew tap を有効にする場合は、`gc-vault` リポジトリを public 化した上で、別途 `zenn-dev/homebrew-tap` リポジトリを作成し、`.goreleaser.yaml` に `brews:` セクションを追加します。

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

### gcloud credentials の削除（手動）

`gc-vault` への移行が完了したら、ローカルに残っている gcloud credentials を以下で削除します（`gc-vault` 自身は gcloud の state に手を出さないため、純正の手順で行います）：

```bash
gcloud auth revoke --all
gcloud auth application-default revoke

# 念のためファイルが残っていないか確認
ls -la ~/.config/gcloud/credentials.db \
       ~/.config/gcloud/application_default_credentials.json 2>/dev/null
```

`gc-vault doctor` がこれらのファイルの残存を WARN で警告します。

### bootstrap SA キーのローテーション（手動）

bootstrap SA キーは、組織のポリシーに従って定期的にローテーションすることを推奨します。bootstrap SA は `roles/iam.serviceAccountTokenCreator` のみを持ち、漏洩時の影響範囲が target SA の権限内に閉じるため、ローテーションは `gc-vault` 内のコマンドではなく **`gcloud` + `op` の手動操作** で行います（[Runbook の 8 章](./docs/runbook-iam-setup.md#8-ローテーション) 参照）。

## Claude Code との併用

[Claude Code](https://claude.com/claude-code) は macOS 上で Unix ドメインソケット等へのアクセスを制限するサンドボックスを有効化しています。これにより `op` CLI から 1Password デスクトップアプリへの通信経路が遮断されるため、Claude Code の Bash ツールから `gc-vault exec` をそのまま呼んでも失敗します。

### 推奨: 同梱の Claude Code Skill をインストールする

このリポジトリの [`.claude/skills/gc-vault/`](./.claude/skills/gc-vault) 配下に、Claude Code 用のスキル定義を同梱しています。Claude Code から `gc-vault` を使う場合は、まずこのスキルをインストールしてください。スキルが Claude Code に対して、後述の「推奨運用」（サンドボックス解除 + 1Password の認証の二段階防御）に沿ったコマンド組み立てを指示します。

#### グローバルインストール（複数プロジェクトで使う場合に推奨）

```bash
mkdir -p ~/.claude/skills
cp -r .claude/skills/gc-vault ~/.claude/skills/
```

または symlink でリポジトリの更新を追従させる:

```bash
mkdir -p ~/.claude/skills
ln -s "$(pwd)/.claude/skills/gc-vault" ~/.claude/skills/gc-vault
```

#### プロジェクト別インストール

特定プロジェクトでのみ使う場合は、そのプロジェクト直下の `.claude/skills/` にコピーしてください。

```bash
mkdir -p /path/to/your-project/.claude/skills
cp -r .claude/skills/gc-vault /path/to/your-project/.claude/skills/
```

スキルを置いた状態で Claude Code を起動すると、`gc-vault` が必要なタイミングで自動的に本スキルが参照され、`dangerouslyDisableSandbox: true` 付きの `gc-vault exec` 呼び出しが提案されます。

> **注意**: Skill での実行時は毎回 1Password の認証を求められますが、これは Claude Code および 1Password のプロセスモデルに依存する挙動であり、両者のいずれかが変更されると挙動が変わる可能性があります。1Password の認証プロンプトが省略されるようになった場合は防御層が一段減るため、運用を見直してください。

### 代替運用: `gc-vault shell` から Claude Code を起動

連続的に多数の GCP 操作を行い、毎回 1Password の認証を承認するのが運用上重い場合は、Claude Code を起動する **前** に `gc-vault shell` でサブシェルに入っておく方法もあります。

```bash
gc-vault shell zenn-dev-develop   # 1Password 認証
claude                            # 借用クレデンシャルを継承して起動
```

**特性**:

- Claude Code 内では環境変数 (`CLOUDSDK_AUTH_ACCESS_TOKEN`, `GOOGLE_APPLICATION_CREDENTIALS` 等) を継承して `gcloud` / `gcloud storage` / `bq` / `terraform` を **直接** 実行できる（`gc-vault exec` 不要、サンドボックス標準のまま）
- 1Password へのアクセスは Claude Code を起動する **前** にユーザーの手元で完結している
- 1Password の認証は `gc-vault shell` 起動時の **1 回のみ**

**トレードオフ**:

- セッション中、shell に展開された credentials は agent から読み出し可能な状態にある
  - `CLOUDSDK_AUTH_ACCESS_TOKEN`（短命アクセストークン、30〜60 分）
  - `GOOGLE_APPLICATION_CREDENTIALS` が指す ADC ファイル（bootstrap SA キーへの参照を含む）
- プロンプトインジェクションでこれらが読み出されると、target SA（readonly）の権限と bootstrap キーの残存期間が攻撃者に渡る
- ただし、Claude Code 内に **1Password 本体への新規アクセス経路は存在しない**（既に取得済みの credentials の範囲に閉じる）

借用トークンが失効したら、Claude Code を終了し、再度 `gc-vault shell` から起動し直します。

### 補足: ベースラインの防御策

どちらの運用でも、以下を併用するとリスクを縮小できます:

- **GCP 側**: target SA を readonly に固定（本ツールの既定設計）、漏洩前提で IAM Conditions / VPC Service Controls / Audit alerts を活用
- **ローテーション周期**: bootstrap キーのローテーション周期は組織のポリシーに従う（より高頻度なローテーションは漏洩時の有効期間を短縮できる）

### 採用しないアプローチ: サンドボックス設定での常時許可

`~/.claude/settings.json` の `sandbox` 設定に必要な許可を加えれば、サンドボックス内でも `gc-vault exec` が動作するように見えるかもしれません。**しかし実際には、`op` CLI が 1Password デスクトップアプリと通信する経路は単一の Unix ソケットでは完結しない** ため、Unix ソケット複数 + macOS 固有の IPC 経路の許可など、実質的にサンドボックスを大きく緩める必要があります。

このアプローチを推奨しない理由:

- 1Password の更新で内部経路が変わると突然動かなくなる、あるいは知らないうちに別経路で疎通するなど、**設定がフラジャイル**
- 一度許可してしまうと、Claude Code 内のプロンプトインジェクション時に 1Password 全体への到達が常時開通している状態になる
- 推奨運用と比べた利点がない: 1Password の認証プロンプトは Claude Code の Bash プロセスモデル由来の挙動なので両方法で同じく出るが、本アプローチでは `dangerouslyDisableSandbox: true` に伴う Bash 承認プロンプトという防御層が失われる

## ドキュメント

- [docs/runbook-iam-setup.md](./docs/runbook-iam-setup.md) — GCP / IAM 側のセットアップ手順
