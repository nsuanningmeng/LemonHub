<div align="center">

![LemonHub](/web/public/logo.png)

# LemonHub

**Multi-tenant AI API gateway with a built-in agent/reseller franchise, an extended referral-commission system, and a support-ticket suite — built on new-api.**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <strong>English</strong> |
  <a href="./README.fr.md">Français</a> |
  <a href="./README.ja.md">日本語</a>
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
  <a href="#overview">Overview</a> •
  <a href="#features">Features</a> •
  <a href="#quick-start">Quick Start</a> •
  <a href="#agent--reseller-franchise">Agent franchise</a> •
  <a href="#deployment">Deployment</a>
</p>

</div>

## Overview

LemonHub is a secondary-development fork of [new-api](https://github.com/QuantumNous/new-api) (which itself builds on [One API](https://github.com/songquanpeng/one-api)). It keeps the full new-api gateway — a unified API in front of 40+ AI providers (OpenAI, Claude, Gemini, Azure, AWS Bedrock, …) with billing, rate-limiting and an admin dashboard — and adds, on top of it:

- a **multi-tenant, agent-franchise layer** (one deployment serves many sub-sites, each run by a reseller with their own domain, payment merchant and prepaid procurement wallet);
- an **extended referral-commission system** (first-recharge bonus, ongoing percentage commission, per-user rates, admin leaderboard, and a cash-settled promoter mode);
- a **support-ticket and email-campaign suite**;
- **one-click client onboarding** (Connect Hub);
- **billing, payment, relay, security and migration** controls.

The relay / channel-forwarding / billing core is kept compatible with upstream so new-api features and fixes merge cleanly; LemonHub's additions live around it.

> [!IMPORTANT]
> - This project is intended solely for lawful and authorized AI API gateway, organization-level authentication, multi-model management, usage analytics, cost accounting, and private/reseller deployment scenarios.
> - You must lawfully obtain upstream API keys, accounts, model services, and interface permissions, and comply with upstream terms of service and applicable laws and regulations.
> - When providing generative AI services to the public, complete all required filing, licensing, content safety, real-name verification, log retention, tax, payment, and upstream authorization obligations required by your jurisdiction.

<a id="features"></a>

## Features

LemonHub provides the following capabilities alongside the upstream new-api gateway. For version-specific changes and upgrade instructions, see [GitHub Releases](https://github.com/nsuanningmeng/LemonHub/releases).

### 1. Multi-tenant sub-sites

- One deployment and one database serve many independent sub-sites; each incoming request is routed to a sub-site by its `Host` (domain middleware).
- Per-site customization: name, logo, notice, footer and homepage hero copy, all configurable per sub-site.
- Per-site data isolation: site-owned users, tokens, logs and related records are scoped by `site_id`, while shared upstream channels and system settings remain under main-site control. Usernames are unique **per site** (`(site_id, username)`); passwords, 2FA and OAuth bindings, registration and login are scoped to the current site.
- A main-site management page to create and administer sub-sites.

### 2. Agent / reseller franchise

- Two roles: **main-site admin** (platform owner — owns the deployment, the upstream channels, and sells wholesale quota) and **sub-site admin** (agent — runs a sub-site through a dedicated Agent Console backed by `SiteAdminAuth` endpoints).
- **Procurement wallet**: each agent prepays the platform into a wallet kept in integer milli-CNY. Every end-user recharge atomically debits the wallet at wholesale price in the same transaction; the wallet never goes negative and every change writes a ledger entry. The main site can top up, adjust, reconcile, void and refund.
- **Per-agent wholesale (discount) rate**: `DiscountRate` is a basis-of-10000 integer (`10000` = face price, `7000` = 70%). Wholesale cost = `face × DiscountRate / 10000`; the agent keeps the margin.
- **Per-site payment collection**: each sub-site configures its **own** EasyPay (易支付) merchant. End-user recharges flow into the agent's merchant; the platform settles the wallet in the same DB transaction (credit user **and** debit agent wallet), idempotently, with **auto-degradation** — when an agent's wallet is drained or unconfigured, online recharge transparently disappears for that sub-site while issued quota and other gateways are unaffected.
- **Per-site redemption codes**: agents generate and void codes scoped to their own site, with cross-site isolation and reconciliation.
- **Per-site custom-model markup** (route A): an agent can mark up specific model calls on their site; the platform's wholesale settlement is never undercut.
- A reseller-franchise landing page, a configurable Contact page, and nav/footer entries.

### 3. Referral commission system

LemonHub's referral system includes:

- Referral reward is credited **after the invitee's first successful top-up** (not at registration), plus an **ongoing percentage commission** on every recharge the invitee makes; both run through an idempotent per-event ledger with stats APIs.
- Stripe refunds and disputes claw back the corresponding quota and referral rewards.
- A user-facing referral detail page and personal contribution leaderboard.
- **Per-user commission-rate override** that overrides the global rate (unset inherits the global; `0` disables commission for that inviter).
- An **admin site-wide referral leaderboard** (summary cards + ranking with search, sort and pagination, behind `AdminAuth`).
- **Cash-settled promoter mode**: a per-user flag for promoters who are paid off-platform in cash. When on, the platform invite bonus is suppressed and recharge commission is recorded in a ledger as cash owed (not credited to the platform balance); a cash-payout ledger tracks the outstanding balance with overpay-safe, concurrency-safe settlement. Money-policy operations (mark promoter, set commission rate, record/view cash settlement) are restricted to the root/owner account.

### 4. Support tickets and email campaigns

- A user ticket desk and an admin ticket-management view, with priority.
- An email-campaign / bulk-promotion tool with attachment upload, bulk send, rate-limiting, cleanup and audit.
- Dedicated marketing SMTP, unsubscribe/suppression controls and recovery for interrupted bulk sends.

### 5. Connect Hub (one-click client onboarding)

- One-click client setup for Claude Code, Codex, Gemini CLI, Chatbox, Cherry Studio, VS Code and more. The generated configuration targets **the domain the user is actually visiting**, so it works correctly across multiple domains.

### 6. Billing and token routing

- **Token multi-group priority failover**: a token can carry an ordered group list and fail over between groups; **tiered/expression billing is computed against the group actually used**.
- The default-frontend ratio editor surfaces models that currently have **no price set**, so they are not silently missed.

### 7. Payment and relay reliability

- EasyPay callbacks are exempted from gzip handling and the global rate-limiter and get a dedicated, lenient backstop limiter; `notify_url` is pinned to a stable domain.
- Payment callback/return URLs and the first-paint page `<title>` follow the visited (trusted) domain, with Host-spoofing protection.
- EasyPay settlement treats signed callbacks as triggers and independently queries the owning merchant gateway; merchant/order identity and the exact stored amount must match before quota or subscriptions are issued.
- Subscription checkouts snapshot fulfillment terms; EasyPay limited-plan orders atomically reserve and safely reuse their pending checkout, preventing plan-edit races and duplicate payable orders.
- Relay retries honor configured 504/524 status codes; non-streaming `BadResponseBody` responses are retryable; when all channels are exhausted the real upstream error is returned.
- Expired subscriptions return users to their original group (`prev_user_group` is preserved across renewal).

### 8. Model-performance settings

- Success-rate threshold, error-code whitelist, and "no data = 100%" handling.
- Manual and scheduled tests of enabled channels contribute health samples to every configured model/group pair; groups that are no longer available are excluded from displayed model success rates.
- Relay and channel-test errors count as failures only when their HTTP status matches the configured performance error-code whitelist; other errors count as successful samples. An empty whitelist counts all errors as failures.
- Performance views follow currently enabled, available, priced model/group pairs; removed groups no longer affect aggregate or trend metrics.

### 9. Security and migration hardening

- SSRF protection for advanced custom channels.
- Private ticket/media access controls, sensitive log redaction, and explicit API-key group selection.
- Database-migration safety on all three engines (SQLite / MySQL / PostgreSQL): fail-closed preflight for `price_amount` precision, subscription price-precision preflight, fast-path fail-stop preflight, and a `site_id` NULL backfill so main-site queries never hide legacy rows.

### 10. Packaging and documentation

- Docker images are published to GitHub Container Registry (`ghcr.io/nsuanningmeng/lemonhub`), multi-arch (amd64 + arm64); `docker-compose` points at the fork image by default.
- GitHub Releases provide server binaries for Linux, macOS and Windows, a Windows Electron app, and checksum files.
- README editions in English, Simplified Chinese, Traditional Chinese, French and Japanese, plus step-by-step sub-site / agent guides.

> New to the agent model? Read the step-by-step guide: **[Sub-site / Agent Franchise Guide (中文)](./docs/subsite-guide.md)** · [English](./docs/subsite-guide.en.md)

## Quick Start

### Docker Compose (recommended)

```bash
git clone https://github.com/nsuanningmeng/LemonHub.git
cd LemonHub

# The supplied Compose file starts PostgreSQL and Redis; change both passwords
# and their matching connection strings before exposing the service.
nano docker-compose.yml

# Start (the compose file already points at the LemonHub image)
docker compose up -d
```

<details>
<summary>Plain Docker command</summary>

```bash
docker pull ghcr.io/nsuanningmeng/lemonhub:latest

# SQLite (default — mount /data to persist)
docker run --name lemonhub -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  ghcr.io/nsuanningmeng/lemonhub:latest

# Remote MySQL (replace the host and credentials; the host must be reachable from the container)
docker run --name lemonhub -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="user:password@tcp(db.example.internal:3306)/lemonhub" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  ghcr.io/nsuanningmeng/lemonhub:latest

# For PostgreSQL, use SQL_DSN="postgresql://user:password@db.example.internal:5432/lemonhub".
```

</details>

After deployment, open `http://localhost:3000` and complete the initial setup wizard to create the **root / platform (main-site) administrator**.

> [!WARNING]
> When operating LemonHub as a public or reseller AI service, first complete all required filing, licensing, content safety, real-name verification, log retention, tax, payment, and upstream authorization obligations.

## Agent / Reseller Franchise

LemonHub is built around two roles:

- **Main-site admin (platform owner)** — owns the deployment and the AI channels (upstream keys), and sells wholesale quota. Creates sub-sites, funds agent wallets, and sets each agent's discount rate.
- **Sub-site admin (agent / reseller)** — runs a sub-site on their own domain, configures their own payment merchant and appearance, and serves their own end-users.

Money flow at a glance (example, discount `7000` = 70%):

```
End-user pays ¥100  ──►  Agent's OWN EasyPay merchant   (agent keeps ¥100)
        │
        ▼ (callback, one DB transaction, idempotent)
   User credited ¥100 of quota   +   Agent procurement wallet debited ¥70
                                          └─ ¥30 is the agent's margin
```

The full walkthrough — what each role must prepare, and every step — is here:
**[Sub-site / Agent Franchise Guide (中文)](./docs/subsite-guide.md)** · **[English](./docs/subsite-guide.en.md)**

## Inherited from new-api

LemonHub keeps new-api's gateway capabilities, including:

- **Formats**: OpenAI Chat/Responses/Realtime, Claude Messages, Google Gemini, Rerank (Cohere/Jina), Image/Audio/Embedding, Midjourney-Proxy, Suno, Dify.
- **Format conversion**: OpenAI ⇄ Claude Messages, OpenAI → Gemini, thinking-to-content, reasoning-effort suffixes.
- **Intelligent routing**: weighted random channels, automatic retry on failure (configurable retry status codes), user-level rate limiting.
- **Billing**: per-request / usage-based / cache-hit accounting, tiered and expression pricing, with optional EasyPay, Stripe, Creem, Waffo and Waffo Pancake payment integrations after merchant setup.
- **Auth**: JWT, WebAuthn/Passkeys, OAuth (GitHub, Discord, OIDC, LinuxDO, Telegram, WeChat).
- **UI**: modern dashboard, seven frontend locales (en, zh, zh-TW, fr, ja, ru, vi), data dashboard, model-performance metrics.

For gateway/API details, refer to the upstream [new-api documentation](https://docs.newapi.pro).

## Deployment

> [!TIP]
> Latest image: `ghcr.io/nsuanningmeng/lemonhub:latest` (multi-arch: amd64 + arm64).

### Requirements

| Component | Requirement |
|---|---|
| Local DB | SQLite (Docker must mount `/data`) |
| Remote DB | MySQL ≥ 5.7.8 or PostgreSQL ≥ 9.6 |
| Cache (recommended) | Redis |
| Engine | Docker / Docker Compose |
| Source build | Go 1.26.8 and Bun |

### Common environment variables

| Variable | Description | Default |
|---|---|---|
| `SESSION_SECRET` | Session secret (required for multi-node) | - |
| `CRYPTO_SECRET` | Encryption secret; nodes sharing Redis must use the same value | Defaults to `SESSION_SECRET` |
| `SESSION_COOKIE_SECURE` | Enable Secure refresh cookies and refresh/logout origin checks for HTTPS deployments | `false` |
| `SESSION_COOKIE_TRUSTED_URL` | Exact HTTPS origins required when `SESSION_COOKIE_SECURE=true`; not a relay CORS list | - |
| `TRUSTED_PROXIES` | Trusted reverse-proxy IPs/CIDRs; `none` ignores forwarded headers | Private/loopback ranges (with a warning) |
| `SQL_DSN` | Database connection string (MySQL/PostgreSQL) | - |
| `REDIS_CONN_STRING` | Redis connection string | - |
| `TRUSTED_REDIRECT_DOMAINS` | Comma-separated trusted domains for payment redirect / multi-domain callbacks | - |
| `PAYMENT_WEBHOOK_RATE_LIMIT` | Generous per-IP backstop for payment notify webhooks (requests / window) | `1800` |
| `PAYMENT_WEBHOOK_RATE_LIMIT_DURATION` | Window for the above (seconds) | `60` |
| `STREAMING_TIMEOUT` | Streaming no-response timeout (seconds) | `300` |
| `RELAY_RESPONSE_HEADER_TIMEOUT` | Upstream response-header timeout (seconds); `0` disables it. Streaming after headers is unaffected; allow enough time for non-streaming generation | `1800` |
| `MAX_REQUEST_BODY_MB` | Max request body (MB, after decompression) | `128` |

Rate-limit and most tuning variables fall back to sensible code defaults, so they are not required in `.env`/compose. See `.env.example` for the documented optional knobs.
For production session and reverse-proxy configuration, see [Authentication and session security](./docs/authentication.md).

### Multi-node

> [!WARNING]
> - Set `SESSION_SECRET`, otherwise login state is inconsistent across nodes.
> - Nodes sharing the same Redis must use the same `CRYPTO_SECRET` (defaults to `SESSION_SECRET`), otherwise cached/encrypted data cannot be shared.

### Retry and cache

- Retry: `Settings → Operation Settings → Route Reliability` (failure retry count + auto-retry status-code ranges).
- Cache: `REDIS_CONN_STRING` (recommended) or `MEMORY_CACHE_ENABLED`.

## Built on new-api

LemonHub is an AGPL-licensed fork. Credit to the upstream projects:

| Project | Role |
|---|---|
| [new-api](https://github.com/QuantumNous/new-api) | Direct upstream — the gateway LemonHub extends |
| [One API](https://github.com/songquanpeng/one-api) | Original project base (MIT) |

LemonHub regularly syncs with upstream new-api.

## License

This project is licensed under the [GNU Affero General Public License v3.0 (AGPLv3)](./LICENSE), inheriting the upstream license.

Per AGPLv3 Section 7 additional terms, modified versions must preserve the author attribution notice `Frontend design and development by New API contributors.` in the appropriate legal/about/footer location, and must preserve a visible link to the original project: <https://github.com/QuantumNous/new-api>.

This is an open-source project developed based on [One API](https://github.com/songquanpeng/one-api) (MIT License).

## Help and Contributing

- Issues and feature requests: [LemonHub Issues](https://github.com/nsuanningmeng/LemonHub/issues)
- Releases and downloads: [GitHub Releases](https://github.com/nsuanningmeng/LemonHub/releases)
- Sub-site guide: [中文](./docs/subsite-guide.md) · [English](./docs/subsite-guide.en.md)
- Gateway/API reference: [new-api docs](https://docs.newapi.pro)

Contributions of all kinds are welcome — bug reports, features, docs and code.

<div align="center">
<sub>LemonHub — an agent-franchise layer on top of <a href="https://github.com/QuantumNous/new-api">new-api</a>.</sub>
</div>
