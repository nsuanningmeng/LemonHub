# 上游 bug 审计修复交付 — 2026-10-07

本文保留最初 200 条上游 issue 的修复交付记录。后续整合 v0.4.45 及 MySQL 8.2 专项审查的版本、附加修复和验收范围，另见 [v0.4.46 数据安全审计](data-safety-audit-v0.4.46.md)；本文的 B21 冻结树不是后续发布的完整源码树。

初始审计的 **78 条确认 issue 已按 76 个独立单元闭环，分 21 批完成独立全面审查**：其中 **74 个代码修复类单元、2 个合同澄清单元**。#7672 与 #6978 属于后者，不能描述为新增“公开渠道实例 owner”或“串行续期”功能。代码修复类包括跨批已覆盖后经真实回归确认的单元，并不等于 74 处孤立补丁。

所有 21 批代码门禁已通过。已审生产源码冻结 tree 为 **`001da08ab912faa23b921faa0324a000e8865bc1`**；这是保存工作区内容的 Git tree，**不是新提交号或已部署版本**。本交付文档在冻结后新增，不改变该源码快照。**最终冻结版本的新镜像 MySQL 隔离演练及独立复核已通过**：35 个生命周期比较阶段、9 项实际原生 SQL 测试，在下述固定版本和测试数据范围内未发现业务数据丢失；这不代表任意生产升级零风险。

## 初始审计口径与计数

原始证据见 [2026-10-06 审计报告](upstream-bug-issues-audit-2026-10-06.md)。该报告保留审计时状态，本文件记录后续处置，不回写历史结论。

- 原仓库：`QuantumNous/new-api`；本项目：`nsuanningmeng/LemonHub`。
- 抓取于 2026-10-06 14:11:27（Asia/Shanghai）：精确 `bug` 标签、所有状态、按创建时间降序，排除 PR 后取 200 条；范围 #7672 至 #6649，open 76、closed 124。
- 本地初始 commit：`3aca293ad20ee0f158ac6ea4d401d6236e8fe6ba`。不以 issue closed 或某个上游提交是否在祖先链中代替实际源码核对。

| 初始判定 | issue 数量 | 本轮处置范围 |
| --- | ---: | --- |
| 确认存在对应问题或明确子问题 | 78 | 下表 76 个独立单元全部闭环。 |
| 疑似 | 4 | 保留原审计结论；不计为已修。 |
| 已修复／等效规避 | 51 | 初始已具备防护；不充作本轮新增修复。 |
| 不适用／非缺陷 | 44 | 不强行移植不同架构或产品诉求。 |
| 证据不足 | 23 | 不等同确认无 bug，也不计为已修。 |
| 合计 | 200 | 没有宣称 200 条全部端到端复现或全部需要修改。 |

重复组为 #6656/#6657、#7130/#7127，各计一个独立单元。76 单元＝74 代码修复类＋2 合同澄清；78 是 issue 条目数，不是根因数。实现中额外发现的备份字符集、PostgreSQL 索引顺序及 ClickHouse CI 就绪探测问题另记，不重复膨胀上述计数。

依据：[完整 backlog](/workspace/scratch/bugfix-2026-10-06/backlog.json)、[阶段记录 STATE](/workspace/scratch/bugfix-2026-10-06/STATE.md)、[最终处置独立核对](/workspace/scratch/bugfix-2026-10-06/final-disposition-validation.md)及其[机器可读记录](/workspace/scratch/bugfix-2026-10-06/final-disposition-validation.json)。本文件的 `/workspace/scratch/...` 链接指本次工作区实际验收附件，随仓库单独复制本文件不会自动携带这些附件；发布或归档时应一并保留附件和哈希清单。

## 逐单元结果

每行只描述本项目实际适用且已验收的范围。issue 链接指原报告；“证据”指相应批次的最终独立审查，其中保留真实 RED→GREEN、命令/日志及限制。早期构造失败、阶段中间失败和被替代 tree 不作为最终通过证据。

