<div align="center">

![LemonHub](/web/default/public/logo.png)

# 🍋 LemonHub

**多租戶白標 · 內建代理加盟體系的 AI API 閘道**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <strong>繁體中文</strong> |
  <a href="./README.md">English</a> |
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
  <a href="#-快速開始">快速開始</a> •
  <a href="#-lemonhub-的不同之處">為什麼選 LemonHub</a> •
  <a href="#-白標--代理加盟">白標/加盟</a> •
  <a href="#-部署">部署</a> •
  <a href="#-基於-new-api-構建">基於 new-api</a>
</p>

</div>

## 📝 專案簡介

**LemonHub** 是 [new-api](https://github.com/QuantumNous/new-api)（其本身基於 [One API](https://github.com/songquanpeng/one-api)）的二次開發版本。它完整保留了 new-api 的能力——在 40+ 家 AI 服務商（OpenAI、Claude、Gemini、Azure、AWS Bedrock 等）之上提供統一閘道，含計費、限流與管理後台——並在此之上疊加了一層 **多租戶、白標、代理加盟** 能力：

> 一套部署 + 一個資料庫即可承載**多個獨立品牌的子站**。每個子站歸屬一個**代理（站長）**，擁有自己的網域、用自己的**收款商戶**收款，並透過**預付的採購錢包**按批發價向平台採購額度。

> [!IMPORTANT]
> - 本專案僅用於合法、授權的 AI API 閘道、組織級鑑權、多模型管理、用量分析、成本核算與私有/分銷部署場景。
> - 你必須合法取得上游 API 金鑰、帳號、模型服務與介面權限，並遵守上游服務條款與適用法律法規。
> - 面向公眾提供生成式 AI 服務時，請先完成所在司法管轄區要求的備案、許可、內容安全、實名核驗、日誌留存、稅務、支付與上游授權等全部義務。

---

## ✨ LemonHub 的不同之處

在 new-api 的全部能力之上，LemonHub 增加了：

| 能力 | 說明 |
|---|---|
| 🏢 **白標子站** | 一套部署承載多個品牌租戶。每個子站擁有獨立網域、名稱、Logo、公告、頁尾與首頁 Hero 文案；請求按 `Host` 路由到對應子站。 |
| 🤝 **代理 / 加盟體系** | 平台方（主站）招募**代理**；每個代理透過專屬的**子站管理控制台**自助管理自己的子站。 |
| 💰 **採購錢包（整數釐帳本）** | 每個代理向平台預付，餘額以整數**釐**（0.001 元）記帳。使用者每次儲值都會原子地按批發價扣減錢包——絕不為負、每次變動都寫流水。 |
| 🏷️ **按代理的折扣（批發）率** | `DiscountRate` 為萬分比整數（`10000`=原價，`7000`=七折）。批發成本 = `面值 × DiscountRate / 10000`，差價歸代理所得。 |
| 💳 **按站收款** | 每個子站設定**自己的**易支付商戶。使用者儲值進入代理自己的商戶帳戶；平台在同一資料庫交易中結算錢包（加使用者額度 **+** 扣代理錢包），且冪等。 |
| 🔻 **自動降級** | 當代理錢包耗盡（或未設定）時，該子站的線上儲值入口會自動消失——不影響已發放額度與其他通道。 |
| 🔒 **按站資料隔離** | 各表均帶 `site_id`；使用者名稱在**每個站內**唯一（`(site_id, username)`）；密碼、2FA 與全部 OAuth 綁定均按站隔離。 |
| 🎟️ **按站兌換碼** | 代理可生成/作廢僅限本站的兌換碼，跨站隔離並支援對帳。 |
| 🔌 **Connect Hub 一鍵設定中心** | 在 API Keys 頁一鍵把外部用戶端（Claude Code / Codex / Gemini CLI / Chatbox / Cherry Studio / VS Code 等）接入本閘道，base URL **以使用者目前存取網域為準**——多網域感知。 |
| 🌐 **多網域支付回呼與標題** | 支付 notify/return 位址與首屏 `<title>` 跟隨使用者存取的（受信任）網域，並防 Host 偽造。 |

> 📖 **不熟悉代理模式？請看保姆級教學：** **[子站 / 代理加盟保姆級教學](./docs/subsite-guide.md)** · [English](./docs/subsite-guide.en.md)

---

## 🚀 快速開始

### 使用 Docker Compose（推薦）

```bash
# 克隆项目
git clone https://github.com/nsuanningmeng/LemonHub.git
cd LemonHub

# 检查/修改配置（数据库密码、ServerAddress 等）
nano docker-compose.yml

# 启动（compose 文件已指向 LemonHub 镜像）
docker compose up -d
```

<details>
<summary><strong>使用 docker 命令</strong></summary>

```bash
# 拉取最新镜像
docker pull ghcr.io/nsuanningmeng/lemonhub:latest

# SQLite（默认，需挂载 /data 持久化）
docker run --name lemonhub -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  ghcr.io/nsuanningmeng/lemonhub:latest

# MySQL / PostgreSQL（设置 SQL_DSN）
docker run --name lemonhub -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:123456@tcp(localhost:3306)/lemonhub" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  ghcr.io/nsuanningmeng/lemonhub:latest
```

</details>

🎉 部署完成後存取 `http://localhost:3000`。**第一個註冊的帳號即為 root / 平台（主站）管理員。**

> [!WARNING]
> 作為公開或分銷 AI 服務營運 LemonHub 時，請先完成備案、許可、內容安全、實名核驗、日誌留存、稅務、支付與上游授權等全部合規義務。

---

## 🤝 白標 / 代理加盟

LemonHub 圍繞兩個角色設計：

- **主站站長（平台方）**——擁有部署、AI 通道（上游金鑰），按批發價售賣額度。負責建立子站、給代理錢包儲值、設定每個代理的折扣率。
- **子站站長（代理）**——在自己的網域上營運品牌子站，設定自己的收款商戶與品牌，服務自己的終端使用者。

**一圖看懂資金流**（範例，折扣 `7000` = 七折）：

```
终端用户支付 ¥100  ──►  代理自己的易支付商户   （这 ¥100 归代理）
        │
        ▼ （回调，单数据库事务，幂等）
   用户到账 ¥100 额度    +    代理采购钱包扣 ¥70
                                  └─ 差价 ¥30 即代理利润
```

每個角色需要準備什麼、每一步點哪裡，完整的保姆級走查見：

➡️ **[📘 子站 / 代理加盟保姆級教學](./docs/subsite-guide.md)** · **[English](./docs/subsite-guide.en.md)**

---

## 🤖 模型與功能支援（繼承自 new-api）

LemonHub 繼承 new-api 的閘道能力，包括：

- **格式**：OpenAI Chat/Responses/Realtime、Claude Messages、Google Gemini、Rerank（Cohere/Jina）、圖像/音訊/Embedding、Midjourney-Proxy、Suno、Dify。
- **格式轉換**：OpenAI ⇄ Claude Messages、OpenAI → Gemini、思維鏈轉正文、推理力度後綴。
- **智慧路由**：通道加權隨機、失敗自動重試（可設定重試狀態碼）、令牌多分組優先級故障轉移、使用者級限流。
- **計費**：按次/按量/快取命中核算、分層/表達式定價、易支付與 Stripe 儲值。
- **鑑權**：JWT、WebAuthn/Passkeys、OAuth（GitHub、Discord、OIDC、LinuxDO、Telegram、微信）。
- **介面**：現代化後台、多語言（zh/en/fr/ja/vi…）、資料看板、模型效能指標。

> 📚 閘道/API 細節請參考上游 [new-api 文件](https://docs.newapi.pro)。

---

## 🚢 部署

> [!TIP]
> **最新映像：** `ghcr.io/nsuanningmeng/lemonhub:latest`（多架構：amd64 + arm64）。

### 環境要求

| 元件 | 要求 |
|---|---|
| **本地資料庫** | SQLite（Docker 須掛載 `/data`） |
| **遠端資料庫** | MySQL ≥ 5.7.8 或 PostgreSQL ≥ 9.6 |
| **快取（推薦）** | Redis |
| **執行引擎** | Docker / Docker Compose |

### 常用環境變數

| 變數 | 說明 | 預設值 |
|---|---|---|
| `SESSION_SECRET` | 工作階段金鑰（**多機部署必填**） | - |
| `CRYPTO_SECRET` | 加密金鑰（**共享 Redis 必填**） | - |
| `SQL_DSN` | 資料庫連線字串（MySQL/PostgreSQL） | - |
| `REDIS_CONN_STRING` | Redis 連線字串 | - |
| `TRUSTED_REDIRECT_DOMAINS` | 受信任網域（逗號分隔），用於支付跳轉/多網域回呼 | - |
| `PAYMENT_WEBHOOK_RATE_LIMIT` | 支付回呼 webhook 的寬鬆按 IP 兜底限流（次/視窗） | `1800` |
| `PAYMENT_WEBHOOK_RATE_LIMIT_DURATION` | 上述限流的視窗（秒） | `60` |
| `STREAMING_TIMEOUT` | 串流無回應逾時（秒） | `300` |
| `MAX_REQUEST_BODY_MB` | 最大請求體（MB，解壓後計算） | `32` |

> 限流及大多數調優變數都有合理的程式碼預設值，因此**無需**寫進 `.env`/compose。可選項見 `.env.example`。

### 多機部署注意

> [!WARNING]
> - **必須設定** `SESSION_SECRET`——否則各節點登入狀態不一致。
> - **共享 Redis 時必須設定** `CRYPTO_SECRET`——否則加密資料無法解密。

### 重試與快取

- **重試**：`設定 → 營運設定 → 路由可靠性`（失敗重試次數 + 自動重試狀態碼範圍）。
- **快取**：`REDIS_CONN_STRING`（推薦）或 `MEMORY_CACHE_ENABLED`。

---

## 🧱 基於 new-api 構建

LemonHub 是一個 AGPL 授權的 fork，特別感謝上游專案：

| 專案 | 角色 |
|---|---|
| [new-api](https://github.com/QuantumNous/new-api) | 直接上游——LemonHub 擴展的閘道 |
| [One API](https://github.com/songquanpeng/one-api) | 最初的專案基座（MIT） |

LemonHub 定期與上游 new-api 同步。relay / 計費 / 通道轉發核心保持與上游位元組級相容，以便乾淨地合併上游特性與修復；LemonHub 的增量（子站隔離、錢包、按站收款、品牌）都圍繞其外圍實現。

---

## 📜 開源授權

本專案繼承上游授權，採用 [GNU Affero 通用公共授權條款 v3.0（AGPLv3）](./LICENSE)。

依據 AGPLv3 第 7 條附加條款，修改版本必須在適當的法律/關於/頁尾位置保留作者署名 `Frontend design and development by New API contributors.`，並保留指向原始專案的可見連結：<https://github.com/QuantumNous/new-api>。

本專案基於 [One API](https://github.com/songquanpeng/one-api)（MIT 授權）開發。

---

## 💬 幫助與貢獻

- 🐛 Issue 與需求：[LemonHub Issues](https://github.com/nsuanningmeng/LemonHub/issues)
- 📘 子站教學：[中文](./docs/subsite-guide.md) · [English](./docs/subsite-guide.en.md)
- 📚 閘道/API 參考：[new-api 文件](https://docs.newapi.pro)

歡迎各種形式的貢獻——Bug 回饋、功能建議、文件與程式碼。

<div align="center">

如果 LemonHub 對你有幫助，歡迎點一個 ⭐️

<sub>🍋 LemonHub —— 在 <a href="https://github.com/QuantumNous/new-api">new-api</a> 之上的白標 / 代理加盟層。</sub>

</div>
