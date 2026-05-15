# Runbook: GCP IAM セットアップ手順

`gc-vault` を利用するために必要な、GCP 側の Service Account / IAM 構成を行う手順です。

> この手順は **個人 1 名 × プロジェクト 1 つ** を最小単位とします。複数プロジェクト（dev / staging / prod など）に対して `gc-vault` を使いたい場合は、対象プロジェクトの数だけセクション 2〜5 を繰り返してください（「[7. 複数プロジェクトに対するセットアップ](#7-複数プロジェクトに対するセットアップ)」参照）。

## 1. 前提条件

### 必要なツール

- `gcloud` CLI（最新版）
- `op` CLI（1Password、認証済み）
- 対象 GCP プロジェクトに対する以下のいずれかの権限を持っていること
  - `roles/owner`
  - `roles/iam.serviceAccountAdmin` + `roles/resourcemanager.projectIamAdmin`
  - 加えて、SA キー発行のため `roles/iam.serviceAccountKeyAdmin`

### 用語

| 語 | 意味 |
|---|---|
| `bootstrap SA` | 1Password に鍵 JSON を保管する、借用元の SA。`serviceAccountTokenCreator` 以外の権限を持たない |
| `target SA` | 実際にリソースを操作する SA。`bootstrap` から借用される側 |
| `USER_HANDLE` | あなたのユーザー識別子（lowercase, 6-30 chars, alphanumeric + dash）。例: `alice` |
| `PROJECT` | 対象 GCP プロジェクトの ID。例: `my-app-dev` |

## 2. 変数の準備

シェルに以下を export してから実行してください。**ご自身の値に置き換えてください**。

```bash
export USER_HANDLE="alice"          # ご自身のハンドル名に置き換え
export PROJECT="my-app-dev"         # 対象 GCP プロジェクト ID に置き換え
export OP_VAULT="Private"           # 1Password の保管先 vault
```

## 3. SA 作成と権限付与

### 3-1. bootstrap SA を作成

```bash
gcloud iam service-accounts create "bootstrap-${USER_HANDLE}" \
  --project="${PROJECT}" \
  --display-name="bootstrap (${USER_HANDLE})" \
  --description="gc-vault bootstrap SA for ${USER_HANDLE}. Stored in 1Password."
```

### 3-2. target SA (readonly) を作成

```bash
gcloud iam service-accounts create "readonly-${USER_HANDLE}" \
  --project="${PROJECT}" \
  --display-name="readonly (${USER_HANDLE})" \
  --description="gc-vault target SA (readonly) for ${USER_HANDLE}."
```

### 3-3. target SA にプロジェクト権限を付与

```bash
gcloud projects add-iam-policy-binding "${PROJECT}" \
  --member="serviceAccount:readonly-${USER_HANDLE}@${PROJECT}.iam.gserviceaccount.com" \
  --role="roles/viewer"
```

> 必要に応じて、`roles/viewer` よりも狭いカスタムロール（例: 特定リソースの閲覧のみ）に置き換えてください。

> プロジェクトの既存 IAM ポリシーに条件付き binding が含まれている場合、`add-iam-policy-binding` 実行時に「条件を指定してください」というプロンプトが表示されます。本ステップは無条件で付与するのが想定挙動なので、対話的に `None` を選択するか、コマンドに `--condition=None` を明示的に追加してください。

### 3-4. bootstrap → target の借用権限を付与

最も重要なステップです。**この設定により bootstrap SA が target SA を借用できるようになります**。

```bash
gcloud iam service-accounts add-iam-policy-binding \
  "readonly-${USER_HANDLE}@${PROJECT}.iam.gserviceaccount.com" \
  --member="serviceAccount:bootstrap-${USER_HANDLE}@${PROJECT}.iam.gserviceaccount.com" \
  --role="roles/iam.serviceAccountTokenCreator" \
  --project="${PROJECT}"
```

> `add-iam-policy-binding` の対象は **target SA リソース自体** であり、プロジェクトではないことに注意。

## 4. bootstrap SA の鍵を発行 → 1Password へ

### 4-1. 鍵 JSON を発行

```bash
TMP_KEY="$(mktemp -t gc-vault-key)"
gcloud iam service-accounts keys create "${TMP_KEY}" \
  --iam-account="bootstrap-${USER_HANDLE}@${PROJECT}.iam.gserviceaccount.com"
```

### 4-2. 1Password にアップロード

ドキュメント名は `${PROJECT}-bootstrap` の形式で統一します（`gc-vault` の config.toml が参照するパス規約）。

```bash
op document create "${TMP_KEY}" \
  --title="${PROJECT}-bootstrap" \
  --vault="${OP_VAULT}"
```

### 4-3. ローカルから完全削除

```bash
rm -P "${TMP_KEY}"   # macOS の上書き削除
```

> **重要**: ローカルファイルシステムには鍵を残さないでください。`~/Downloads` や `/tmp` への保管は厳禁。

## 5. 動作確認

### 5-1. 1Password から取得できるか

```bash
op document get "${PROJECT}-bootstrap" --vault="${OP_VAULT}" | jq -r .client_email
# → bootstrap-${USER_HANDLE}@${PROJECT}.iam.gserviceaccount.com
```

`client_email` が表示されれば OK。

### 5-2. 借用トークンで実コマンドが動くか

bootstrap SA を「源（source）アカウント」として gcloud に登録し、target SA の access token を取得して、そのトークンで実 API が呼べることを一気通貫で確認します。

> **注意**: gcloud CLI は `GOOGLE_APPLICATION_CREDENTIALS` 環境変数を **見ません**（これは ADC / クライアントライブラリ用）。gcloud の認証は `gcloud auth activate-service-account` で切り替える必要があります。ただしユーザーの通常の gcloud 設定を汚さないため、隔離された設定ディレクトリ（`CLOUDSDK_CONFIG`）を使います。

```bash
TMP_KEY="$(mktemp -t gc-vault-test-key)"
TMP_CONFIG="$(mktemp -d -t gc-vault-test-config)"

op document get "${PROJECT}-bootstrap" --vault="${OP_VAULT}" > "${TMP_KEY}"
chmod 0600 "${TMP_KEY}"

# bootstrap SA を隔離 config に登録（~/.config/gcloud は触らない）
CLOUDSDK_CONFIG="${TMP_CONFIG}" gcloud auth activate-service-account \
  --key-file="${TMP_KEY}" --quiet

# bootstrap → target の借用トークンを取得
ACCESS_TOKEN="$(CLOUDSDK_CONFIG="${TMP_CONFIG}" gcloud auth print-access-token \
  --impersonate-service-account="readonly-${USER_HANDLE}@${PROJECT}.iam.gserviceaccount.com")"

# 取得したトークンを CLOUDSDK_AUTH_ACCESS_TOKEN にセットして実コマンドを実行
# (ここでは CLOUDSDK_CONFIG は元に戻して、通常の gcloud 設定で動作させる)
CLOUDSDK_AUTH_ACCESS_TOKEN="${ACCESS_TOKEN}" \
  gcloud projects describe "${PROJECT}"
# → プロジェクト情報が表示されれば OK

rm -rf "${TMP_CONFIG}"
rm -P "${TMP_KEY}"
unset ACCESS_TOKEN
```

確認ポイントは 2 段階：

- `print-access-token` がエラーなく完了し `ACCESS_TOKEN` が取得できれば、bootstrap → target の impersonate 権限が成立している
- `gcloud projects describe` がプロジェクト情報を返せば、取得したトークンで実 API が呼べている

> `PERMISSION_DENIED: ... iam.serviceAccounts.getAccessToken` のエラーが出る場合、3-4 で付与した `roles/iam.serviceAccountTokenCreator` の binding が反映途中の可能性があります。1〜2 分待ってから再実行してください。それでも解消しない場合は、`gcloud iam service-accounts get-iam-policy "readonly-${USER_HANDLE}@${PROJECT}.iam.gserviceaccount.com" --project="${PROJECT}"` で binding が想定通り付与されているかを確認してください。

> **補足**: 上記の手順は手動検証用です。実際の `gc-vault` MVP では gcloud CLI を介さず、`iamcredentials.googleapis.com` の REST API を直接呼んで借用トークンを取得するため、`CLOUDSDK_CONFIG` の隔離は不要になります。

## 6. 既存のクレデンシャルを削除（移行時のみ）

`gc-vault` 導入前に `gcloud auth login` 等を行っていた場合、ローカルに残っている credentials を削除します。

```bash
gcloud auth revoke --all
gcloud auth application-default revoke --quiet 2>/dev/null || true

# ファイルの存在確認（あれば削除）
rm -P -f ~/.config/gcloud/credentials.db
rm -P -f ~/.config/gcloud/application_default_credentials.json
rm -P -f ~/.config/gcloud/access_tokens.db
```

## 7. 複数プロジェクトに対するセットアップ

複数の GCP プロジェクトに対して `gc-vault` を使いたい場合、セクション 2〜5 を `PROJECT` の値を変えて繰り返します。

```bash
# 例: 3 環境構成 (dev / stg / prod)

export USER_HANDLE="alice"
export OP_VAULT="Private"

# --- dev ---
export PROJECT="my-app-dev"
# セクション 3-5 を実行

# --- stg ---
export PROJECT="my-app-stg"
# セクション 3-5 を実行

# --- prod ---
export PROJECT="my-app-prod"
# セクション 3-5 を実行
```

完了時、以下が成立しているはずです：

- 各プロジェクトに `bootstrap-${USER_HANDLE}` と `readonly-${USER_HANDLE}` の 2 SA が存在
- 1Password (`${OP_VAULT}`) に `${PROJECT}-bootstrap` の名前で各プロジェクト分のドキュメントが保管されている
- ローカルには鍵 JSON が一切残っていない

## 8. ローテーション

```bash
export USER_HANDLE="alice"
export PROJECT="my-app-dev"
export OP_VAULT="Private"

# 既存キー一覧
gcloud iam service-accounts keys list \
  --iam-account="bootstrap-${USER_HANDLE}@${PROJECT}.iam.gserviceaccount.com"

# 新キー発行
TMP_KEY="$(mktemp -t gc-vault-rotate)"
gcloud iam service-accounts keys create "${TMP_KEY}" \
  --iam-account="bootstrap-${USER_HANDLE}@${PROJECT}.iam.gserviceaccount.com"

# 1Password を上書き（旧バージョンは履歴に保持される）
op document edit "${PROJECT}-bootstrap" "${TMP_KEY}" --vault="${OP_VAULT}"

rm -P "${TMP_KEY}"

# 旧キーを削除
gcloud iam service-accounts keys delete <OLD_KEY_ID> \
  --iam-account="bootstrap-${USER_HANDLE}@${PROJECT}.iam.gserviceaccount.com"
```

> bootstrap SA は `roles/iam.serviceAccountTokenCreator` のみを持ち漏洩時のリスクが限定的なため、ローテーションは `gc-vault` のコマンドではなく上記の手動手順で行う方針です。

## 9. 監査確認

借用操作は Cloud Audit Logs に記録されます。以下のクエリで自分の借用履歴を確認できます（`<USER_HANDLE>` と `<PROJECT>` は適宜置き換えてください）：

```
resource.type="service_account"
protoPayload.methodName="GenerateAccessToken"
protoPayload.authenticationInfo.principalEmail="bootstrap-<USER_HANDLE>@<PROJECT>.iam.gserviceaccount.com"
```

`protoPayload.serviceAccountDelegationInfo` フィールドに `bootstrap → target` のチェーンが記録されており、誰がどの target SA を借用したかが追跡可能です。
