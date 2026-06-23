<div align="center">

![LemonHub](/web/default/public/logo.png)

# 🍋 LemonHub

**エージェント／リセラー・フランチャイズ機能を内蔵した、マルチテナント・ホワイトラベル対応の AI API ゲートウェイ**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <a href="./README.md">English</a> |
  <a href="./README.fr.md">Français</a> |
  <strong>日本語</strong>
</p>

<p align="center">
  <a href="https://github.com/nsuanningmeng/LemonHub/blob/main/LICENSE">
    <img src="https://img.shields.io/github/license/nsuanningmeng/LemonHub?color=brightgreen" alt="license">
  </a><!--
  --><a href="https://github.com/nsuanningmeng/LemonHub/releases/latest">
    <img src="https://img.shields.io/github/v/release/nsuanningmeng/LemonHub?color=brightgreen&include_prereleases" alt="release">
  </a><!--
  --><a href="https://github.com/nsuanningmeng/LemonHub/pkgs/container/lemonhub">
    <img src="https://img.shields.io/badge/ghcr.io-lemonhub-blue" alt="docker">
  </a>
</p>

<p align="center">
  <a href="#-quick-start">クイックスタート</a> •
  <a href="#-what-makes-lemonhub-different">LemonHub の特徴</a> •
  <a href="#-white-label--agent-franchise">ホワイトラベル</a> •
  <a href="#-deployment">デプロイ</a> •
  <a href="#-built-on-new-api">new-api をベースに構築</a>
</p>

</div>

## 📝 プロジェクト概要

