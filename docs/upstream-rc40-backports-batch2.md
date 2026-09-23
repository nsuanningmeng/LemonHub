# rc.40 范围第二批兼容移植

日期：2026-09-23。基线：`c5db99d035f790ae92ffa10bef8145493ee10d34`。
本批按三组行为移植四个上游提交；不整体合并 rc.40。
首批兼容记录见 [第一批](upstream-rc40-backports-batch1.md)。

## 来源与设计决策

| 上游提交 | 本地适配 |
| --- | --- |
| `2906e4f779b715f282ae11203211dca77051d5af` | GitHub 旧用户名命中不再自动改绑数字 ID 或登录；提示通过已有方式登录后重新绑定。已有会话重绑只核验永久 subject，继续使用本项目子站身份事务。 |
| `55a6cd2a44eaf189bbd565dd2cdfa591d19b06d6` | 在现有 HTTP Responses handler 中增加生成用量累计，不引入 WebSocket 架构。缺 usage 时覆盖文本、函数参数、推理摘要、推理文本、拒绝内容与完成输出快照。 |
| `d3874db61fce61b6c0f5c37523b21912a3f2658d`、`6237d9d77eb0544329b88ff96d71c1715af4268f` | 内存限流按实际接受请求分配，成功额度采用并发预留及结果确认，失败释放，0 禁用成功上限；不移植请求策略重构。 |

### GitHub 身份归属

- GitHub 用户名可被重新使用，公共邮箱也不能证明旧 LemonHub 账号归属。非数字旧用户名命中只返回固定重绑引导，不读取该用户作为已认证主体、不改写绑定、不创建登录会话。
- 纯数字 login 不作为旧用户名匹配依据；现有永久数字 subject 登录仍按 `site_id` 隔离。
- 已登录用户重绑的站点来自认证账号，不能被请求 Host 改变。一次性 OAuth flow、session 校验以及 `UpdateUserBindColumn` 的 `users` / `external_identity_claims` 同事务更新保持。
- 删除不再调用的 `UpdateGitHubId` 迁移入口；不引入上游全局查重、绕过 claim 的直接更新或新账号验证协议。
- **历史边界**：旧 schema 没有记录 `github_id` 是旧用户名还是永久数字 ID。历史纯数字用户名恰好等于另一个 GitHub 账号永久 ID 时，仅凭现有字符串无法区分；本批没有批量猜测、迁移或封禁这些绑定。数字用户名测试验证“不作为旧用户名迁移证据”，不代表已消除同值历史歧义。相关历史账号应按可靠归属证据核验后，通过已有登录方式重新绑定。

### Responses 用量与计费

- terminal response 中存在 usage 即优先使用真实值，包括输出为 0 或全 0；完整保留缓存读写、输入/输出明细及内部 BillingUsage 快照。
- 仅收到 created/in_progress、空 completed、空流或没有生成证据的明确失败，不补估 prompt token。
- 缺 usage 且已观察到生成文本、工具参数或已完成工具调用时，累计可观察输出并补估请求 prompt。item-done 与 terminal 快照不和相同输出的 delta 重复相加。
- 已观察到生成后中断或显式失败，可以结算已发生的生成；错误标记和收费分别判断。估算无法恢复未发送的隐藏推理、丢失片段或精确供应商账单。
- 使用原预扣、结算、退款及饱和额度链，不新增第二次结算或重试。保留公开模型名、工具调用收费、token 归属和子站 markup。图片调用原计费规则不变。
- 成功限流定义为 HTTP `<400` 且没有已知协议或传输错误；`incomplete`、`cancelled` 或正常 EOF 本身不等同失败。明确 Responses failure、扫描/解析异常、超时等会释放成功预留；缺少 completed 本身不改变判定。

### 内存限流

