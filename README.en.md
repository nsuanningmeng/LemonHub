<div align="center">

![LemonHub](/web/default/public/logo.png)

# 🍋 LemonHub

**Multi-tenant, white-label AI API Gateway with a built-in agent / reseller franchise system**

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
  <a href="#-quick-start">Quick Start</a> •
  <a href="#-what-makes-lemonhub-different">Why LemonHub</a> •
  <a href="#-white-label--agent-franchise">White-label</a> •
  <a href="#-deployment">Deployment</a> •
  <a href="#-built-on-new-api">Built on new-api</a>
</p>

</div>

## 📝 Project Description

**LemonHub** is a secondary-development fork of [new-api](https://github.com/QuantumNous/new-api) (which itself builds on [One API](https://github.com/songquanpeng/one-api)). It keeps the full power of new-api — a unified gateway in front of 40+ AI providers (OpenAI, Claude, Gemini, Azure, AWS Bedrock, …) with billing, rate-limiting and an admin dashboard — and adds a **multi-tenant, white-label, agent-franchise layer** on top:

> One deployment + one database can serve **many independently-branded sub-sites**, each owned by an **agent (reseller)** who runs their own domain, collects payments into their **own payment merchant**, and purchases wholesale quota from the platform through a **prepaid procurement wallet**.

> [!IMPORTANT]
> - This project is intended solely for lawful and authorized AI API gateway, organization-level authentication, multi-model management, usage analytics, cost accounting, and private/reseller deployment scenarios.
> - You must lawfully obtain upstream API keys, accounts, model services, and interface permissions, and comply with upstream terms of service and applicable laws and regulations.
> - When providing generative AI services to the public, complete all required filing, licensing, content safety, real-name verification, log retention, tax, payment, and upstream authorization obligations required by your jurisdiction.

---

## ✨ What makes LemonHub different

On top of everything new-api offers, LemonHub adds:

| Capability | Description |
|---|---|
| 🏢 **White-label sub-sites** | One deployment serves many branded tenants. Each sub-site has its own domain(s), name, logo, notice, footer and homepage hero copy. Requests are routed to a sub-site by their `Host`. |
| 🤝 **Agent / reseller franchise** | The platform owner (main site) onboards **agents**; each agent owns and self-administers a sub-site through a dedicated **Agent Console**. |
| 💰 **Procurement wallet (整数厘 ledger)** | Each agent prepays the platform into a wallet kept in integer *milli-CNY* (厘). Every user recharge atomically debits the wallet at wholesale price — never goes negative, every change writes a ledger entry. |
| 🏷️ **Per-agent discount (wholesale) rate** | `DiscountRate` is a basis-of-10000 integer (`10000` = face price, `7000` = 70%). Wholesale cost = `face × DiscountRate / 10000`; the agent keeps the margin. |
| 💳 **Per-site payment collection** | Each sub-site configures its **own** EasyPay (易支付) merchant. User recharges flow into the agent's own merchant account; the platform settles the wallet in the same DB transaction (credit user **+** debit agent wallet), idempotently. |
| 🔻 **Auto-degradation** | When an agent's wallet is drained (or unconfigured), online recharge transparently disappears for that sub-site — issued quota and other gateways are unaffected. |
| 🔒 **Per-site data isolation** | Every table carries a `site_id`; usernames are unique **per site** (`(site_id, username)`); passwords, 2FA and all OAuth bindings are isolated per site. |
| 🎟️ **Per-site redemption codes** | Agents generate/void redemption codes scoped to their own site, with cross-site isolation and reconciliation. |
| 🔌 **Connect Hub** | One-click client setup (Claude Code / Codex / Gemini CLI / Chatbox / Cherry Studio / VS Code …) that targets **the domain the user is actually visiting** — multi-domain aware. |
| 🌐 **Multi-domain payment callbacks & titles** | Payment notify/return URLs and the first-paint `<title>` follow the visited (trusted) domain, with Host-spoofing protection. |

> 📖 **New to the agent model? Read the step-by-step guide:** **[Sub-site / Agent Franchise Guide (中文)](./docs/subsite-guide.md)** · [English](./docs/subsite-guide.en.md)

---

## 🚀 Quick Start

### Using Docker Compose (Recommended)

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
<summary><strong>Using a plain Docker command</strong></summary>

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

🎉 After deployment, open `http://localhost:3000`. The first registered account becomes the **root / platform (main-site) administrator**.

> [!WARNING]
> When operating LemonHub as a public or reseller AI service, first complete all required filing, licensing, content safety, real-name verification, log retention, tax, payment, and upstream authorization obligations.

---

## 🤝 White-label / Agent Franchise

LemonHub is built around two roles:

- **Main-site admin (platform owner / 主站站长)** — owns the deployment, the AI channels (upstream keys), and sells wholesale quota. Creates sub-sites, funds agent wallets, and sets each agent's discount rate.
- **Sub-site admin (agent / reseller / 子站站长)** — runs a branded sub-site on their own domain, configures their own payment merchant and branding, and serves their own end-users.

**The money flow at a glance** (example, discount `7000` = 70%):

```
End-user pays ¥100  ──►  Agent's OWN EasyPay merchant   (agent keeps ¥100)
        │
        ▼ (callback, one DB transaction, idempotent)
   User credited ¥100 of quota   +   Agent procurement wallet debited ¥70
                                          └─ ¥30 is the agent's margin
```

The full, hand-holding walkthrough — what each role must prepare, and every click — is here:

➡️ **[📘 Sub-site / Agent Franchise Guide (中文，保姆级)](./docs/subsite-guide.md)** · **[English](./docs/subsite-guide.en.md)**

---

## 🤖 Model & Feature Support (inherited from new-api)

LemonHub inherits new-api's gateway capabilities, including:

- **Formats**: OpenAI Chat/Responses/Realtime, Claude Messages, Google Gemini, Rerank (Cohere/Jina), Image/Audio/Embedding, Midjourney-Proxy, Suno, Dify.
- **Format conversion**: OpenAI ⇄ Claude Messages, OpenAI → Gemini, thinking-to-content, reasoning-effort suffixes.
- **Intelligent routing**: weighted random channels, automatic retry on failure (configurable retry status codes), token multi-group priority failover, user-level rate limiting.
- **Billing**: per-request / usage-based / cache-hit accounting, tiered/expression pricing, EasyPay & Stripe top-up.
- **Auth**: JWT, WebAuthn/Passkeys, OAuth (GitHub, Discord, OIDC, LinuxDO, Telegram, WeChat).
- **UI**: modern dashboard, multi-language (zh/en/fr/ja/vi…), data dashboard, model performance metrics.

> 📚 For gateway/API details, refer to the upstream [new-api documentation](https://docs.newapi.pro).

---

## 🚢 Deployment

> [!TIP]
> **Latest image:** `ghcr.io/nsuanningmeng/lemonhub:latest` (multi-arch: amd64 + arm64).

### Requirements

| Component | Requirement |
|---|---|
| **Local DB** | SQLite (Docker must mount `/data`) |
| **Remote DB** | MySQL ≥ 5.7.8 or PostgreSQL ≥ 9.6 |
| **Cache (recommended)** | Redis |
| **Engine** | Docker / Docker Compose |

### Common environment variables

| Variable | Description | Default |
|---|---|---|
| `SESSION_SECRET` | Session secret (**required** for multi-node) | - |
| `CRYPTO_SECRET` | Encryption secret (**required** for shared Redis) | - |
| `SQL_DSN` | Database connection string (MySQL/PostgreSQL) | - |
| `REDIS_CONN_STRING` | Redis connection string | - |
| `TRUSTED_REDIRECT_DOMAINS` | Comma-separated trusted domains for payment redirect / multi-domain callbacks | - |
| `PAYMENT_WEBHOOK_RATE_LIMIT` | Generous per-IP backstop for payment notify webhooks (requests / window) | `1800` |
| `PAYMENT_WEBHOOK_RATE_LIMIT_DURATION` | Window for the above (seconds) | `60` |
| `STREAMING_TIMEOUT` | Streaming no-response timeout (seconds) | `300` |
| `MAX_REQUEST_BODY_MB` | Max request body (MB, after decompression) | `32` |

> Rate-limit and most tuning variables fall back to sensible code defaults, so they are **not** required in `.env`/compose. See `.env.example` for the documented optional knobs.

### Multi-node notes

> [!WARNING]
> - **Set** `SESSION_SECRET` — otherwise login state is inconsistent across nodes.
> - **With shared Redis, set** `CRYPTO_SECRET` — otherwise encrypted data cannot be decrypted.

### Retry & cache

- **Retry**: `Settings → Operation Settings → Route Reliability` (failure retry count + auto-retry status-code ranges).
- **Cache**: `REDIS_CONN_STRING` (recommended) or `MEMORY_CACHE_ENABLED`.

---

## 🧱 Built on new-api

LemonHub is an AGPL-licensed fork. Huge credit to the upstream projects:

| Project | Role |
|---|---|
| [new-api](https://github.com/QuantumNous/new-api) | Direct upstream — the gateway LemonHub extends |
| [One API](https://github.com/songquanpeng/one-api) | Original project base (MIT) |

LemonHub regularly syncs with upstream new-api. The relay / billing / channel-forwarding core is kept byte-for-byte compatible so upstream features and fixes can be merged cleanly; LemonHub's additions live around it (sub-site isolation, wallets, per-site payment, branding).

---

## 📜 License

This project is licensed under the [GNU Affero General Public License v3.0 (AGPLv3)](./LICENSE), inheriting the upstream license.

Per AGPLv3 Section 7 additional terms, modified versions must preserve the author attribution notice `Frontend design and development by New API contributors.` in the appropriate legal/about/footer location, and must preserve a visible link to the original project: <https://github.com/QuantumNous/new-api>.

This is an open-source project developed based on [One API](https://github.com/songquanpeng/one-api) (MIT License).

---

## 💬 Help & Contributing

- 🐛 Issues & feature requests: [LemonHub Issues](https://github.com/nsuanningmeng/LemonHub/issues)
- 📘 Sub-site guide: [中文](./docs/subsite-guide.md) · [English](./docs/subsite-guide.en.md)
- 📚 Gateway/API reference: [new-api docs](https://docs.newapi.pro)

Contributions of all kinds are welcome — bug reports, features, docs, and code.

<div align="center">

If LemonHub helps you, please consider giving it a ⭐️

<sub>🍋 LemonHub — a white-label / agent-franchise layer on top of <a href="https://github.com/QuantumNous/new-api">new-api</a>.</sub>

</div>