**LemonHub** は [new-api](https://github.com/QuantumNous/new-api)（さらにその基盤は [One API](https://github.com/songquanpeng/one-api)）の二次開発フォークです。new-api の全機能 — 40 以上の AI プロバイダー（OpenAI、Claude、Gemini、Azure、AWS Bedrock、…）を統一ゲートウェイの背後に集約し、課金・レート制限・管理ダッシュボードを備える — をそのまま維持しつつ、その上に **マルチテナント・ホワイトラベル・エージェントフランチャイズのレイヤー** を追加します:

> 1 つのデプロイ + 1 つのデータベースで、**独立したブランドを持つ多数のサブサイト** を運用できます。各サブサイトは **エージェント（リセラー）** が所有し、自身のドメインを運営し、支払いを **自身の決済加盟店** に集め、**前払いの仕入れウォレット** を通じてプラットフォームから卸値でクォータを購入します。

> [!IMPORTANT]
> - 本プロジェクトは、合法かつ正当に認可された AI API ゲートウェイ、組織レベルの認証、マルチモデル管理、使用状況分析、コスト計算、プライベート／リセラー型デプロイのシナリオでの利用のみを目的としています。
> - 上流の API キー、アカウント、モデルサービス、インターフェース権限を合法的に取得し、上流の利用規約および適用される法令を遵守しなければなりません。
> - 一般向けに生成 AI サービスを提供する場合は、お住まいの法域で求められる届出、ライセンス、コンテンツ安全性、実名確認、ログ保管、税務、決済、上流認可などの義務をすべて完了してください。

---

## ✨ LemonHub の特徴

new-api が提供するすべての機能に加えて、LemonHub は以下を追加します:

| 機能 | 説明 |
|---|---|
| 🏢 **ホワイトラベル・サブサイト** | 1 つのデプロイで多数のブランド付きテナントを運用。各サブサイトは独自のドメイン、名称、ロゴ、お知らせ、フッター、ホームページのヒーロー文言を持ちます。リクエストは `Host` によってサブサイトへルーティングされます。 |
| 🤝 **エージェント／リセラー・フランチャイズ** | プラットフォーム所有者（メインサイト）が **エージェント** をオンボーディングします。各エージェントは専用の **エージェントコンソール** を通じてサブサイトを所有し、自己管理します。 |
| 💰 **仕入れウォレット（整数厘の台帳）** | 各エージェントは整数の *ミリ人民元*（厘）で管理されるウォレットへプラットフォームに前払いします。ユーザーのチャージごとに卸値でウォレットからアトミックに引き落とされ — 残高は決してマイナスにならず、すべての変更が台帳エントリとして記録されます。 |
| 🏷️ **エージェント別の割引（卸値）レート** | `DiscountRate` は 10000 を基準とした整数です（`10000` = 表示価格、`7000` = 70%）。卸値コスト = `表示価格 × DiscountRate / 10000`。差額がエージェントの利幅となります。 |
| 💳 **サイト別の決済集金** | 各サブサイトは **自身の** EasyPay（易支付）加盟店を設定します。ユーザーのチャージはエージェント自身の加盟店アカウントへ流入し、プラットフォームは同一の DB トランザクション内でウォレットを精算します（ユーザーへのクレジット **+** エージェントウォレットの引き落とし）。これは冪等に実行されます。 |
| 🔻 **自動デグレード** | エージェントのウォレットが枯渇（または未設定）になると、そのサブサイトではオンラインチャージが透過的に非表示になります — 発行済みのクォータや他のゲートウェイには影響しません。 |
| 🔒 **サイト別のデータ分離** | すべてのテーブルが `site_id` を保持します。ユーザー名は **サイトごと** に一意です（`(site_id, username)`）。パスワード、2FA、すべての OAuth 連携はサイトごとに分離されます。 |
| 🎟️ **サイト別の引き換えコード** | エージェントは自身のサイトに限定した引き換えコードを生成／無効化でき、サイト間の分離と照合が行われます。 |
| 🔌 **Connect Hub** | ワンクリックのクライアント設定（Claude Code / Codex / Gemini CLI / Chatbox / Cherry Studio / VS Code …）が、**ユーザーが実際にアクセスしているドメイン** を対象にします — マルチドメインに対応しています。 |
| 🌐 **マルチドメインの決済コールバック＆タイトル** | 決済の notify／return URL および初回描画時の `<title>` は、アクセスされた（信頼済みの）ドメインに追従し、Host 偽装に対する保護が行われます。 |

> 📖 **エージェントモデルが初めての方へ。ステップバイステップのガイドをご覧ください:** **[サブサイト／エージェントフランチャイズ・ガイド（中文）](./docs/subsite-guide.md)** · [English](./docs/subsite-guide.en.md)

---

## 🚀 クイックスタート

### Docker Compose を使う（推奨）

```bash
# Clone the project
git clone https://github.com/nsuanningmeng/LemonHub.git
cd LemonHub

# Review/edit the configuration (DB password, ServerAddress, etc.)
nano docker-compose.yml

# Start (the compose file already points at the LemonHub image)
docker compose up -d
```

<details>
<summary><strong>素の Docker コマンドを使う</strong></summary>

```bash
# Pull the latest image
docker pull ghcr.io/nsuanningmeng/lemonhub:latest

# SQLite (default — mount /data to persist)
docker run --name lemonhub -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  ghcr.io/nsuanningmeng/lemonhub:latest

# MySQL / PostgreSQL (set SQL_DSN)
docker run --name lemonhub -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:123456@tcp(localhost:3306)/lemonhub" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  ghcr.io/nsuanningmeng/lemonhub:latest
```

</details>

🎉 デプロイ後、`http://localhost:3000` を開きます。最初に登録されたアカウントが **root ／ プラットフォーム（メインサイト）管理者** になります。

> [!WARNING]
> LemonHub を公開またはリセラー型の AI サービスとして運用する場合は、まず、求められる届出、ライセンス、コンテンツ安全性、実名確認、ログ保管、税務、決済、上流認可などの義務をすべて完了してください。

---

## 🤝 ホワイトラベル／エージェントフランチャイズ

LemonHub は 2 つのロールを中心に構築されています:

- **メインサイト管理者（プラットフォーム所有者 / 主站站长）** — デプロイ、AI チャネル（上流キー）を所有し、卸値でクォータを販売します。サブサイトを作成し、エージェントのウォレットに資金を入れ、各エージェントの割引レートを設定します。
- **サブサイト管理者（エージェント / リセラー / 子站站长）** — 自身のドメイン上でブランド付きのサブサイトを運営し、自身の決済加盟店とブランディングを設定し、自身のエンドユーザーにサービスを提供します。

**お金の流れの概要**（例、割引 `7000` = 70%）:

```
End-user pays ¥100  ──►  Agent's OWN EasyPay merchant   (agent keeps ¥100)
        │
        ▼ (callback, one DB transaction, idempotent)
   User credited ¥100 of quota   +   Agent procurement wallet debited ¥70
                                          └─ ¥30 is the agent's margin
```

各ロールが準備すべきこと、そしてすべてのクリック操作を網羅した、手取り足取りのウォークスルーはこちらです:

➡️ **[📘 サブサイト／エージェントフランチャイズ・ガイド（中文，保姆级）](./docs/subsite-guide.md)** · **[English](./docs/subsite-guide.en.md)**

---

## 🤖 モデル＆機能サポート（new-api から継承）

LemonHub は new-api のゲートウェイ機能を継承しており、以下を含みます:

- **フォーマット**: OpenAI Chat/Responses/Realtime、Claude Messages、Google Gemini、Rerank（Cohere/Jina）、Image/Audio/Embedding、Midjourney-Proxy、Suno、Dify。
- **フォーマット変換**: OpenAI ⇄ Claude Messages、OpenAI → Gemini、thinking-to-content、reasoning-effort のサフィックス。
- **インテリジェントルーティング**: 重み付きランダムチャネル、失敗時の自動リトライ（リトライ対象ステータスコードは設定可能）、トークンのマルチグループ優先フェイルオーバー、ユーザーレベルのレート制限。
- **課金**: リクエスト単位／使用量ベース／キャッシュヒットの計算、段階的／式ベースの価格設定、EasyPay & Stripe チャージ。
- **認証**: JWT、WebAuthn/Passkeys、OAuth（GitHub、Discord、OIDC、LinuxDO、Telegram、WeChat）。
- **UI**: モダンなダッシュボード、多言語（zh/en/fr/ja/vi…）、データダッシュボード、モデルパフォーマンス指標。

> 📚 ゲートウェイ／API の詳細については、上流の [new-api ドキュメント](https://docs.newapi.pro) を参照してください。

---

## 🚢 デプロイ

> [!TIP]
> **最新イメージ:** `ghcr.io/nsuanningmeng/lemonhub:latest`（マルチアーキテクチャ: amd64 + arm64）。

### 要件

| コンポーネント | 要件 |
|---|---|
| **ローカル DB** | SQLite（Docker は `/data` をマウントする必要があります） |
| **リモート DB** | MySQL ≥ 5.7.8 または PostgreSQL ≥ 9.6 |
| **キャッシュ（推奨）** | Redis |
| **エンジン** | Docker / Docker Compose |

### 一般的な環境変数

| 変数 | 説明 | デフォルト |
|---|---|---|
| `SESSION_SECRET` | セッションシークレット（マルチノードでは **必須**） | - |
| `CRYPTO_SECRET` | 暗号化シークレット（共有 Redis では **必須**） | - |
| `SQL_DSN` | データベース接続文字列（MySQL/PostgreSQL） | - |
| `REDIS_CONN_STRING` | Redis 接続文字列 | - |
| `TRUSTED_REDIRECT_DOMAINS` | 決済リダイレクト／マルチドメインコールバック用の信頼済みドメイン（カンマ区切り） | - |
| `PAYMENT_WEBHOOK_RATE_LIMIT` | 決済 notify webhook 向けの IP 単位の余裕を持たせた上限（リクエスト数／ウィンドウ） | `1800` |
| `PAYMENT_WEBHOOK_RATE_LIMIT_DURATION` | 上記のウィンドウ（秒） | `60` |
| `STREAMING_TIMEOUT` | ストリーミングの無応答タイムアウト（秒） | `300` |
| `MAX_REQUEST_BODY_MB` | リクエストボディの最大サイズ（MB、解凍後） | `32` |

> レート制限とほとんどのチューニング変数は、コードの妥当なデフォルト値にフォールバックするため、`.env`/compose で指定する必要は **ありません**。文書化されたオプションの設定項目については `.env.example` を参照してください。

### マルチノードに関する注意

> [!WARNING]
> - `SESSION_SECRET` を **設定** してください — そうしないとノード間でログイン状態が不整合になります。
> - **共有 Redis を使用する場合は** `CRYPTO_SECRET` を **設定** してください — そうしないと暗号化されたデータを復号できません。

### リトライ＆キャッシュ

- **リトライ**: `設定 → 運用設定 → ルート信頼性`（失敗時のリトライ回数 + 自動リトライ対象のステータスコード範囲）。
- **キャッシュ**: `REDIS_CONN_STRING`（推奨）または `MEMORY_CACHE_ENABLED`。

---

## 🧱 new-api をベースに構築

LemonHub は AGPL ライセンスのフォークです。上流プロジェクトに多大な感謝を捧げます:

| プロジェクト | 役割 |
|---|---|
| [new-api](https://github.com/QuantumNous/new-api) | 直接の上流 — LemonHub が拡張するゲートウェイ |
| [One API](https://github.com/songquanpeng/one-api) | 元となるプロジェクト基盤（MIT） |

LemonHub は上流の new-api と定期的に同期します。リレー／課金／チャネル転送のコアは、上流の機能や修正をクリーンにマージできるよう、バイト単位で互換性を保っています。LemonHub の追加機能はその周辺に存在します（サブサイト分離、ウォレット、サイト別決済、ブランディング）。

---

## 📜 ライセンス

本プロジェクトは [GNU Affero General Public License v3.0（AGPLv3）](./LICENSE) の下でライセンスされており、上流のライセンスを継承します。

AGPLv3 第 7 条の追加条項に従い、改変版は適切な法的／About／フッターの箇所に著者帰属表示 `Frontend design and development by New API contributors.` を保持し、かつ元プロジェクトへの可視的なリンク <https://github.com/QuantumNous/new-api> を保持しなければなりません。

これは [One API](https://github.com/songquanpeng/one-api)（MIT ライセンス）をベースに開発されたオープンソースプロジェクトです。

---

## 💬 ヘルプ＆コントリビュート

- 🐛 Issue と機能リクエスト: [LemonHub Issues](https://github.com/nsuanningmeng/LemonHub/issues)
- 📘 サブサイトガイド: [中文](./docs/subsite-guide.md) · [English](./docs/subsite-guide.en.md)
- 📚 ゲートウェイ／API リファレンス: [new-api docs](https://docs.newapi.pro)

あらゆる種類のコントリビュートを歓迎します — バグ報告、機能、ドキュメント、コード。

<div align="center">

LemonHub が役に立ったら、ぜひ ⭐️ をお願いします

<sub>🍋 LemonHub — <a href="https://github.com/QuantumNous/new-api">new-api</a> の上に構築されたホワイトラベル／エージェントフランチャイズのレイヤー。</sub>

</div>