- 单个请求不会按管理员配置的大上限一次分配切片；队列只保存已接受请求，LRU 用于清理空闲 key。
- 准入时原子检查窗口内成功数与在途预留；成功在完成时计入窗口，失败及 panic 路径幂等释放。
- 在途预留不因空闲 bucket 清理而消失，清理不会提前丢弃比清理周期更长的有效窗口。
- 最终 relay 错误即便已提交 HTTP 200 也不算成功。每次上游尝试重置错误状态，避免失败尝试污染最终成功。
- Redis 路径共享最终结果分类，并修正总量拒绝后的提前返回；**本批没有完成 Redis 成功额度的并发 reservation，不能声称 Redis 与内存并发保证一致**。

## 数据库与安全范围

- 无生产数据库字段、索引、启动迁移或依赖版本变更；不重写历史余额、价格、账单或用户绑定。
- 保留 SQLite / MySQL / PostgreSQL 的既有 GORM 实现与事务边界；本批未运行 PostgreSQL 或 MariaDB 实机验收。
- 首批 Gemini usage-only 尾帧、新 GPT pricing presets、额度饱和审计及 token 64 位余额规则继续保留。
- 不引入实验性插件、Responses WebSocket、Passkey 迁移、新请求重试策略、降低预扣下限、表达式架构重构或视频/音乐业务变化。

## 验证

- 三组均先用确定性回归复现问题，再验证修复；覆盖旧用户名重用/数字字符串/既有数字绑定/子站归属/错误隐私、真实零 usage/输出去重/失败后生成、限流失败释放/0 禁用/两请求竞争/清理与幂等。
- 根模块全量 `go test -mod=readonly ./...`、`go vet ./...`、`go build ./...` 通过；relaykit 在 `GOWORK=off` 下同样三项全通过，检查前后 Go 源文件哈希一致。
- 前端未改动；Bun 冻结依赖安装、49 个文件共 317 项测试（`--maxWorkers=2`）、类型检查和生产构建通过。
- MySQL **5.7.44 / 8.0.44** 各自通过 migration、binding、channel、billing、github 五套实机测试；所有 suite 与 shutdown 退出码为 0。
- 迁移覆盖旧 rc25 全字段/子站财务快照、重复升级、Token INT→BIGINT、充值状态及身份排序规则冲突预检。5.7 按数据库能力跳过“不相关函数索引”子例，该子例在 8.0 实测通过。
- 身份实机覆盖 legacy 拒绝、两子站同数字 subject、已有会话重绑、claim/users 主动失败回滚与竞争重绑；渠道验证持久化全字段和密钥不变。
- 新账单实机覆盖函数参数/推理中断流、空流、仅 created、真实全零、实际 cache write；验证预扣差额退还、真实缓存案例 51 quota、50 亿 token 余额、40 亿已用量、重复结算幂等、token 更新失败后的资金差额补偿、原子钱包流水失败回滚。请求 Host 不改变认证用户子站的 1.5 倍 markup，两个子站进货钱包不受 relay 结算影响。
- 独立交叉审查未发现本批新增阻断。全仓规则扫描的 7 critical / 8 high 命中与首批相同，经人工逐项复核为固定/测试 SQL、PEM 标记、测试凭据、验证占位值或已清洗 HTML；没有从这些命中确认可利用漏洞。规则扫描不等同渗透测试或完整供应链审计。
- 本机未运行 race detector；并发测试使用同步屏障和明确状态断言，没有随机压力或睡眠判断。

本地证据根目录：
`C:/Users/wangz/.codex/visualizations/2026/09/23/01a0cd6e-715a-78a0-a92b-b520d20b5be9/sep23-batch2-validation/`。
最终 MySQL 证据在 `mysql-attempt2/`。首轮账单夹具漏设 `StreamingTimeout` 而中止，修复的是测试初始化；首轮原日志完整保留。复验使用另一组全新 UUID datadir，脚本核对 `@@datadir`、拒绝已占端口和已有 fixture 库、仅监听 loopback，并在 finally 关闭自己的实例；未连接生产库。
