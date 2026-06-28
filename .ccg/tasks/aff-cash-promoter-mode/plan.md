# 实施计划 — 专业推广者(现金结算)模式

## 目标
为「邀请人」新增一个按用户标记的 **现金结算推广者** 模式。开启后：
- ① 邀请奖励额度 `first_bonus`：**不**发放给该邀请人（QuotaForInviter 不入其 aff_quota/aff_history）
- ② 充值返佣 `recharge_commission`：**仍按比例写入台账**(AffiliateCommission)，但**不**计入其 aff_quota/aff_history（作为线下现金折算依据）
- 被邀请新用户的注册奖励 QuotaForInvitee：**照常发放**（与邀请人身份无关）
- 普通用户：行为完全不变

## 关键决策（已与用户确认）
1. 返佣 → 只记台账，不进平台额度
2. 新用户注册奖励 → 照常发放
3. 标记方式 → 管理员「用户编辑」加开关

## 数据模型
新增 `User.AffCashSettled bool` → 列 `aff_cash_settled`。
- **不加** `default` 标签（规避 bool 默认值导致的 AutoMigrate ALTER churn，参考 SubscriptionPlan.Enabled 已知问题）
- AutoMigrate 自动 ADD COLUMN，跨 SQLite/MySQL/PG，老数据 NULL→Go 扫描为 false
- 无需手写迁移

## 实施步骤（按层归属）

### Layer 1 — 后端核心（model）
**`model/user.go`**
- struct 新增 `AffCashSettled bool` 字段（紧邻 AffCommissionPercent，带注释说明语义）
- `Edit(updatePassword, updateAffCommission)` → 新增第 3 参 `updateAffCashSettled bool`；为真时 `updates["aff_cash_settled"] = newUser.AffCashSettled`

**`model/affiliate.go`** — `SettleReferralOnTopUp`
- inviter Select 增加 `aff_cash_settled`
- `if inviter.AffCashSettled { inviterReward = 0 }`（invitee reward 不变 → first_bonus 仍给新用户、仍记 ledger、aff_count 仍 +1，但不入邀请人额度）
- `creditPlatformQuota := !inviter.AffCashSettled` 传入 commission 结算
- `settleAffiliateRechargeCommission(...)` 新增参数 `creditPlatformQuota bool`：写完 ledger 行后，若为 false 则跳过对邀请人 aff_quota/aff_history 的 Update
- 日志区分：推广者写「推广返佣(现金结算)记账 X，未计入平台额度」；普通用户保持原文案

### Layer 2 — 后端接口 + 报表（依赖 L1）
**`controller/user.go`** — `EditUser`
- rawFields 检测 `aff_cash_settled` 是否出现 → `affCashSettledProvided`
- 传入 `updatedUser.Edit(updatePassword, affCommissionProvided, affCashSettledProvided)`

**`model/affiliate.go`** — `GetAffAdminLeaderboard`（让"一键统计应付现金"可用）
- `AffAdminLeaderboardItem` 增加 `IsCashSettled bool` + `TotalCommissionQuota int64`（该邀请人 recharge_commission 终身累计，来自 ledger）
- user Select 增加 `aff_cash_settled`；现有每页 ledger 分组查询追加 `SUM(commission_quota)` 终身合计（无新增 N+1）

### Layer 3 — 前端（web/default）
- `features/users/types.ts` → user 类型加 `aff_cash_settled?: boolean`
- `features/users/components/users-mutate-drawer.tsx` → AffCommission 字段下加 Switch「专业推广者(现金结算)」+ 说明；表单默认值 + 提交载荷
- `features/referral/types.ts` + `referral-admin-leaderboard.tsx` → 推广者徽标 + 现金返佣累计列
- i18n：`locales/en.json` + `zh.json` 加新键 → `bun run i18n:sync`

### Layer 4 — 测试
**`model/affiliate_test.go`**（deterministic table tests, testify）
- 推广者：first_bonus 不入邀请人 aff_quota/aff_history；invitee 仍得 QuotaForInvitee；aff_count +1；ledger 有 first_bonus + recharge_commission 行且金额正确；邀请人 aff_quota 不因返佣增加
- 普通用户：行为不变（回归）
- 幂等：webhook 重试不双发（推广者路径）

## 验证
- `go build ./...` + `go test ./model/...`
- 前端 `bun run build`（typecheck）
- Smoke：启动应用，确认 AutoMigrate 无 ALTER churn（重启不反复 ALTER aff_cash_settled）

## 增量 2 — 现金结算批次（已结清水位线，解决 I1）
- 新表 `AffiliateCashPayout`（inviter/amount/note/operator/created_at）记录每笔线下现金结算；AutoMigrate（两处列表）注册。
- `RecordAffiliateCashPayout`（事务内重算 outstanding，单笔 > 未结额则拒绝，防超付）+ `GetAffiliateCashPayouts`（历史，新→旧）。
- 返佣榜 item 拆为 `cash_commission_total`(累计) / `cash_commission_paid`(已结) / `cash_commission_owed`(未结=总−已结，clamp≥0)。
- 接口：`GET /api/user/aff/admin/cash-payouts`、`POST /api/user/aff/admin/cash-payout`（AdminAuth）。
- 前端：返佣榜每行「记录现金结算」弹窗（未结/累计/已结 + 预填未结金额 + 备注 + 结算历史），客户端也挡超付；i18n。
- 测试：部分结算→全额结清→再结报错且不写行；超付拒绝；校验；历史顺序；普通用户 outstanding=0 拒绝。

## 风险
- **high（资金/额度）**：核心改动在结算路径。缓解：保留全部既有幂等保证；新增分支只「少发/不入额度」，不改既有金额计算；单测覆盖推广者 + 普通用户两条路径。
