<div align="center">

![LemonHub](/web/public/logo.png)

# LemonHub

**エージェント／リセラー・フランチャイズ、拡張された紹介報酬システム、サポートチケット一式を内蔵した、マルチテナントの AI API ゲートウェイ。new-api をベースに構築。**

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
  <a href="#概要">概要</a> •
  <a href="#features">主な機能</a> •
  <a href="#クイックスタート">クイックスタート</a> •
  <a href="#エージェント--リセラーフランチャイズ">エージェントフランチャイズ</a> •
  <a href="#デプロイ">デプロイ</a>
</p>

</div>

## 概要

LemonHub は [new-api](https://github.com/QuantumNous/new-api)（さらにその基盤は [One API](https://github.com/songquanpeng/one-api)）の二次開発フォークです。new-api のゲートウェイ機能 — 40 以上の AI プロバイダー（OpenAI、Claude、Gemini、Azure、AWS Bedrock、…）の前段に統一 API を置き、課金・レート制限・管理ダッシュボードを備える — をそのまま維持しつつ、その上に以下を追加します:

- **マルチテナント・エージェントフランチャイズのレイヤー**（1 つのデプロイで多数のサブサイトを運用し、各サブサイトはリセラーが自身のドメイン・決済加盟店・前払いの仕入れウォレットで運営します）。
- **拡張された紹介報酬システム**（初回チャージボーナス、継続的な歩合報酬、ユーザー別レート、管理者向けリーダーボード、現金精算のプロモーターモード）。
- **サポートチケットとメールキャンペーンの一式**。
- **ワンクリックのクライアント設定**（Connect Hub）。
- **課金・決済・リレー・セキュリティ・マイグレーション** に関する制御機能。

リレー／チャネル転送／課金のコアは上流との互換性を保っているため、new-api の機能や修正はクリーンにマージできます。LemonHub の追加機能はその周辺に存在します。

> [!IMPORTANT]
> - 本プロジェクトは、合法かつ正当に認可された AI API ゲートウェイ、組織レベルの認証、マルチモデル管理、使用状況分析、コスト計算、プライベート／リセラー型デプロイのシナリオでの利用のみを目的としています。
> - 上流の API キー、アカウント、モデルサービス、インターフェース権限を合法的に取得し、上流の利用規約および適用される法令を遵守しなければなりません。
> - 一般向けに生成 AI サービスを提供する場合は、お住まいの法域で求められる届出、ライセンス、コンテンツ安全性、実名確認、ログ保管、税務、決済、上流認可などの義務をすべて完了してください。

<a id="features"></a>

## 主な機能

LemonHub は、上流 new-api のゲートウェイ機能に加えて、以下の機能を提供します。バージョンごとの変更点とアップグレード手順は [GitHub Releases](https://github.com/nsuanningmeng/LemonHub/releases) を参照してください。

### 1. マルチテナントのサブサイト

- 1 つのデプロイ・1 つのデータベースで、多数の独立したサブサイトを運用します。受信した各リクエストは `Host`（ドメインミドルウェア）によってサブサイトへルーティングされます。
- サイトごとのカスタマイズ：名称、ロゴ、お知らせ、フッター、ホームページのヒーロー文言を、すべてサブサイトごとに設定できます。
- サイト別のデータ分離: サイトに属するユーザー、トークン、ログなどのレコードを `site_id` で分離し、共通の上流チャネルとシステム設定はメインサイトが管理します。ユーザー名は **サイトごと** に一意です（`(site_id, username)`）。パスワード、2FA、OAuth 連携、登録・ログインは現在のサイトに限定されます。
- サブサイトを作成・管理するためのメインサイト管理ページ。

### 2. エージェント／リセラー・フランチャイズ

- 2 つのロール: **メインサイト管理者**（プラットフォーム所有者 — デプロイと上流チャネルを所有し、卸値でクォータを販売します）と **サブサイト管理者**（エージェント — `SiteAdminAuth` エンドポイントに支えられた専用のエージェントコンソールを通じてサブサイトを運営します）。
- **仕入れウォレット**: 各エージェントは整数のミリ人民元（厘）で管理されるウォレットへプラットフォームに前払いします。ユーザーのチャージごとに、同一トランザクション内で卸値からウォレットがアトミックに引き落とされます。ウォレットは決してマイナスにならず、すべての変更が台帳エントリとして記録されます。メインサイトはチャージ、調整、照合、取り消し、返金を行えます。
- **エージェント別の卸値（割引）レート**: `DiscountRate` は 10000 を基準とした整数です（`10000` = 表示価格、`7000` = 70%）。卸値コスト = `表示価格 × DiscountRate / 10000`。差額がエージェントの利幅となります。
- **サイト別の決済集金**: 各サブサイトは **自身の** EasyPay（易支付）加盟店を設定します。ユーザーのチャージはエージェントの加盟店へ流入し、プラットフォームは同一の DB トランザクション内でウォレットを精算します（ユーザーへのクレジット **かつ** エージェントウォレットの引き落とし）。これは冪等に実行され、**自動デグレード** を備えます — エージェントのウォレットが枯渇または未設定になると、そのサブサイトではオンラインチャージが透過的に非表示になり、発行済みのクォータや他のゲートウェイには影響しません。
- **サイト別の引き換えコード**: エージェントは自身のサイトに限定したコードを生成・無効化でき、サイト間の分離と照合が行われます。
- **サイト別のカスタムモデル上乗せ**（ルート A）: エージェントは自身のサイトで特定のモデル呼び出しに価格を上乗せできます。プラットフォームの卸値精算が下回られることはありません。
- リセラーフランチャイズのランディングページ、設定可能な Contact ページ、ナビ／フッターのエントリ。

### 3. 紹介報酬システム

LemonHub の紹介報酬システムには、以下の機能があります:

- 紹介報酬は **被招待者が初めてチャージに成功した後** に付与され（登録時ではありません）、さらに被招待者が行うすべてのチャージに対して **継続的な歩合報酬** が発生します。いずれも冪等なイベント単位の台帳で処理され、統計 API を備えます。
- Stripe の返金や支払い異議申立てが発生した場合は、該当するクォータと紹介報酬を取り消します。
- ユーザー向けの紹介詳細ページと個人の貢献度リーダーボード。
- グローバルレートを上書きする **ユーザー別の報酬レート設定**（未設定の場合はグローバル値を継承し、`0` はその招待者の報酬を無効化します）。
- **管理者向けの全サイト紹介リーダーボード**（サマリーカード + 検索・ソート・ページネーション付きのランキング、`AdminAuth` の背後）。
- **現金精算のプロモーターモード**: プラットフォーム外で現金支払いを受けるプロモーター向けのユーザー単位フラグです。オンにすると、プラットフォームの招待ボーナスは抑制され、チャージ報酬は未払いの現金として台帳に記録されます（プラットフォーム残高には加算されません）。現金支払い台帳が未払い残高を追跡し、過払い防止・並行処理安全な精算を行います。資金ポリシー操作（プロモーター指定、報酬レート設定、現金精算の記録／閲覧）は root／オーナーアカウントに限定されます。

### 4. サポートチケットとメールキャンペーン

- 優先度に対応した、ユーザー向けチケットデスクと管理者向けチケット管理ビュー。
- メールキャンペーン／一括プロモーションツール: 添付ファイルのアップロード、一括送信、レート制限、クリーンアップ、監査。
- マーケティング専用 SMTP、配信停止と送信抑制の管理、中断した一斉送信の復旧。

### 5. Connect Hub（ワンクリックのクライアント設定）

- Claude Code、Codex、Gemini CLI、Chatbox、Cherry Studio、VS Code などのワンクリック設定。生成される設定は **ユーザーが実際にアクセスしているドメイン** を対象とするため、複数のドメインにまたがって正しく動作します。

### 6. 課金とトークンルーティング

- **トークンのマルチグループ優先フェイルオーバー**: トークンは順序付きのグループリストを持ち、グループ間でフェイルオーバーできます。**段階的／式ベースの課金は、実際に使用されたグループに基づいて計算されます**。
- デフォルトフロントエンドのレート（倍率）エディタは、現在 **価格が未設定** のモデルを表示するため、見落とされることがありません。

### 7. 決済とリレーの信頼性

- EasyPay のコールバックは gzip 処理とグローバルレートリミッターの対象外となり、専用の緩やかなバックストップリミッターが適用されます。`notify_url` は安定したドメインに固定されます。
- 決済のコールバック／リターン URL と初回描画ページの `<title>` は、アクセスされた（信頼済みの）ドメインに追従し、Host 偽装に対する保護を備えます。
- EasyPay の署名付きコールバックは照合のきっかけとして扱い、注文を所有する加盟店のゲートウェイへサーバーから独立して問い合わせます。クォータまたはサブスクリプションの付与前に、加盟店と注文の識別情報、保存済みの正確な金額が一致することを確認します。
- サブスクリプションの購入時には付与条件のスナップショットを保存します。EasyPay の購入回数制限付きプランでは、購入枠をアトミックに確保し、保留中の購入手続きを安全に再利用することで、プラン変更との競合や重複決済を防ぎます。
- リレーのリトライは設定された 504/524 ステータスコードに従います。非ストリーミングの `BadResponseBody` 応答もリトライ対象です。すべてのチャネルを使い切った場合は、実際の上流エラーを返します。
- 期限切れのサブスクリプションでは、ユーザーを元のグループへ戻します（`prev_user_group` は更新をまたいで保持されます）。

### 8. モデルパフォーマンス設定

- 成功率のしきい値、エラーコードのホワイトリスト、「データなし = 100%」の扱い。
- 有効なチャネルの手動・定期テスト結果は、設定済みのモデルとグループの各組み合わせの健全性サンプルに反映されます。現在利用できないグループは、表示するモデルの成功率から除外されます。
- リレーおよびチャネルテストのエラーは、HTTP ステータスが設定済みの性能評価用エラーコード一覧に一致する場合のみ失敗として数えます。それ以外のエラーは成功サンプルとして扱い、一覧が空の場合はすべてのエラーを失敗として数えます。
- 性能表示は、現在有効で利用可能かつ価格が設定されたモデルとグループの組み合わせに従います。削除されたグループは集計値や推移に影響しません。

### 9. セキュリティとマイグレーションの堅牢化

- 高度なカスタムチャネル向けの SSRF 対策。
- 非公開のチケット・メディアへのアクセス制御、機密情報を含むログのマスキング、API キー作成時の明示的なグループ選択。
- 3 つのエンジン（SQLite / MySQL / PostgreSQL）すべてでのデータベースマイグレーションの安全性: `price_amount` 精度のフェイルクローズドなプリフライト、サブスクリプション価格精度のプリフライト、ファストパスのフェイルストップ・プリフライト、そしてメインサイトのクエリが旧来の行を隠さないようにする `site_id` の NULL バックフィル。

### 10. パッケージングとドキュメント

- Docker イメージは GitHub Container Registry（`ghcr.io/nsuanningmeng/lemonhub`）に公開され、マルチアーキテクチャ（amd64 + arm64）に対応します。`docker-compose` はデフォルトでフォークのイメージを指します。
- GitHub Releases では、Linux・macOS・Windows 向けのサーバーバイナリ、Windows 向け Electron アプリ、チェックサムファイルを提供します。
- 英語・簡体字中国語・繁体字中国語・フランス語・日本語の README と、サブサイト／代理店向けの手順ガイドを提供します。

> エージェントモデルが初めての方は、ステップバイステップのガイドをご覧ください: **[サブサイト／エージェントフランチャイズ・ガイド（中文）](./docs/subsite-guide.md)** · [English](./docs/subsite-guide.en.md)

## クイックスタート

### Docker Compose を使う（推奨）

```bash
git clone https://github.com/nsuanningmeng/LemonHub.git
cd LemonHub

# 付属の Compose ファイルは PostgreSQL と Redis を起動します。公開前に両方の
# パスワードと、それぞれに対応する接続文字列を変更してください。
nano docker-compose.yml

# 起動します（Compose ファイルはすでに LemonHub のイメージを指定しています）。
docker compose up -d
```

<details>
<summary>素の Docker コマンド</summary>

```bash
docker pull ghcr.io/nsuanningmeng/lemonhub:latest

# SQLite（デフォルト。データを永続化するため /data をマウントします）
docker run --name lemonhub -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  ghcr.io/nsuanningmeng/lemonhub:latest

# リモート MySQL（ホストと認証情報を変更してください。コンテナから到達できるホストが必要です）
docker run --name lemonhub -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="user:password@tcp(db.example.internal:3306)/lemonhub" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  ghcr.io/nsuanningmeng/lemonhub:latest

# PostgreSQL の場合は SQL_DSN="postgresql://user:password@db.example.internal:5432/lemonhub" を使用します。
```

</details>

デプロイ後、`http://localhost:3000` を開き、初期設定ウィザードで **root／プラットフォーム（メインサイト）管理者** を作成します。

> [!WARNING]
> LemonHub を公開またはリセラー型の AI サービスとして運用する場合は、まず、求められる届出、ライセンス、コンテンツ安全性、実名確認、ログ保管、税務、決済、上流認可などの義務をすべて完了してください。

## エージェント / リセラー・フランチャイズ

LemonHub は 2 つのロールを中心に構築されています:

- **メインサイト管理者（プラットフォーム所有者）** — デプロイと AI チャネル（上流キー）を所有し、卸値でクォータを販売します。サブサイトを作成し、エージェントのウォレットに資金を入れ、各エージェントの割引レートを設定します。
- **サブサイト管理者（エージェント／リセラー）** — 自身のドメインでサブサイトを運営し、自身の決済加盟店と外観を設定し、自身のエンドユーザーにサービスを提供します。

資金の流れ（例：割引率 `7000` = 70%）:

```
利用者が ¥100 支払う ──► 代理店自身の EasyPay 加盟店口座（代理店が ¥100 を受領）
        │
        ▼ （コールバック後、単一の DB トランザクションで冪等に処理）
  利用者に ¥100 相当の利用枠を付与 ＋ 代理店の仕入れウォレットから ¥70 を引き落とし
                                             └─ ¥30 が代理店の利幅
```

各ロールが準備すべきこと、そしてすべての手順を網羅した完全なウォークスルーはこちらです:
**[サブサイト／エージェントフランチャイズ・ガイド（中文）](./docs/subsite-guide.md)** · **[English](./docs/subsite-guide.en.md)**

## new-api から継承した機能

LemonHub は new-api のゲートウェイ機能を継承しており、以下を含みます:

- **フォーマット**: OpenAI Chat/Responses/Realtime、Claude Messages、Google Gemini、Rerank（Cohere/Jina）、Image/Audio/Embedding、Midjourney-Proxy、Suno、Dify。
- **フォーマット変換**: OpenAI ⇄ Claude Messages、OpenAI → Gemini、thinking-to-content、reasoning-effort のサフィックス。
- **インテリジェントルーティング**: 重み付きランダムチャネル、失敗時の自動リトライ（リトライ対象ステータスコードは設定可能）、ユーザーレベルのレート制限。
- **課金**: リクエスト単位／使用量ベース／キャッシュヒットの計算、段階的・式ベースの価格設定。加盟店設定後は EasyPay、Stripe、Creem、Waffo、Waffo Pancake の決済連携を利用できます。
- **認証**: JWT、WebAuthn/Passkeys、OAuth（GitHub、Discord、OIDC、LinuxDO、Telegram、WeChat）。
- **UI**: モダンなダッシュボード、フロントエンドの 7 言語（en、zh、zh-TW、fr、ja、ru、vi）、データダッシュボード、モデルパフォーマンス指標。

ゲートウェイ／API の詳細については、上流の [new-api ドキュメント](https://docs.newapi.pro) を参照してください。

## デプロイ

> [!TIP]
> 最新イメージ: `ghcr.io/nsuanningmeng/lemonhub:latest`（マルチアーキテクチャ: amd64 + arm64）。

### 要件

| コンポーネント | 要件 |
|---|---|
| ローカル DB | SQLite（Docker は `/data` をマウントする必要があります） |
| リモート DB | MySQL ≥ 5.7.8 または PostgreSQL ≥ 9.6 |
| キャッシュ（推奨） | Redis |
| エンジン | Docker / Docker Compose |
| ソースからのビルド | Go 1.26.8 と Bun |

### 一般的な環境変数

| 変数 | 説明 | デフォルト |
|---|---|---|
| `SESSION_SECRET` | セッションシークレット（マルチノードでは必須） | - |
| `CRYPTO_SECRET` | 暗号化シークレット（Redis を共有するノード間で同一にする） | 既定では `SESSION_SECRET` を使用 |
| `SESSION_COOKIE_SECURE` | HTTPS 環境で Secure 属性付きの更新用 Cookie と、更新・ログアウト時のオリジン検証を有効にする | `false` |
| `SESSION_COOKIE_TRUSTED_URL` | `SESSION_COOKIE_SECURE=true` のときに必要な HTTPS オリジンの完全一致リスト。リレーの CORS 許可リストではありません | - |
| `TRUSTED_PROXIES` | 信頼するリバースプロキシの IP／CIDR。`none` は転送ヘッダーを無視します | プライベート／ループバック範囲（警告あり） |
| `SQL_DSN` | データベース接続文字列（MySQL/PostgreSQL） | - |
| `REDIS_CONN_STRING` | Redis 接続文字列 | - |
| `TRUSTED_REDIRECT_DOMAINS` | 決済リダイレクト／マルチドメインコールバック用の信頼済みドメイン（カンマ区切り） | - |
| `PAYMENT_WEBHOOK_RATE_LIMIT` | 決済 notify webhook 向けの IP 単位の余裕を持たせた上限（リクエスト数／ウィンドウ） | `1800` |
| `PAYMENT_WEBHOOK_RATE_LIMIT_DURATION` | 上記のウィンドウ（秒） | `60` |
| `STREAMING_TIMEOUT` | ストリーミングの無応答タイムアウト（秒） | `300` |
| `RELAY_RESPONSE_HEADER_TIMEOUT` | 上流のレスポンスヘッダー待機タイムアウト（秒）。`0` で無効化します。ヘッダー受信後のストリーミングには影響しません。非ストリーミング生成に十分な時間を設定してください | `1800` |
| `MAX_REQUEST_BODY_MB` | リクエストボディの最大サイズ（MB、解凍後） | `128` |

レート制限とほとんどのチューニング変数は、コードの妥当なデフォルト値にフォールバックするため、`.env`/compose で指定する必要はありません。文書化されたオプションの設定項目については `.env.example` を参照してください。

本番環境のセッションとリバースプロキシの設定については、[認証とセッションのセキュリティ](./docs/authentication.md) を参照してください。

### マルチノード

> [!WARNING]
> - `SESSION_SECRET` を設定してください。設定しないとノード間でログイン状態が不整合になります。
> - 同じ Redis を共有するノードは同じ `CRYPTO_SECRET`（既定では `SESSION_SECRET`）を使用してください。異なるとキャッシュ／暗号化データを共有できません。

### リトライとキャッシュ

- リトライ: `Settings → Operation Settings → Route Reliability`（失敗時のリトライ回数 + 自動リトライ対象のステータスコード範囲）。
- キャッシュ: `REDIS_CONN_STRING`（推奨）または `MEMORY_CACHE_ENABLED`。

## new-api をベースに構築

LemonHub は AGPL ライセンスのフォークです。上流プロジェクトに感謝します:

| プロジェクト | 役割 |
|---|---|
| [new-api](https://github.com/QuantumNous/new-api) | 直接の上流 — LemonHub が拡張するゲートウェイ |
| [One API](https://github.com/songquanpeng/one-api) | 元となるプロジェクト基盤（MIT） |

LemonHub は上流の new-api と定期的に同期します。

## ライセンス

本プロジェクトは [GNU Affero General Public License v3.0（AGPLv3）](./LICENSE) の下でライセンスされており、上流のライセンスを継承します。

AGPLv3 第 7 条の追加条項に従い、改変版は適切な法的／About／フッターの箇所に著者帰属表示 `Frontend design and development by New API contributors.` を保持し、かつ元プロジェクトへの可視的なリンク <https://github.com/QuantumNous/new-api> を保持しなければなりません。

これは [One API](https://github.com/songquanpeng/one-api)（MIT ライセンス）をベースに開発されたオープンソースプロジェクトです。

## ヘルプとコントリビュート

- Issue と機能リクエスト: [LemonHub Issues](https://github.com/nsuanningmeng/LemonHub/issues)
- リリースとダウンロード: [GitHub Releases](https://github.com/nsuanningmeng/LemonHub/releases)
- サブサイトガイド: [中文](./docs/subsite-guide.md) · [English](./docs/subsite-guide.en.md)
- ゲートウェイ／API リファレンス: [new-api docs](https://docs.newapi.pro)

あらゆる種類のコントリビュートを歓迎します — バグ報告、機能、ドキュメント、コード。

<div align="center">
<sub>LemonHub — <a href="https://github.com/QuantumNous/new-api">new-api</a> を基盤とする代理店フランチャイズ機能。</sub>
</div>
