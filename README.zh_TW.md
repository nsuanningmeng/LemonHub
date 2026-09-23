<div align="center">

![LemonHub](/web/public/logo.png)

# LemonHub

**多租戶 AI API 閘道，內建代理（分銷）加盟體系、增強版邀請返佣系統與工單套件 —— 基於 new-api 二次開發。**

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
  <a href="#專案簡介">專案簡介</a> •
  <a href="#features">主要功能</a> •
  <a href="#快速開始">快速開始</a> •
  <a href="#代理與分銷加盟">代理加盟</a> •
  <a href="#部署">部署</a>
</p>

</div>

## 專案簡介

LemonHub 是 [new-api](https://github.com/QuantumNous/new-api)（其本身基於 [One API](https://github.com/songquanpeng/one-api)）的二次開發分支。它保留 new-api 完整的閘道能力 —— 統一代理 40+ AI 服務商（OpenAI、Claude、Gemini、Azure、AWS Bedrock 等），含計費、限流與管理後台 —— 並在其之上新增：

- **多租戶、代理加盟層**：一套部署即可承載多個獨立子站，每個子站由一名代理（分銷商）營運，擁有自己的網域、收款商戶與預付進貨錢包；
- **增強版邀請返佣系統**：首充到帳、按比例持續返佣、按使用者差異化比例、管理員全站返佣榜，以及現金結算推廣者模式；
- **工單與郵件推廣套件**；
- **用戶端一鍵設定**（Connect Hub）；
- **計費、支付、中繼、安全與遷移**方面的管理能力。

中繼 / 通道轉發 / 計費核心與上游保持相容，便於乾淨地合併 new-api 的新特性與修復；LemonHub 的新增能力都圍繞其外圍實現。

> [!IMPORTANT]
> - 本專案僅用於合法、獲授權的 AI API 閘道、組織級鑑權、多模型管理、用量分析、成本核算與私有 / 分銷部署等場景。
> - 你必須合法取得上游 API 金鑰、帳號、模型服務與介面權限，並遵守上游服務條款及適用法律法規。
> - 向公眾提供生成式 AI 服務時，請先完成所在司法管轄區要求的備案、許可、內容安全、實名、日誌留存、稅務、支付與上游授權等義務。

<a id="features"></a>

## 主要功能

除上游 new-api 閘道能力外，LemonHub 還提供以下功能。各版本的具體變更與升級說明請見 [GitHub Releases](https://github.com/nsuanningmeng/LemonHub/releases)。

### 1. 多租戶子站

- 一套部署、一個資料庫承載多個獨立子站；每個請求按 `Host`（網域中介軟體）路由到對應子站。
- 按站客製化：站點名稱、Logo、公告、頁尾、首頁主視覺文案均可逐站設定。
- 按站資料隔離：本站使用者、令牌、日誌等紀錄由 `site_id` 限定，共用的上游渠道及系統設定仍由主站管理。使用者名稱**按站唯一**（`(site_id, username)`）；密碼、2FA、OAuth 綁定及註冊、登入均限定在目前站點。
- 主站提供子站建立與管理頁面。

### 2. 代理 / 分銷加盟

- 兩類角色：**主站站長**（平台方 —— 擁有部署、上游通道，批發額度）與**子站站長**（代理 —— 透過專屬代理後台營運子站，後端由 `SiteAdminAuth` 端點支撐）。
- **進貨錢包**：代理向平台預付，錢包以整數釐（毫元）記帳。每筆使用者儲值在同一交易內按批發價原子扣減錢包；錢包永不為負，每次變動寫入帳本。主站可儲值、調整、對帳、作廢與退款。
- **按代理批發（折扣）率**：`DiscountRate` 為萬分比整數（`10000` = 原價，`7000` = 七折）。批發成本 = `原價 × DiscountRate / 10000`，差價歸代理。
- **按站收款**：每個子站設定自己的易支付商戶。終端使用者儲值進入代理自己的商戶；平台在同一資料庫交易內結算錢包（給使用者加額度**並**扣代理錢包），冪等，並帶**自動降級** —— 當代理錢包耗盡或未設定時，該子站的線上儲值入口自動隱藏，已發放額度與其他通道不受影響。
- **按站兌換碼**：代理生成 / 作廢僅限本站的兌換碼，跨站隔離並對帳。
- **按站自訂模型加價**（route A）：代理可對本站特定模型呼叫加價，平台的批發結算永不被壓價。
- 代理加盟登陸頁、可設定的聯絡我們頁，以及導覽 / 頁尾入口。

### 3. 邀請返佣系統

LemonHub 的邀請返佣系統包括：

- 邀請獎勵在**被邀請人首次儲值成功後**到帳（而非註冊時），並對被邀請人的每次儲值**按比例持續返佣**；兩者均走冪等的逐筆帳冊並提供統計介面。
- Stripe 退款及爭議付款會追回相應的額度與邀請獎勵。
- 面向使用者的返佣詳情頁與個人貢獻排行榜。
- **按使用者差異化返佣比例**，可覆寫全域比例（不設則繼承全域；設為 `0` 則對該邀請人停發）。
- **管理員全站返佣榜**（彙總卡 + 排行榜，支援搜尋、排序、分頁，受 `AdminAuth` 保護）。
- **現金結算推廣者模式**：針對線下以現金結算的推廣者的按使用者開關。開啟後不再發放平台邀請獎勵額度，儲值返佣以帳冊記為應付現金（不計入平台餘額）；現金結算帳冊追蹤未結餘額，結算防超付、並行安全。資金類操作（標記推廣者、設定返佣比例、記錄 / 檢視現金結算）僅限 root / 站長帳號。

### 4. 工單與郵件推廣

- 使用者工單台與管理員工單管理，支援優先級。
- 郵件推廣 / 群發工具支援附件上傳、群發、限流、清理與稽核。
- 獨立的推廣郵件 SMTP、取消訂閱與抑制寄送控制，以及中斷群發任務的恢復機制。

### 5. Connect Hub（用戶端一鍵設定）

- 為 Claude Code、Codex、Gemini CLI、Chatbox、Cherry Studio、VS Code 等提供一鍵設定。產生的設定指向**使用者目前實際存取的網域**，因此在多網域下都能正確運作。

### 6. 計費與令牌路由

- **令牌多分組優先級故障轉移**：令牌可攜帶有序分組列表並在分組間轉移；**tiered / 表達式計費按實際使用的分組**計算。
- 預設前端的倍率編輯器會顯示**目前未定價**的模型，避免被悄悄遺漏。

### 7. 支付與中繼可靠性

- 易支付回呼豁免 gzip 處理與全域限流，並配專用的寬鬆兜底限流；`notify_url` 固定回穩定網域。
- 支付回呼 / 返回位址與首屏頁面 `<title>` 跟隨使用者存取的（受信任）網域，並防 Host 偽造。
- 易支付結算將已簽章回呼視為查單觸發條件，由伺服器向訂單所屬商戶的閘道獨立查驗；確認商戶、訂單身分及與本機紀錄完全一致的金額後，才發放額度或訂閱。
- 訂閱下單時保存履約條件快照；易支付限購方案訂單以原子操作預留名額，並安全複用待付款訂單，避免方案編輯競態及重複建立待付款訂單。
- 中繼重試遵循設定的 504/524 狀態碼；非串流 `BadResponseBody` 回應可重試；通道耗盡時回傳真實的上游錯誤。
- 訂閱到期後，使用者會回到原分組（續訂時保留 `prev_user_group`）。

### 8. 模型效能設定

- 成功率閾值、錯誤碼白名單、無資料按 100% 處理。
- 已啟用渠道的手動及定時測試，會為設定的每個模型、每個分組記錄健康樣本；已不可用的分組不再計入顯示的模型成功率。
- 中繼與渠道測試錯誤，只有在 HTTP 狀態碼命中效能設定的錯誤碼白名單時才計為失敗；其他錯誤計為成功樣本。白名單留空時，所有錯誤都計為失敗。
- 效能頁面依目前已啟用、使用者可用且已定價的模型 / 分組顯示；已移除的分組不再影響整體及趨勢指標。

### 9. 安全與遷移保護

- 進階自訂渠道具備 SSRF 防護。
- 私有工單與媒體存取控制、敏感日誌遮蔽，以及 API 金鑰明確選擇分組。
- 三種資料庫（SQLite / MySQL / PostgreSQL）上的遷移安全：`price_amount` 精度的失敗即停預檢、訂閱價格精度預檢、快速路徑失敗即停預檢，以及 `site_id` NULL 兜底回填，確保主站查詢不會漏掉歷史資料。

### 10. 打包與文件

- Docker 映像發布到 GitHub Container Registry（`ghcr.io/nsuanningmeng/lemonhub`），多架構（amd64 + arm64）；`docker-compose` 預設指向分支映像。
- GitHub Releases 提供 Linux、macOS、Windows 伺服器程式、Windows Electron 應用程式及校驗檔案。
- 提供英文、簡體中文、繁體中文、法文及日文 README，以及分步的子站 / 代理教學。

> 第一次接觸代理模式？請看分步指南：**[子站 / 代理加盟保姆級指南（中文）](./docs/subsite-guide.md)** · [English](./docs/subsite-guide.en.md)

## 快速開始

### Docker Compose（推薦）

```bash
git clone https://github.com/nsuanningmeng/LemonHub.git
cd LemonHub

# 隨附的 Compose 檔案預設啟動 PostgreSQL 與 Redis；對外開放服務前，
# 請修改這兩項的密碼及對應的連線字串。
nano docker-compose.yml

# 啟動（Compose 檔案已指向 LemonHub 映像）
docker compose up -d
```

<details>
<summary>使用原生 Docker 指令</summary>

```bash
docker pull ghcr.io/nsuanningmeng/lemonhub:latest

# SQLite（預設 —— 須掛載 /data 才能持續保存資料）
docker run --name lemonhub -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  ghcr.io/nsuanningmeng/lemonhub:latest

# 遠端 MySQL（請替換主機名稱與憑證；容器必須能連線至該主機）
docker run --name lemonhub -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="user:password@tcp(db.example.internal:3306)/lemonhub" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  ghcr.io/nsuanningmeng/lemonhub:latest

# PostgreSQL 可使用 SQL_DSN="postgresql://user:password@db.example.internal:5432/lemonhub"。
```

</details>

部署完成後開啟 `http://localhost:3000`，完成首次初始化精靈，建立 **root / 平台（主站）管理員**。

> [!WARNING]
> 以公開或分銷形式對外提供 AI 服務前，請先完成備案、許可、內容安全、實名、日誌留存、稅務、支付與上游授權等所有必要義務。

## 代理與分銷加盟

LemonHub 圍繞兩類角色構建：

- **主站站長（平台方）** —— 擁有部署與 AI 通道（上游金鑰），批發額度。建立子站、為代理錢包儲值、設定每個代理的折扣率。
- **子站站長（代理 / 分銷商）** —— 在自己的網域上營運子站，設定自己的收款商戶與外觀，服務自己的終端使用者。

資金流一覽（範例，折扣 `7000` = 七折）：

```
終端使用者支付 ¥100  ──►  代理自己的易支付商戶（代理收取 ¥100）
        │
        ▼ （回呼；同一筆資料庫交易，具冪等性）
   使用者獲得 ¥100 額度   +   代理進貨錢包扣除 ¥70
                                     └─ ¥30 為代理利潤
```

完整流程 —— 每個角色需準備什麼、每一步怎麼操作 —— 見：
**[子站 / 代理加盟保姆級指南（中文）](./docs/subsite-guide.md)** · **[English](./docs/subsite-guide.en.md)**

## 繼承自 new-api

LemonHub 保留 new-api 的閘道能力，包括：

- **格式**：OpenAI Chat/Responses/Realtime、Claude Messages、Google Gemini、Rerank（Cohere/Jina）、圖像/音訊/Embedding、Midjourney-Proxy、Suno、Dify。
- **格式轉換**：OpenAI ⇄ Claude Messages、OpenAI → Gemini、thinking 轉 content、reasoning-effort 後綴。
- **智慧路由**：加權隨機通道、失敗自動重試（可設定重試狀態碼）、使用者級限流。
- **計費**：按次 / 按量 / 快取命中計費，tiered 與表達式定價；完成商戶設定後，可選用易支付、Stripe、Creem、Waffo 及 Waffo Pancake 支付整合。
- **鑑權**：JWT、WebAuthn/Passkeys、OAuth（GitHub、Discord、OIDC、LinuxDO、Telegram、微信）。
- **介面**：現代化後台、七種前端語言（en、zh、zh-TW、fr、ja、ru、vi）、資料看板、模型效能指標。

閘道 / API 細節請參考上游 [new-api 文件](https://docs.newapi.pro)。

## 部署

> [!TIP]
> 最新映像：`ghcr.io/nsuanningmeng/lemonhub:latest`（多架構：amd64 + arm64）。

### 環境要求

| 元件 | 要求 |
|---|---|
| 本地資料庫 | SQLite（Docker 需掛載 `/data`） |
| 遠端資料庫 | MySQL ≥ 5.7.8 或 PostgreSQL ≥ 9.6 |
| 快取（推薦） | Redis |
| 執行引擎 | Docker / Docker Compose |
| 從原始碼建置 | Go 1.26.8 與 Bun |

### 常用環境變數

| 變數 | 說明 | 預設值 |
|---|---|---|
| `SESSION_SECRET` | 工作階段金鑰（多節點必填） | - |
| `CRYPTO_SECRET` | 加密金鑰（共享 Redis 的節點需一致） | 預設取 `SESSION_SECRET` |
| `SESSION_COOKIE_SECURE` | 為 HTTPS 部署啟用安全刷新 Cookie 和 refresh/logout 來源驗證 | `false` |
| `SESSION_COOKIE_TRUSTED_URL` | 啟用 `SESSION_COOKIE_SECURE=true` 時必填的精確 HTTPS 來源；不是中繼 CORS 白名單 | - |
| `TRUSTED_PROXIES` | 受信任反向代理的 IP/CIDR；`none` 表示忽略轉送標頭 | 私有及迴環網段（啟動時會警告） |
| `SQL_DSN` | 資料庫連線字串（MySQL/PostgreSQL） | - |
| `REDIS_CONN_STRING` | Redis 連線字串 | - |
| `TRUSTED_REDIRECT_DOMAINS` | 支付跳轉 / 多網域回呼的受信任網域（逗號分隔） | - |
| `PAYMENT_WEBHOOK_RATE_LIMIT` | 支付回呼 webhook 的寬鬆按 IP 兜底（次 / 視窗） | `1800` |
| `PAYMENT_WEBHOOK_RATE_LIMIT_DURATION` | 上一項的視窗（秒） | `60` |
| `STREAMING_TIMEOUT` | 串流無回應逾時（秒） | `300` |
| `RELAY_RESPONSE_HEADER_TIMEOUT` | 等待上游回應標頭的逾時（秒）；設為 `0` 則不限制。標頭送達後的串流傳輸不受影響，非串流生成須預留足夠時間 | `1800` |
| `MAX_REQUEST_BODY_MB` | 最大請求內容（MB，解壓後） | `128` |

限流及大多數調優變數都有合理的程式碼預設值，因此不必寫進 `.env`/compose。可選項的說明見 `.env.example`。
正式環境的工作階段與反向代理設定請見[身分驗證與工作階段安全](./docs/authentication.md)。

### 多節點

> [!WARNING]
> - 必須設定 `SESSION_SECRET`，否則各節點登入狀態不一致。
> - 共享 Redis 的節點必須使用相同的 `CRYPTO_SECRET`（預設取 `SESSION_SECRET`），否則快取/加密資料無法互通。

### 重試與快取

- 重試：`設定 → 營運設定 → 路由可靠性`（失敗重試次數 + 自動重試狀態碼區間）。
- 快取：`REDIS_CONN_STRING`（推薦）或 `MEMORY_CACHE_ENABLED`。

## 基於 new-api

LemonHub 是採用 AGPL 授權的分支。感謝上游專案：

| 專案 | 角色 |
|---|---|
| [new-api](https://github.com/QuantumNous/new-api) | 直接上游 —— LemonHub 擴展的閘道 |
| [One API](https://github.com/songquanpeng/one-api) | 最初的專案基礎（MIT） |

LemonHub 會定期與上游 new-api 同步。

## 授權條款

本專案採用 [GNU Affero 通用公共授權條款 v3.0（AGPLv3）](./LICENSE)，繼承上游授權條款。

根據 AGPLv3 第 7 條附加條款，修改版必須在適當的法律 / 關於 / 頁尾位置保留作者署名 `Frontend design and development by New API contributors.`，並保留指向原始專案的可見連結：<https://github.com/QuantumNous/new-api>。

本專案基於 [One API](https://github.com/songquanpeng/one-api)（MIT 授權條款）開發。

## 幫助與貢獻

- 問題與需求：[LemonHub Issues](https://github.com/nsuanningmeng/LemonHub/issues)
- 版本與下載：[GitHub Releases](https://github.com/nsuanningmeng/LemonHub/releases)
- 子站指南：[中文](./docs/subsite-guide.md) · [English](./docs/subsite-guide.en.md)
- 閘道 / API 參考：[new-api 文件](https://docs.newapi.pro)

歡迎各類貢獻 —— 缺陷回報、功能、文件與程式碼。

<div align="center">
<sub>LemonHub —— 在 <a href="https://github.com/QuantumNous/new-api">new-api</a> 之上的代理加盟層。</sub>
</div>
