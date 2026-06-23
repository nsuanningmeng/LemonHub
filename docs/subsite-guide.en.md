# 🍋 LemonHub Sub-site / Agent Franchise — Step-by-Step Guide

> This guide walks you **from zero** through turning LemonHub into a distribution system with "a platform operator (main site) + multiple agents (sub-sites)".
> There are two main tracks: the **main-site operator** (platform) and the **sub-site operator** (agent). Every step tells you **what to prepare** + **where to click**.
> Chinese version: [subsite-guide.md](./subsite-guide.md)

---

## 0. Understand 5 concepts first (2 minutes)

| Concept | One-line explanation |
|---|---|
| **Main site (site_id = 0)** | The platform operator's own site. It owns the deployment, the AI channels (upstream API Keys), and sells quota at a **wholesale price**. The very first registered account is the main-site super administrator. |
| **Sub-site (white-label tenant)** | An independently branded site: its own **domain**, name, Logo, notice, and homepage copy. It belongs to a specific **agent**. One deployment + one database can host many sub-sites. |
| **Agent (sub-site operator)** | The person who owns and self-manages a sub-site. They collect their own money and serve their own users on their own domain. |
| **Purchasing wallet** | The balance an agent **prepays** to the platform (accounted in integer "mills", 1 yuan = 1000 mills). Every time a user tops up, the platform deducts an amount at the wholesale price from this wallet. **When the wallet runs dry → the sub-site's online top-up entry automatically disappears (downgrade).** |
| **DiscountRate** | The discount at which the agent buys, in basis points (per ten-thousand): `10000` = full price (no margin), `7000` = 70% (30% off). **Wholesale cost = face value × discount rate ÷ 10000**, and the spread is the agent's profit. |

### How money flows (example: discount 7000 = 70% / 30% off)

```
End user pays ¥100 on the "sub-site"
        │ (goes into the agent's own EasyPay merchant, ¥100 belongs to the agent)
        ▼ EasyPay async callback to LemonHub (single DB transaction, idempotent)
   User receives ¥100 worth of quota   +   Agent's purchasing wallet is debited ¥70
                                          └─ Spread ¥30 = the agent's profit on this order
```

> Key point: **the user's money goes into the agent's merchant; the platform only deducts the cost of goods at the wholesale price from the agent's "purchasing wallet".** So the agent must top up the purchasing wallet first, otherwise a "downgrade" is triggered — users will not see online top-up on that sub-site.

---

## 1. What each side needs to prepare (pre-launch checklist)

### 🧑‍💼 Main-site operator preparation checklist
- [ ] A server + a **main domain** (e.g. `lemonhub.com`), with LemonHub deployed (see [README](../README.md)).
- [ ] A database (for production, MySQL ≥ 5.7.8 or PostgreSQL ≥ 9.6 recommended) + Redis (recommended for multi-node/caching).
- [ ] **AI channels / models / groups / ratios** already configured (this is the "goods" you sell).
- [ ] **Payment compliance confirmation** completed (the compliance terms in System Settings, a prerequisite toggle for paid features).
- [ ] The ability to set up **DNS resolution + reverse proxy** for sub-site domains (so a `sub-site domain` points to this deployment).
- [ ] (Optional) The main site's own EasyPay merchant, for the main site's self-operated top-up.

### 🧑‍🔧 Sub-site operator (agent) preparation checklist
- [ ] A **sub-site domain** (e.g. `agent-a.com` or `ai.agent-a.com`), willing to CNAME/resolve to the platform deployment.
- [ ] A set of **your own EasyPay (Rainbow EasyPay, etc.) merchant**: `Merchant ID`, `Merchant Key`, `Payment Gateway URL`, with support for `alipay`/`wxpay`, etc.
- [ ] In the EasyPay merchant dashboard, set the **callback/notify domain** to allow your sub-site domain (critical! otherwise "paid but shows unpaid").
- [ ] A Logo image URL (optional), site name, notice/footer copy.
- [ ] Prepay a **purchasing wallet** balance to the main-site operator.

---

## 2. Main-site operator actions (Step-by-Step)

