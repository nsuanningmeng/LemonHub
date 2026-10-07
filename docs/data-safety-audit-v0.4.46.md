# v0.4.46 数据安全审计与发布验收

日期：2026-10-07。**发布前代码审查、隔离候选 MySQL 8.2 生命周期、原生测试、有限真实流量及资源清理已通过独立复核。** 本文记录这些已执行证据和适用边界。正式 CI、Release/GHCR 产物与发布后 pending-batch 停机验证，另在 Release 正文或安全摘要附件记录，不将本地候选等同已发布镜像。

## 版本来源与部署边界

本次以公开 v0.4.45 为升级对照，保留它的完整 HTML 联系页修复和前端、桌面依赖审计修复。对应发布源码为 `aee5cff5f3c81c70a7bff12ec4bff1fe9d86bedb`；本地合并提交 `179792cc81d13a6124c97b01cb46022507cb2ba0` 已纳入该源码。B22 已审提交为 `f3c2442c9dd4f778ebae96aa68f86f8c7ed41f2a`；随后 B23 已审提交为 `9c8ae7dd919841567800edcf4a1550b6c7c28beb`。B24 最终代码门禁对应提交 `7ccccc5176018cde0eaf8c15fa440d42d05086a3`。最终发布还包含后续已审文档，其 commit 身份将在 Release 证据中记录。

用户已确认生产应用为 v0.4.45，数据库为 MySQL 8.2，使用 `${APP_PATH}/mysql_data:/var/lib/mysql` 宿主机绑定目录，并启用 Redis、`BATCH_UPDATE_ENABLED=true`、`ERROR_LOG_ENABLED=true` 和 `Asia/Shanghai` 时区。该信息来自提供的部署配置和版本确认；本次没有连接或检查生产主机、数据库、Redis 或实际挂载。生产应用的运行 image ID / digest 仍未知。

只读发布调查当时，公开 GHCR `v0.4.45` 与 `latest` 指向同一 manifest digest：`sha256:b18fda29dcb140fe57c8a974ac5992cd42fc7772b8e48e427f604d770349c7bd`。这是注册表对照身份，不能据此断言用户主机已拉取或正在运行该 digest。最终演练和新版本发布仍须记录实际使用的镜像及提取二进制身份。

完整操作步骤由 [MySQL 8.2 应用升级说明](mysql82-production-upgrade.md)维护。应用更新不包含 MySQL 引擎升级，也不替换数据库目录。不得将本地测试资源或旧演练卷视作生产数据的副本。

## 原 200 条 issue 审计

原审计取 `QuantumNous/new-api` 精确 `bug` 标签、按创建时间降序的最新 200 条 issue，排除 PR。抓取时为 open 76、closed 124；不以关闭状态直接判断本项目是否已经修复。

| 初始分类 | 条目数 | 本次表述 |
| --- | ---: | --- |
| 确认存在 | 78 | 合并重复问题后 76 个独立单元已闭环。 |
| 疑似 | 4 | 不计为已修复。 |
| 已修复或等效规避 | 51 | 不充作新增修复。 |
| 不适用或非缺陷 | 44 | 保留架构和产品语义边界。 |
| 证据不足 | 23 | 不等同确认没有缺陷。 |
| 合计 | 200 | 不声称全部端到端复现。 |

重复组是 `#6656/#6657`、`#7130/#7127`。76 个单元由 **74 个代码修复类单元和 2 个澄清单元**组成：`#7672` 不公开渠道实例私名作为模型 owner；`#6978` 不把并行订阅购买改成串行续期。新增发布审查问题 B22–B24 单独登记，不回填或膨胀原 78/76 计数。

原 21 批均完成独立全面审查。已审生产快照是 Git tree `001da08ab912faa23b921faa0324a000e8865bc1`，不是最终发布 commit。逐项 issue、行为、别名、批次和证据见[初始审计](upstream-bug-issues-audit-2026-10-06.md)与[修复交付记录](upstream-bug-fixes-2026-10-07.md)。最终发布包含后续合并和新修复，必须重新绑定最终源码及产物，不能只引用这个历史 tree 的通过状态。

## 发布前新增修复及门禁

