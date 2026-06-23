<div align="center">

![LemonHub](/web/default/public/logo.png)

# 🍋 LemonHub

**多租户白标 · 内置代理加盟体系的 AI API 网关**

<p align="center">
  <strong>简体中文</strong> |
  <a href="./README.zh_TW.md">繁體中文</a> |
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
  <a href="#-快速开始">快速开始</a> •
  <a href="#-lemonhub-的不同之处">为什么选 LemonHub</a> •
  <a href="#-白标--代理加盟">白标/加盟</a> •
  <a href="#-部署">部署</a> •
  <a href="#-基于-new-api-构建">基于 new-api</a>
</p>

</div>

## 📝 项目简介

**LemonHub** 是 [new-api](https://github.com/QuantumNous/new-api)（其本身基于 [One API](https://github.com/songquanpeng/one-api)）的二次开发版本。它完整保留了 new-api 的能力——在 40+ 家 AI 服务商（OpenAI、Claude、Gemini、Azure、AWS Bedrock 等）之上提供统一网关，含计费、限流与管理后台——并在此之上叠加了一层 **多租户、白标、代理加盟** 能力：

> 一套部署 + 一个数据库即可承载**多个独立品牌的子站**。每个子站归属一个**代理（站长）**，拥有自己的域名、用自己的**收款商户**收款，并通过**预付的采购钱包**按批发价向平台采购额度。

> [!IMPORTANT]
> - 本项目仅用于合法、授权的 AI API 网关、组织级鉴权、多模型管理、用量分析、成本核算与私有/分销部署场景。
> - 你必须合法获取上游 API 密钥、账号、模型服务与接口权限，并遵守上游服务条款与适用法律法规。
> - 面向公众提供生成式 AI 服务时，请先完成所在司法辖区要求的备案、许可、内容安全、实名核验、日志留存、税务、支付与上游授权等全部义务。

---

## ✨ LemonHub 的不同之处

在 new-api 的全部能力之上，LemonHub 增加了：

| 能力 | 说明 |
|---|---|
| 🏢 **白标子站** | 一套部署承载多个品牌租户。每个子站拥有独立域名、名称、Logo、公告、页脚与首页 Hero 文案；请求按 `Host` 路由到对应子站。 |
| 🤝 **代理 / 加盟体系** | 平台方（主站）招募**代理**；每个代理通过专属的**子站管理控制台**自助管理自己的子站。 |
| 💰 **采购钱包（整数厘账本）** | 每个代理向平台预付，余额以整数**厘**（0.001 元）记账。用户每次充值都会原子地按批发价扣减钱包——绝不为负、每次变动都写流水。 |
| 🏷️ **按代理的折扣（批发）率** | `DiscountRate` 为万分比整数（`10000`=原价，`7000`=七折）。批发成本 = `面值 × DiscountRate / 10000`，差价归代理所得。 |
| 💳 **按站收款** | 每个子站配置**自己的**易支付商户。用户充值进入代理自己的商户账户；平台在同一数据库事务中结算钱包（加用户额度 **+** 扣代理钱包），且幂等。 |
| 🔻 **自动降级** | 当代理钱包耗尽（或未配置）时，该子站的在线充值入口会自动消失——不影响已发放额度与其他通道。 |
| 🔒 **按站数据隔离** | 各表均带 `site_id`；用户名在**每个站内**唯一（`(site_id, username)`）；密码、2FA 与全部 OAuth 绑定均按站隔离。 |
| 🎟️ **按站兑换码** | 代理可生成/作废仅限本站的兑换码，跨站隔离并支持对账。 |
| 🔌 **Connect Hub 一键配置中心** | 在 API Keys 页一键把外部客户端（Claude Code / Codex / Gemini CLI / Chatbox / Cherry Studio / VS Code 等）接入本网关，base URL **以用户当前访问域名为准**——多域名感知。 |
| 🌐 **多域名支付回调与标题** | 支付 notify/return 地址与首屏 `<title>` 跟随用户访问的（受信任）域名，并防 Host 伪造。 |

> 📖 **不熟悉代理模式？请看保姆级教程：** **[子站 / 代理加盟保姆级教程](./docs/subsite-guide.md)** · [English](./docs/subsite-guide.en.md)

---

## 🚀 快速开始

### 使用 Docker Compose（推荐）

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

🎉 部署完成后访问 `http://localhost:3000`。**第一个注册的账号即为 root / 平台（主站）管理员。**

> [!WARNING]
> 作为公开或分销 AI 服务运营 LemonHub 时，请先完成备案、许可、内容安全、实名核验、日志留存、税务、支付与上游授权等全部合规义务。

---

## 🤝 白标 / 代理加盟

LemonHub 围绕两个角色设计：

- **主站站长（平台方）**——拥有部署、AI 通道（上游密钥），按批发价售卖额度。负责创建子站、给代理钱包充值、设置每个代理的折扣率。
- **子站站长（代理）**——在自己的域名上运营品牌子站，配置自己的收款商户与品牌，服务自己的终端用户。

**一图看懂资金流**（示例，折扣 `7000` = 七折）：

```
终端用户支付 ¥100  ──►  代理自己的易支付商户   （这 ¥100 归代理）
        │
        ▼ （回调，单数据库事务，幂等）
   用户到账 ¥100 额度    +    代理采购钱包扣 ¥70
                                  └─ 差价 ¥30 即代理利润
```

每个角色需要准备什么、每一步点哪里，完整的保姆级走查见：

➡️ **[📘 子站 / 代理加盟保姆级教程](./docs/subsite-guide.md)** · **[English](./docs/subsite-guide.en.md)**

---

## 🤖 模型与功能支持（继承自 new-api）

LemonHub 继承 new-api 的网关能力，包括：

- **格式**：OpenAI Chat/Responses/Realtime、Claude Messages、Google Gemini、Rerank（Cohere/Jina）、图像/音频/Embedding、Midjourney-Proxy、Suno、Dify。
- **格式转换**：OpenAI ⇄ Claude Messages、OpenAI → Gemini、思维链转正文、推理力度后缀。
- **智能路由**：渠道加权随机、失败自动重试（可配置重试状态码）、令牌多分组优先级失败转移、用户级限流。
- **计费**：按次/按量/缓存命中核算、分层/表达式定价、易支付与 Stripe 充值。
- **鉴权**：JWT、WebAuthn/Passkeys、OAuth（GitHub、Discord、OIDC、LinuxDO、Telegram、微信）。
- **界面**：现代化后台、多语言（zh/en/fr/ja/vi…）、数据看板、模型性能指标。

> 📚 网关/API 细节请参考上游 [new-api 文档](https://docs.newapi.pro)。

---

## 🚢 部署

> [!TIP]
> **最新镜像：** `ghcr.io/nsuanningmeng/lemonhub:latest`（多架构：amd64 + arm64）。

### 环境要求

| 组件 | 要求 |
|---|---|
| **本地数据库** | SQLite（Docker 须挂载 `/data`） |
| **远程数据库** | MySQL ≥ 5.7.8 或 PostgreSQL ≥ 9.6 |
| **缓存（推荐）** | Redis |
| **运行引擎** | Docker / Docker Compose |

### 常用环境变量

| 变量 | 说明 | 默认值 |
|---|---|---|
| `SESSION_SECRET` | 会话密钥（**多机部署必填**） | - |
| `CRYPTO_SECRET` | 加密密钥（**共享 Redis 必填**） | - |
| `SQL_DSN` | 数据库连接串（MySQL/PostgreSQL） | - |
| `REDIS_CONN_STRING` | Redis 连接串 | - |
| `TRUSTED_REDIRECT_DOMAINS` | 受信任域名（逗号分隔），用于支付跳转/多域名回调 | - |
| `PAYMENT_WEBHOOK_RATE_LIMIT` | 支付回调 webhook 的宽松按 IP 兜底限流（次/窗口） | `1800` |
| `PAYMENT_WEBHOOK_RATE_LIMIT_DURATION` | 上述限流的窗口（秒） | `60` |
| `STREAMING_TIMEOUT` | 流式无响应超时（秒） | `300` |
| `MAX_REQUEST_BODY_MB` | 最大请求体（MB，解压后计算） | `32` |

> 限流及大多数调优变量都有合理的代码默认值，因此**无需**写进 `.env`/compose。可选项见 `.env.example`。

### 多机部署注意

> [!WARNING]
> - **必须设置** `SESSION_SECRET`——否则各节点登录状态不一致。
> - **共享 Redis 时必须设置** `CRYPTO_SECRET`——否则加密数据无法解密。

### 重试与缓存

- **重试**：`设置 → 运营设置 → 路由可靠性`（失败重试次数 + 自动重试状态码范围）。
- **缓存**：`REDIS_CONN_STRING`（推荐）或 `MEMORY_CACHE_ENABLED`。

---

## 🧱 基于 new-api 构建

LemonHub 是一个 AGPL 授权的 fork，特别感谢上游项目：

| 项目 | 角色 |
|---|---|
| [new-api](https://github.com/QuantumNous/new-api) | 直接上游——LemonHub 扩展的网关 |
| [One API](https://github.com/songquanpeng/one-api) | 最初的项目基座（MIT） |

LemonHub 定期与上游 new-api 同步。relay / 计费 / 渠道转发核心保持与上游字节级兼容，以便干净地合并上游特性与修复；LemonHub 的增量（子站隔离、钱包、按站收款、品牌）都围绕其外围实现。

---

## 📜 开源许可

本项目继承上游许可，采用 [GNU Affero 通用公共许可证 v3.0（AGPLv3）](./LICENSE)。

依据 AGPLv3 第 7 条附加条款，修改版本必须在适当的法律/关于/页脚位置保留作者署名 `Frontend design and development by New API contributors.`，并保留指向原始项目的可见链接：<https://github.com/QuantumNous/new-api>。

本项目基于 [One API](https://github.com/songquanpeng/one-api)（MIT 许可）开发。

---

## 💬 帮助与贡献

- 🐛 Issue 与需求：[LemonHub Issues](https://github.com/nsuanningmeng/LemonHub/issues)
- 📘 子站教程：[中文](./docs/subsite-guide.md) · [English](./docs/subsite-guide.en.md)
- 📚 网关/API 参考：[new-api 文档](https://docs.newapi.pro)

欢迎各种形式的贡献——Bug 反馈、功能建议、文档与代码。

<div align="center">

如果 LemonHub 对你有帮助，欢迎点一个 ⭐️

<sub>🍋 LemonHub —— 在 <a href="https://github.com/QuantumNous/new-api">new-api</a> 之上的白标 / 代理加盟层。</sub>

</div>