> Do everything while logged in as the main-site super administrator on the **main domain**.

### Step 1 · Basic configuration (one-time)
1. Log in to the main-site admin → `Settings → System Settings`: fill in the **Server Address (ServerAddress)** as your main domain (e.g. `https://lemonhub.com`).
2. In `Settings`, complete the **payment compliance confirmation** (check the compliance terms). Until confirmed, no top-up/redemption or other paid entries will appear on any site.
3. `Channels / Models / Groups / Ratios`: configure the models you want to sell, the upstream channels, and the group ratios. These are the channels your sub-site users ultimately use.

### Step 2 · Create a "main-site account" for the agent
> A sub-site's "owning agent" must first be a **regular user account on the main site** (not an admin/super administrator).

1. `User Management → Create User`, create a regular user, e.g. username `agentA`, and set an initial password.
2. Note down this username — you'll use it as the "owning agent" when creating the sub-site in the next step.

> When you create the sub-site, the system automatically promotes `agentA` to "sub-site administrator" and **binds it to the new sub-site**. After that, `agentA` only logs in under that sub-site domain and can only manage its own sub-site.

### Step 3 · Create the sub-site
1. Go to the **"Sub-site Management"** page in the left menu (exclusive to main-site administrators).
2. Click **"Create Sub-site"** and fill in the drawer on the right:

| Field | What to fill | Notes |
|---|---|---|
| **Name** | The agent's brand name, e.g. `A Intelligence` | Shown in the sub-site's title/brand area |
| **Domains** | One per line, e.g. `agent-a.com` | **At least one**; a domain can only bind to one sub-site |
| **Owner Username** | `agentA` | Must be a **regular user on the main site** (not an admin) |
| **Logo URL** | Image address (optional) | Sub-site Logo |
| **Status** | `Normal` | `Disabled` = deactivate this sub-site |
| **Discount Rate** | e.g. `7000` (30% off) | `10000` = full price. Wholesale cost = face value × this ÷ 10000 |
| **Wallet Warn Threshold** | e.g. `50000` (= ¥50, unit **mills**) | Alerts when the balance falls below it; `0` = no alert |
| **Notice / Footer** | Optional | Notice supports Markdown |
| **Homepage Hero (Badge / Title 1 / Title 2)** | Optional | White-label copy for the sub-site's homepage headline; leave empty to use defaults |
| **Advanced · Pay Config** | Usually leave empty | The agent's payment collection is normally filled in by the agent in the sub-site console; this is a JSON advanced entry |

3. Click **"Save"**. After successful creation, `agentA` has been promoted to sub-site administrator and bound to this sub-site.

> 💡 **Unit reminder**: in the creation drawer, the "Wallet Warn Threshold" is in **mills** (1 yuan = 1000 mills, ¥50 = `50000`). The later "wallet recharge" dialog, however, takes input in **yuan** — don't mix them up.

### Step 4 · Top up the agent's purchasing wallet
> This is the capital the agent uses to "buy goods". No money = the sub-site's online top-up downgrades and disappears.

1. In the "Sub-site Management" list, find the sub-site → click **"Wallet"** in the row's actions.
2. In the **Wallet Management** drawer:
   - **Recharge**: fill in "Recharge amount (yuan)" + a note → submit. This **increases** the agent's purchasing wallet balance (representing the agent paying you to buy goods).
   - **Manual Adjustment**: fill in an amount (can be **negative** for deduction/correction) + a **required** note.
   - **Wallet Logs**: every change is recorded for reconciliation.
3. After recharging, the balance takes effect immediately.

### Step 5 · Point the sub-site domain to this deployment (DNS + reverse proxy)
1. Have the agent resolve the sub-site domain (`agent-a.com`) to your server (A record or CNAME).
2. In your reverse proxy (Nginx / Caddy / Cloudflare, etc.), forward that domain to the LemonHub container (default `:3000`), and configure the HTTPS certificate.
3. **Be sure to preserve and pass through the `Host` header** (LemonHub routes requests to the corresponding sub-site by `Host`). Nginx example: `proxy_set_header Host $host;`
4. Open `https://agent-a.com` — it should display that sub-site's brand (name/Logo). If it shows the main-site brand, the domain didn't match (check domain spelling, caching, and `Host` pass-through).