| 批次 | 范围 | 当前可确认结果 |
| --- | --- | --- |
| B22，3 个较大单元 | 看板统计和退出保存；渠道缓存与显式能力修复；持久化设置和密钥初始化 | 全面代码门禁通过。冻结 tree `3265392f2d30ea75f3f5406079e7c2028defa55b`，12 个增量路径逐字节与 hash 绑定。相关根包、model、controller、service 完整检查均 exit 0；目标故障回归和 race 证据已审。原生 MySQL 8.2 与最终镜像演练另列，不能由代码门禁代替。 |
| B23，3 个较大单元 | 首次 setup 的数据库故障和并发保护；内置权限初始化事务；渠道覆盖配置权限及前端保存 | **全面代码门禁通过。** 冻结 tree `fe3ad94a759d2354f7c781e4d2b582ec8abce437`，提交 `9c8ae7dd919841567800edcf4a1550b6c7c28beb`。独立目标审查及 root/model/controller/service/authz 五包完整相关检查实际 exit 0；前端针对性回归、类型与涉及文件 lint 通过。最终发布构建仍单列。 |
| B24 | 注册配置完整校验、映射原子替换及本地写入顺序；Setup 并发发布；版本打包和构建范围 | **全面代码门禁通过。** 修复、独立源码审查、race 及最终增量 test/vet/build 已通过。冻结 tree `ad5030bbad34def55f9fd2825317eeb774e6d01d`，12 个增量路径。 |

### B22 已审行为与限制

看板 `quota_data` 批量保存使用独立快照和受检查的事务，查询/写入失败不再清掉全部待写数据后伪报成功。确认回滚时合并回待写缓存；提交或回滚结果不确定时隔离快照，禁止自动重放。更新旧可空统计使用明确的 NULL 处理，按选中行更新，避免对历史重复维度重复累加。退出最后一轮保存的失败会向上传递。

这修复看板统计和退出错误报告，**不表示钱包/token 余额原来都只存在批量内存队列中**。正常余额预扣仍走 SQL；订阅和异步任务有各自已审的持久回执、账本和对账约束。普通钱包退款、日志和统计也不因此获得一个跨所有表的通用 exactly-once 事务。

渠道缓存加载失败保留原已发布映射；不得把读取异常当作需要删除重建的信号。只有显式管理员修复才在受检查事务中重建派生能力；确认提交后发布缓存，不确定提交不盲目重放。

设置先完整读取和校验再替换内存；保存先确认 SQL 成功再发布内存值。退订密钥只在确认真实缺失或空值时初始化，竞争时读取数据库胜者，不能因读取失败替换原值。该批不声称已完成所有注册配置的类型校验；后者由 B24 修复并通过单独全面门禁。

### B23 已审范围

初始化区分“确实未初始化”和查询失败，将 root、模式选项和 setup 标记纳入一个事务，并处理并发首次初始化及提交确认丢失；数据库异常时不能重新开放匿名初始化。

内置权限初始化将角色、内置策略替换和私有 enforcer 加载放入受检查事务，确认提交后才发布，保留自定义/用户策略；从节点初始化不写入。启动中途失败不能留下部分权限或发布一个未持久化的授权视图。

渠道覆盖配置对列表、搜索、分组、详情及更新回显统一限制：raw Header/Parameter Override 仅 root 或 ChannelSecretView 可读，其余只返回配置存在标记。ChannelSensitiveWrite 独立授权明确替换/清除；前端未编辑字段省略，不能把隐藏默认空串保存回数据库。完整 channel key 仍走 root 与安全证明专用端点。后端已有 26 个实际 HTTP 回归叶子及 race 通过；前端隐藏值保存、明确清除和权限切换回归，以及全 Web 类型检查、涉及文件 lint 已通过。前端证据保留了扩展测试中浏览器 fixture 缺失造成的失败及修正后的成功；整批冻结审查已通过；随后完成的根仓库及前端集成检查见下文。

### B24 已审修复与补验范围

注册配置先校验本次提供的所有已注册值，任何格式或范围错误都不能造成部分 SQL 保存或内存发布。映射更新保留既有对象身份和未知字段兼容约定；本地设置写入从 SQL 到发布按同一顺序串行执行。独立审查进一步复现了 setup 提交确认延迟与普通设置写入交错的真实竞态，最终初始化入口采用同一个写入锁，保留数据库固定主键的跨进程首次初始化保护。该约束不等于跨节点内存同步或多个独立读取操作的全局线性一致性。

