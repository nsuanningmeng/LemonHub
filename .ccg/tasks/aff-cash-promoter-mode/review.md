# 审查记录 — aff-cash-promoter-mode

## 验证结果
- `go build ./...` ✓ | `go vet ./model/` ✓ | `go test ./model/` 全包通过 ✓
- 前端 `bun run typecheck` + `bun run build` ✓
- Smoke：fresh DB 启动 + 重启两次，`/api/status` 正常；sqlite3 确认 `users.aff_cash_settled` 列已建；重启**无 ALTER churn** ✓

## 对抗式审查（内部 Agent，资金路径）
**Critical：无。** 两种模式内部一致；幂等保持（trade_no+kind 唯一索引 + 存在性检查不变）；普通路径逐字节回归；provided-flag 保留语义正确（省略字段不清零）；返佣榜 SQL 占位符绑定顺序正确、跨三库无方言依赖；日志按模式分支正确。

### Warning 处置
- **W1（已修复）**：原 `total_commission_quota` 按"全时段 recharge_commission"求和作为现金依据。若管理员把一个**已产生平台返佣**的老用户中途切成推广者，旧的(已入平台额度的)返佣会被错误计入"应付现金"，导致重复支付。
  → 修复：`AffiliateCommission` 增加 `cash_settled` 标记（结算时按 `!creditPlatformQuota` 写入；默认 false = 历史行/普通行，**无需回填**）。返佣榜字段改名 `cash_commission_owed` = `SUM(commission_quota) WHERE kind=recharge_commission AND cash_settled=true`。切换模式只从切换点起累计现金，旧行天然排除。
- **W2（评估后不改，保持跨函数一致）**：`month_commission_quota` 含 first_bonus 的 commission_quota，与新列口径不同。但对**推广者**而言 first_bonus.commission_quota=0，月度即纯返佣，不受影响；仅普通用户行存在显示差异。改它会与 `GetAffAdminSummary` / `GetAffStats` 的月度口径产生分歧，得不偿失。新列已明确命名"应付现金返佣"，不与月度直接可比。
- **W3（已修复）**：补充返佣榜单测（mixed first_bonus + recharge_commission + cash-settled），断言 `cash_commission_owed` 只计 cash_settled 行、first_bonus 不计入、normal 邀请人为 0。

### Info（暂记，未做）
- **I1**：`cash_commission_owed` 是累计值、无"已结清水位线"；多次现金结算需人工减去上次已付。可后续加结算批次/水位线。
- **I2**：`recordManageAuditFor` 未记录 `aff_cash_settled` 旧→新值（与现有 aff_commission_percent 一致）。资金开关后续可加专项审计。

## 增量 2 审查 — 现金结算批次（对抗式审查，资金路径）
**Critical：无。**
- **W1（已修复）**：`RecordAffiliateCashPayout` 原「读 SUM → 判断 → 插入」在 MySQL/PG 上有 TOCTOU 并发窗口（同一推广者两笔并发结算可超付）。本仓库明确不用 `FOR UPDATE`（GORM v2 `gorm:query_option` 不可靠 + SQLite 拒绝，见 `site_topup.go:38`），故按仓库惯例（`CompleteEpayTopUp` 原子条件 UPDATE）改为：User 增加权威计数列 `aff_cash_paid`，用**带上限的条件 UPDATE** `WHERE aff_cash_paid + ? <= total` 在行写锁下重判当前值推进，并发结算串行化、永不超付；跨三库安全。返佣榜 `paid` 改读该计数列（省一条分组查询）。
- **I1（保留）**：「全额结清」预填在非默认 `quotaPerUnit` 下可能因 6 位小数四舍五入略超未结而被客户端挡住（纯 UX，绝不超付：客户端+服务端双重防超付）。
- **I2（保留）**：model 层错误中英混用，经 `common.ApiError` 原样透出（与现有 model 约定一致，cosmetic）。
- **I3（保留/历史）**：`month_commission_quota` 仍含 first_bonus，与新 cash 列口径不同；pre-existing，无资金影响。
- 已确认 clean：超付逻辑、跨库 `commonTrueVal`/参数顺序、total/paid/owed 一致性、AdminAuth/校验/审计、前端单位换算（复用 user-quota-dialog）、AutoMigrate（两表+两列，三库便携）。

## 事故与恢复 — 前端 mass reformat
某前端子 Agent 运行了 `bun run format`（`scripts/format-with-protected-headers.mjs --write`，Prettier），把整个 `web/default` 重排了 **764 个文件**（仓库本就不是 prettier-clean）。已恢复：保存 17 个特性文件 → `git checkout HEAD -- web/default/`（撤销全部重排，未跟踪新文件保留）→ 还原 17 个特性文件。最终前端仅余特性文件；typecheck + build 通过。`referral-admin-leaderboard.tsx` 内含约 114 行重排噪声（真实改动≈70/10），属单文件可接受。**教训**：特性任务中禁止运行 `bun run format`。

## Codex 审查（安全 + MySQL 升级迁移数据安全）— 75/100，无 Critical
- **H1 推广者/返佣/现金结算权限粒度（已修复）**：原任意 admin 可改 `aff_cash_settled`/`aff_commission_percent` 及调用现金结算接口。用户选「仅 root」→ 两个现金结算接口加 `middleware.RootAuth()`；EditUser 对这两字段的**实际变更**做 root 门禁（按 change 而非 presence，避免锁死普通 admin 的常规编辑）；前端对应隐藏控件（非 root 不显示）。
- **H2 int64 溢出（已修复）**：`aff_cash_paid + ? <= ?` 改为 `aff_cash_paid <= total-amount`（Go 侧相减，无 DB 端加法溢出），行为等价。
- **H3 MySQL/PG 迁移 NULL 安全（已加测试验证）**：legacy 行 `cash_settled=NULL`；新增 `TestCashOwed_LegacyNullCashSettledExcluded` 证明 `cash_settled = <true>` 过滤 NULL-safe（NULL 行被排除，已入额度的历史返佣不会被当成应付现金）；`aff_cash_paid BIGINT NOT NULL DEFAULT 0` 由 AutoMigrate 回填 0。结论：升级无数据丢失/重复支付。
- **M1 审计补全（已修复）**：现金结算审计补 `payout_id` + `note`。
- **M4 前端查询串（已修复）**：`getAffAdminCashPayouts` 改用 `URLSearchParams`。
- **M2（按设计保留）**：允许在关闭 cash 标志后继续支付历史欠款（残留欠款应可结清）——intended。
- 已确认 clean：接口 AdminAuth/RootAuth、SQL 参数化（commonTrueVal 非用户输入）、note React 转义无 XSS、历史 NULL 安全、varchar(255) utf8mb4。

## 结论
核心资金路径无 Critical；W1 已用 `cash_settled` 行级标记彻底修复（现金统计对中途切换也正确）；W3 补测；W2/I1/I2 为已知项，记录在案。