### Step 6 · Day-to-day operations
- **Reconciliation**: the "Sub-site Management" page has **"Reconcile"** to verify each sub-site's wallet account.
- **Alerts**: watch for balances below the threshold, and remind agents to top up in time.
- **Deactivation**: change a sub-site's status to `Disabled` to take it offline (without affecting other sites).
- **Change discount/brand**: edit the sub-site drawer anytime (the owning agent is fixed after creation and cannot be changed).

---

## 3. Sub-site operator (agent) actions (Step-by-Step)

> Do everything while logged in on **your own sub-site domain** (e.g. `https://agent-a.com/login`), using the `agentA` account the main-site operator gave you.

### Step 1 · Log in and enter the "Sub-site Management Console"
1. Open `https://agent-a.com` in your browser and log in as `agentA`.
2. Enter the **"Sub-site Management Console / Agent Console"**. It has 4 tabs:

| Tab | Purpose |
|---|---|
| **Overview & Wallet** | View purchasing wallet balance, warn threshold, and recent logs |
| **Redemption Codes** | Generate / void redemption codes for this site |
| **Branding Settings** | Change name / Logo / notice / footer / homepage copy |
| **Payment Settings** | Configure your own EasyPay merchant (**most critical**) |

### Step 2 · Configure branding (Branding Settings)
- Fill in the site name, Logo, notice, footer, and homepage Hero copy → save. This is the brand end users see.

### Step 3 · Configure payment collection (Payment Settings) — the most important part
> This step determines that the users' money goes into **your** pocket. You need to have an EasyPay merchant set up first.

In **Payment Settings**, fill in:

| Field | What to fill | Example |
|---|---|---|
| **Merchant ID** | Your EasyPay merchant's PID | `1001` |
| **Merchant Key** | Your EasyPay merchant's key | `(secret)` |
| **Payment Gateway URL** | Your EasyPay platform's gateway address | `https://pay.example.com` |
| **Payment Methods** | Comma-separated | `alipay,wxpay` |

When done, click **Save**.

> ⚠️ **The callback domain must be correct** (otherwise "paid but shows unpaid"):
> 1. In your **EasyPay merchant dashboard**, set the allowed "async notify / callback domain" to your sub-site domain (`agent-a.com`).
> 2. Confirm the sub-site domain is correctly resolved + reverse-proxied to LemonHub (Section 2, Step 5).
> For **sub-site** top-ups, LemonHub uses "the sub-site's own domain" for the callback and verifies the signature with "the sub-site's own key", so these two must match.

### Step 4 · Self-test a top-up
1. Register/log in with a regular user account on `https://agent-a.com`.
2. Go to the top-up page; you should see the online top-up entry (prerequisites: **the wallet has a balance** + **payment collection is configured**).
3. Run a small top-up end to end: after a successful payment, the user's quota is credited, your purchasing wallet is debited at the wholesale price, and you can see it in the logs.
4. If you don't see the online top-up entry → it's most likely **purchasing wallet balance ≤ 0** or **payment collection not fully configured** (downgrade).

### Step 5 · Day-to-day operations
- **Redemption codes**: generate them on the "Redemption Codes" page as needed and hand them to users.
- **Keep the wallet funded**: the balance is consumed as users top up — **top up with the main-site operator in advance** to avoid a downgrade cutting off top-ups.
- **Watch the logs**: the overview page lets you check wallet logs for reconciliation.

---

## 4. Billing / discount-rate calculation examples

Assume the unit price, group ratios, etc. are already configured on the main site, and an end user tops up once with "face value ¥M":