最终替代快照的 model/controller race 通过；受影响 model/controller/root 包测试、vet 和嵌入最终前端的 Go 构建均 exit 0。此前完整根模块及独立 relaykit 的 test/vet/build 也均 exit 0，后续改动未触及 relaykit。替代快照的 12 个路径均与工作区、归档、Git tree/blob 和 SHA-256 独立比对一致，B24 最终代码门禁已通过。

发布配置另修复了“构建成功但前端版本标记缺失”的问题：保留现有 Rsbuild 定义，只注入选定版本字符串，不导出整个进程环境。Docker 构建上下文排除本地凭证、数据库数据和备份；CI 增补隔离的 MySQL 8.2 原生测试。初次成功退出但版本缺失的两个构建产物已作废，保留为失败证据。

## MySQL 8.2 实际演练：本地候选验收通过

<!-- FINAL_MYSQL82_RESULTS: local lifecycle/native/finite traffic/cleanup reviewed; published artifacts and pending-batch stop remain separate. -->

本轮在新建隔离资源中执行 **MySQL 8.2 同引擎应用更新**，未访问用户生产数据库或旧审计卷。**生命周期、14 个原生目标、4 次有限真实 HTTP 流量及资源清理均已通过独立复核**。`release82v046a` 的 9 个实际比较均无未批准差异，14 个最终源码原生目标均实际 exit 0。第一轮复用容器名称造成先前阶段的停机原始记录被覆盖；已在另一新隔离运行 `release82v046b` 重做生命周期比较，并完整保留 9 组按阶段和完整容器 ID 命名的停机 inspect/日志，全部退出 0、非 OOM，该证据缺口已关闭。补验没有重跑或重复计数原生目标，也不扩成另一种数据库引擎测试。

### 隔离候选身份

| 项目 | 本次实际身份或边界 |
| --- | --- |
| MySQL | 实际查询为 **8.2.0**；镜像 ID `sha256:bc861cf238f24a71398f27b6eb77051fe60b834e003f33e4a36e3e19c37df1d1`，manifest `sha256:212fe73edca5df6ff14826d5eb975c914bfb91f82a2e923f9050568f99525da1` |
| 旧应用对照 | 固定官方 v0.4.45 manifest `sha256:b18fda29dcb140fe57c8a974ac5992cd42fc7772b8e48e427f604d770349c7bd`，amd64 image ID `sha256:6763a2ff5ee5f9bdd1074cf358ca0f1086283dce5a6f67900f9128e78845600b` |
| 本地候选源码与前端 | B24 tree `ad5030bbad34def55f9fd2825317eeb774e6d01d`；嵌入本报告下方已审的 242 文件前端产物 |
| 本地候选构建 | 在宿主机用 Go 1.26.8 编译 linux/amd64 二进制，显式链接版本 v0.4.46，再复制到上述固定官方 v0.4.45 运行时基础镜像；**不是完整 Dockerfile / GitHub Actions 构建产物** |
| 本地候选产物 | image ID `sha256:e91bcfbb134bc073b18dff8698de3ad67b016a53d20c7682923e1e4e68892075`；镜像内提取二进制 SHA-256 `2cb81387e87ae177d2948bb99d782dab2d46034f6b1083886b4ba9d6fee96b07` 与编译产物一致。此 ID **不是已发布 Release/GHCR digest** |
| 实际应用响应 | 保存的真实 `/api/status` 响应分别报告旧 v0.4.45、新 v0.4.46，且开启 batch；与前端 Chromium 使用 stub 的检查分开 |
| Redis 与应用配置 | 隔离 Redis 8.10.2，`BATCH_UPDATE_ENABLED=true`、`ERROR_LOG_ENABLED=true`、`TZ=Asia/Shanghai`。Redis 身份不等于生产运行 digest；保留性阶段关闭三个外部后台任务，不能代表繁忙生产流量 |

发布后仍须针对实际 Actions/GHCR 产物另验来源、镜像内版本及 MySQL 8.2 启动和数据合同；本地候选结果不能替代它们。

### 数据比较与原生测试范围