| 上游 issue（含别名） | 批次 | 最终行为／明确边界 | 证据 |
| --- | --- | --- | --- |
| [#7657](https://github.com/QuantumNous/new-api/issues/7657) | B01 | 正常退出先停止资金生产者并等待结算/退款，再排空批量记账；不确定提交不盲重放，排空/退款失败不会伪装干净退出。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch1-review.md) |
| [#7229](https://github.com/QuantumNous/new-api/issues/7229) | B01 | Gemini 缓存输入按模态拆分，缓存图片/音频不再同时作为 fresh 媒体重复计费；费用说明与实际扣款一致。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch1-review.md) |
| [#7409](https://github.com/QuantumNous/new-api/issues/7409) | B01 | Images generations/edits 与流式 usage 的图像输出明细进入 canonical completion details，表达式 img_o 可取到真实值。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch1-review.md) |
| [#6911](https://github.com/QuantumNous/new-api/issues/6911) | B02 | 允许钱包溢出时，最终费用按订阅可用额与钱包差额事务结算；持久 receipt 保证重入、退款与拆分日志一致。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch2-review.md) |
| [#7551](https://github.com/QuantumNous/new-api/issues/7551) | B02 | 新增默认关闭的输出前 refusal 免单选项；启用且完整证据成立时免单并保留原始 usage，歧义/重复字段或已有输出不误免。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch2-review.md) |
| [#7225](https://github.com/QuantumNous/new-api/issues/7225) | B02 | 传统渠道测试结算计入分组倍率并使用安全额度计算；真实钱包/订阅不因此被健康测试扣费。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch2-review.md) |
| [#7290](https://github.com/QuantumNous/new-api/issues/7290) | B03 | 新增可空总输入统计，缓存输入进入日志聚合、排行榜及 TPM；保留原始 fresh 计费值，旧日志 NULL 不猜算。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch3-review.md) |
| [#7394](https://github.com/QuantumNous/new-api/issues/7394) | B03 | 表达式编译结果判断正文依赖；无需 param 的表达式不读取/保留全正文，动态 $env/别名访问保守捕获。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch3-review.md) |
| [#6744](https://github.com/QuantumNous/new-api/issues/6744) | B03 | Linux 内存保护识别可见 cgroup v1/v2 限额和层级压力；无有效限额回退宿主，不承诺瞬时 OOM 或 CPU 配额保护。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch3-review.md) |
| [#7578](https://github.com/QuantumNous/new-api/issues/7578) | B04 | Responses 图片转 Chat 使用结构化 image_url/url/detail，避免将 URL 或图片块当普通文本。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch4-review.md) |
| [#7498](https://github.com/QuantumNous/new-api/issues/7498) | B04 | Responses 工具输出保留文本，将图片等媒体放到完整 tool 批次之后的 user 多模态消息，保持工具序列。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch4-review.md) |
| [#6822](https://github.com/QuantumNous/new-api/issues/6822) | B04 | Responses created_at 兼容有边界的数字时间戳，含浮点快照；快照与 usage 不因该字段被丢弃。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch4-review.md) |
| [#7454](https://github.com/QuantumNous/new-api/issues/7454) | B05 | Claude URL 图片、document/file 跨协议保留可表示媒体；无法解析的 provider 私有文件引用明确拒绝，不静默丢失。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch5-review.md) |
| [#7407](https://github.com/QuantumNous/new-api/issues/7407) | B05 | Claude tool_result 图片保持多模态并置于连续工具批次之后，不再把 Base64 序列化成正文。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch5-review.md) |
| [#7224](https://github.com/QuantumNous/new-api/issues/7224) | B05 | 解析与重建内容块保留 cache_control，覆盖实际原始 JSON 和 Claude system/user 转换路径。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch5-review.md) |
| [#7432](https://github.com/QuantumNous/new-api/issues/7432) | B06 | 敏感词扫描递归提取 Responses 工具输出文本；结构化媒体仍不作为 Base64 正文扫描。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch6-review.md) |
| [#7148](https://github.com/QuantumNous/new-api/issues/7148) | B06 | Claude 嵌套 tool_result 与 Responses compact 本地估算区分文本和媒体，避免内联图片被算成大量文本 token。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch6-review.md) |
| [#6656](https://github.com/QuantumNous/new-api/issues/6656) / [#6657](https://github.com/QuantumNous/new-api/issues/6657) | B06 | 策略拒绝返回稳定 4xx/错误码并隐藏命中词；保留管理员既有大小写/子串匹配规则，不擅自忽略短词。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch6-review.md) |
| [#7455](https://github.com/QuantumNous/new-api/issues/7455) | B07 | Gemini parametersJsonSchema 保留 const、additionalProperties、oneOf 等完整 schema；往返转换不静默削弱约束。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch7-review.md) |
| [#7456](https://github.com/QuantumNous/new-api/issues/7456) | B07 | 可等价的工具选择/并行/strict 等约束保真；跨协议无法承载时返回静态 typed 400/SkipRetry，实际预扣完整退款。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch7-review.md) |
| [#7564](https://github.com/QuantumNous/new-api/issues/7564) | B07 | 保留 Gemini 的既有 system/developer 合并字节；新增管理员可见的固定脱敏诊断，不声称 developer 权限层级等效。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch7-review.md) |
| [#7215](https://github.com/QuantumNous/new-api/issues/7215) | B08 | Claude effort 映射到 OpenAI reasoning_effort，保留源上下文与管理员最后覆写的既有优先级。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch8-review.md) |
| [#6715](https://github.com/QuantumNous/new-api/issues/6715) | B08 | Vertex Gemini 模式实际接通 Claude 请求/回程转换，覆盖流、非流、工具及 usage；不再仅让渠道测试表面成功。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch8-review.md) |
| [#7479](https://github.com/QuantumNous/new-api/issues/7479) | B08 | 高级自定义 Responses→Claude 选项同时接通宿主请求、响应和流生命周期；前端开放的是已验证真实路径。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch8-review.md) |
| [#7489](https://github.com/QuantumNous/new-api/issues/7489) | B09 | 客户端不请求 usage 时只剥离 usage，保留同帧 finish_reason、工具及其他有效协议内容。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch9-review.md) |
| [#7005](https://github.com/QuantumNous/new-api/issues/7005) | B09 | 合法当前 role/text/tool 帧即时转发，消除逐帧滞后；必要尾部 usage 处理保持原收费语义。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch9-review.md) |
| [#7399](https://github.com/QuantumNous/new-api/issues/7399) | B09 | 识别上游 JSON/SSE error；提交前保留正确错误/重试，提交后只报流错误且不重复生成或伪装正常成功。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch9-review.md) |
| [#7062](https://github.com/QuantumNous/new-api/issues/7062) | B10 | 真实下游取消与计费分开：已知 usage（包括零）优先，缺失时仅估算已观察输出；无输出取消退款，结算只发生一次。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch10-review.md) |
| [#7134](https://github.com/QuantumNous/new-api/issues/7134) | B10 | 确认下游取消且无已接受上游故障时，性能样本中性排除并保留审计；不误禁用渠道或将真正上游故障洗成取消。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch10-review.md) |
| [#6649](https://github.com/QuantumNous/new-api/issues/6649) | B10 | 已接受的合法协议终态优先于较晚的取消/扫描竞争，正常完成不再被覆盖为 client_gone。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch10-review.md) |
| [#7231](https://github.com/QuantumNous/new-api/issues/7231) | B11 | 普通及 multipart 上游 HTTP 继承真实请求生命周期；完整非流 usage 写出前取消仍按已知用量结算，未知部分 JSON 保留退款规则。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch11-review.md) |
| [#6860](https://github.com/QuantumNous/new-api/issues/6860) | B11 | Ollama 取消/写失败及时关闭并等待上游读取退出，保留已返回 done 帧和其零用量；不发送模型卸载或承诺供应商停止计费。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch11-review.md) |
| [#7385](https://github.com/QuantumNous/new-api/issues/7385) | B11 | stream_format=audio 按块转发和 flush 原始二进制；保留 native TTS 计费，压缩时长解析有内存/临时文件上限。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch11-review.md) |
| [#7130](https://github.com/QuantumNous/new-api/issues/7130) / [#7127](https://github.com/QuantumNous/new-api/issues/7127) | B12 | 以前批已修的实际首个合法业务帧 Write/Flush 为提交边界，并在本批复验；不在错误判定前仅为降低 TTFT 提交 HTTP 200。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch12-review.md) |
| [#6981](https://github.com/QuantumNous/new-api/issues/6981) | B12 | 数据已提交后允许上游 SSE 注释与定时保活；共享串行写入，取消并清理定时器，不提前提交或拼接并发帧。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch12-review.md) |
| [#7059](https://github.com/QuantumNous/new-api/issues/7059) | B12 | 已提交 Responses 在异常中断时发唯一协议失败终态并保留故障事实；不伪造 completed，也不重试已输出内容。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch12-review.md) |
| [#7495](https://github.com/QuantumNous/new-api/issues/7495) | B13 | Chat→Responses 严格保护单响应生命周期；finish 后生成、重复冲突终态等无效序列明确失败，不向已关闭 item 发 delta。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch13-review.md) |
| [#7094](https://github.com/QuantumNous/new-api/issues/7094) | B13 | 工具名/ID 完整后才发布流式 function_call，缓存并补发参数；非流空白名不写入历史，保留实际 usage。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch13-review.md) |
| [#6682](https://github.com/QuantumNous/new-api/issues/6682) | B13 | Responses reasoning token 明细放在正确 output_tokens_details，合法零保留；不为该报告盲加 [DONE] 或承诺修复其所有 EOF 主张。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch13-review.md) |
| [#6864](https://github.com/QuantumNous/new-api/issues/6864) | B14 | 任务提交时私有持久化实际选中密钥，轮询、分组抓取及内容代理共用解析；历史多密钥身份不明时拒绝猜选。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch14-review.md) |
| [#7591](https://github.com/QuantumNous/new-api/issues/7591) | B14 | 提交及差额/退款日志从任务快照保留原 multi_key_index（包括 0），仅管理员可见；密钥不进入公开任务或用户日志。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch14-review.md) |
| [#7040](https://github.com/QuantumNous/new-api/issues/7040) | B14 | 内部恢复探测可逐个测试自动禁用 key，成功后按最新身份事务恢复；手动禁用、重复歧义或已替换 key 不被误恢复。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch14-review.md) |
| [#6840](https://github.com/QuantumNous/new-api/issues/6840) | B15 | Redis 模式共享带 TTL 的验证码并原子比较消费，错误码不消费、正确仅一次；故障不退回本地，站点/用途/邮箱隔离。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch15-review.md) |
| [#7180](https://github.com/QuantumNous/new-api/issues/7180) | B15 | 创建、编辑及启用令牌验证实际用户可选分组（含多分组）；既有 relay 使用权限继续生效，不声称原先已能越权调用。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch15-review.md) |
| [#6661](https://github.com/QuantumNous/new-api/issues/6661) | B15 | 后端按单一 now 派生 effective_status 并筛选/分页/计数，保持存储启停状态；NULL、到期边界和旧状态有实际三数据库控制。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch15-review.md) |
| [#6995](https://github.com/QuantumNous/new-api/issues/6995) | B16 | Ollama 按 RelayFormat 返回 Claude text/thinking/tool/usage 和合法终态；Claude 无 [DONE]，EOF/错误不强行 message_stop。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch16-review.md) |
| [#7348](https://github.com/QuantumNous/new-api/issues/7348) | B16 | 显式 JSON 透传+参数覆写使用既有 raw override 引擎；无覆写保原始字节，重试重置运行时头/审计，multipart 维持原路径。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch16-review.md) |
| [#7475](https://github.com/QuantumNous/new-api/issues/7475) | B17 | Ali 每个 image 独立按顺序返回，共用该 choice 最后非空 text-only prompt；choices 下载失败整单静态 502/不重试/退款。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch17-review.md) |
| [#7540](https://github.com/QuantumNous/new-api/issues/7540) | B17 | 已知类型且定额估算的媒体不再无谓下载；真正需要 MIME/尺寸时仍走既有 SSRF、重定向和请求内缓存保护。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch17-review.md) |
| [#7268](https://github.com/QuantumNous/new-api/issues/7268) | B18 | 完整严格解析后才显示条件价格，保留时区、路径条件及 else 互补；未知/非线性/不安全表达式整体回退原式。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch18-review.md) |
| [#7011](https://github.com/QuantumNous/new-api/issues/7011) | B18 | 前端估算补齐五个时间函数并使用同一时间快照；对无法证明与 Go 一致的时区显示不可估算，后端计费时钟不变。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch18-review.md) |
| [#7392](https://github.com/QuantumNous/new-api/issues/7392) | B18 | 仅可证明的固定线性/常量表达式显示 token/request 标签；未知保持动态，分类不改变 tiered_expr 路由或收费公式。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch18-review.md) |
| [#7599](https://github.com/QuantumNous/new-api/issues/7599) | B19 | 充值账单 Success/Pending/Expired 在渲染时翻译，切换语言即时更新。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch19-review.md) |
| [#7584](https://github.com/QuantumNous/new-api/issues/7584) | B19 | 使用完整插值文案并区分 Usage tokens 与 API 令牌；保留金额/折扣单位，状态与 Enable/Disable 动作分开；Inviter 既有翻译只补完整句。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch19-review.md) |
| [#7576](https://github.com/QuantumNous/new-api/issues/7576) | B19 | 删除确认用完整 Trans 句子，用户名作为加粗 React 文本而非可解释 markup；精确确认及提交条件不变。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch19-review.md) |
| [#7574](https://github.com/QuantumNous/new-api/issues/7574) | B19 | 双因素步骤使用完整 current/total/label 句子，保留原安全状态机与接口。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch19-review.md) |
| [#7573](https://github.com/QuantumNous/new-api/issues/7573) | B19 | 余额不足解释按语言展示并保留完整旧中文诊断用于兼容识别；错误码/状态/SkipRetry、原因链和资金规则不变。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch19-review.md) |
| [#7585](https://github.com/QuantumNous/new-api/issues/7585) | B19 | font-sans 引用实际已打包的 Public Sans Variable，保留 fallback，无新增远程字体。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch19-review.md) |
| [#7417](https://github.com/QuantumNous/new-api/issues/7417) | B19 | 汇率输入允许既有精度对应的 0.0001 步长，6.7081 可输入保存，正值校验保留。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch19-review.md) |
| [#7363](https://github.com/QuantumNous/new-api/issues/7363) | B19 | codex-* 在用量日志显示 OpenAI 图标，只改展示分类。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch19-review.md) |
| [#7666](https://github.com/QuantumNous/new-api/issues/7666) | B20 | 分流图 token_id=0 显示“无 API 令牌”，不一概冒称渠道测试；身份与统计不变。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch20-review.md) |
| [#7580](https://github.com/QuantumNous/new-api/issues/7580) | B20 | 完整 bucket 时间戳作为图表身份并按数值排序，标签单独展示；跨年同 MM-DD 不合并，DST 重复小时保留。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch20-review.md) |
| [#7393](https://github.com/QuantumNous/new-api/issues/7393) | B20 | 移除不足七点的伪造补点，保留每个真实桶和总和；空图/单点有效，不把一天 week 查询造为六周历史。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch20-review.md) |
| [#7222](https://github.com/QuantumNous/new-api/issues/7222) | B20 | 模型广场实际筛选结果/分组改变时重置分页状态，恢复正确上一页与空结果行为。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch20-review.md) |
| [#6885](https://github.com/QuantumNous/new-api/issues/6885) | B20 | 分组批量操作只收集已选且可选的真实子行 ID，去重并排除聚合父行；数量和提交使用同一列表。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch20-review.md) |
| [#6804](https://github.com/QuantumNous/new-api/issues/6804) | B20 | Auto 分组始终有 Auto 标识，Cross-group 仅在启用跨组重试时显示，徽章允许换行。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch20-review.md) |
| [#6957](https://github.com/QuantumNous/new-api/issues/6957) | B20 | 个人侧栏选项受管理员范围约束，保留用户隐藏偏好与运行时 AND 权限检查。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch20-review.md) |
| [#6681](https://github.com/QuantumNous/new-api/issues/6681) | B20 | 兑换码额度允许空字符串编辑态，提交时按原零值/数值规则转换，不在每次退格时强制写回 0。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch20-review.md) |
| [#7634](https://github.com/QuantumNous/new-api/issues/7634) | B21 | 资源失败显示专用提示；每 tab/session+哈希 build 最多一次自动刷新，storage 失败保持人工重试，真实 HTTP 500/401 不循环。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch21-review.md) |
| [#7123](https://github.com/QuantumNous/new-api/issues/7123) | B21 | Home/About 有限重发并接受当前 frame 的合法 ready，仅发送 theme/lang；保留 opaque sandbox，变化/卸载清理旧任务。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch21-review.md) |
| [#6745](https://github.com/QuantumNous/new-api/issues/6745) | B21 | CCSwitch 用当前令牌实际 /v1/models 授权结果，隔离身份/分组/模型限制并丢弃过期响应与选择，不用用户全模型列表代替。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch21-review.md) |
| [#6688](https://github.com/QuantumNous/new-api/issues/6688) | B21 | 提供 Admin 可读的最小支付合规能力接口；真实失败独立呈现，Root 配置仍受保护，普通用户被拒绝。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch21-review.md) |
| [#7177](https://github.com/QuantumNous/new-api/issues/7177) | B21 | 渠道测试的 rerank 检测优先于 embedding，保留显式 endpoint；真实 HTTP 验证路径、DTO 和响应格式。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch21-review.md) |
| [#7672](https://github.com/QuantumNous/new-api/issues/7672) | B21 | 合同澄清：公开 owned_by 继续代表 provider/type；文档解释 advanced_custom，不暴露私有 channel.Name，也未实现实例 owner 功能。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch21-review.md) |
| [#6978](https://github.com/QuantumNous/new-api/issues/6978) | B21 | 合同/界面澄清：购买立即生效、独立额度、周期可重叠，并提示现有同套餐期限；未实现串行续期，不改历史订单或余额。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch21-review.md) |
| [#7175](https://github.com/QuantumNous/new-api/issues/7175) | B21 | Claude 非空 stop_sequences 转 Chat 始终为数组，保序；空/null 省略，原生 OpenAI 合法 string 继续支持。 | [审查](/workspace/scratch/bugfix-2026-10-06/batch21-review.md) |

## 逐批门禁和冻结身份

B01–B18 为每批 2–3 个大单元，B19–B21 为每批 8 个小单元。每批完成实现、适用测试/构建和独立全面审查后才开放下一批；未来批次只读准备不替代门禁。下表所有状态均为最终 **PASS**，哈希标识该批实际审查快照，不代表各批创建了 commit。全部 21 批均有实际 checks 记录；B05–B07 同时附上独立冻结 manifest。

| 批次 | 独立单元数 | 已审 tree | 最终门禁记录 |
| --- | ---: | --- | --- |
| B01 | 3 | `14db548c4108c8ddefa1a83b30698fbd6a78b50d` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch1-checks.json) |
| B02 | 3 | `646d9c80cf8220ad3a22cbf47f2f9d0ed35311d3` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch2-checks.json) |
| B03 | 3 | `4aec2e7571541b884d8f5279550b870ca64a06a3` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch3-checks.json) |
| B04 | 3 | `60cc9705154aa7bece6120a74edc5ca3c500ea12` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch4-checks.json) |
| B05 | 3 | `bef16b44c2419a86f86d41f61d66e7fe2cb0121c` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch5-checks.json) · [manifest](/workspace/scratch/bugfix-2026-10-06/batch5-final-review-manifest.json) |
| B06 | 3 | `1ea03e180a8438809bc65ccd8da6642a6e6b50ed` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch6-checks.json) · [manifest](/workspace/scratch/bugfix-2026-10-06/batch6-final-review-manifest.json) |
| B07 | 3 | `897f795d233a630ae226852538fc3d0e3396b122` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch7-checks.json) · [manifest](/workspace/scratch/bugfix-2026-10-06/batch7-final-review-manifest.json) |
| B08 | 3 | `e026aa0c047b5015f854d8feed51bd416e5ef04b` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch8-checks.json) |
| B09 | 3 | `c3900dd399bd7cd3d27368f8fb9fe3f23b2a558a` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch9-checks.json) |
| B10 | 3 | `066b62451116fefe95cbe0b8aabf17021ecbdc31` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch10-checks.json) |
| B11 | 3 | `35ce868e2d6792b40a04b653c1c3e1324f7974f8` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch11-checks.json) |
| B12 | 3 | `ee6a771cdfcad43fb448e403fb1f1856176c57bf` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch12-checks.json) |
| B13 | 3 | `2c8fafbb308dcf30fc119270ca5165f0c5f2ed9f` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch13-checks.json) |
| B14 | 3 | `0ed8aa46c81daefe66dc8defe5f2177733332559` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch14-checks.json) |
| B15 | 3 | `b60832bb8482c67b61dd08765b2528d1d406d78e` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch15-checks.json) |
| B16 | 2 | `6684751b3f9c6367b8c7744c22831fc1e7f32670` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch16-checks.json) |
| B17 | 2 | `3bc824a7c6fa62371f14afd8e558cc4b67b94808` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch17-checks.json) |
| B18 | 3 | `9135f78b630c0145f17384d790bc575ee29f48f2` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch18-checks.json) |
| B19 | 8 | `fe71fa16747df23cc36eefea3e3df6226fe03614` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch19-checks.json) |
| B20 | 8 | `bba5c37b7827a519f90da7e5dd2970b4f2657f03` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch20-checks.json) |
| B21 | 8 | `001da08ab912faa23b921faa0324a000e8865bc1` | [PASS](/workspace/scratch/bugfix-2026-10-06/batch21-checks.json) |

最终 B21 门禁再次确认根模块全量 Go 测试及构建、独立 `relaykit` 的 `GOWORK=off` 全量测试及构建，以及 Web 类型检查、涉及 TS/TSX 文件 lint 和生产构建均实际退出 0。适用批次另有并发/race、真实 HTTP/隔离 SQL、实际浏览器或 Go/Intl 时区对照。未把不同批次重复执行的测试简单累加成一个“总通过数”，也未把局部单测描述为所有数据库或供应商端到端保证。

最终集中证据：[B21 checks](/workspace/scratch/bugfix-2026-10-06/batch21-checks.json)、[全面审查](/workspace/scratch/bugfix-2026-10-06/batch21-review.md)、[独立安全/语义审查](/workspace/scratch/bugfix-2026-10-06/batch21-security-semantics-review.md)、[逐文件 tree/archive/blob/SHA 对照](/workspace/scratch/bugfix-2026-10-06/batch21-review-independent-manifest.json)。lint 无 error；记录的一条非阻断 `prefer-template` warning 未冒充为零 warning。

## 资金、协议与权限约束

- **资金**：退出排空与订阅事务 receipt 防止静默丢账和重复结算；不确定提交不自动重放非幂等信用。费用使用既有安全舍入/饱和规则。取消不是一律免费：已接受终态和权威零 usage 优先，未知 usage 只估算可观察输出；各端点既有 fault/refund 合同不被统一改写。完整原始 usage 与收费统计分开。
- **供应商边界**：Claude 输出前 refusal 免单默认关闭，必须显式启用且证据充分。新 Ollama→Claude 非流要求实际 done，provider error/缺 done 为 typed 502；native Chat 的旧非流/非 context 故障合同保留。Ali 新 choices 下载失败全单退款，但旧 output.results 部分返回/收费合同未宣称修好；成功时 provider image_count 仍是既有收费依据。
- **协议**：不能表示的工具约束明确拒绝并退款；不能伪造供应商不具有的 developer 层级。SSE 以真正业务 Write/Flush 为提交边界；流已提交后的故障按对应协议发送合法失败事件或关闭连接，不伪造成功终态、二次生成或重复退款；保留 B16 原生流非 context 故障仅关闭连接的既有范围。取消上游 HTTP 不等于保证其模型计算或供应商收费停止。
- **权限与隐私**：多密钥任务按提交身份绑定，密钥快照私有持久化，审计仅含管理员可见的多密钥标记/原索引，原始密钥不写入任何日志；自动恢复不覆盖手工禁用。令牌分组由真实用户服务端策略约束。Admin 只得到最小合规能力信息，不开放 Root 系统选项。公开 owned_by 不泄露私有渠道名，诊断不带 prompt/key/URL 秘密。
- **验证码**：集群所有节点必须启用并连接同一 Redis；Redis 故障 fail closed，无本地 fallback。关闭 Redis 仅保证单节点原子消费。邮件发送失败只撤销本次码；成功验证后即消费，后续业务失败需要重发，旧内存码不迁移。详见 [认证部署说明](authentication.md)。
- **表达式与 UI**：展示分类不改变 billing_mode、路由或后端求值；不可完整证明的公式整体显示原式。浏览器时区数据无法匹配 Go 时明确不可估算，不虚报 UTC 价格。国际化保留金额单位、删除精确确认、2FA 流程和旧机器可识别错误片段。
- **浏览器恢复**：资源自动刷新按 session/build 有界，storage 故障只允许人工恢复。iframe 保留原 sandbox 权限，仅对当前 opaque frame 的精确 source/null-origin 接受 ready，发送的只有公开 theme/lang；有限重发不承诺任意第三方永远协作。

## MySQL 最终新镜像演练

**最终结果：实际执行、独立原始快照复核和本轮资源清理均 PASS。** 本次重新编译了冻结 tree `001da08ab912faa23b921faa0324a000e8865bc1` 的应用并构建新镜像；结论来自最终版本的实际运行，不复用 B01–B04 历史通过结果冒充最终验收。[最终报告](/workspace/scratch/final-mysql-summary.md)、[机器可读证据索引](/workspace/scratch/final-mysql-summary.json)和[独立最终审查](/workspace/scratch/final-mysql-runner/independent-final-mysql-review.md)保留全部身份、原始比较、命令退出状态和限制。

| 实际路径 | 服务器版本 | 生命周期比较阶段 | 实际原生 SQL 测试 | 独立复核／清理 |
| --- | --- | ---: | ---: | --- |
| 同版本应用升级、容器重建、新卷恢复 | MySQL 5.7.44 | 9 | 3 | [PASS](/workspace/scratch/final-mysql-results/b21final001dad/57-independent-lane-review.json)／exit 0 |
| 同版本应用升级、容器重建、新卷恢复 | MySQL 8.0.46 | 9 | 3 | [PASS](/workspace/scratch/final-mysql-results/b21final001dad/80-independent-lane-review.json)／exit 0 |
| 同版本应用升级、容器重建、新卷恢复 | MySQL 8.4.11 | 9 | 3 | [PASS](/workspace/scratch/final-mysql-results/b21final001dad/84-independent-lane-review.json)／exit 0 |
| 新隔离数据卷上的物理引擎升级 | 5.7.44 → 8.0.46 → 8.4.11 | 8 | 不重复计数 | [PASS](/workspace/scratch/final-mysql-results/b21physical001da/independent-physical-review.json)／[清理 exit 0](/workspace/scratch/final-mysql-results/b21physical001da/physical-cleanup-result.json) |
| 合计 | 固定上述三个版本 | 35 | 9 | 独立重算不另加阶段数 |

每条同版本路径包含原基线恢复等价性、旧应用启动、新应用迁移、应用重建、数据库同卷重建、重建后应用启动、dump 恢复到另一新卷、恢复后应用启动、原生测试后保存库再比较。原生测试在三个分别新建的受保护空 schema 中执行令牌分组升级、旧 receipt 迁移和订阅事务结算，均有真实 PASS、无 SKIP；保存数据的 schema 未交给这些测试。

物理路径从已通过审查的最终 5.7 dump 恢复到另一新卷，实际执行 5.7 恢复、启动及重启，随后经过 8.0 升级/启动/认证检查与 8.4 升级/启动，共 8 次原始快照检查。八个完整 server header 元组与预审版本、SQL mode、时区、event scheduler OFF、字符集和排序规则精确一致。在 8.0 仅将三个本轮测试账号 `root@localhost`、`root@%`、`audit@%` 切换到 `caching_sha2_password`，逐个核对后进行了真实 TCP 连接快照；没有启用旧认证插件绕过 8.4，也没有直接 5.7→8.4 或物理降级。

### 数据保留范围与比较规则

最终样本为 **46 张应用表、26 张非空非运行态表、45 条非运行态记录**（含 3 条 Casbin 策略，去除策略为 42 条），其中 11 条为新增保留哨兵；不是压力数据集。先证明旧 schema/原业务数据等价，再加入哨兵。检查逐行内容、重复行数量、NULL/零值差异、字节、列定义、完整索引和 catalog；覆盖中文/emoji、50 亿额度或输入统计、receipt 差额/负值、旧令牌 NULL 状态、任务原始密钥元数据/合法 index 0 和已完成账本。

应用迁移仅允许预先列明的 **11 个 receipt 字段及 1 个可空总输入字段**。窄运行态例外是 `system_instances` 运行记录及预先确认的内置策略 surrogate ID；策略内容及其他业务行仍精确比较。原/新应用都存在的初始化操作只允许本样本 `authz_roles`、`external_identity_claims` 自增计数分别增加 2、3；仅实际 MySQL 5.7.44 重启时允许这两个计数精确重建为 `MAX(id)+1`。其他计数保持严格。物理 5.7→8.0 只允许已证明的三种整数显示宽度变化，8.0→8.4 不放宽类型；索引排序不改变键顺序或重复数量。没有接受业务行丢失、NULL 改成零或任意 JSON 重写作为例外。

### 镜像和源码身份

| 产物 | 实际身份 |
| --- | --- |
| 最终新应用 image ID | `sha256:64ed1bf8eab061ba9c3924172ac8e76e6b819041ad502dcac07e1edf2f57e68b` |
| 从最终镜像提取的应用二进制 SHA-256 | `4b4736c24668940b5b358af4a59569764c0c84c3673d913fb78c5a13df7c642a` |
| 最终源码 manifest SHA-256 | `903b8e2aaa21a63af0429720b8393e6034d0bb8b4f4620157fae570721c0ae57` |
| 最终前端 manifest SHA-256 | `ec24512fdb694e8dd1d3b4cab59b9eb358f90075e163050e56a285e334a5a3fa` |
| 固定运行基础镜像 | `ghcr.io/nsuanningmeng/lemonhub:v0.4.44@sha256:a920239e261aa3f267b3874bc67d9923ce6bb3782ff65d94a31720f23a78a68a` |

[构建 provenance](/workspace/scratch/final-mysql-builds/b21final001da-retry1/provenance.json)、[源码 manifest](/workspace/scratch/final-mysql-runner/final-source-manifest.json)及[前端 manifest](/workspace/scratch/final-mysql-runner/final-web-manifest.json)绑定源码、241 个前端产物和从镜像取出的二进制。三个 MySQL 引擎的不可变 image ID、dump、工具、配置及每阶段哈希见[实际运行配置](/workspace/scratch/final-mysql-runner/run-config.sameengine-phase-final.json)和最终证据索引。旧应用是由原 commit `3aca293ad20ee0f158ac6ea4d401d6236e8fe6ba` 重建的源码对照，未声称等同某个不可取得的原部署二进制。

### 失败记录、清理和限制

早期尝试均原样保留：构建包装器先因空 VERSION 拒绝；随后三轮本次新建资源分别因 internal network 未发布端口、选中旧失败 utf8 dump 已将 emoji 变成问号、严格 catalog 检查发现既有初始化/5.7 重启计数行为而停止。修正仅涉及演练包装器、输入备份选择及经独立证明的精确阶段规则；正确 seed 来自已经验证的 utf8mb4 历史备份，没有修改业务数据来迎合比较，也未把这些失败归咎于新生产源码造成的数据损失。

成功及失败尝试的新建资源均按身份、标签和引用关系审核后精确清理，实际 release exit 0，最终资源登记为空；dump、受保护快照、日志和哈希证据保留。原数据库/服务、原数据卷及旧审计卷未被访问或升级。

这些结论适用于**固定镜像、清洁停机、隔离的 46 表样本和指定原生事务测试**，不证明生产规模性能、断电/崩溃恢复、任意历史版本兼容、物理降级安全或所有用户数据都零风险，也不把样本备份称为完整生产灾备。实际操作仍须遵循 [MySQL 容器升级安全说明](mysql-container-upgrade-safety.md)：持久卷不是可丢弃容器层，先备份并在隔离实例演练恢复，5.7 dump/restore 显式 `utf8mb4`；恢复参数无法挽回导出阶段已经损坏的字符。本次验证不授权直接升级真实生产卷。

## 额外发现与交付限制

已在对应批次修复且不计入 78/76 的附带问题：B01 备份/恢复字符集导致 emoji 损坏；B02 PostgreSQL 索引检查依赖 GORM 返回的错误列序，改用 catalog 明确顺序；B04 ClickHouse CI 就绪探测避免命中仅本机可达的临时启动服务。证据分别见 [B01 数据库记录](/workspace/scratch/bugfix-2026-10-06/mysql-baseline.md)、[B02 全面审查](/workspace/scratch/bugfix-2026-10-06/batch2-review.md)、[B04 数据库补充审查](/workspace/scratch/bugfix-2026-10-06/review-batch4-db-supplements.md)。

验收主要使用记录式本地供应商、隔离数据库、实际 HTTP/浏览器和明确故障注入；没有声称调用全部真实付费模型、支付平台或全部历史时区。初始疑似/证据不足条目保持原分类；#6978 不改变并行购买，#7672 不公开渠道私名。本文记录本地已审修复交付，不表示已提交、合并、发布或部署。