| Discount Rate | User pays (into agent's merchant) | Agent purchasing wallet debit (wholesale) | Agent gross margin | Quota credited to user |
|---|---|---|---|---|
| `10000` (full price) | ¥M | ¥M | ¥0 | Quota for face value ¥M |
| `8000` (20% off) | ¥M | ¥M × 0.8 | ¥M × 0.2 | Quota for face value ¥M |
| `7000` (30% off) | ¥M | ¥M × 0.7 | ¥M × 0.3 | Quota for face value ¥M |
| `5000` (50% off) | ¥M | ¥M × 0.5 | ¥M × 0.5 | Quota for face value ¥M |

> The **lower** the discount rate, the cheaper the agent buys and the higher the profit; the main site earns the wholesale spread on volume. The exact "face value → quota" is determined by the main site's unit price/group ratios.

---

## 5. FAQ / troubleshooting

**Q1: The user paid, EasyPay shows success, but the sub-site shows unpaid?**
- The sub-site's **payment callback domain** is mismatched: the callback domain allowed in the EasyPay merchant dashboard ≠ the sub-site domain; or the sub-site domain isn't resolved/reverse-proxied to LemonHub.
- The sub-site domain's reverse proxy **doesn't pass through `Host`**: LemonHub doesn't receive the correct Host → signature verification/routing fails.
- Occasionally: network jitter in the EasyPay callback. LemonHub's callback endpoint is already exempt from gzip/global rate limiting and guarantees returning plain `success`; if it still happens occasionally, the main site can fall back to "order completion" (an admin manually completing the order).

**Q2: The sub-site doesn't show the "Online Top-up" entry?**
- The sub-site's **purchasing wallet balance ≤ 0** (downgrade) → the main site recharges the wallet.
- **Payment collection not fully configured** (Merchant ID/Key/Gateway URL — any one missing) → the sub-site completes it in "Payment Settings".
- The main site **hasn't completed payment compliance confirmation** → the main site confirms it in System Settings.

**Q3: Visiting the sub-site domain shows the main-site brand?**
- The domain didn't match: check the domain spelling in "Sub-site Management", whether it was saved, whether DNS has propagated, the reverse proxy's `Host` pass-through, and browser/CDN caching.

**Q4: The agent (agentA) logs in via the main domain but can't get into their own console?**
- The agent account was **bound to the sub-site** when the sub-site was created — log in via the **sub-site domain** (`https://agent-a.com/login`).

**Q5: Can I change a sub-site's owning agent?**
- The owning agent is fixed at creation (it involves role and site_id binding) and cannot be changed in editing. Brand/discount/status/payment can be changed anytime.

**Q6: Why is the wallet balance unit sometimes "mills" and sometimes "yuan"?**
- The internal ledger uses **integer mills** (1 yuan = 1000 mills) to guarantee precision isn't lost. The "Wallet Warn Threshold" in the creation drawer is filled in **mills**; the wallet recharge dialog takes input in **yuan** and the UI displays in ¥.

---

## 6. Security / compliance notes

- **Data isolation**: each sub-site's data is isolated by `site_id`; usernames are unique per site; password/2FA/OAuth are isolated per site. Agents cannot see other sites' data.
- **Agent permission boundary**: a sub-site administrator can only manage their own site (brand/payment/wallet viewing/redemption codes), and cannot change the domain/discount/ownership (those belong to the main site).
- **Financial correctness**: the wallet uses integer mills, atomic conditional deduction, never goes negative, and writes a log on every change; top-up settlement is idempotent (duplicate/concurrent callbacks won't issue twice).
- **Compliance**: when providing generative-AI services to the public, the main-site operator must complete obligations such as filing/registration, content safety, real-name verification, log retention, taxes, payments, and upstream authorization; the agent bears their own payment-collection compliance.

---

## 7. Quick reference (one-page cheat sheet)

**Main-site operator:** deploy → configure channels/models/groups → confirm compliance → create the agent's main-site account → create the sub-site (domain + ownership + discount + brand) → top up the wallet → DNS + reverse proxy for the sub-site domain (pass through Host) → reconcile/renew/operate.

**Sub-site operator:** get the account → log in on the sub-site domain → enter the agent console → configure branding → configure payment collection (EasyPay merchant + callback domain) → self-test a top-up → issue redemption codes/operate → renew with the main site before the balance runs out.

---

> This doc is updated as features evolve. If something doesn't match the actual UI, please report it in [LemonHub Issues](https://github.com/nsuanningmeng/LemonHub/issues).