9 个比较覆盖原种子恢复等价、旧应用启动保留、迁移、新应用重建、数据库同卷重建、重建数据库后应用启动、备份恢复后应用启动前、恢复后应用启动，以及原生套件结束后的保留性。比较包括 **46 张表**的结构、完整索引与 catalog；其中 **45 张非运行时表**逐行比较，`system_instances` 的运行时行及自增计数单独排除，不把它写成所有 46 张表的运行时行完全不变。

迁移策略预先按源码审定，精确允许订阅预扣回执的 11 个新增字段和日志 `input_tokens_total` 的 1 个可空新增字段，共 **12 个新增列/DDL 差异**；既有列、索引、历史数据及新增字段的默认值/NULL 按策略核验。未根据候选运行后的差异放宽策略。启动阶段仅允许已核的 `authz_roles` 自增 +2、`external_identity_claims` 自增 +3，以及 3 条已知内置授权的代理 ID 和自增 +3，权限内容保持；非启动比较保持精确计数，自定义策略不受豁免。没有忽略整张权限表或全部自增变化。

14 个原生目标全部绑定同一最终 tree，在隔离 schema 中实际执行，覆盖充值、钱包、token 额度/分组/长度、RC25 和宽列迁移、身份排序规则、OAuth 绑定、会话刷新、订阅结算/回执、统计写入故障及权限初始化。**唯一跳过子项**是 `TestPreflightExternalIdentityClaimsMySQLCollation/completed_wide_indexes_remain_idempotent_after_large_prefix_is_disabled`：MySQL 8.2 不再提供 `innodb_large_prefix` 变量。其他目标均无 SKIP，不表述为“全部子项无跳过”。此前 B22 的 13 套与 B23 授权专项是早期冻结源码证据，不再加进本轮 14 个目标的计数。

本次 utf8mb4 备份已恢复到另一新卷；备份与恢复后原始快照、逐阶段比较及 canary 结果均保留。其有效性只覆盖实际种子和固定身份/配置，不等于任意生产数据的零丢失证明。

| 分项验收 | 当前状态 |
| --- | --- |
| 逐阶段停机与生命周期最终复核 | **通过。** 补验 9 个比较及 9 组唯一阶段/完整 ID 的停机原始记录已独立重算复核；首次记录覆盖缺口已关闭 |
| Redis/BATCH 代表性 HTTP 运行 | **通过。** 旧/新应用各一次真实成功和受控 400，共 4 次 HTTP/provider 调用；请求 ID、计费、日志、统计、两段完整原业务保留性均独立复核 |
| 生命周期运行资源清理 | **通过。** 两次运行均按独立许可精确清理各自 7 个卷/网络对象，release exit 0，保存的清理后登记与当前登记均为空；不涉及原数据库或旧审计资源。流量运行另按许可清理 3 个卷及 1 个网络，release exit 0、登记为空，备份保留 |
| 实际发布镜像验证 | 尚未发布，后续另验 Actions/GHCR 来源、版本、8.2 启动及数据合同 |

### 4 次真实流量与精确对账

最终成功运行 `traffic82v046d` 使用同一隔离 Redis、BATCH=true、错误日志开启和 Asia/Shanghai 时区。旧版 S1 成功、E1 返回受控 400，候选 S2 成功、E2 返回受控 400，实际 provider 调用精确为 4 次，没有重试或把失败尝试混入本轮计数。每次成功按 `(10 prompt × 1 + 2 completion × 4) × group 1 = 18` 计费，两次错误各为 0。

| SQL / 日志结果 | 旧版完成后 | 候选完成后累计 |
| --- | ---: | ---: |
| 钱包 / token 剩余额度 | 9982 / 9982 | 9964 / 9964 |
| 用户已用 / 请求数 / 渠道已用 | 18 / 1 / 18 | 36 / 2 / 36 |
| token 已用 | 18 | 36 |
| 消费日志条数 / quota | 1 / 18 | 2 / 36 |
| 错误日志条数 / quota | 1 / 0 | 2 / 0 |
| 停机后看板次数 / quota / tokens | 1 / 18 / 12 | 2 / 36 / 24 |

