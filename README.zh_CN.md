<div align="center">

![LemonHub](/web/default/public/logo.png)

# LemonHub

**多租户 AI API 网关，内置代理（分销）加盟体系、增强版邀请返佣系统与工单套件 —— 基于 new-api 二次开发。**

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
  <a href="#项目简介">项目简介</a> •
  <a href="#与-new-api-的差异">与 new-api 的差异</a> •
  <a href="#快速开始">快速开始</a> •
  <a href="#代理与分销加盟">代理加盟</a> •
  <a href="#部署">部署</a>
</p>

</div>

## 项目简介

LemonHub 是 [new-api](https://github.com/QuantumNous/new-api)（其本身基于 [One API](https://github.com/songquanpeng/one-api)）的二次开发分支。它保留 new-api 完整的网关能力 —— 统一代理 40+ AI 服务商（OpenAI、Claude、Gemini、Azure、AWS Bedrock 等），含计费、限流与管理后台 —— 并在其之上新增：

- **多租户、代理加盟层**：一套部署即可承载多个独立子站，每个子站由一名代理（分销商）运营，拥有自己的域名、收款商户和预付进货钱包；
- **增强版邀请返佣系统**：首充到账、按比例持续返佣、按用户差异化比例、管理员全站返佣榜，以及现金结算推广者模式；
- **工单与邮件推广套件**；
- **客户端一键配置**（Connect Hub）；
- 以及一系列**计费、支付、中继、安全与迁移**方面的改动。

中继 / 渠道转发 / 计费核心与上游保持兼容，便于干净地合并 new-api 的新特性与修复；LemonHub 的新增能力都围绕其外围实现。

> [!IMPORTANT]
> - 本项目仅用于合法、获授权的 AI API 网关、组织级鉴权、多模型管理、用量分析、成本核算与私有 / 分销部署等场景。
> - 你必须合法获取上游 API 密钥、账号、模型服务与接口权限，并遵守上游服务条款及适用法律法规。
> - 向公众提供生成式 AI 服务时，请先完成所在司法辖区要求的备案、许可、内容安全、实名、日志留存、税务、支付与上游授权等义务。

## 与 new-api 的差异

以下是分支在上游 new-api 之上新增或改动的全部内容，按领域分组。这是自首个 LemonHub 版本以来的累计清单，并非单次发布。

### 1. 多租户子站

- 一套部署、一个数据库承载多个独立子站；每个请求按 `Host`（域名中间件）路由到对应子站。
- 按站定制：站点名称、Logo、公告、页脚、首页主视觉文案均可逐站配置。
- 按站数据隔离：每张表带 `site_id`；用户名按站唯一（`(site_id, username)`）；密码、2FA 与所有 OAuth 绑定按站隔离；注册、登录、OAuth 均限定在当前站点。跨站访问有越权测试覆盖。
- 主站提供子站创建与管理页面。

### 2. 代理 / 分销加盟

- 两类角色：**主站站长**（平台方 —— 拥有部署、上游渠道，批发额度）与**子站站长**（代理 —— 通过专属代理后台运营子站，后端由 `SiteAdminAuth` 端点支撑）。
- **进货钱包**：代理向平台预付，钱包以整数厘（毫元）记账。每笔用户充值在同一事务内按批发价原子扣减钱包；钱包永不为负，每次变动写入账本。主站可充值、调整、对账、作废与退款。
- **按代理批发（折扣）率**：`DiscountRate` 为万分比整数（`10000` = 原价，`7000` = 七折）。批发成本 = `原价 × DiscountRate / 10000`，差价归代理。
- **按站收款**：每个子站配置自己的易支付商户。终端用户充值进入代理自己的商户；平台在同一数据库事务内结算钱包（给用户加额度**并**扣代理钱包），幂等，并带**自动降级** —— 当代理钱包耗尽或未配置时，该子站的在线充值入口自动隐藏，已发放额度与其他通道不受影响。
- **按站兑换码**：代理生成 / 作废仅限本站的兑换码，跨站隔离并对账。
- **按站自定义模型加价**（route A）：代理可对本站特定模型调用加价，平台的批发结算永不被压价。
- 代理加盟落地页、可配置的联系我们页，以及导航 / 页脚入口。

### 3. 邀请返佣系统

上游仅提供一次性邀请奖励。LemonHub 将其替换为完整的返佣系统：

- 邀请奖励改为在**被邀请人首次充值成功后**到账（而非注册即发），并对被邀请人的每次充值**按比例持续返佣**；两者均走幂等的逐笔台账并提供统计接口。
- 面向用户的返佣详情页与个人贡献排行榜。
- **按用户差异化返佣比例**，可覆盖全局比例（不设则继承全局；设为 `0` 则对该邀请人停发）。
- **管理员全站返佣榜**（汇总卡 + 排行榜，支持搜索、排序、分页，受 `AdminAuth` 保护）。
- **现金结算推广者模式**：针对线下以现金结算的推广者的按用户开关。开启后不再发放平台邀请奖励额度，充值返佣以台账记为应付现金（不计入平台余额）；现金结算台账跟踪未结余额，结算防超付、并发安全。资金类操作（标记推广者、设置返佣比例、记录 / 查看现金结算）仅限 root / 站长账号。

### 4. 工单与邮件推广

- 用户工单台与管理员工单管理，支持优先级。
- 邮件推广 / 群发工具：表结构迁移、附件上传、群发、限流、清理与审计。
- 后端安全单测覆盖上传处理、Markdown XSS、头注入、越权与优先级。

### 5. Connect Hub（客户端一键配置）

- 为 Claude Code、Codex、Gemini CLI、Chatbox、Cherry Studio、VS Code 等提供一键配置。生成的配置指向**用户当前实际访问的域名**，因此在多域名下都能正确工作。

### 6. 计费与令牌路由

- **令牌多分组优先级失败转移**：令牌可携带有序分组列表并在分组间转移；**tiered / 表达式计费按实际使用的分组**计算。
- 默认前端的倍率编辑器会显示**当前未定价**的模型，避免被悄悄漏掉。

### 7. 支付与中继可靠性

- 易支付回调豁免 gzip 处理与全局限流，并配专用的宽松兜底限流；`notify_url` 固定回稳定域名。
- 支付回调 / 返回地址与首屏页面 `<title>` 跟随用户访问的（可信）域名，并防 Host 伪造；修复标题闪烁问题。
- 易支付结算的并发 / 幂等测试。
- 中继重试遵循配置的 504/524 状态码；非流式 `BadResponseBody` 响应可重试；渠道耗尽时回传真实的上游错误而非笼统报错。
- 订阅修复：订阅过期后正确退回用户原分组（续费时保留 `prev_user_group`）。

### 8. 模型性能设置

- 成功率阈值、错误码白名单、无数据按 100% 处理。

### 9. 安全与迁移加固

- 针对高级自定义渠道的 SSRF 加固。
- 三种数据库（SQLite / MySQL / PostgreSQL）上的迁移安全：`price_amount` 精度的失败即停预检、订阅价格精度预检、快速路径失败即停预检，以及 `site_id` NULL 兜底回填，确保主站查询不会漏掉历史数据。

### 10. 打包与文档

- Docker 镜像发布到 GitHub Container Registry（`ghcr.io/nsuanningmeng/lemonhub`），多架构（amd64 + arm64）；`docker-compose` 默认指向分支镜像。
- 提供全语言 README 与保姆级子站 / 代理教程。

> 第一次接触代理模式？请看分步指南：**[子站 / 代理加盟保姆级指南（中文）](./docs/subsite-guide.md)** · [English](./docs/subsite-guide.en.md)

## 快速开始

### Docker Compose（推荐）

```bash
git clone https://github.com/nsuanningmeng/LemonHub.git
cd LemonHub

# 检查 / 编辑配置（数据库密码、ServerAddress 等）
nano docker-compose.yml

# 启动（compose 文件已指向 LemonHub 镜像）
docker compose up -d
```

<details>
<summary>使用原生 Docker 命令</summary>

```bash
docker pull ghcr.io/nsuanningmeng/lemonhub:latest

# SQLite（默认 —— 需挂载 /data 以持久化）
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

部署完成后访问 `http://localhost:3000`。**首个注册账号将成为 root / 平台（主站）管理员。**

> [!WARNING]
> 以公开或分销形式对外提供 AI 服务前，请先完成备案、许可、内容安全、实名、日志留存、税务、支付与上游授权等所有必要义务。

## 代理与分销加盟

LemonHub 围绕两类角色构建：

- **主站站长（平台方）** —— 拥有部署与 AI 渠道（上游密钥），批发额度。创建子站、为代理钱包充值、设定每个代理的折扣率。
- **子站站长（代理 / 分销商）** —— 在自己的域名上运营子站，配置自己的收款商户与外观，服务自己的终端用户。

资金流一览（示例，折扣 `7000` = 七折）：

```
终端用户付 ¥100  ──►  代理自己的易支付商户   （¥100 全进代理账户）
        │
        ▼ （回调，一个数据库事务，幂等）
   用户到账 ¥100 额度   +   代理进货钱包扣 ¥70
                                  └─ ¥30 为代理利润
```

完整流程 —— 每个角色需准备什么、每一步怎么操作 —— 见：
**[子站 / 代理加盟保姆级指南（中文）](./docs/subsite-guide.md)** · **[English](./docs/subsite-guide.en.md)**

## 继承自 new-api

LemonHub 保留 new-api 的网关能力，包括：

- **格式**：OpenAI Chat/Responses/Realtime、Claude Messages、Google Gemini、Rerank（Cohere/Jina）、图像/音频/Embedding、Midjourney-Proxy、Suno、Dify。
- **格式转换**：OpenAI ⇄ Claude Messages、OpenAI → Gemini、thinking 转 content、reasoning-effort 后缀。
- **智能路由**：加权随机渠道、失败自动重试（可配置重试状态码）、用户级限流。
- **计费**：按次 / 按量 / 缓存命中计费，tiered 与表达式定价，易支付与 Stripe 充值。
- **鉴权**：JWT、WebAuthn/Passkeys、OAuth（GitHub、Discord、OIDC、LinuxDO、Telegram、微信）。
- **界面**：现代化后台、多语言（zh/en/fr/ja/vi…）、数据看板、模型性能指标。

网关 / API 细节请参考上游 [new-api 文档](https://docs.newapi.pro)。

## 部署

> [!TIP]
> 最新镜像：`ghcr.io/nsuanningmeng/lemonhub:latest`（多架构：amd64 + arm64）。

### 环境要求

| 组件 | 要求 |
|---|---|
| 本地数据库 | SQLite（Docker 需挂载 `/data`） |
| 远程数据库 | MySQL ≥ 5.7.8 或 PostgreSQL ≥ 9.6 |
| 缓存（推荐） | Redis |
| 运行引擎 | Docker / Docker Compose |

### 常用环境变量

| 变量 | 说明 | 默认值 |
|---|---|---|
| `SESSION_SECRET` | 会话密钥（多节点必填） | - |
| `CRYPTO_SECRET` | 加密密钥（共享 Redis 时必填） | - |
| `SQL_DSN` | 数据库连接串（MySQL/PostgreSQL） | - |
| `REDIS_CONN_STRING` | Redis 连接串 | - |
| `TRUSTED_REDIRECT_DOMAINS` | 支付跳转 / 多域名回调的可信域名（逗号分隔） | - |
| `PAYMENT_WEBHOOK_RATE_LIMIT` | 支付回调 webhook 的宽松按 IP 兜底（次 / 窗口） | `1800` |
| `PAYMENT_WEBHOOK_RATE_LIMIT_DURATION` | 上一项的窗口（秒） | `60` |
| `STREAMING_TIMEOUT` | 流式无响应超时（秒） | `300` |
| `MAX_REQUEST_BODY_MB` | 最大请求体（MB，解压后） | `32` |

限流及大多数调优变量都有合理的代码默认值，因此不必写进 `.env`/compose。可选项的说明见 `.env.example`。

### 多节点

> [!WARNING]
> - 必须设置 `SESSION_SECRET`，否则各节点登录态不一致。
> - 共享 Redis 时必须设置 `CRYPTO_SECRET`，否则加密数据无法解密。

### 重试与缓存

- 重试：`设置 → 运营设置 → 路由可靠性`（失败重试次数 + 自动重试状态码区间）。
- 缓存：`REDIS_CONN_STRING`（推荐）或 `MEMORY_CACHE_ENABLED`。

## 基于 new-api

LemonHub 是采用 AGPL 许可的分支。感谢上游项目：

| 项目 | 角色 |
|---|---|
| [new-api](https://github.com/QuantumNous/new-api) | 直接上游 —— LemonHub 扩展的网关 |
| [One API](https://github.com/songquanpeng/one-api) | 最初的项目基础（MIT） |

LemonHub 会定期与上游 new-api 同步。

## 许可证

本项目采用 [GNU Affero 通用公共许可证 v3.0（AGPLv3）](./LICENSE)，继承上游许可证。

根据 AGPLv3 第 7 条附加条款，修改版必须在适当的法律 / 关于 / 页脚位置保留作者署名 `Frontend design and development by New API contributors.`，并保留指向原始项目的可见链接：<https://github.com/QuantumNous/new-api>。

本项目基于 [One API](https://github.com/songquanpeng/one-api)（MIT 许可证）开发。

## 帮助与贡献

- 问题与需求：[LemonHub Issues](https://github.com/nsuanningmeng/LemonHub/issues)
- 子站指南：[中文](./docs/subsite-guide.md) · [English](./docs/subsite-guide.en.md)
- 网关 / API 参考：[new-api 文档](https://docs.newapi.pro)

欢迎各类贡献 —— 缺陷反馈、功能、文档与代码。

<div align="center">
<sub>LemonHub —— 在 <a href="https://github.com/QuantumNous/new-api">new-api</a> 之上的代理加盟层。</sub>
</div>