每阶段在停机前实际等待退款及用户/token/渠道统计的 SQL 屏障通过。停机前看板分别观察到 `0/0/0` 与 `1/18/12`，正常停机后才核对上表完整值；未把 HTTP 完成或健康状态当作统计已持久化。4 个实际响应 request ID 与消费/错误日志一一对应，候选 S2 的 `input_tokens_total=10`。旧版启动、候选迁移及两段请求前后快照同时保护全部原业务行多重集、NULL、私有 JSON、结构、索引、catalog；仅允许明确测试账户的计费/访问字段和测试日志/看板变化，不豁免 vendors 或整张业务表。

这是停止新生产者、先达到 SQL 屏障的有限健康流程。没有执行可选繁忙请求 S3，不证明断电、强制杀进程或任意并发尾部的自动恢复。此次流量独立审查及精确清理均已完成；脚本的合成控制未计入 4 次真实请求。

### 旧版停机的实际边界与未通过尝试

代表性流量的尝试 `traffic82v046c` 在真实旧版 S1 成功、E1 返回受控 400 后，因 HTTP driver 的断言失败，在进入批量统计 SQL 等待屏障前停止旧应用。独立只读对账确认：钱包和 token 均剩 **9982**、token 已用 **18**，消费日志 quota **18**、错误日志 quota **0**，看板为 **1 次 / 18 quota / 12 tokens**；但 `users.used_quota`、`users.request_count`、`channels.used_quota` 均仍为 **0**。这些原始结果保留，未补账、降低期望或记为流量通过。

这证明旧 v0.4.45 首次停机前还必须等上述三个批量字段实际落库。请求完成、钱包正确、正常退出或给出 150 秒外层宽限，都不能替代该屏障；新版本的退出修复不能追溯保护旧进程。最终成功流量已先核对完整 SQL 统计与退款，再正常停止应用。另两次前置失败分别为测试种子缺少 OpenAI vendor（0 次业务 HTTP/provider 调用）、隔离 provider 被继承的 HTTP 代理拦截（1 次 HTTP、0 次 provider 调用、完整退款）。首轮失败清理曾先停数据库，旧应用退出 137，该过程不作正常停机通过证据。修正仅限明确种子和测试网络/driver，数据库保留性比较与业务期望未放宽，所有失败证据保留。

正式 GHCR 镜像发布后还需另测新版本在**已确认存在 pending batch**时正常停机的行为，并记录结果。主流量验收合同要求在 SQL 屏障通过后停机，不能提前冒充该额外发布后证明，也没有执行可选 S3 繁忙请求尾部场景。

此前已完成的另一轮演练覆盖 MySQL **5.7.44、8.0.46、8.4.11**：27 个同版本阶段，加 8 个 5.7→8.0→8.4 物理升级阶段，共 35 个阶段，另有 9 项实际原生测试。这是原 21 批冻结版本的历史证据，见[原演练范围](upstream-bug-fixes-2026-10-07.md)。**它不是 MySQL 8.2 的测试结果，也不证明后续新增源码或本次最终发布镜像。**

## 对提供的生产配置的操作要求

本次未访问生产环境。以下要求来自实际给出的配置与已核源码，不代替运维对主机的检查。

1. 固定并记录应用、MySQL 镜像及 `/var/lib/mysql` 的真实挂载源，保留原 SQL 目标、日志库、Redis 连接/命名空间、会话密钥和时区。不要创建另一个空目录来“修复”路径，看到初始化页先排查连接与挂载。
2. 先在入口停止新写入，旧 v0.4.45 进程保持运行，等待请求、长连接、后台计费、退款及最后一轮写入完成；所有写入节点都适用。必须核对 `users.used_quota`、`users.request_count`、`channels.used_quota` 等批量统计实际落库；钱包正确不代表它们已保存。单条成功日志、固定等待或健康检查不能证明没有待写尾部。
3. 给新应用配置大于内部退出预算的外层宽限。提供的 Compose 没有该设置，默认 10 秒存在强制终止风险；升级文档示例使用内部 120 秒、外层 130 秒。**新配置不追溯作用于尚未重新创建的旧容器，旧进程也没有新退出代码。** 首次停止应按升级文档明确处理。
4. 排空后备份并在独立 MySQL 8.2 实例、新卷验证恢复，显式 utf8mb4；独立日志库另行备份。保存主机故障后仍可取回的副本。非空 dump 或单次导入成功不足以证明恢复完整。
5. 仅更新应用服务，先启动一个迁移主节点。MySQL DDL 不是整个启动迁移的总事务；异常时保留原数据和日志，不清库重建，不顺便升级数据库/Redis 镜像，不执行 `down -v`。新版本会拒绝无法读取或解析的历史持久化设置；按日志定位、备份后修正，不得通过删除全部设置、重置密钥或重新初始化绕过。

详细命令和回退步骤只维护在 [MySQL 8.2 应用升级说明](mysql82-production-upgrade.md)，避免两套步骤不一致。回退若恢复旧备份，必须处理升级后新增业务数据，不能直接覆盖后声称没有损失。

内存队列、隔离快照和普通日志不是持久 WAL。断电、SIGKILL、磁盘故障、持续数据库错误、未完成退款和提交确认丢失不在“自动零损失恢复”承诺内；不确定资金提交必须对账，不能盲目补扣或退款。生产配置中的空密码 root 账户另需受控加固，不应在本次镜像切换时未经验证单方面更改认证。

## 最终发布验证与产物

<!-- FINAL_RELEASE_PROOF: repository records pre-publication evidence; append post-publication identities to Release body/assets. -->

| 检查或产物 | 当前状态 |
| --- | --- |
| B23 / B24 全面门禁及最终源码冻结 | B23、B24 均已通过；B24 替代 tree 的 12 个路径及最终增量 test/vet/build 已独立绑定 |
| 根 Go 模块 / relaykit | 完整 test/vet/build 均实际 exit 0；relaykit 使用独立模块检查。随后 setup 修复的 model/controller/root 测试、vet 与嵌入最终前端的构建再次 exit 0，不虚构 Go 用例总数 |
| 前端全量测试 | 113 个文件、723 个测试通过，0 skip，实际 exit 0；保留 3 条 jsdom `Window.scrollTo` 环境警告，无失败或未处理异常 |
| 前端最终类型、lint 与构建 | 全量测试后仅 `web/rsbuild.config.ts` 变化，其余 1261 个 Web 文件字节不变。最终类型检查、该配置 lint、实际生产/开发定义保留检查、生产构建和 Chromium 检查均 exit 0 |
| 桌面验证 | `npm ci --ignore-scripts` 实际 exit 0、安装 322 个包；指定 Node 测试 9 项通过、0 skip；`npm audit` exit 0、0 公告。16 个已跟踪 Electron 文件和 lock 字节不变。主进程测试模拟 Electron/进程边界，下载器测试走真实依赖及本地 HTTP/proxy/TLS；没有安装 Electron 可执行文件、打包或启动 GUI |
| Bun 依赖公告审计 | `bun audit --json` 实际 exit 0，结果 `{}`；这是执行时注册表公告结果，不保证所有漏洞均被发现 |
| Go 可达漏洞扫描 | `govulncheck@v1.8.0`，Go 1.26.8，漏洞库更新时间 2026-10-01 20:24:15 UTC。根模块实际 exit 0，可达符号 0、已导入包 0；另报告 1 个仅 required-module 层公告 `GO-2026-5932`（`x/crypto v0.56.0` 中未导入、未调用的 `openpgp`，无已知修复版本）。独立 relaykit exit 0，未发现公告 |
| MySQL 8.2 数据保留与运行身份 | 本地隔离候选生命周期、14 个原生目标、4 次真实流量和精确清理均通过；旧版未等统计落库的失败边界保留。正式发布镜像及 pending-batch S3 停机另验 |
| 发布 commit / tag / 时间及 CI | 在 Release 正文/附件记录实际最终提交及对应 CI URL、结果，不能由本地通过状态替代 |
| GitHub Release、完整资产和 checksum | 由发布流程产生后记录实际 URL、清单、校验和及来源证明 |
| GHCR 双架构镜像、签名/来源及 latest | 由发布流程产生后逐项核验 manifest digest、OCI revision/version、架构、签名/来源和工作流结果 |

最终前端产物包含 **242 个文件、58,242,509 字节**，完整 dist manifest 的 SHA-256 为 `7d84e9f74c569812b997751174f7b4e46f6d5f4f2114a788d84eb41982aa7856`。真实 Chromium 加载未修改的构建产物，运行时、HTML、meta、localStorage 和 CSS 标记均为 `rv.v0.4.46.2k6e8r7p`，无页面错误或开发工具；未导出私有及无关 VITE 环境哨兵。浏览器的 `/api/status` 使用本地 stub，**不证明 Go 应用的运行版本**；上方 MySQL 实际运行记录单独确认了本地候选 v0.4.46，正式发布镜像仍另验。Go 漏洞扫描也不覆盖容器操作系统包或尚未公开的漏洞。

只读远端复核（2026-10-07 15:19 UTC）确认 main 仍为 `aee5cff5f3c81c70a7bff12ec4bff1fe9d86bedb`，GitHub branch API 返回 `protected=false`；`v0.4.46` tag 尚不存在，公开 latest Release 仍为 v0.4.45，现有 v0.5.0 草稿保持不变。此状态是查询时观察，不保证随后无人修改远端。

发布流程遵循现有门禁：本地审查通过后推送，等待**最终提交**的 CI 成功，再创建 tag。现有 CI 不因普通分支 push 自动触发，应使用对应 PR 或显式工作流调度。tag push 会分别触发 Release 与 Docker 构建，二者独立运行，不等待彼此或 CI；不得用推送 tag 来替代发布前验证。Release 工作流先上传完整资产至草稿，再检查 tag 身份并公开；Docker 工作流构建 amd64/arm64、版本 manifest 与签名，仅最新稳定版本的 tag push 有资格推进 `latest`。手动 Docker 调度不会推进 `latest`。

**发布后证据记录在 [v0.4.46 Release](https://github.com/nsuanningmeng/LemonHub/releases/tag/v0.4.46) 正文或附件**：实际发布 commit/tag、CI/Release/Docker 工作流 URL、各架构与汇总镜像 digest、二进制及资产校验和、签名/来源验证、实际运行版本。仓库内文档保存发布前已审事实和验收要求，不要求同一个发布提交写入自身 hash。附加摘要仅包含版本、计数、审查结论和 hash 索引；不上传原始 SQL、数据库 dump、凭证或含环境变量的 inspect 输出。该链接用于承载公开后的实际验收记录，不把尚未执行的发布步骤写成已通过。

## 证据保留

已完成原 21 批的仓库记录由[修复交付文档](upstream-bug-fixes-2026-10-07.md)索引。新增本地证据目前位于 `/workspace/scratch/mysql82-release-audit/`，包括 `STATE.md`、`integrated-v045.json`、`b22-independent-review.md`、`b22-independent-final-binding.json` 和 `b23-independent-review.md`，以及 `native-b22-independent-broader-review.json`、`native-b22-independent-quota-review.json` 和 `native-b23-independent-authz-review.json`；B23 覆盖配置证据位于 `/workspace/scratch/b23-channel-overrides/backend-report.md` 和 `frontend-review.md`，前端实际结果及 hash 另见同目录 `frontend-checks.json`。最终前端检查见 `final-web/result.json`、`final-web/summary.md` 与 `final-web/independent-version-packaging-review.json`；Go 扫描见 `final-vuln/summary.md`，桌面验证见 `final-desktop/result.json` 与 `final-desktop/summary.md`，B24 最终绑定见 `b24-independent-final-binding.json`。MySQL 8.2 结果见 `results/release82v046a/independent-lane-final-review.json`、`results/release82v046b/independent-lifecycle-supplement-final-review.json` 及两目录的 `cleanup-result.json`；第一轮原始停机记录缺口与第二轮关闭证据都保留。旧版未等统计落库的实际边界见 `results/traffic82v046c/independent-old-partial-persistence-reconciliation.json`；成功 4 次流量及清理见 `results/traffic82v046d/independent-traffic-final-review.json`、`cleanup-result.json`。最终本地汇总为 `final-local-summary.json`，SHA-256 `ce91720b50ae71b334d98e97f290ba647ef057919165935e812d07d48c90625b`。这些是工作区证据，单独复制仓库文档不会自动带上附件；发布时提供经脱敏的结果与 hash 索引，原始敏感材料不随 Release 公开。

本记录不把中途编译/fixture 失败、旧快照或未执行测试算作通过，不声称访问生产、完成部署、验证所有供应商或覆盖任意数据库历史结构。
