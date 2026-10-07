# 上游最新 200 个 bug issue 对照审计 — 2026-10-06

已完整核对 200 个 issue 的正文、评论与本地相关源码：**确认存在对应问题或部分缺陷 78 条，疑似 4 条，已修复／等效规避 51 条，不适用／非缺陷 44 条，证据不足 23 条**。这里按 issue 计数；同根因重复、部分命中及受特定供应商限制的条目均保留范围说明，不能理解为 78 个独立根因。

## 范围与方法

- 本项目：[nsuanningmeng/LemonHub](https://github.com/nsuanningmeng/LemonHub)，本地 `work` 分支 `3aca293ad20ee0f158ac6ea4d401d6236e8fe6ba`；只读核对远端 `main` 为同一提交。
- 原仓库由 README 与 GitHub fork 元数据共同确认：[QuantumNous/new-api](https://github.com/QuantumNous/new-api)，不是更早的 one-api。
- 抓取时间：**2026-10-06 14:11:27（Asia/Shanghai）**。口径为精确标签 `bug`、全部状态、按 **created 降序**、排除 PR 后取最新 200 条；不是按更新时间，也不是只查 open。
- 范围：#7672（2026-10-05 20:27）至 #6649（2026-08-04 20:15），均为北京时间；上游 open 76 条、closed 124 条。
- 6 个审查 agent 分批并行，主 agent 复核高风险路径、数据覆盖和全部源码路径／行号。结合本地 rc.37/rc.40/rc.41 回移记录查验实际源码，不以 commit 不在祖先链或上游 closed 状态判定是否修复。
- **35 条确认项有本地函数、handler、计费函数或前端函数行为探针**；部分只验证 issue 中某个子问题。其余确认项以静态调用链为证据。没有进行 200 条端到端复现，也没有真实付费模型或生产支付调用。
- Go 使用现有 1.26.8 工具链、隔离数据和 overlay 注入测试；前端使用现有依赖。部分测试用正确预期断言失败来复现缺陷，部分探针断言当前错误行为而通过；二者均不是修复验收。
- 本次只审计并保存报告，未修改业务源码、未提交 commit、未向上游发布 issue 或评论。

## 分类解释

| 判定 | 数量 | 含义 |
| --- | ---: | --- |
| 确认存在 | 78 | 已找到对应本地缺陷或报告中的明确子问题；以逐项范围为准。 |
| 疑似 | 4 | 本地有相关路径，但触发条件或因果链尚不足以确认。 |
| 已修复／等效规避 | 51 | 本地已修复、已有替代实现或明确阻断错误路径；不保证相关完整新功能均已引入。 |
| 不适用／非缺陷 | 44 | 未引入对应架构、配置／第三方行为、功能需求、设计偏好等，不能当成本项目同 bug。 |
| 证据不足 | 23 | 原报告或运行证据不足，或本地未复现；不代表不存在。 |

优先级是本次影响评估：P1 优先处理资金、资源、请求内容和关键协议问题；P2 常规功能／可靠性；P3 展示和文案。仅确认项和疑似项的优先级用于待办排序。

## 建议优先处理

| Issue | 影响与触发条件 | 验证 | 关键代码 |
| --- | --- | --- | --- |
| [7657](https://github.com/QuantumNous/new-api/issues/7657) | 启用批量记账后正常停止未做最终排空，尾部余额、退款及用量增量可能丢失。 | 静态退出链确认 | [main.go:243](/workspace/LemonHub/main.go:243) |
| [6911](https://github.com/QuantumNous/new-api/issues/6911) | 订阅允许钱包溢出：预扣 60,000、实际 160,000、订阅总额 100,000 时结算报错，钱包不补扣。 | 本地计费＋SQLite复现 | [service/billing_session.go:62](/workspace/LemonHub/service/billing_session.go:62) |
| [7229](https://github.com/QuantumNous/new-api/issues/7229) | 缓存图片同时按缓存价和图片价收费；构造 100 个全缓存图片 token，期望 10 实收 110 quota。 | 本地生产计费函数复现 | [service/billing_usage.go:190](/workspace/LemonHub/service/billing_usage.go:190) |
| [7551](https://github.com/QuantumNous/new-api/issues/7551) | Claude 输出前 refusal、零输出仍按输入 token 计费；输入 412、倍率 5 时计算 2,060 quota。 | 本地计费函数复现；仅拒绝前零输出范围 | [relay/channel/claude/relay-claude.go:30](/workspace/LemonHub/relay/channel/claude/relay-claude.go:30) |
| [7498](https://github.com/QuantumNous/new-api/issues/7498) | Responses 工具结果图片被序列化为普通文本；同类 Claude 工具结果见 #7407，本地估算见 #7148。 | 实际协议转换复现 | [relaykit/relayconvert/internal/oai_responses/to_oai_chat_req.go:222](/workspace/LemonHub/relaykit/relayconvert/internal/oai_responses/to_oai_chat_req.go:222) |
| [7475](https://github.com/QuantumNous/new-api/issues/7475) | 阿里单个 choice 含多张图片时只返回最后一张，图数／计费信息仍可为多张。 | 两图输入实际只返回 second.png | [relay/channel/ali/dto.go:107](/workspace/LemonHub/relay/channel/ali/dto.go:107) |
| [7489](https://github.com/QuantumNous/new-api/issues/7489) | 末帧同时有 usage、finish_reason，但客户端未请求 usage 时，终止信息被吞。 | 本地尾帧处理函数复现 | [relay/channel/openai/helper.go:155](/workspace/LemonHub/relay/channel/openai/helper.go:155) |
| [7432](https://github.com/QuantumNous/new-api/issues/7432) | Responses function_call_output.output 未进入敏感词扫描文本，已启用的过滤可被绕过。 | 本地元数据提取复现＋扫描调用链 | [relaykit/dto/openai_request.go:1080](/workspace/LemonHub/relaykit/dto/openai_request.go:1080) |
| [7062](https://github.com/QuantumNous/new-api/issues/7062) | Chat→Responses→Chat 已交付内容后客户端取消，返回 nil usage＋500，最终错误分支退款。 | handler 复现；退款链静态确认 | [relay/channel/openai/chat_via_responses.go:315](/workspace/LemonHub/relay/channel/openai/chat_via_responses.go:315) |
| [7394](https://github.com/QuantumNous/new-api/issues/7394) | 即使表达式不调用 param()，也读取并保留完整请求体，使磁盘缓存重新占用内存。 | 静态预扣／存储调用链 | [relay/helper/price.go:298](/workspace/LemonHub/relay/helper/price.go:298) |
| [6840](https://github.com/QuantumNous/new-api/issues/6840) | 验证码仅在单进程 map；无粘性会话的多节点部署发信与校验落不同节点时失败。 | 静态注册／验证调用链 | [common/verification.go:38](/workspace/LemonHub/common/verification.go:38) |
| [6744](https://github.com/QuantumNous/new-api/issues/6744) | 容器内存保护读取宿主机使用率，无法可靠反映 cgroup 内存限额。 | 静态监控／保护调用链 | [common/system_monitor.go:65](/workspace/LemonHub/common/system_monitor.go:65) |

修复建议分组：先处理退出记账与订阅／媒体计费，再处理媒体和工具协议转换、流终止及取消结算，随后处理集群／容器运行和管理界面。每项具体建议见后面的详细条目，涉及钱包、订阅及站点隔离的改动需独立回归。

## 避免误判的边界

- #6656／#6657 是敏感词子串匹配的重复报告；#7130／#7127 是 SSE 头未及时 flush 的同一问题。#7498／#7407／#7148 是媒体文本化的不同协议或计数入口，修复时可合并规划，但不能默认一处改动全部解决。
- #7456 仅确认工具选择约束静默丢失；本地没有标题提到的新 Diagnostics 框架。#6682 仅确认 Responses reasoning usage 字段位置；不认定该帖所有 EOF／[DONE] 主张。
- #7011 后端已支持时间函数，剩余问题在前端费用估算。#7180 能创建无权分组的 token，但实际 relay 使用仍被 403 阻止，不能宣称已越权调用。
- #6978 确认重复购买产生有效期重叠；不声称额度丢失，是否改为串行续期需要明确产品语义。#7564 Gemini 无 developer 角色，确认的是静默合并的语义降级。
- #7670 的原始完整表达式本地可解析，保留为证据不足；#7470 的 choices:null 本地正常，只有不符合数组契约的 {} 报错，未计为同 bug。
- GPT‑6 参数、TLS 配置共享、Ollama done 帧工具调用、充值 int32 限额、繁体语言缓存等已有本地修复，已从待修范围排除。
- 任务插件／Responses WebSocket 等尚未引入的上游架构记为不适用，这不表示本地具备这些新功能；countTokens 的等效规避是明确拒绝，非完整实现计数。
- #6872 标签配色偏好、#6877 合法公网直连出口 IP 提示属于增强需求，未计入确认缺陷。

## 200 条完整索引

每条详细理由、全部源码证据、触发／复现及建议紧随索引。

| 序号 | Issue／标题 | 上游状态 | 本地判定 | 优先级 | 本地行为探针 |
| ---: | --- | --- | --- | --- | --- |
| 1 | [7672 · model list: `owned_by` shows `advanced_custom` instead of name for multiple advanced_custom channels.](https://github.com/QuantumNous/new-api/issues/7672) | open | 确认存在 | P3 | — |
| 2 | [7670 · 复杂动态计费表达式已经正确结算，但前端列表页会显示“无匹配结果”，容易误导管理员。](https://github.com/QuantumNous/new-api/issues/7670) | open | 证据不足 | — | — |
| 3 | [7666 · [Bug] 分流图将 token_id=0 的渠道测试流量显示为 "Unknown Token"](https://github.com/QuantumNous/new-api/issues/7666) | open | 确认存在 | P3 | — |
| 4 | [7657 · [Bug] 容器正常重启时未排空在途结算和批量记账，重启后可能留下账目缺口](https://github.com/QuantumNous/new-api/issues/7657) | open | 确认存在 | P1 | — |
| 5 | [7646 · fixed pricing is not supported for task usage expressions](https://github.com/QuantumNous/new-api/issues/7646) | open | 不适用／非缺陷 | — | — |
| 6 | [7640 · [求助，也可能是功能请求]希望可以实现每一个渠道都遵循`自动重试`规则](https://github.com/QuantumNous/new-api/issues/7640) | open | 不适用／非缺陷 | — | — |
| 7 | [7634 · [Bug] Web: a failed async chunk load shows a "500" error page instead of reloading](https://github.com/QuantumNous/new-api/issues/7634) | open | 确认存在 | P2 | — |
| 8 | [7632 · StreamScannerHandler 中 end_reason 存在竞态，导致 scanner_error 被 client_gone 覆盖](https://github.com/QuantumNous/new-api/issues/7632) | open | 疑似 | P2 | — |
| 9 | [7626 · [Bug] gpt-6-sol / gpt-6-luna 参数兼容问题：`max_tokens` 未转换](https://github.com/QuantumNous/new-api/issues/7626) | closed | 已修复／等效规避 | — | — |
| 10 | [7619 · Docker Compose 部署  pg 密码问题](https://github.com/QuantumNous/new-api/issues/7619) | closed | 证据不足 | — | — |
| 11 | [7618 · newapi 很多人机账户怎么批量删除](https://github.com/QuantumNous/new-api/issues/7618) | closed | 已修复／等效规避 | — | — |
| 12 | [7613 · [Bug] PassKey验证逻辑矛盾，root无法在多台设备上登录](https://github.com/QuantumNous/new-api/issues/7613) | closed | 不适用／非缺陷 | — | — |
| 13 | [7612 · [Bug] 客户端时区与服务器时区不匹配时，无法进行修改密码、PassKey等操作](https://github.com/QuantumNous/new-api/issues/7612) | closed | 证据不足 | — | — |
| 14 | [7607 · TLS_INSECURE_SKIP_VERIFY=true 时"上游价格同步"必失败：models.dev 返回 h2 乱码、basellm.github.io EOF](https://github.com/QuantumNous/new-api/issues/7607) | closed | 已修复／等效规避 | — | — |
| 15 | [7603 · [bug]  web: long messages can't be wrap in tooltip at channel table item - "status" in "Auto-disabled" status](https://github.com/QuantumNous/new-api/issues/7603) | closed | 已修复／等效规避 | — | — |
| 16 | [7599 · Billing history shows the top-up status in English in every language (Success, Pending, Expired)](https://github.com/QuantumNous/new-api/issues/7599) | closed | 确认存在 | P3 | — |
| 17 | [7595 · 主节点服务重启失败，报错重复创建索引](https://github.com/QuantumNous/new-api/issues/7595) | open | 不适用／非缺陷 | — | — |
| 18 | [7593 · [Bug] Anthropic /v1/messages tools 字段未转换为 OpenAI 格式，上游报 400 field Tools[0].Type invalid](https://github.com/QuantumNous/new-api/issues/7593) | open | 已修复／等效规避 | — | — |
| 19 | [7591 · [Bug] 多密钥渠道经任务插件提交时，消费日志缺少 key 序号（multi_key_index）](https://github.com/QuantumNous/new-api/issues/7591) | open | 确认存在 | P2 | — |
| 20 | [7587 · 给用户扣了个雷霆大费用](https://github.com/QuantumNous/new-api/issues/7587) | closed | 证据不足 | — | — |
| 21 | [7585 · Web UI: the bundled Public Sans font is never used (--font-sans asks for 'Public Sans', the package registers 'Public Sans Variable')](https://github.com/QuantumNous/new-api/issues/7585) | closed | 确认存在 | P3 | — |
| 22 | [7584 · Six UI labels are hard-coded in English or built from fragments (usage-log Tokens header, top-up presets, plan status, redemption dialog, price note, inviter ID)](https://github.com/QuantumNous/new-api/issues/7584) | closed | 确认存在 | P3 | — |
| 23 | [7580 · Dashboard charts draw January before December when the range crosses a year (time labels sorted as text)](https://github.com/QuantumNous/new-api/issues/7580) | closed | 确认存在 | P2 | 有（范围见详情） |
| 24 | [7578 · fix(relayconvert): Responses→Chat converter delivery image_url as text，causing 400](https://github.com/QuantumNous/new-api/issues/7578) | open | 确认存在 | P1 | 有（范围见详情） |
| 25 | [7576 · Delete Account dialog: confirmation label built from fragments reads "类型 admin 以确认" (wrong in 6 of 7 languages)](https://github.com/QuantumNous/new-api/issues/7576) | closed | 确认存在 | P3 | — |
| 26 | [7574 · Two-factor setup dialog shows "Step1of 3:Scan QR Code" (no spaces, word order not translatable)](https://github.com/QuantumNous/new-api/issues/7574) | closed | 确认存在 | P3 | — |
| 27 | [7573 · Insufficient-quota errors are always Chinese, ignoring the user's language](https://github.com/QuantumNous/new-api/issues/7573) | open | 确认存在 | P3 | — |
| 28 | [7564 · OpenAI developer messages are merged into Gemini systemInstruction with no notice](https://github.com/QuantumNous/new-api/issues/7564) | open | 确认存在 | P2 | — |
| 29 | [7563 · Claude Messages：message 级 output_config（per-message effort）转发时被丢弃，上游返回 400](https://github.com/QuantumNous/new-api/issues/7563) | closed | 已修复／等效规避 | — | — |
| 30 | [7558 · rc40中调用k3模型时，如果请求中有视频或者图片时，预处理会很慢，比上游首字及耗时能多出5-10秒](https://github.com/QuantumNous/new-api/issues/7558) | closed | 证据不足 | — | — |
| 31 | [7556 · jev 任务插件创建的渠道，在模型广场的端点类型仍为 openai](https://github.com/QuantumNous/new-api/issues/7556) | open | 不适用／非缺陷 | — | — |
| 32 | [7551 · [Bug] Claude 上游在输出前 refusal（content 为空、output_tokens 0）时仍按 input_tokens 扣费，Anthropic 对此不计费](https://github.com/QuantumNous/new-api/issues/7551) | open | 确认存在 | P1 | 有（范围见详情） |
| 33 | [7549 · [Bug] Claude Messages 缺少 server_tool_use.id 时 Responses 转换 panic 且失败流仍计费](https://github.com/QuantumNous/new-api/issues/7549) | open | 不适用／非缺陷 | — | — |
| 34 | [7546 · gpt-6-luna测试失败](https://github.com/QuantumNous/new-api/issues/7546) | closed | 已修复／等效规避 | — | — |
| 35 | [7545 · 设置模型自定义图标后 web 页面持续 429：图标下拉一次加载全部 LobeHub 图标 chunk，默认 GLOBAL_WEB_RATE_LIMIT 被打满](https://github.com/QuantumNous/new-api/issues/7545) | open | 不适用／非缺陷 | — | — |
| 36 | [7540 · [Bug] 流式图片请求在 Token 统计阶段重复下载已知类型图片，导致内网 URL 被 SSRF 拦截并返回 500](https://github.com/QuantumNous/new-api/issues/7540) | open | 确认存在 | P2 | — |
| 37 | [7511 · [Bug] 非 Root 管理员可见系统设置入口，但访问后被重定向到 403](https://github.com/QuantumNous/new-api/issues/7511) | closed | 已修复／等效规避 | — | — |
| 38 | [7506 · [Bug] v1.0.0-rc.39 动态加载供应商图标失败，多个页面仅显示首字母](https://github.com/QuantumNous/new-api/issues/7506) | open | 不适用／非缺陷 | — | — |
| 39 | [7503 · 模型徽标行高过小导致 g/p 下伸笔画被裁切](https://github.com/QuantumNous/new-api/issues/7503) | closed | 不适用／非缺陷 | — | — |
| 40 | [7498 · [BUG] Responses→Chat 转换器把 function_call_output 的图片内容拍平成 base64 文本，触发上游 prompt 超限](https://github.com/QuantumNous/new-api/issues/7498) | closed | 确认存在 | P1 | 有（范围见详情） |
| 41 | [7495 · [BUG] oai_chat_to_oai_responses 转换器：流中途出现 finish_reason 后 reasoning delta 失去 active item](https://github.com/QuantumNous/new-api/issues/7495) | closed | 确认存在 | P2 | 有（范围见详情） |
| 42 | [7492 · 后台显示调用的模型名不一致，glm-5.3-flash变成了glm-5-3-flash](https://github.com/QuantumNous/new-api/issues/7492) | closed | 不适用／非缺陷 | — | — |
| 43 | [7489 · fix(relay): terminal stream chunk with finish_reason is dropped when client does not request usage](https://github.com/QuantumNous/new-api/issues/7489) | open | 确认存在 | P1 | 有（范围见详情） |
| 44 | [7486 · 流式 /v1/responses：HTTP 200 开流后 eof/未完成仍扣费，同会话再请求会被记第二笔,很紧急！！！！！！！！！！！！！](https://github.com/QuantumNous/new-api/issues/7486) | closed | 证据不足 | — | — |
| 45 | [7485 · [Bug] v1.0.0-rc.38 CC Switch 模型选择下拉选择框定位异常](https://github.com/QuantumNous/new-api/issues/7485) | closed | 已修复／等效规避 | — | — |
| 46 | [7479 · [Bug] “高级自定义”渠道的前端缺少了 Responses -> Message 等转换选项](https://github.com/QuantumNous/new-api/issues/7479) | open | 确认存在 | P2 | — |
| 47 | [7478 · Task Plugin 创建异步任务时仅接受 HTTP 200，合法的 201 Created 被误判为 channel error](https://github.com/QuantumNous/new-api/issues/7478) | closed | 不适用／非缺陷 | — | — |
| 48 | [7475 · [BUG] 阿里通义千问渠道图片生成 n>1 时只返回最后一张图，计费仍按 n 张](https://github.com/QuantumNous/new-api/issues/7475) | closed | 确认存在 | P1 | 有（范围见详情） |
| 49 | [7470 · vLLM upstream returns "choices" as null, causing "cannot unmarshal" error in stream parsing](https://github.com/QuantumNous/new-api/issues/7470) | open | 不适用／非缺陷 | — | — |
| 50 | [7467 · [Bug] Responses WebSocket 开关在 Sub2API 等渠道保存后自动关闭](https://github.com/QuantumNous/new-api/issues/7467) | closed | 不适用／非缺陷 | — | — |
| 51 | [7465 · 部分模型定价转换异常](https://github.com/QuantumNous/new-api/issues/7465) | open | 不适用／非缺陷 | — | — |
| 52 | [7456 · RequestResult.Diagnostics is discarded on the success path, so tool policy losses are never logged or returned](https://github.com/QuantumNous/new-api/issues/7456) | open | 确认存在 | P1 | 有（范围见详情） |
| 53 | [7455 · CleanFunctionParameters strips const, additionalProperties and oneOf from tool schemas, and parametersJsonSchema is never emitted](https://github.com/QuantumNous/new-api/issues/7455) | open | 确认存在 | P1 | 有（范围见详情） |
| 54 | [7454 · Claude/OpenAI conversion drops file and document blocks, and turns url-source images into "data:;base64,%!s(<nil>)"](https://github.com/QuantumNous/new-api/issues/7454) | open | 确认存在 | P1 | 有（范围见详情） |
| 55 | [7435 · 更新后管理员登录提示此验证方式当前不可用。重置密码后依然无法使用](https://github.com/QuantumNous/new-api/issues/7435) | closed | 不适用／非缺陷 | — | — |
| 56 | [7432 · 在 /v1/responses API 格式中存在的 绕过 敏感词检测的问题](https://github.com/QuantumNous/new-api/issues/7432) | open | 确认存在 | P1 | 有（范围见详情） |
| 57 | [7420 · 模型无法配置上下文窗口和输入输出上下文长度](https://github.com/QuantumNous/new-api/issues/7420) | closed | 不适用／非缺陷 | — | — |
| 58 | [7417 · [Bug] 「货币与展示」页面汇率输入多位小数（如 6.7081）时会弹出格式校验警告](https://github.com/QuantumNous/new-api/issues/7417) | closed | 确认存在 | P3 | — |
| 59 | [7409 · Images API 未将 output_tokens_details 映射到 img_o，表达式少收图输出](https://github.com/QuantumNous/new-api/issues/7409) | closed | 确认存在 | P1 | — |
| 60 | [7407 · [Bug] Claude tool_result 内嵌图片在转换为 OpenAI Chat 格式时被序列化为 Base64 纯文本，导致上游视觉失效且产生数十万文本 Token](https://github.com/QuantumNous/new-api/issues/7407) | closed | 确认存在 | P1 | 有（范围见详情） |
| 61 | [7402 · 模型定价计费设置计费时间，前端页面显示时间和表达式中的时间不一致](https://github.com/QuantumNous/new-api/issues/7402) | open | 证据不足 | — | — |
| 62 | [7399 · 流式模型式下上游报错503:“Our servers are currently overloaded. Please try again later.” 却不重试](https://github.com/QuantumNous/new-api/issues/7399) | closed | 确认存在 | P1 | — |
| 63 | [7394 · tiered_expr pre-consume reads the whole request body into memory even when the expression never calls param() (defeats the disk cache; retained until settlement)](https://github.com/QuantumNous/new-api/issues/7394) | open | 确认存在 | P1 | — |
| 64 | [7393 · Dashboard: charts drop real time buckets and fabricate empty ones when fewer than 7 buckets are returned](https://github.com/QuantumNous/new-api/issues/7393) | open | 确认存在 | P2 | 有（范围见详情） |
| 65 | [7392 · [Bug] 将按 Token 计费转换到按表达式计费之后，模型广场会将转换之后的所有模型价格都显示为动态计费](https://github.com/QuantumNous/new-api/issues/7392) | open | 确认存在 | P3 | — |
| 66 | [7385 · /v1/audio/speech 的 stream_format=audio 未实现实时流式透传](https://github.com/QuantumNous/new-api/issues/7385) | open | 确认存在 | P2 | — |
| 67 | [7363 · [Bug] 使用日志中的 codex-* 模型未显示 OpenAI 图标](https://github.com/QuantumNous/new-api/issues/7363) | closed | 确认存在 | P3 | — |
| 68 | [7361 · [BUG] v1.0.0-rc.37 常驻内存暴涨至 ~1.8GB（rc.36 仅 ~75MB），小内存机器触发 OOM](https://github.com/QuantumNous/new-api/issues/7361) | closed | 证据不足 | — | — |
| 69 | [7360 · bug: Endpoint Type combobox in Test Channel Connection dialog auto-opens its dropdown](https://github.com/QuantumNous/new-api/issues/7360) | closed | 已修复／等效规避 | — | — |
| 70 | [7354 · [Bug] 数据看板用户统计：周粒度下默认时间范围未选中](https://github.com/QuantumNous/new-api/issues/7354) | closed | 已修复／等效规避 | — | — |
| 71 | [7348 · [Bug] 开启请求体透传的渠道上参数覆盖（param_override）静默失效](https://github.com/QuantumNous/new-api/issues/7348) | closed | 确认存在 | P2 | — |
| 72 | [7338 · OAuth 敏感操作验证在 IdP 返回 Cross-Origin-Opener-Policy 时必定失败](https://github.com/QuantumNous/new-api/issues/7338) | open | 不适用／非缺陷 | — | — |
| 73 | [7319 · audio没有兼容大小写](https://github.com/QuantumNous/new-api/issues/7319) | closed | 已修复／等效规避 | — | — |
| 74 | [7309 · 模型广场 性能页面 会泄露 隐藏的分组](https://github.com/QuantumNous/new-api/issues/7309) | open | 已修复／等效规避 | — | — |
| 75 | [7307 · Claude→OpenAI conversion passes mid-conversation system messages through, breaking strict upstreams](https://github.com/QuantumNous/new-api/issues/7307) | closed | 不适用／非缺陷 | — | — |
| 76 | [7296 · 请求日志中，阶梯计费模型的日志计费摘要误报「动态计费 · 无匹配结果」](https://github.com/QuantumNous/new-api/issues/7296) | closed | 已修复／等效规避 | — | — |
| 77 | [7290 · Claude 用量日志的 prompt_tokens 只记录未命中缓存的 fresh token，导致日志显示与 TPM／token 统计漏掉全部缓存输入](https://github.com/QuantumNous/new-api/issues/7290) | open | 确认存在 | P2 | — |
| 78 | [7286 · “高级自定义”渠道配置路由内的”添加分流“按钮不见了](https://github.com/QuantumNous/new-api/issues/7286) | closed | 已修复／等效规避 | — | — |
| 79 | [7283 · [Bug] Gemini 渠道 :countTokens 被当作 generateContent 执行：无 totalTokens、延迟 24-44s、且按生成计费](https://github.com/QuantumNous/new-api/issues/7283) | open | 已修复／等效规避 | — | — |
| 80 | [7282 · [Bug] 模型广场 24 小时成功率柱状条间距不均匀](https://github.com/QuantumNous/new-api/issues/7282) | closed | 不适用／非缺陷 | — | — |
| 81 | [7268 · Time-based billing tiers are not displayed correctly on the pricing page](https://github.com/QuantumNous/new-api/issues/7268) | closed | 确认存在 | P2 | 有（范围见详情） |
| 82 | [7267 · Users table quota display regressed after ea7cb0ba4](https://github.com/QuantumNous/new-api/issues/7267) | open | 已修复／等效规避 | — | — |
| 83 | [7252 · fix(ollama): streaming tool_calls dropped when Ollama returns them in the final `done:true` frame (e.g. qwen3-coder)](https://github.com/QuantumNous/new-api/issues/7252) | closed | 已修复／等效规避 | — | — |
| 84 | [7247 · Higress AI 网关使用 Transfer-Encoding: chunked 发送请求体给上游 New API 服务，但 New API 无法正确解析 chunked 请求体，导致返回 400 "invalid JSON request body"。](https://github.com/QuantumNous/new-api/issues/7247) | closed | 证据不足 | — | — |
| 85 | [7235 · kimi-k3 动态工具调用异常](https://github.com/QuantumNous/new-api/issues/7235) | closed | 疑似 | P2 | — |
| 86 | [7231 · 非流式请求客户端超时后仍继续上游请求，并在连接断开后扣费](https://github.com/QuantumNous/new-api/issues/7231) | closed | 确认存在 | P1 | 有（范围见详情） |
| 87 | [7229 · 严重的计费BUG-图像缓存命中重复计费](https://github.com/QuantumNous/new-api/issues/7229) | open | 确认存在 | P1 | 有（范围见详情） |
| 88 | [7225 · 渠道测试 settleTestQuota 漏乘分组倍率](https://github.com/QuantumNous/new-api/issues/7225) | closed | 确认存在 | P2 | 有（范围见详情） |
| 89 | [7224 · ParseContent() 丢失 cache_control](https://github.com/QuantumNous/new-api/issues/7224) | closed | 确认存在 | P1 | 有（范围见详情） |
| 90 | [7222 · [Bug] 模型广场卡片视图：切换分组后分页跳到最后一页，上一页按钮失灵](https://github.com/QuantumNous/new-api/issues/7222) | open | 确认存在 | P2 | — |
| 91 | [7215 · 使用 Anthropic API 格式发起请求时，思考等级参数未正确映射至 OpenAI 上游的 reasoning_effort](https://github.com/QuantumNous/new-api/issues/7215) | closed | 确认存在 | P2 | 有（范围见详情） |
| 92 | [7205 · [Bug] 原生 Gemini thinkingLevel 大写枚举被误判为模型不支持](https://github.com/QuantumNous/new-api/issues/7205) | closed | 不适用／非缺陷 | — | — |
| 93 | [7201 · qwen3.7-max/qwen3.8-max are incorrectly trimmed to qwen3.7/qwen3.8 after v1.0.0-rc.31](https://github.com/QuantumNous/new-api/issues/7201) | closed | 已修复／等效规避 | — | — |
| 94 | [7194 · 最新版本视频模型生成成功后获取制品提示媒体预览失败，请重试，接口请问tasks状态为404](https://github.com/QuantumNous/new-api/issues/7194) | open | 不适用／非缺陷 | — | — |
| 95 | [7185 · 插件抢占模型名后，模型映射无法切换到其他插件，导致请求失败](https://github.com/QuantumNous/new-api/issues/7185) | closed | 不适用／非缺陷 | — | — |
| 96 | [7184 · fix: SSRF allowed_ports string-slice config is not parsed correctly](https://github.com/QuantumNous/new-api/issues/7184) | open | 已修复／等效规避 | — | — |
| 97 | [7180 · 用户可越权添加其他用户组 api 令牌](https://github.com/QuantumNous/new-api/issues/7180) | closed | 确认存在 | P2 | — |
| 98 | [7177 · Rerank channel is incorrectly recognized as embedding, causing test failure](https://github.com/QuantumNous/new-api/issues/7177) | open | 确认存在 | P2 | — |
| 99 | [7175 · [Bug] Claude Messages 转 OpenAI Chat 时单元素 stop_sequences 被转换为 string，导致部分上游 400](https://github.com/QuantumNous/new-api/issues/7175) | open | 确认存在 | P2 | 有（范围见详情） |
| 100 | [7174 · 用户可创建空分组密钥，管理员修改用户分组后密钥会请求修改后的分组](https://github.com/QuantumNous/new-api/issues/7174) | closed | 已修复／等效规避 | — | — |
| 101 | [7155 · 模型映射没有执行](https://github.com/QuantumNous/new-api/issues/7155) | closed | 不适用／非缺陷 | — | — |
| 102 | [7148 · 内联图片 Base64 被计入文本 Token：Claude tool_result 与 Responses compact 本地估算异常（模板重提）](https://github.com/QuantumNous/new-api/issues/7148) | open | 确认存在 | P1 | 有（范围见详情） |
| 103 | [7144 · [Bug] 模型广场打开排序下拉菜单时顶部导航栏左右晃动](https://github.com/QuantumNous/new-api/issues/7144) | closed | 已修复／等效规避 | — | — |
| 104 | [7141 · [BUG] Traditional Chinese (zhTW) always flips back to Simplified Chinese on page reload](https://github.com/QuantumNous/new-api/issues/7141) | closed | 已修复／等效规避 | — | — |
| 105 | [7136 · logs表upstream_request_id 值都是空的,不利于上游渠道排查问题](https://github.com/QuantumNous/new-api/issues/7136) | closed | 不适用／非缺陷 | — | — |
| 106 | [7134 · 客户端取消的流式请求被计为模型失败，持续污染性能成功率](https://github.com/QuantumNous/new-api/issues/7134) | closed | 确认存在 | P2 | 有（范围见详情） |
| 107 | [7130 · TTFT延迟 网关设置了 SSE 响应头但在首个数据帧之前从不 flush](https://github.com/QuantumNous/new-api/issues/7130) | open | 确认存在 | P2 | — |
| 108 | [7127 · 多个 channel handler 设置了 SSE 头但从不 flush](https://github.com/QuantumNous/new-api/issues/7127) | closed | 确认存在 | P2 | — |
| 109 | [7123 · 自定义首页 iframe 首屏语言/主题同步存在竞态，可能无法跟随中文](https://github.com/QuantumNous/new-api/issues/7123) | open | 确认存在 | P3 | — |
| 110 | [7113 · auto分组设置后无效果，令牌选择分组时无auto选项](https://github.com/QuantumNous/new-api/issues/7113) | closed | 不适用／非缺陷 | — | — |
| 111 | [7106 · bug: 使用日志顶部用量显示 ¥0，但消费明细 quota 非零](https://github.com/QuantumNous/new-api/issues/7106) | closed | 已修复／等效规避 | — | — |
| 112 | [7099 · 28版本无法启用](https://github.com/QuantumNous/new-api/issues/7099) | closed | 不适用／非缺陷 | — | — |
| 113 | [7098 · rc.26~rc.28 启动崩溃循环：PostgreSQL(PgBouncer) 下 schema 自检报 "insufficient arguments"](https://github.com/QuantumNous/new-api/issues/7098) | closed | 不适用／非缺陷 | — | — |
| 114 | [7094 · fix: Responses conversion writes empty function_call.name into history, breaking session replay](https://github.com/QuantumNous/new-api/issues/7094) | closed | 确认存在 | P2 | 有（范围见详情） |
| 115 | [7089 · 游乐场无法使用](https://github.com/QuantumNous/new-api/issues/7089) | closed | 证据不足 | — | — |
| 116 | [7087 · bug: Responses SSE added item 缺少空数组时 Codex 丢失 active item](https://github.com/QuantumNous/new-api/issues/7087) | closed | 不适用／非缺陷 | — | — |
| 117 | [7084 · Task-plugin (video) submits bypass channel model_mapping: submit body finalized before ModelMappedHelper (rc.27)](https://github.com/QuantumNous/new-api/issues/7084) | closed | 不适用／非缺陷 | — | — |
| 118 | [7063 · new-api-v1.0.0-rc.26.exe给我安装到哪里去了，我在仓库没找到任何关于如何卸载的字段](https://github.com/QuantumNous/new-api/issues/7063) | closed | 不适用／非缺陷 | — | — |
| 119 | [7062 · 流式请求中 client_gone 导致 New API 计费为 0，但上游已正常扣费且客户端收到完整响应](https://github.com/QuantumNous/new-api/issues/7062) | closed | 确认存在 | P1 | 有（范围见详情） |
| 120 | [7061 · token计费和上游差距较大](https://github.com/QuantumNous/new-api/issues/7061) | closed | 证据不足 | — | — |
| 121 | [7059 · Bug: upstream TCP reset leaves Responses SSE without a terminal event](https://github.com/QuantumNous/new-api/issues/7059) | open | 确认存在 | P2 | 有（范围见详情） |
| 122 | [7047 · chatgpt经newapi访问glm报错：tools[7].type:type is illegal](https://github.com/QuantumNous/new-api/issues/7047) | closed | 证据不足 | — | — |
| 123 | [7040 · [Bug] 多密钥渠道所有 Key 自动禁用后，定期测试无法恢复渠道](https://github.com/QuantumNous/new-api/issues/7040) | open | 确认存在 | P2 | — |
| 124 | [7018 · v1.0.0-rc.25版本：配置额度的上限变小了](https://github.com/QuantumNous/new-api/issues/7018) | closed | 已修复／等效规避 | — | — |
| 125 | [7011 · 【Bug】表达式模式不支持 weekday 函数，无法通过表达式实现周一至周五的高峰定价](https://github.com/QuantumNous/new-api/issues/7011) | closed | 确认存在 | P2 | 有（范围见详情） |
| 126 | [7007 · [Bug] 渠道重试未按优先级选择，实际按渠道 ID 顺序处理](https://github.com/QuantumNous/new-api/issues/7007) | closed | 证据不足 | — | — |
| 127 | [7005 · OaiStreamHandler 逐帧滞后转发,两级 new-api 串联时首字延迟被放大到「上游帧间隔」量级](https://github.com/QuantumNous/new-api/issues/7005) | open | 确认存在 | P2 | — |
| 128 | [6995 · claude code 接入 ollama 的时候 会报错 Unable to validate model: undefined is not an object (evaluating 'U.usage.input_tokens')](https://github.com/QuantumNous/new-api/issues/6995) | closed | 确认存在 | P2 | — |
| 129 | [6985 · Admin cannot unbind built-in OAuth/OIDC account bindings](https://github.com/QuantumNous/new-api/issues/6985) | closed | 已修复／等效规避 | — | — |
| 130 | [6981 · StreamScannerHandler drops upstream SSE comment heartbeats during active streams](https://github.com/QuantumNous/new-api/issues/6981) | open | 确认存在 | P2 | 有（范围见详情） |
| 131 | [6978 · [Bug] 同一套餐未到期时续费，新订阅从购买时刻重新起算，提前续费损失剩余时长](https://github.com/QuantumNous/new-api/issues/6978) | open | 确认存在 | P2 | — |
| 132 | [6976 · 视频模型 配置按次计费后的前置余额扣费报错问题](https://github.com/QuantumNous/new-api/issues/6976) | closed | 已修复／等效规避 | — | — |
| 133 | [6972 · 活跃登录会话数量已达上限。请在一台已登录的设备上打开“登录会话”，使用“退出其他登录会话”将其撤销。如果无法访问任何已登录设备，请重置密码以退出所有会话。](https://github.com/QuantumNous/new-api/issues/6972) | open | 不适用／非缺陷 | — | — |
| 134 | [6971 · 无法开启在线支付](https://github.com/QuantumNous/new-api/issues/6971) | closed | 证据不足 | — | — |
| 135 | [6963 · 账号密码暴露](https://github.com/QuantumNous/new-api/issues/6963) | closed | 已修复／等效规避 | — | — |
| 136 | [6959 · RC.25 的 docker 镜像，裸启动后没有自动进入 setup](https://github.com/QuantumNous/new-api/issues/6959) | closed | 已修复／等效规避 | — | — |
| 137 | [6957 · v1.0.0-rc.25 UI控制台权限控制无效：系统管理关闭侧边栏聊天区域后，个人用户仍显示这些配置项](https://github.com/QuantumNous/new-api/issues/6957) | open | 确认存在 | P2 | — |
| 138 | [6952 · 重大BUG："活跃登录会话数量已达上限"导致管理员彻底无法登录](https://github.com/QuantumNous/new-api/issues/6952) | closed | 疑似 | P2 | — |
| 139 | [6947 · 转发请求缺少 ResponseHeaderTimeout：上游不返回响应头时 goroutine 与请求体永久驻留，累积至 OOM（rc.23 实测 172.9h 死亡）](https://github.com/QuantumNous/new-api/issues/6947) | open | 已修复／等效规避 | — | — |
| 140 | [6940 · v1.0.0-rc.25 充值确认支付失败](https://github.com/QuantumNous/new-api/issues/6940) | closed | 已修复／等效规避 | — | — |
| 141 | [6939 · 调用deepseek-v4-flash会报错：400 The 'reasoning_content’ in the thinking mode must be passed back to the API.](https://github.com/QuantumNous/new-api/issues/6939) | closed | 不适用／非缺陷 | — | — |
| 142 | [6937 · undefinedDockerfile.dev build fails because relaykit/go.mod is not copied before go mod downloadDockerfile.dev build fails because relaykit/go.mod is not copied before go mod download](https://github.com/QuantumNous/new-api/issues/6937) | closed | 已修复／等效规避 | — | — |
| 143 | [6936 · [Withdrawn]](https://github.com/QuantumNous/new-api/issues/6936) | closed | 证据不足 | — | — |
| 144 | [6929 · 提示余额不足，但是账户余额$7.18，未限制令牌配额](https://github.com/QuantumNous/new-api/issues/6929) | closed | 不适用／非缺陷 | — | — |
| 145 | [6923 · 阶梯计费的规则组中时间规则存在异常](https://github.com/QuantumNous/new-api/issues/6923) | closed | 已修复／等效规避 | — | — |
| 146 | [6920 · v1.0.0-rc.25 充值页面 /api/user/amount 无法正常获取待支付金额](https://github.com/QuantumNous/new-api/issues/6920) | closed | 已修复／等效规避 | — | — |
| 147 | [6919 · 订阅管理页面没有“手动绑定”按钮](https://github.com/QuantumNous/new-api/issues/6919) | closed | 不适用／非缺陷 | — | — |
| 148 | [6911 · 订阅预扣成功但最终补扣溢出时未按 allow_wallet_overflow 扣除钱包差额](https://github.com/QuantumNous/new-api/issues/6911) | open | 确认存在 | P1 | 有（范围见详情） |
| 149 | [6908 · API密钥编辑，提示 额度值超出有效范围，最大值为 2147483647](https://github.com/QuantumNous/new-api/issues/6908) | closed | 已修复／等效规避 | — | — |
| 150 | [6898 · [bug] 请求覆写规则似乎没有被执行](https://github.com/QuantumNous/new-api/issues/6898) | closed | 不适用／非缺陷 | — | — |
| 151 | [6897 · claude对接new-api 输入缓存命中率低，但是最近版本new-api正常](https://github.com/QuantumNous/new-api/issues/6897) | closed | 证据不足 | — | — |
| 152 | [6894 · Playground 编辑消息时输入光标跳回文首，表现为从右往左插入](https://github.com/QuantumNous/new-api/issues/6894) | closed | 已修复／等效规避 | — | — |
| 153 | [6887 · 突发500问题，无法进入playground](https://github.com/QuantumNous/new-api/issues/6887) | closed | 证据不足 | — | — |
| 154 | [6885 · 批量模式和标签模式同时启用时，批量模式无效](https://github.com/QuantumNous/new-api/issues/6885) | open | 确认存在 | P2 | 有（范围见详情） |
| 155 | [6883 · [Bug] Chat Completions → OpenAI Responses 转换中 cached_tokens 未参与结算](https://github.com/QuantumNous/new-api/issues/6883) | closed | 已修复／等效规避 | — | — |
| 156 | [6877 · [bug?/feat req]管理后台无明确提示、若不配置worker易暴露源站IP](https://github.com/QuantumNous/new-api/issues/6877) | open | 不适用／非缺陷 | — | — |
| 157 | [6876 · 活跃登录会话数量已达上限。请在一台已登录的设备上打开“登录会话”，使用“退出其他登录会话”将其撤销。如果无法访问任何已登录设备，请重置密码以退出所有会话](https://github.com/QuantumNous/new-api/issues/6876) | closed | 不适用／非缺陷 | — | — |
| 158 | [6872 · 使用日志里模型标签的颜色都变成一样了，看着好累](https://github.com/QuantumNous/new-api/issues/6872) | open | 不适用／非缺陷 | — | — |
| 159 | [6864 · 阿里视频任务轮询不支持多密钥模式（multi-key）](https://github.com/QuantumNous/new-api/issues/6864) | open | 确认存在 | P2 | — |
| 160 | [6860 · 在游乐场内调用ollama渠道的模型输出一半终止后Ollama侧还在继续推理思考](https://github.com/QuantumNous/new-api/issues/6860) | open | 确认存在 | P2 | — |
| 161 | [6859 · Bug: Chat→Claude Messages 转换静默丢弃无参数工具](https://github.com/QuantumNous/new-api/issues/6859) | closed | 已修复／等效规避 | — | — |
| 162 | [6857 · Stripe 官方最近调整了 API 的兼容性规则导致stripe的sdk版本号报错,请更新最新版本中的sttipe的sdk版本号](https://github.com/QuantumNous/new-api/issues/6857) | closed | 疑似 | P2 | — |
| 163 | [6840 · 集群部署注册验证码提示错误](https://github.com/QuantumNous/new-api/issues/6840) | open | 确认存在 | P1 | — |
| 164 | [6831 · [Bug] 单笔充值金额超过 int32 额度上限（默认 QuotaPerUnit 下约 $4,294.96）时回调被拒：客户已付款，但订单永久 pending、额度未入账](https://github.com/QuantumNous/new-api/issues/6831) | closed | 已修复／等效规避 | — | — |
| 165 | [6822 · /v1/responses 流式快照 created_at 为浮点时三个快照事件被丢弃，usage 丢失、计费退化为估算](https://github.com/QuantumNous/new-api/issues/6822) | closed | 确认存在 | P1 | 有（范围见详情） |
| 166 | [6817 · 测试 Anthropic 渠道(type=14)时因空 tools:[] 导致 400 错误](https://github.com/QuantumNous/new-api/issues/6817) | closed | 已修复／等效规避 | — | — |
| 167 | [6805 · database is locked (SQLITE_BUSY) cannot start a transaction within a transaction](https://github.com/QuantumNous/new-api/issues/6805) | closed | 证据不足 | — | — |
| 168 | [6804 · [Bug] Auto-group API keys: group badge missing and "Cross-group" badge rendered unconditionally](https://github.com/QuantumNous/new-api/issues/6804) | open | 确认存在 | P2 | — |
| 169 | [6797 · [Bug] Anthropic stream 空 body:上游无 "data:" 前缀的裸 JSON 帧被 StreamScannerHandler 丢弃(deepseek-v4-pro)](https://github.com/QuantumNous/new-api/issues/6797) | closed | 不适用／非缺陷 | — | — |
| 170 | [6782 · task lease expired，日志无法删除](https://github.com/QuantumNous/new-api/issues/6782) | closed | 已修复／等效规避 | — | — |
| 171 | [6781 · [使用] 使用日志不显示缓存以及invalid tool parameters](https://github.com/QuantumNous/new-api/issues/6781) | open | 证据不足 | — | — |
| 172 | [6775 · 模型元信息不准确，例如vendor](https://github.com/QuantumNous/new-api/issues/6775) | open | 已修复／等效规避 | — | — |
| 173 | [6771 · 出现10%-20% 请求日志不扣费情况](https://github.com/QuantumNous/new-api/issues/6771) | closed | 证据不足 | — | — |
| 174 | [6763 · 启用 Turnstile 后密码错误会导致后续登录校验失败](https://github.com/QuantumNous/new-api/issues/6763) | closed | 已修复／等效规避 | — | — |
| 175 | [6752 · IP白名单功能过滤的IP不对](https://github.com/QuantumNous/new-api/issues/6752) | open | 不适用／非缺陷 | — | — |
| 176 | [6748 · Claude Code title generation requests can cause 429 retry amplification across channels](https://github.com/QuantumNous/new-api/issues/6748) | closed | 不适用／非缺陷 | — | — |
| 177 | [6746 · rerank模型配置后代理请求返回null](https://github.com/QuantumNous/new-api/issues/6746) | open | 证据不足 | — | — |
| 178 | [6745 · [Bug] CC Switch dialog model list not filtered by API key's group](https://github.com/QuantumNous/new-api/issues/6745) | open | 确认存在 | P2 | — |
| 179 | [6744 · 容器部署下 monitor_memory_threshold 永不触发：system_monitor 读的是宿主机 /proc/meminfo 而非 cgroup 限额](https://github.com/QuantumNous/new-api/issues/6744) | open | 确认存在 | P1 | — |
| 180 | [6732 · 设置 GLOBAL_API_RATE_LIMIT=1000000 时内存限流器资源占用高导致OOM](https://github.com/QuantumNous/new-api/issues/6732) | closed | 已修复／等效规避 | — | — |
| 181 | [6724 · 模型定价-上游价格同步崩溃](https://github.com/QuantumNous/new-api/issues/6724) | closed | 证据不足 | — | — |
| 182 | [6715 · [Bug] Vertex AI Gemini 的 /v1/messages 请求未转换，渠道测试却误报转换成功](https://github.com/QuantumNous/new-api/issues/6715) | closed | 确认存在 | P2 | 有（范围见详情） |
| 183 | [6712 · Claude Code/Desktop 直连 OpenGateway 需经 cc-headless 适配层(模型映射/格式整流/媒体降级)](https://github.com/QuantumNous/new-api/issues/6712) | closed | 不适用／非缺陷 | — | — |
| 184 | [6710 · 渠道管理先清空模型再点击获取上游模型列表，清空前的模型依然被勾选](https://github.com/QuantumNous/new-api/issues/6710) | closed | 已修复／等效规避 | — | — |
| 185 | [6700 · 加余额后，日志的user应是被操作的人，实际显示为操作管理员](https://github.com/QuantumNous/new-api/issues/6700) | open | 不适用／非缺陷 | — | — |
| 186 | [6696 · qwen3-rerank 模型Bearer认证失效](https://github.com/QuantumNous/new-api/issues/6696) | closed | 不适用／非缺陷 | — | — |
| 187 | [6694 · anthropic 协议发送错误](https://github.com/QuantumNous/new-api/issues/6694) | closed | 证据不足 | — | — |
| 188 | [6689 · Bug: Advanced Custom渠道测试连接选择 /v1/messages 端点时构造 OpenAI chat 请求体，导致 Anthropic→OpenAI Chat 转换器报错 (convert_request_failed)](https://github.com/QuantumNous/new-api/issues/6689) | closed | 已修复／等效规避 | — | — |
| 189 | [6688 · [Bug] 非 root 管理员无法管理订阅套餐：合规状态读取依赖 root-only 的 /api/option/，前端将 403 误判为"未确认合规"](https://github.com/QuantumNous/new-api/issues/6688) | open | 确认存在 | P2 | — |
| 190 | [6684 · 无法识别lobehub上智谱的模型图标](https://github.com/QuantumNous/new-api/issues/6684) | closed | 已修复／等效规避 | — | — |
| 191 | [6682 · /v1/responses 流式响应在 Codex 客户端中断流、EOF、JSON 解析失败](https://github.com/QuantumNous/new-api/issues/6682) | closed | 确认存在 | P2 | 有（范围见详情） |
| 192 | [6681 · 兑换码的新值与修改页面的额度输入框，有值时，点击键盘的backspace键，删除到最后会强制显示为0](https://github.com/QuantumNous/new-api/issues/6681) | open | 确认存在 | P3 | — |
| 193 | [6680 · 兑换码编辑页额度精度损失](https://github.com/QuantumNous/new-api/issues/6680) | closed | 已修复／等效规避 | — | — |
| 194 | [6671 · Ali 渠道对未传 top_p 的请求强制注入 0.001，导致百炼部分模型 400 且静默改变采样行为](https://github.com/QuantumNous/new-api/issues/6671) | closed | 已修复／等效规避 | — | — |
| 195 | [6661 · [Bug] API 密钥状态筛选把启用/过期当作互斥状态，导致「已过期」筛不全](https://github.com/QuantumNous/new-api/issues/6661) | open | 确认存在 | P2 | — |
| 196 | [6659 · 额度计费显示问题](https://github.com/QuantumNous/new-api/issues/6659) | open | 已修复／等效规避 | — | — |
| 197 | [6657 · Sensitive word filter false positives on short substrings (agent sessions)](https://github.com/QuantumNous/new-api/issues/6657) | closed | 确认存在 | P2 | 有（范围见详情） |
| 198 | [6656 · Sensitive word filter: short substring matches cause false positives and kill long agent sessions](https://github.com/QuantumNous/new-api/issues/6656) | closed | 确认存在 | P2 | 有（范围见详情） |
| 199 | [6650 · 「选择同步渠道」打开弹窗时页面卡死](https://github.com/QuantumNous/new-api/issues/6650) | open | 已修复／等效规避 | — | — |
| 200 | [6649 · stream_status.end_reason shows "client_gone" for streams that completed normally (SSE scanner race)](https://github.com/QuantumNous/new-api/issues/6649) | open | 确认存在 | P2 | 有（范围见详情） |

## 逐项证据与建议

### 001 · #7672 · model list: `owned_by` shows `advanced_custom` instead of name for multiple advanced_custom channels.

**确认存在** · P3 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7672)

静态确认同一行为：模型所属方按渠道类型而非具体渠道实例名生成。advanced_custom 多渠道无法从 owned_by 区分；这是展示能力缺口，不涉及错误分流。

- [controller/model.go:133](/workspace/LemonHub/controller/model.go:133)：getPreferredModelOwners 查询并缓存的是 channelType，随后调用 channelOwnerName。
- [controller/model.go:115](/workspace/LemonHub/controller/model.go:115)：channelOwnerName 只初始化 ChannelType，返回适配器名称。
- [controller/model.go:168](/workspace/LemonHub/controller/model.go:168)：生成模型条目时把该类型名称写入 OwnedBy。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：若产品决定公开实例名，按所选渠道实例生成 owner，并处理同模型多实例及渠道信息披露约定。

### 002 · #7670 · 复杂动态计费表达式已经正确结算，但前端列表页会显示“无匹配结果”，容易误导管理员。

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7670)

原 issue 的完整多行表达式在当前本地可解析；matched_tier=standard 时返回正确 tier 和价格，未复现原报告。列表的解析失败文案仍为无匹配结果，但不能仅依据兜底分支断言该表达式触发缺陷。

- [web/src/features/usage-logs/lib/format.ts:333](/workspace/LemonHub/web/src/features/usage-logs/lib/format.ts:333)：getTieredBillingSummary 先解析表达式，再按 other.matched_tier 匹配。
- [web/src/features/usage-logs/components/columns/common-logs-columns.tsx:212](/workspace/LemonHub/web/src/features/usage-logs/components/columns/common-logs-columns.tsx:212)：仅 summary 为 null 时显示 Dynamic Pricing / No matching results。

验证／触发边界：本地运行 repro-batch1.ts，原多行表达式返回 tier.label=standard；日志见 /workspace/scratch/upstream-bug-audit/repro-batch1-ui.log。

建议：获取问题日志真实 expr_b64/matched_tier；独立改进解析失败时的说明，并优先展示实际 tier 标签。

### 003 · #7666 · [Bug] 分流图将 token_id=0 的渠道测试流量显示为 "Unknown Token"

**确认存在** · P3 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7666)

静态确认：渠道测试 TokenId 为零，Flow 名称补全跳过零值，前端将其显示为 Unknown Token。

- [controller/channel-test.go:527](/workspace/LemonHub/controller/channel-test.go:527)：写 TokenName=模型测试，但没有实际 API TokenId。
- [model/usedata_flow.go:98](/workspace/LemonHub/model/usedata_flow.go:98)：fillFlowTokenNames 跳过 TokenID==0。
- [web/src/features/dashboard/lib/flow.ts:199](/workspace/LemonHub/web/src/features/dashboard/lib/flow.ts:199)：tokenID<=0 直接返回 Unknown Token。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：将零 TokenID 标成无 API 令牌，或保存调用来源以展示模型测试。

### 004 · #7657 · [Bug] 容器正常重启时未排空在途结算和批量记账，重启后可能留下账目缺口

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7657)

静态确认正常退出仍未排空批量记账：HTTP Shutdown 完成后只落盘看板缓存，周期 batchUpdate 的余额/用量/次数缓存可能在下一轮前丢失。默认 Compose 启用了该模式。未把上游实测金额视为本地复现。

- [main.go:243](/workspace/LemonHub/main.go:243)：srv.Shutdown 后仅 SaveQuotaDataCache，随即 server exited；没有记账 batchUpdate/flush。
- [model/utils.go:33](/workspace/LemonHub/model/utils.go:33)：批量更新仅由后台无限循环 sleep(interval) 后触发，没有关闭/排空接口。
- [model/utils.go:23](/workspace/LemonHub/model/utils.go:23)：余额、Token余额、已用额度、渠道已用及次数暂存在进程内 map。
- [docker-compose.yml:37](/workspace/LemonHub/docker-compose.yml:37)：默认配置 BATCH_UPDATE_ENABLED=true。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：停止接收请求并等待结算/退款任务后，同步排空批量记账与在途flush；失败要重试/持久化或明确失败退出。

### 005 · #7646 · fixed pricing is not supported for task usage expressions

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7646)

当前本地没有上游 task usage expression 引擎及该报错；任务定价仍走经典按次/按量 ModelPriceHelperPerCall。仅配置文本动态表达式不会启用任务表达式，这是未引入功能而非同一个 fixed() 拒绝分支。

- [relay/relay_task.go:206](/workspace/LemonHub/relay/relay_task.go:206)：任务提交调用 ModelPriceHelperPerCall。
- [relay/helper/price.go:204](/workspace/LemonHub/relay/helper/price.go:204)：任务价格只读取 ModelPrice/ModelRatio，没有 task usage expression 计算。
- [relay/helper/price.go:287](/workspace/LemonHub/relay/helper/price.go:287)：tiered helper 单独面向文本 token 估算。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：后续引入任务表达式定价时，单独适配 fixed()、预扣与最终结算。

### 006 · #7640 · [求助，也可能是功能请求]希望可以实现每一个渠道都遵循`自动重试`规则

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7640)

这是改为每渠道独立重试预算的功能请求。当前同样使用整次请求的全局重试预算，但没有违反现有重试语义；issue 正文也确认这种设计。

- [controller/relay.go:200](/workspace/LemonHub/controller/relay.go:200)：重试循环由 retryParam.GetRetry()<=common.RetryTimes 控制。
- [controller/relay.go:274](/workspace/LemonHub/controller/relay.go:274)：shouldRetry 使用全局剩余预算，没有逐渠道99次的约定。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：若需要该能力，应新增清晰的每渠道预算与总预算，防止乘法放大请求。

### 007 · #7634 · [Bug] Web: a failed async chunk load shows a "500" error page instead of reloading

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7634)

静态确认 ChunkLoadError 仍进入 GeneralError 并被显示为500；本地无 Rspack chunk 专用一次重载恢复逻辑。TanStack 当前模块失载识别只覆盖原生 import 错误文本。

- [web/src/routes/__root.tsx:153](/workspace/LemonHub/web/src/routes/__root.tsx:153)：根路由 errorComponent=GeneralError。
- [web/src/features/errors/general-error.tsx:62](/workspace/LemonHub/web/src/features/errors/general-error.tsx:62)：无法取得 response.status 时渲染500。
- [web/src/main.tsx:99](/workspace/LemonHub/web/src/main.tsx:99)：创建路由器，没有针对 Rspack ChunkLoadError 的恢复处理。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：识别资源块加载失败，按build/session限制一次重载并提供失败重试提示，避免把浏览器资源错误标成HTTP500。

### 008 · #7632 · StreamScannerHandler 中 end_reason 存在竞态，导致 scanner_error 被 client_gone 覆盖

**疑似** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7632)

代码存在终止原因先写者获胜：请求取消与 scanner_error 同时发生时可能保留 client_gone。但上游断流本身并不会必然取消下游 c.Request.Context；收到 http2 body closed 也可能正是 cleanup 主动关闭上游的结果。未据原统计数据确认故障因果。

- [relay/common/stream_status.go:45](/workspace/LemonHub/relay/common/stream_status.go:45)：SetEndReason 使用 sync.Once，后写 scanner_error 不会覆盖已记录原因。
- [relay/helper/stream_scanner.go:283](/workspace/LemonHub/relay/helper/stream_scanner.go:283)：scanner.Err 在扫描协程记录 scanner_error。
- [relay/helper/stream_scanner.go:298](/workspace/LemonHub/relay/helper/stream_scanner.go:298)：仅下游请求 Context.Done 触发 client_gone，随后 cleanup 关闭上游Body。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：同时记录取消来源和扫描错误/时序，设计主因优先级；用受控独立下游取消与上游断流复现实验先证实。

### 009 · #7626 · [Bug] gpt-6-sol / gpt-6-luna 参数兼容问题：`max_tokens` 未转换

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7626)

本地已移植 GPT-6 Sol/Luna 能力修复，含日期快照；实际适配器会把 max_tokens 移至 max_completion_tokens，保留显式零值。

- [relaykit/dto/openai_request.go:264](/workspace/LemonHub/relaykit/dto/openai_request.go:264)：isGPT6SolLuna 明确识别 sol/luna，设置 UseMaxCompletionTokens=true。
- [relay/channel/openai/adaptor.go:373](/workspace/LemonHub/relay/channel/openai/adaptor.go:373)：目标 MaxCompletionTokens 为空时复制 MaxTokens。

验证／触发边界：运行 repro-batch1.go，Sol/Luna 的 UseMaxCompletionTokens 均为 true；repro-batch1.log 保留输出。

建议：无需重复移植 d0cb7347c；保留当前能力与显式零值回归。

### 010 · #7619 · Docker Compose 部署  pg 密码问题

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7619)

现有证据是 PostgreSQL SASL 密码不匹配，未给可复现的脱敏DSN/实际密码和首次初始化状态。本地 DSN 来源为环境变量，Compose 的持久卷会保留数据库原密码，未发现代码锁定123456。特殊字符URL编码或已初始化卷均可能造成该现象。

- [docker-compose.yml:29](/workspace/LemonHub/docker-compose.yml:29)：应用 SQL_DSN 含独立连接密码。
- [docker-compose.yml:83](/workspace/LemonHub/docker-compose.yml:83)：POSTGRES_PASSWORD 用于数据库容器初始化。
- [docker-compose.yml:86](/workspace/LemonHub/docker-compose.yml:86)：pg_data 持久化数据库，改环境变量不等价于更改已有数据库密码。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：用全新隔离卷和非默认脱敏密码核对；对已有库执行正规密码轮换，并检查DSN百分号编码。

### 011 · #7618 · newapi 很多人机账户怎么批量删除

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7618)

当前前端已有选中用户批量删除能力，无需逐个手工点击。过滤增强部分属于功能请求。

- [web/src/features/users/components/data-table-bulk-actions.tsx:81](/workspace/LemonHub/web/src/features/users/components/data-table-bulk-actions.tsx:81)：handleDeleteAll 调用 handleBatchDeleteUsers(selectedIds)。
- [web/src/features/users/components/data-table-bulk-actions.tsx:157](/workspace/LemonHub/web/src/features/users/components/data-table-bulk-actions.tsx:157)：批量永久删除带人数及确认对话框。
- [web/src/features/users/components/users-table.tsx:248](/workspace/LemonHub/web/src/features/users/components/users-table.tsx:248)：用户表挂载批量操作工具栏。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：无需为批量删除重复开发；海量用户的筛选/跨页选择另行按需求设计。

### 012 · #7613 · [Bug] PassKey验证逻辑矛盾，root无法在多台设备上登录

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7613)

本地密码登录只检查启用的TOTP 2FA；单独绑定Passkey不会把密码登录改成Passkey挑战，因此不存在所述root只能一台设备登录路径。上游评论也把其原实现解释为安全设计。

- [controller/user.go:78](/workspace/LemonHub/controller/user.go:78)：密码验证后调用 IsTwoFAEnabled。
- [controller/user.go:102](/workspace/LemonHub/controller/user.go:102)：仅返回 require_2fa 挑战。
- [controller/user.go:114](/workspace/LemonHub/controller/user.go:114)：未启用2FA直接 setupLogin，没有Passkey强制分支。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：保留本地登录策略；多设备Passkey可用同步凭据或跨设备认证。

### 013 · #7612 · [Bug] 客户端时区与服务器时区不匹配时，无法进行修改密码、PassKey等操作

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7612)

不能确认是时区缺陷：issue 承认调整实际时钟后恢复，评论指出时钟偏差。当前安全验证使用服务端签发/验证的proof_token，前端验证钩子直接传proof、不比较客户端过期时间；未复现跨时区操作失败。

- [web/src/features/auth/secure-verification/hooks/use-secure-verification.ts:159](/workspace/LemonHub/web/src/features/auth/secure-verification/hooks/use-secure-verification.ts:159)：验证成功后直接使用 proof.proof_token 调用操作。
- [service/auth_token.go:137](/workspace/LemonHub/service/auth_token.go:137)：proof过期时间来自服务端time.Now。
- [service/auth_token.go:205](/workspace/LemonHub/service/auth_token.go:205)：服务端JWT校验有效期并允许5秒leeway。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：分别验证仅改时区与实际时钟偏差，补充失败响应体；不要通过取消安全有效期验证解决。

### 014 · #7607 · TLS_INSECURE_SKIP_VERIFY=true 时"上游价格同步"必失败：models.dev 返回 h2 乱码、basellm.github.io EOF

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7607)

本地相关HTTP客户端已克隆TLS配置，HTTP/2不会再污染全局 InsecureTLSConfig.NextProtos。

- [service/http_client.go:120](/workspace/LemonHub/service/http_client.go:120)：中继 transport 使用 InsecureTLSConfig.Clone()。
- [controller/ratio_sync.go:202](/workspace/LemonHub/controller/ratio_sync.go:202)：价格同步 transport 同样克隆，避免共享可变指针。
- [common/init.go:103](/workspace/LemonHub/common/init.go:103)：默认transport也使用Clone。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：无需重复修复，保持所有transport各自的TLS配置副本。

### 015 · #7603 · [bug]  web: long messages can't be wrap in tooltip at channel table item - "status" in "Auto-disabled" status

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7603)

本地自动禁用原因提示已增加任意位置换行，已有对应回归用例。

- [web/src/features/channels/components/channels-columns.tsx:984](/workspace/LemonHub/web/src/features/channels/components/channels-columns.tsx:984)：Tooltip限制max-w-xs。
- [web/src/features/channels/components/channels-columns.tsx:987](/workspace/LemonHub/web/src/features/channels/components/channels-columns.tsx:987)：原因文本使用wrap-anywhere。
- [web/src/features/channels/components/__tests__/channels-columns-status-tooltip-content.test.tsx:63](/workspace/LemonHub/web/src/features/channels/components/__tests__/channels-columns-status-tooltip-content.test.tsx:63)：已有无断点长串换行测试。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：无需重复移植811212067。

### 016 · #7599 · Billing history shows the top-up status in English in every language (Success, Pending, Expired)

**确认存在** · P3 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7599)

静态确认账单历史直接把英文状态label交给StatusBadge，中文词条虽存在却未调用t。

- [web/src/features/wallet/components/dialogs/billing-history-dialog.tsx:222](/workspace/LemonHub/web/src/features/wallet/components/dialogs/billing-history-dialog.tsx:222)：label={statusConfig.label}。
- [web/src/i18n/locales/zh.json:4844](/workspace/LemonHub/web/src/i18n/locales/zh.json:4844)：已有Success对应成功，无需新增字典才能修复。
- [web/src/components/status-badge.tsx:173](/workspace/LemonHub/web/src/components/status-badge.tsx:173)：StatusBadge直接渲染label，不会在组件内部补翻译。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：渲染处使用t(statusConfig.label)，覆盖Success/Pending/Expired。

### 017 · #7595 · 主节点服务重启失败，报错重复创建索引

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7595)

上游评论已定位OceanBase把唯一列column_key报PRI，触发其GORM MySQL驱动重复建索引。本项目承诺SQLite/MySQL/PostgreSQL，未宣称OceanBase；不能把OceanBase方言兼容问题算成支持数据库上的同bug。本地仍需独立迁移验证，不能据关闭状态判已修。

- [go.mod:59](/workspace/LemonHub/go.mod:59)：本地mysql驱动v1.4.3，与评论分析v1.5.7也不同。
- [model/main.go:291](/workspace/LemonHub/model/main.go:291)：本地使用GORM AutoMigrate进行实际模型迁移。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：若将OceanBase纳入支持范围，再以其真实information_schema做幂等迁移测试并适配。

### 018 · #7593 · [Bug] Anthropic /v1/messages tools 字段未转换为 OpenAI 格式，上游报 400 field Tools[0].Type invalid

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7593)

当前本地非透传Claude→OpenAI转换确实将name/input_schema包装为type:function/function.parameters；实际调用转换器输出合法，未复现所述未转换。全局或渠道透传是另一个配置分支。

- [relay/channel/openai/adaptor.go:68](/workspace/LemonHub/relay/channel/openai/adaptor.go:68)：ConvertClaudeRequest调用service.ConvertRequest到OpenAI格式。
- [relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:81](/workspace/LemonHub/relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:81)：逐工具组装Type=function和Function.Parameters。
- [relay/channel/openai/adaptor.go:175](/workspace/LemonHub/relay/channel/openai/adaptor.go:175)：Claude输入在OpenAI渠道走chat/completions端点。

验证／触发边界：运行 repro-batch1.go，工具weather输出type:function/function.parameters；详见repro-batch1.log。

建议：若线上仍出现，检查实际全局/渠道透传、参数覆盖与出站JSON；当前转换器无需此修复。

### 019 · #7591 · [Bug] 多密钥渠道经任务插件提交时，消费日志缺少 key 序号（multi_key_index）

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7591)

静态确认本地普通异步视频/图片任务成功消费日志同样缺密钥序号。任务插件本身未引入，但issue明确还覆盖所有异步任务；本地LogTaskConsumption单独组装Other，没有复制多密钥context。

- [service/task_billing.go:172](/workspace/LemonHub/service/task_billing.go:172)：构造任务other仅写is_task、request_path、定价及模型信息。
- [service/task_billing.go:187](/workspace/LemonHub/service/task_billing.go:187)：仅补quota饱和信息，随后写消费日志，没有is_multi_key或multi_key_index。
- [service/log_info_generate.go:100](/workspace/LemonHub/service/log_info_generate.go:100)：普通文本日志在这里写多密钥序号，任务不走该函数。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：抽取通用管理审计字段填充，并把提交时key序号快照到Task以用于轮询差额/退款日志。

### 020 · #7587 · 给用户扣了个雷霆大费用

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7587)

报告没有可核对请求、上游usage、倍率与最终日志原始字段，不能判断本地存在超扣；评论称无usage时的本地估算。当前也有缺usage回退估算，但这仅说明可能路径，不等于同一计费故障。

- [service/text_quota.go:249](/workspace/LemonHub/service/text_quota.go:249)：usage为nil时使用GetEstimatePromptTokens。
- [service/text_quota.go:399](/workspace/LemonHub/service/text_quota.go:399)：结算通过effectiveBillingUsage选择计费usage。
- [service/text_quota.go:451](/workspace/LemonHub/service/text_quota.go:451)：按计算结果进行最终SettleBilling。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：取得脱敏原请求、usage、账单倍率和billing_path，区分预扣、估算与最终实扣后再定缺陷。

### 021 · #7585 · Web UI: the bundled Public Sans font is never used (--font-sans asks for 'Public Sans', the package registers 'Public Sans Variable')

**确认存在** · P3 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7585)

静态确认变量字体包注册Public Sans Variable，但主题引用Public Sans；默认sans因此找不到该字体而回退。

- [web/src/styles/index.css:22](/workspace/LemonHub/web/src/styles/index.css:22)：导入@fontsource-variable/public-sans。
- [web/src/styles/theme.css:22](/workspace/LemonHub/web/src/styles/theme.css:22)：--font-sans仅写Public Sans,sans-serif。
- [web/node_modules/@fontsource-variable/public-sans/index.css:3](/workspace/LemonHub/web/node_modules/@fontsource-variable/public-sans/index.css:3)：安装包@font-face的family实际为Public Sans Variable。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：主题font-sans优先使用Public Sans Variable，再保留fallback。

### 022 · #7584 · Six UI labels are hard-coded in English or built from fragments (usage-log Tokens header, top-up presets, plan status, redemption dialog, price note, inviter ID)

**确认存在** · P3 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7584)

静态确认该组国际化缺口仍存在：Tokens列头、充值Pay/Save/Minimum及OFF硬编码；订阅状态使用Enable/Disable动词；价格注释仍由碎片组成。至少这些明确子项与原issue一致。

- [web/src/features/usage-logs/components/columns/common-logs-columns.tsx:647](/workspace/LemonHub/web/src/features/usage-logs/components/columns/common-logs-columns.tsx:647)：header直接为Tokens。
- [web/src/features/wallet/components/recharge-form-card.tsx:271](/workspace/LemonHub/web/src/features/wallet/components/recharge-form-card.tsx:271)：Pay和Save为裸英文。
- [web/src/features/wallet/components/recharge-form-card.tsx:300](/workspace/LemonHub/web/src/features/wallet/components/recharge-form-card.tsx:300)：Minimum占位符为裸英文模板。
- [web/src/features/wallet/lib/format.ts:72](/workspace/LemonHub/web/src/features/wallet/lib/format.ts:72)：折扣直接返回百分比OFF。
- [web/src/features/subscriptions/components/subscriptions-columns.tsx:118](/workspace/LemonHub/web/src/features/subscriptions/components/subscriptions-columns.tsx:118)：状态Badge使用t(Enable)。
- [web/src/features/pricing/components/model-details.tsx:1020](/workspace/LemonHub/web/src/features/pricing/components/model-details.tsx:1020)：价格单位说明由翻译片段、变量和tokens拼接。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：用完整带插值翻译键，状态使用Enabled/Disabled；同时核对兑换与邀请人相关片段。

### 023 · #7580 · Dashboard charts draw January before December when the range crosses a year (time labels sorted as text)

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7580)

已本地复现：跨年两条数据2025-12-31和2026-01-01被生成01-01在前、12-31在后的图表；年月日标签排序导致时序错乱。

- [web/src/features/dashboard/lib/charts.ts:230](/workspace/LemonHub/web/src/features/dashboard/lib/charts.ts:230)：先按不含年份的formatChartTime聚合。
- [web/src/features/dashboard/lib/charts.ts:262](/workspace/LemonHub/web/src/features/dashboard/lib/charts.ts:262)：时间键使用字符串sort。
- [web/src/features/dashboard/lib/charts.ts:795](/workspace/LemonHub/web/src/features/dashboard/lib/charts.ts:795)：用户趋势也按格式化时间字符串排序。

验证／触发边界：运行repro-batch1.ts调用真实processChartData；repro-batch1-ui.log中barData时间从01-01开始再到12-26…12-31。

建议：用原始桶起始时间戳聚合排序，仅在展示阶段格式化；跨年时标签包含年以避免碰撞。

### 024 · #7578 · fix(relayconvert): Responses→Chat converter delivery image_url as text，causing 400

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7578)

已本地复现Responses→Chat将input_image.image_url字符串原样发出，且同级detail被丢失；严格Chat上游要求image_url对象会拒绝该请求。

- [relaykit/relayconvert/internal/oai_responses/to_oai_chat_req.go:280](/workspace/LemonHub/relaykit/relayconvert/internal/oai_responses/to_oai_chat_req.go:280)：input_image通过responsesImagePartToChatImageURL写入Chat内容。
- [relaykit/relayconvert/internal/oai_responses/to_oai_chat_req.go:452](/workspace/LemonHub/relaykit/relayconvert/internal/oai_responses/to_oai_chat_req.go:452)：存在image_url键时直接返回原值，未包装url或保留同级detail。

验证／触发边界：运行repro-batch1.go，输出{"type":"image_url","image_url":"https://example.test/a.png"}而非对象；详见repro-batch1.log。未访问真实供应商。

建议：将字符串归一为{url:...}并保留detail，同时覆盖function_call_output媒体复用路径。

### 025 · #7576 · Delete Account dialog: confirmation label built from fragments reads "类型 admin 以确认" (wrong in 6 of 7 languages)

**确认存在** · P3 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7576)

静态确认删除账户确认说明仍由Type、用户名、to confirm碎片拼接；中文Type词条为名词类型。

- [web/src/features/profile/components/dialogs/delete-account-dialog.tsx:145](/workspace/LemonHub/web/src/features/profile/components/dialogs/delete-account-dialog.tsx:145)：{t(Type)} username {t(to confirm)}。
- [web/src/i18n/locales/zh.json:5331](/workspace/LemonHub/web/src/i18n/locales/zh.json:5331)：Type翻译为类型。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：用完整插值/Trans键表达请输入用户名以确认，保留用户名加粗。

### 026 · #7574 · Two-factor setup dialog shows "Step1of 3:Scan QR Code" (no spaces, word order not translatable)

**确认存在** · P3 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7574)

静态确认2FA设置步骤说明没有分隔，Step、数字、of 3:、步骤名四片直接相邻；也无法按语言调整词序。

- [web/src/features/profile/components/dialogs/two-fa-setup-dialog.tsx:140](/workspace/LemonHub/web/src/features/profile/components/dialogs/two-fa-setup-dialog.tsx:140)：相邻渲染t(Step)、step+1、t(of 3:)及stepLabel，无空格。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：使用完整Step {{current}} of {{total}}: {{label}}翻译键。

### 027 · #7573 · Insufficient-quota errors are always Chinese, ignoring the user's language

**确认存在** · P3 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7573)

静态确认额度不足错误仍为硬编码中文，未通过后端i18n。上游评论指出错误文案也被自动禁用逻辑匹配，因此修复需兼容机器识别。

- [service/billing_session.go:262](/workspace/LemonHub/service/billing_session.go:262)：fmt.Errorf直接写用户额度不足中文。
- [service/billing_session.go:411](/workspace/LemonHub/service/billing_session.go:411)：另一钱包预扣分支同样硬编码中文。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：保留稳定错误码/内部兼容识别，再针对客户端语言生成展示信息，避免直接替换破坏上下游自动禁用。

### 028 · #7564 · OpenAI developer messages are merged into Gemini systemInstruction with no notice

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7564)

静态确认描述的语义降级：OpenAI system与developer文本被合并到单一Gemini systemInstruction，未保留角色边界。Gemini没有developer角色；是否拒绝或告警需要产品策略，不能假设存在原生等价映射。

- [relaykit/relayconvert/internal/oai_chat/to_gemini_chat_req.go:240](/workspace/LemonHub/relaykit/relayconvert/internal/oai_chat/to_gemini_chat_req.go:240)：system与developer进入完全同一分支。
- [relaykit/relayconvert/internal/oai_chat/to_gemini_chat_req.go:404](/workspace/LemonHub/relaykit/relayconvert/internal/oai_chat/to_gemini_chat_req.go:404)：strings.Join(systemContent,"\n")写入单个systemInstruction。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：文档/转换元信息明确告知有损映射，或提供严格模式拒绝无法保留的角色语义。

### 029 · #7563 · Claude Messages：message 级 output_config（per-message effort）转发时被丢弃，上游返回 400

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7563)

本地ClaudeMessage DTO已有message级OutputConfig RawMessage，原生Claude转换保留该字段，并有对应effort-only系统消息回归。

- [relaykit/dto/claude.go:124](/workspace/LemonHub/relaykit/dto/claude.go:124)：ClaudeMessage声明OutputConfig json:output_config,omitempty。
- [relay/channel/claude/relay_claude_test.go:435](/workspace/LemonHub/relay/channel/claude/relay_claude_test.go:435)：TestConvertClaudeRequestPreservesMessageOutputConfig覆盖空content系统消息。
- [relay/channel/claude/relay_claude_test.go:459](/workspace/LemonHub/relay/channel/claude/relay_claude_test.go:459)：验证上游消息effort=high，普通消息不新增该字段。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：无需重复移植c2b7a9a9e。

### 030 · #7558 · rc40中调用k3模型时，如果请求中有视频或者图片时，预处理会很慢，比上游首字及耗时能多出5-10秒

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7558)

本地的确有转发前媒体下载/Token估算，可能增加首字等待；但没有原URL、大小、开关、直连基线和剖析，不能确认固定5–10秒回归或归责网络。

- [service/token_counter.go:238](/workspace/LemonHub/service/token_counter.go:238)：是否抓媒体由GetMediaToken等配置决定。
- [service/token_counter.go:261](/workspace/LemonHub/service/token_counter.go:261)：未知类型或允许URL媒体估算时同步LoadFileSource。
- [service/token_counter.go:276](/workspace/LemonHub/service/token_counter.go:276)：随后按图片等类型计算token。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：使用相同媒体请求分别关闭/启用本地估算并记录下载与计数耗时，再决定避免重复抓取或并行化。

### 031 · #7556 · jev 任务插件创建的渠道，在模型广场的端点类型仍为 openai

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7556)

本项目未引入jev/task plugin动态任务渠道机制，因此不存在插件端点元数据未进入模型广场的同一路径。现有任务适配器为硬编码供应商枚举。

- [relay/relay_adaptor.go:144](/workspace/LemonHub/relay/relay_adaptor.go:144)：GetTaskAdaptor仅列举Suno及各内置渠道，无task plugin分支。
- [docs/upstream-rc41-backports.md:79](/workspace/LemonHub/docs/upstream-rc41-backports.md:79)：任务插件保留为独立后续适配范围。

验证／触发边界：静态核对；未启动线上服务或真实供应商联调。

建议：未来引入插件时一并接入模型支持端点的元数据。

### 032 · #7551 · [Bug] Claude 上游在输出前 refusal（content 为空、output_tokens 0）时仍按 input_tokens 扣费，Anthropic 对此不计费

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7551)

本地已验证上游refusal标记不影响计费：零输出但input_tokens=412、模型倍率5仍计算2060额度并标为可计费。拒绝标记只进入日志，PostTextConsumeQuota照常结算。

- [relay/channel/claude/relay-claude.go:30](/workspace/LemonHub/relay/channel/claude/relay-claude.go:30)：refusal仅设置ContextKeyAdminRejectReason。
- [service/text_quota.go:75](/workspace/LemonHub/service/text_quota.go:75)：hasBillableUsage只根据总tokens/工具附加费。
- [service/text_quota.go:407](/workspace/LemonHub/service/text_quota.go:407)：读取拒绝原因后照常calculateTextQuotaSummary。
- [service/text_quota.go:451](/workspace/LemonHub/service/text_quota.go:451)：照常SettleBilling；拒绝原因到480行才写日志。

验证／触发边界：Go overlay单测 TestAudit7551RefusalChargesInput 通过，实际输出refusal=true prompt=412 completion=0 quota=2060 billable=true；日志repro-7551.log。验证了本地计算路径，未实际充值扣款/调用Anthropic。

建议：为已确认输出前拒绝且无内容/输出的Anthropic响应标记不可计费，并贯通原生/转换/流式、按次及表达式结算；保留真实usage用于审计。

### 033 · #7549 · [Bug] Claude Messages 缺少 server_tool_use.id 时 Responses 转换 panic 且失败流仍计费

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7549)

上游新hosted-tool桥接路径未移植：本地无StartHostedTool/to_oai_responses_hosted_stream实现。当前Responses→Claude只保留function工具，web_search被跳过，故不会进入所述缺ID桥接panic。静默丢hosted工具是另一个兼容缺口，不能宣称协议功能已经修好。

- [relaykit/relayconvert/internal/oai_responses/req_helpers.go:111](/workspace/LemonHub/relaykit/relayconvert/internal/oai_responses/req_helpers.go:111)：非function工具被continue，包括web_search。
- [relaykit/relayconvert/internal/oai_responses/to_claude_messages_req.go:50](/workspace/LemonHub/relaykit/relayconvert/internal/oai_responses/to_claude_messages_req.go:50)：只将提取出的函数声明编码为Claude工具。
- [relaykit/relayconvert/text_converter_registry.go:172](/workspace/LemonHub/relaykit/relayconvert/text_converter_registry.go:172)：Claude→Responses响应走Claude→Chat→Responses旧多跳链。

验证／触发边界：运行repro-batch1.go，含web_search和max_output_tokens=64的Responses请求转换成功但Claude请求无tools；repro-batch1.log有输出。

建议：后续引入hosted工具桥接时必须覆盖缺ID、终态错误和失败结算；当前可明确报不支持避免静默丢工具。

### 034 · #7546 · gpt-6-luna测试失败

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7546)

本地已移植 GPT-6 Sol/Luna 能力修复，含日期快照；实际适配器会把 max_tokens 移至 max_completion_tokens，保留显式零值。

- [relaykit/dto/openai_request.go:264](/workspace/LemonHub/relaykit/dto/openai_request.go:264)：isGPT6SolLuna 明确识别 sol/luna，设置 UseMaxCompletionTokens=true。
- [relay/channel/openai/adaptor.go:373](/workspace/LemonHub/relay/channel/openai/adaptor.go:373)：目标 MaxCompletionTokens 为空时复制 MaxTokens。

验证／触发边界：运行 repro-batch1.go，Sol/Luna 的 UseMaxCompletionTokens 均为 true；repro-batch1.log 保留输出。

建议：无需重复移植 d0cb7347c；保留当前能力与显式零值回归。

### 035 · #7545 · 设置模型自定义图标后 web 页面持续 429：图标下拉一次加载全部 LobeHub 图标 chunk，默认 GLOBAL_WEB_RATE_LIMIT 被打满

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7545)

本地未移植rc.39图标懒加载；同IP静态资源共限流的设计存在，但不能据此认定打开图标下拉必触发429。

- [web/src/lib/lobe-icon.tsx:28](/workspace/LemonHub/web/src/lib/lobe-icon.tsx:28)：仍用同步 import * as LobeIcons，未引入每图标动态import。
- [router/web-router.go:40](/workspace/LemonHub/router/web-router.go:40)：web限流仍在static前，但issue所需数百个独立图标chunk请求触发条件不成立。

验证／触发边界：静态核对；未进行真实供应商/浏览器端到端验证。

### 036 · #7540 · [Bug] 流式图片请求在 Token 统计阶段重复下载已知类型图片，导致内网 URL 被 SSRF 拦截并返回 500

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7540)

静态确认同根因：Qwen已知图片URL在启用媒体token统计的流模式仍被下载，私网SSRF拒绝会中断请求。

- [service/token_counter.go:261](/workspace/LemonHub/service/token_counter.go:261)：已知FileType只要URL且shouldFetchFiles仍强制LoadFileSource；失败直接return。
- [service/token_counter.go:249](/workspace/LemonHub/service/token_counter.go:249)：默认非流式可跳过媒体下载，流式则下载。
- [service/token_counter.go:286](/workspace/LemonHub/service/token_counter.go:286)：非OpenAI文本模型的图片最终仅固定加520，下载对这一估算并非必要。

验证／触发边界：静态调用链确认；未接入私网真实Qwen服务。

建议：仅需要尺寸/内容的分支获取媒体；保留未知类型检测和SSRF校验。

### 037 · #7511 · [Bug] 非 Root 管理员可见系统设置入口，但访问后被重定向到 403

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7511)

当前菜单和路由权限一致；rc41低冲突移植记录中的dfd3cd893已落地。

- [web/src/hooks/use-sidebar-data.ts:195](/workspace/LemonHub/web/src/hooks/use-sidebar-data.ts:195)：System Settings菜单requiredRole=ROLE.SUPER_ADMIN。
- [web/src/routes/_authenticated/system-settings/route.tsx:29](/workspace/LemonHub/web/src/routes/_authenticated/system-settings/route.tsx:29)：父路由同样限制SUPER_ADMIN。

验证／触发边界：静态核对；未进行真实供应商/浏览器端到端验证。

### 038 · #7506 · [Bug] v1.0.0-rc.39 动态加载供应商图标失败，多个页面仅显示首字母

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7506)

未采用引入问题的9c3d3aeb3懒加载架构；不能把其它原因的fallback图标算作同一bug。

- [web/src/lib/lobe-icon.tsx:28](/workspace/LemonHub/web/src/lib/lobe-icon.tsx:28)：同步导入整套LobeIcons，不存在issue所述lazy chunk失败路径。
- [web/src/lib/lobe-icon.tsx:116](/workspace/LemonHub/web/src/lib/lobe-icon.tsx:116)：直接从同步注册表取组件。

验证／触发边界：静态核对；未进行真实供应商/浏览器端到端验证。

### 039 · #7503 · 模型徽标行高过小导致 g/p 下伸笔画被裁切

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7503)

本地ModelBadge不同于上游问题代码，缺少造成下伸笔画裁切的line-clamp-2条件。

- [web/src/features/usage-logs/components/model-badge.tsx:150](/workspace/LemonHub/web/src/features/usage-logs/components/model-badge.tsx:150)：模型文字使用whitespace-nowrap，没有line-clamp-2的overflow裁切。

验证／触发边界：静态核对；未进行真实供应商/浏览器端到端验证。

### 040 · #7498 · [BUG] Responses→Chat 转换器把 function_call_output 的图片内容拍平成 base64 文本，触发上游 prompt 超限

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7498)

本地最小程序确认Responses工具图片输出变成tool.content内的base64 JSON文本，导致视觉信息丢失与文本token膨胀。上游已修不代表本地已修。

- [relaykit/relayconvert/internal/oai_responses/to_oai_chat_req.go:222](/workspace/LemonHub/relaykit/relayconvert/internal/oai_responses/to_oai_chat_req.go:222)：function_call_output把output交给responseToolOutputToChatContent。
- [relaykit/relayconvert/internal/oai_responses/to_oai_chat_req.go:547](/workspace/LemonHub/relaykit/relayconvert/internal/oai_responses/to_oai_chat_req.go:547)：非字符串output整体Marshal为文本，未提取媒体块。

验证／触发边界：已复现，repro-batch2.log的7498行：tool.content包含data:image/png;base64,YWJj，且没有后续媒体消息。

建议：保留工具纯文本，将媒体块延迟到整个tool batch结束后作为user多模态内容输出，保持工具消息连续。

### 041 · #7495 · [BUG] oai_chat_to_oai_responses 转换器：流中途出现 finish_reason 后 reasoning delta 失去 active item

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7495)

本地最小帧序列确认：若上游在finish_reason后继续reasoning，同一已关闭item会收到delta。该输入不是常规单choice标准终止序列，影响限定于此类兼容上游。

- [relaykit/relayconvert/internal/oai_chat/to_oai_responses_stream_resp.go:108](/workspace/LemonHub/relaykit/relayconvert/internal/oai_chat/to_oai_responses_stream_resp.go:108)：遇非空finish_reason调用doneDeltaEvents。
- [relaykit/relayconvert/internal/oai_chat/to_oai_responses_stream_resp.go:169](/workspace/LemonHub/relaykit/relayconvert/internal/oai_chat/to_oai_responses_stream_resp.go:169)：只有!reasoningStarted时开启item。
- [relaykit/relayconvert/internal/oai_chat/to_oai_responses_stream_resp.go:262](/workspace/LemonHub/relaykit/relayconvert/internal/oai_chat/to_oai_responses_stream_resp.go:262)：关闭reasoning后reasoningDone=true，不复位started或创建新段。

验证／触发边界：已复现，repro-batch2.log：7495 closed-item reasoning delta count=1。

建议：遇后续delta重新创建reasoning item并维护summary part生命周期，或明确拒绝不合法上游序列。

### 042 · #7492 · 后台显示调用的模型名不一致，glm-5.3-flash变成了glm-5-3-flash

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7492)

评论说明上游新版展示的是上游响应的model，并非网关改请求名；本地未引入该响应模型显示机制，也未发现把glm-5.3-flash统一改写为5-3的代码。

- [service/log_info_generate.go:86](/workspace/LemonHub/service/log_info_generate.go:86)：本地仅在IsModelMapped时记录upstream_model_name。
- [web/src/features/usage-logs/lib/format.ts:251](/workspace/LemonHub/web/src/features/usage-logs/lib/format.ts:251)：actualModel来自映射后的upstream_model_name，无新响应模型展示字段。

验证／触发边界：静态核对；未进行真实供应商/浏览器端到端验证。

### 043 · #7489 · fix(relay): terminal stream chunk with finish_reason is dropped when client does not request usage

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7489)

本地最小测试已复现终止帧被吞，导致要求finish_reason的客户端误报中断并重试。

- [relay/channel/openai/helper.go:155](/workspace/LemonHub/relay/channel/openai/helper.go:155)：有效usage且客户端未请求usage时，仅按content/reasoning非空决定是否发送尾帧，忽略finish_reason/tool_calls。

验证／触发边界：父agent overlay TestAudit7489TerminalChoiceMustReachClient失败：shouldSend=false。日志repro/root-repro.log。

建议：有效finish_reason或tool_calls等协议字段也必须保留；只剥离客户端未请求的usage。

### 044 · #7486 · 流式 /v1/responses：HTTP 200 开流后 eof/未完成仍扣费，同会话再请求会被记第二笔,很紧急！！！！！！！！！！！！！

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7486)

部分流已生成后EOF仍计量的行为存在；本地已限制纯生命周期事件无生成不计费，并标Estimated。issue中的第二枪是新的请求，不能认定同一请求被重复结算。是否应免费、是否应新增对外计量信号属于产品规则，缺少同请求重复扣费证据。

- [relay/channel/openai/responses_usage.go:129](/workspace/LemonHub/relay/channel/openai/responses_usage.go:129)：缺usage时按实际观察到的生成量估算。
- [relay/channel/openai/responses_usage.go:148](/workspace/LemonHub/relay/channel/openai/responses_usage.go:148)：仅已观察到生成时计入prompt估算。
- [relay/responses_handler.go:168](/workspace/LemonHub/relay/responses_handler.go:168)：成功DoResponse后调用PostTextConsumeQuota。

验证／触发边界：静态核对未正常终态仍可计量；未做真实上游计费对账。

建议：明确中断后的收费契约与日志状态；若要客户端幂等重试需单独设计，不能直接把第二请求视为同一账单。

### 045 · #7485 · [Bug] v1.0.0-rc.38 CC Switch 模型选择下拉选择框定位异常

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7485)

已移植2d7aef741并有专门CC Switch回归用例；旧版非Portal下拉定位已替换。

- [web/src/features/keys/components/dialogs/cc-switch-dialog.tsx:115](/workspace/LemonHub/web/src/features/keys/components/dialogs/cc-switch-dialog.tsx:115)：使用useComboboxAnchor与共享Combobox。
- [web/src/features/keys/components/dialogs/cc-switch-dialog.tsx:148](/workspace/LemonHub/web/src/features/keys/components/dialogs/cc-switch-dialog.tsx:148)：ComboboxContent anchor={anchor}，采用已有Portal定位。

验证／触发边界：静态核对；未进行真实供应商/浏览器端到端验证。

### 046 · #7479 · [Bug] “高级自定义”渠道的前端缺少了 Responses -> Message 等转换选项

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7479)

静态确认用户无法在高级自定义选中已有relaykit的Responses→Messages能力；本地缺口同时在前端和宿主接线，不能只补一个下拉项。

- [web/src/features/channels/lib/advanced-custom.ts:34](/workspace/LemonHub/web/src/features/channels/lib/advanced-custom.ts:34)：转换选项缺Responses→Claude Messages等组合。
- [relaykit/relayconvert/request_registry.go:71](/workspace/LemonHub/relaykit/relayconvert/request_registry.go:71)：relaykit已有内部ResponsesToClaude转换器。
- [relay/channel/advancedcustom/adaptor.go:285](/workspace/LemonHub/relay/channel/advancedcustom/adaptor.go:285)：宿主advancedcustom响应switch同样未开放该组合。

验证／触发边界：静态确认；未运行实际渠道。

建议：同时注册宿主转换路由、请求/响应处理、前端选项及路径约束后再开放。

### 047 · #7478 · Task Plugin 创建异步任务时仅接受 HTTP 200，合法的 201 Created 被误判为 channel error

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7478)

本地没有Task Plugin、buildSubmitRequest或parseSubmitResponse，原issue复现路径不可用。内置任务链有同类仅接受200代码，但未核实某已支持provider需201/202，单列潜在边界，不算同issue确认。

- [constant/channel.go:183](/workspace/LemonHub/constant/channel.go:183)：本地渠道注册包含Codex/AdvancedCustom等，没有Task Plugin。
- [relay/relay_task.go:248](/workspace/LemonHub/relay/relay_task.go:248)：内置任务共用链仍只接受200；属于同类边界，不能证明issue所述Task Plugin+腾讯wand路径可达。

验证／触发边界：静态核对；未进行真实供应商/浏览器端到端验证。

### 048 · #7475 · [BUG] 阿里通义千问渠道图片生成 n>1 时只返回最后一张图，计费仍按 n 张

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7475)

本地仍保留旧阿里转换器；上游rc39改任务插件删除旧函数的修复并未覆盖本地。n>1同choice图片仅返回最后一张。

- [relay/channel/ali/dto.go:107](/workspace/LemonHub/relay/channel/ali/dto.go:107)：每个choice只创建一个ImageData。
- [relay/channel/ali/dto.go:120](/workspace/LemonHub/relay/channel/ali/dto.go:120)：遍历各image反复覆盖同一个Url。
- [relay/channel/ali/dto.go:129](/workspace/LemonHub/relay/channel/ali/dto.go:129)：content循环结束仅append一次。
- [relay/channel/ali/image.go:339](/workspace/LemonHub/relay/channel/ali/image.go:339)：计费n倍率优先使用上游usage.image_count，未随丢图减为实际返回张数。

验证／触发边界：父agent overlay TestAudit7475PreserveEveryGeneratedImage失败：expected=2，实际仅second.png。日志repro/root-repro.log。

建议：每个图片content创建并append独立ImageData，妥善关联revised prompt并验证usage数量。

### 049 · #7470 · vLLM upstream returns "choices" as null, causing "cannot unmarshal" error in stream parsing

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7470)

本地实测choices:null和[]均成功；只有{}会报issue所述cannot unmarshal object。后者是上游不符合数组契约的负载，issue把null归因解析失败不成立，不作为同bug确认。

- [relaykit/dto/openai_response.go:148](/workspace/LemonHub/relaykit/dto/openai_response.go:148)：ChatCompletionsStreamResponse使用choices切片，标准null可正常解码。
- [relay/channel/openai/relay-openai.go:148](/workspace/LemonHub/relay/channel/openai/relay-openai.go:148)：确会把JSON类型错误记录为流soft error。

验证／触发边界：已复现对照，repro-batch2.log：null=<nil>、[]=<nil>、{}=cannot unmarshal object。

建议：若需要兼容特定vLLM错误实现，可专门容忍空对象；先保存真实SSE证据确认。

### 050 · #7467 · [Bug] Responses WebSocket 开关在 Sub2API 等渠道保存后自动关闭

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7467)

本地尚未引入Responses WebSocket开关及保存能力，全仓无该字段/supportsResponsesWebSocket函数；不存在保存true被前端改false的当前路径。

- [relaykit/dto/channel_settings.go:13](/workspace/LemonHub/relaykit/dto/channel_settings.go:13)：ChannelSettings没有responses_websocket_enabled。
- [docs/upstream-rc41-backports.md:79](/workspace/LemonHub/docs/upstream-rc41-backports.md:79)：本地记录将Responses WebSocket保留为后续独立适配。

验证／触发边界：静态核对；未进行真实供应商/浏览器端到端验证。

### 051 · #7465 · 部分模型定价转换异常

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7465)

上游issue报错来自/api/option/model_pricing/convert及模型路由检查；本地未实现此接口/检查，所以不是当前同bug。

- [router/api-router.go:290](/workspace/LemonHub/router/api-router.go:290)：option路由没有model_pricing/convert接口。
- [web/src/features/system-settings/models/model-pricing-sheet.tsx:140](/workspace/LemonHub/web/src/features/system-settings/models/model-pricing-sheet.tsx:140)：本地使用原有模型定价编辑面板，未引入上游路由验证式价格迁移入口。

验证／触发边界：静态核对；未进行真实供应商/浏览器端到端验证。

### 052 · #7456 · RequestResult.Diagnostics is discarded on the success path, so tool policy losses are never logged or returned

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7456)

部分命中：正文关键问题allowed_tools静默丢失可本地复现，required且仅允许tool_b被转换为auto且全部工具仍可选；标题所述Diagnostics被丢弃不适用于本地尚未引入该字段的架构。

- [relaykit/relayconvert/internal/shared/claude/tool_choice.go:23](/workspace/LemonHub/relaykit/relayconvert/internal/shared/claude/tool_choice.go:23)：对象tool_choice只读取function.name，不解析allowed_tools。
- [relaykit/relayconvert/internal/shared/claude/tool_choice.go:34](/workspace/LemonHub/relaykit/relayconvert/internal/shared/claude/tool_choice.go:34)：parallel_tool_calls存在时将未解析的选择回退成auto。
- [relaykit/relayconvert/request_registry.go:36](/workspace/LemonHub/relaykit/relayconvert/request_registry.go:36)：本地RequestResult没有Diagnostics字段，与issue新版诊断架构不同。

验证／触发边界：已复现，repro-batch2.log的7456：tool_choice={type:auto,disable_parallel_tool_use:true}且tools含tool_a/tool_b。

建议：保留/显式拒绝无法承载的allowed_tools与调用者限制，并加入成功路径可观察诊断；不要只移植Diagnostics记录表面改动。

### 053 · #7455 · CleanFunctionParameters strips const, additionalProperties and oneOf from tool schemas, and parametersJsonSchema is never emitted

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7455)

本地最小转换确认const约束变成{}，oneOf与additionalProperties丢失，未以parametersJsonSchema保留，也无告知。

- [relaykit/relayconvert/internal/shared/gemini/schema.go:9](/workspace/LemonHub/relaykit/relayconvert/internal/shared/gemini/schema.go:9)：允许列表无const/oneOf/additionalProperties。
- [relaykit/relayconvert/internal/shared/gemini/schema.go:53](/workspace/LemonHub/relaykit/relayconvert/internal/shared/gemini/schema.go:53)：递归只复制允许字段。
- [relaykit/relayconvert/internal/oai_chat/to_gemini_chat_req.go:194](/workspace/LemonHub/relaykit/relayconvert/internal/oai_chat/to_gemini_chat_req.go:194)：实际工具输出调用CleanFunctionParameters。

验证／触发边界：已复现，repro-batch2.log的7455：properties.value={}。

建议：用支持完整JSON Schema的parametersJsonSchema路径，或对语义损失显式诊断/拒绝。

### 054 · #7454 · Claude/OpenAI conversion drops file and document blocks, and turns url-source images into "data:;base64,%!s(<nil>)"

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7454)

三种内容丢失/破坏都在本地复现：OpenAI file_id→Claude空content，Claude document→Chat无messages，Claude URL图片→无效data URL。

- [relaykit/dto/openai_request.go:490](/workspace/LemonHub/relaykit/dto/openai_request.go:490)：file_id无FileData返回nil文件源。
- [relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:169](/workspace/LemonHub/relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:169)：URL图片仍按base64格式化Source.Data。
- [relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:160](/workspace/LemonHub/relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:160)：switch没有document分支。
- [relaykit/dto/openai_request.go:507](/workspace/LemonHub/relaykit/dto/openai_request.go:507)：MimeType无json tag泄漏成非标准字段。

验证／触发边界：已复现，repro-batch2.log三条7454：URL为data:;base64,%!s(<nil>)，document无messages，file_id的content=[]。

建议：分别支持源URL、可解析文件引用及document映射；不支持的跨provider文件ID明确报错，不要静默丢弃。

### 055 · #7435 · 更新后管理员登录提示此验证方式当前不可用。重置密码后依然无法使用

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7435)

评论最终确认上游管理员禁用全站passkey而用户仍绑定passkey的登录流程；本地密码登录流程没有这一强制passkey二次校验，不能认定同锁出bug。

- [controller/user.go:77](/workspace/LemonHub/controller/user.go:77)：本地密码登录后只检查TOTP 2FA。
- [controller/user.go:114](/workspace/LemonHub/controller/user.go:114)：未启用TOTP则setupLogin，不会因注册过passkey被强制转passkey验证。

验证／触发边界：静态核对；未进行真实供应商/浏览器端到端验证。

### 056 · #7432 · 在 /v1/responses API 格式中存在的 绕过 敏感词检测的问题

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7432)

本地GetTokenCountMeta最小复现中function_call_output.output的标记完全不在CombineText内，可绕过此内容上的敏感词检测。

- [relaykit/dto/openai_request.go:1080](/workspace/LemonHub/relaykit/dto/openai_request.go:1080)：Responses Input结构只有Type/Role/Content，没有output。
- [relaykit/dto/openai_request.go:1122](/workspace/LemonHub/relaykit/dto/openai_request.go:1122)：ParseInput仅遍历content，跳过function_call_output.output。
- [controller/relay.go:146](/workspace/LemonHub/controller/relay.go:146)：敏感词仅扫描meta.CombineText。

验证／触发边界：已复现，repro-batch2.log：7432 function_call_output CombineText=""，输入output为AUDIT_SENSITIVE_SENTINEL。

建议：递归提取合法Responses工具输出的文本块，避免把图片base64当文本，并复用完整文本抽取做审核。

### 057 · #7420 · 模型无法配置上下文窗口和输入输出上下文长度

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7420)

本地同样未提供该功能，但issue请求增加可配置上下文窗口，未说明已有能力损坏；按功能缺口而非复现bug归类。

- [model/model_meta.go:24](/workspace/LemonHub/model/model_meta.go:24)：Model元数据仅名称/描述/图标/标签等，无context_window/max_input/max_output字段。
- [web/src/features/models/types.ts:243](/workspace/LemonHub/web/src/features/models/types.ts:243)：表单使用现有ModelFormValues schema。

验证／触发边界：静态核对；未进行真实供应商/浏览器端到端验证。

### 058 · #7417 · [Bug] 「货币与展示」页面汇率输入多位小数（如 6.7081）时会弹出格式校验警告

**确认存在** · P3 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7417)

静态确认HTML步进约束会使6.7081不满足step=0.01；表单数值存储并不限制两位小数，仍存在原生格式校验不一致。

- [web/src/features/system-settings/general/pricing-section.tsx:248](/workspace/LemonHub/web/src/features/system-settings/general/pricing-section.tsx:248)：USDExchangeRate数字输入step=0.01。
- [web/src/features/system-settings/general/pricing-section.tsx:299](/workspace/LemonHub/web/src/features/system-settings/general/pricing-section.tsx:299)：自定义汇率同样step=0.01。

验证／触发边界：静态确认HTML校验契约，未启动浏览器。

建议：将step与允许精度统一或使用any，并验证小数输入与保存。

### 059 · #7409 · Images API 未将 output_tokens_details 映射到 img_o，表达式少收图输出

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7409)

静态确认Images usage输出明细缺失，tiered img_o读不到上游image_tokens；影响图输出价与文本输出价不同的表达式。

- [relay/channel/openai/relay_image.go:70](/workspace/LemonHub/relay/channel/openai/relay_image.go:70)：normalizeOpenAIUsage映射input/output总数，但仅复制InputTokensDetails。
- [relay/channel/openai/relay_image.go:80](/workspace/LemonHub/relay/channel/openai/relay_image.go:80)：没有将OutputTokensDetails.ImageTokens写入CompletionTokenDetails。
- [service/tiered_settle.go:44](/workspace/LemonHub/service/tiered_settle.go:44)：img_o仅从usage.CompletionTokenDetails.ImageTokens取值。

验证／触发边界：静态确认；输入output_tokens_details.image_tokens=1120后此函数不会填充CompletionTokenDetails.ImageTokens。

建议：同步输出明细到canonical completion details，覆盖generations/edits及流式usage路径。

### 060 · #7407 · [Bug] Claude tool_result 内嵌图片在转换为 OpenAI Chat 格式时被序列化为 Base64 纯文本，导致上游视觉失效且产生数十万文本 Token

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7407)

本地最小Claude→Chat转换确认内嵌图片被当JSON纯文本，视觉信息丢失且base64产生大量文本token。

- [relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:198](/workspace/LemonHub/relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:198)：tool_result数组直接Marshal并SetStringContent。

验证／触发边界：已复现，repro-batch2.log的7407：role=tool的content包含整段source.data base64字符串。

建议：按工具批次保留工具文本、提取图片成为后续user多模态消息；不能把base64直接计入文本。

### 061 · #7402 · 模型定价计费设置计费时间，前端页面显示时间和表达式中的时间不一致

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7402)

issue只有截图无精确表达式；维护者解释<=12覆盖12整点小时故可显示到13，未证明计算错误。本地前端和表达式均采用显式半开区间，现有资料无法确认同bug。

- [web/src/features/pricing/components/dynamic-pricing-breakdown.tsx:123](/workspace/LemonHub/web/src/features/pricing/components/dynamic-pricing-breakdown.tsx:123)：当前时段摘要直接显示rangeStart:00~rangeEnd:00。
- [web/src/features/pricing/lib/billing-expr.ts:397](/workspace/LemonHub/web/src/features/pricing/lib/billing-expr.ts:397)：当前解析规范区间为>=start && <end。
- [pkg/billingexpr/run.go:111](/workspace/LemonHub/pkg/billingexpr/run.go:111)：hour函数返回整数小时。

验证／触发边界：未复现；需具体原表达式、时区和显示文本才能逐值比较。

### 062 · #7399 · 流式模型式下上游报错503:“Our servers are currently overloaded. Please try again later.” 却不重试

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7399)

静态确认正文所述HTTP200内嵌错误可能被当正常流/响应结算且不触发重试。真正HTTP503走另一错误路径，不能把标题里的503与实际200混淆；已向下游发流后也不能无损自动重试。

- [relay/channel/openai/relay-openai.go:257](/workspace/LemonHub/relay/channel/openai/relay-openai.go:257)：HTTP200 JSON error仅Type非空才识别；仅code/message会漏检。
- [relay/channel/openai/relay-openai.go:133](/workspace/LemonHub/relay/channel/openai/relay-openai.go:133)：流式解析/转发未识别error对象为NewAPIError。
- [relay/channel/openai/relay-openai.go:285](/workspace/LemonHub/relay/channel/openai/relay-openai.go:285)：漏检后可按估算prompt补usage。

验证／触发边界：静态确认；未调用真实过载服务。

建议：识别JSON/SSE error并返回明确失败，开流前可重试；开流后报告流错误并避免把无生成错误响应计为正常消费。

### 063 · #7394 · tiered_expr pre-consume reads the whole request body into memory even when the expression never calls param() (defeats the disk cache; retained until settlement)

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7394)

静态确认不使用param的表达式仍完整读取/保留大正文，磁盘缓存节省的内存被重新分配，重试还可能复制。

- [relay/helper/price.go:298](/workspace/LemonHub/relay/helper/price.go:298)：tiered表达式预扣无条件ResolveIncomingBillingExprRequestInput。
- [relay/helper/billing_expr_request.go:29](/workspace/LemonHub/relay/helper/billing_expr_request.go:29)：无条件读取完整正文，无param使用分析。
- [relay/helper/billing_expr_request.go:61](/workspace/LemonHub/relay/helper/billing_expr_request.go:61)：调用storage.Bytes，磁盘缓存重新读入堆。
- [relay/helper/billing_expr_request.go:69](/workspace/LemonHub/relay/helper/billing_expr_request.go:69)：已有冻结输入再Clone整份正文。
- [relay/helper/price.go:343](/workspace/LemonHub/relay/helper/price.go:343)：把requestInput存入info.BillingRequestInput，正文在请求生命周期继续保留。

验证／触发边界：静态调用链确认；未把issue的生产RSS数值冒充本地测量。

建议：分析表达式是否使用param，按需读取和冻结正文；header/token/time专用表达式不获取body。

### 064 · #7393 · Dashboard: charts drop real time buckets and fabricate empty ones when fewer than 7 buckets are returned

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7393)

已本地调用真实processChartData：两天真实桶被7周合成桶覆盖，第一天quota100从曲线中丢失且出现6个未请求零桶。

- [web/src/features/dashboard/lib/charts.ts:278](/workspace/LemonHub/web/src/features/dashboard/lib/charts.ts:278)：不足7个时间桶时丢弃times并return padded。
- [web/src/lib/time.ts:172](/workspace/LemonHub/web/src/lib/time.ts:172)：week只是当日起+6天标签，不把日期聚合到统一周。

验证／触发边界：已复现，repro-batch2-chart.log：真实09-14和09-15两桶，图数据仅保留09-15=200，09-14=100消失。

建议：保留所有真实桶；如补点应合并去重并限制查询范围，或去掉伪造补点。

### 065 · #7392 · [Bug] 将按 Token 计费转换到按表达式计费之后，模型广场会将转换之后的所有模型价格都显示为动态计费

**确认存在** · P3 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7392)

静态确认即使只是固定token价格表达式也统一标为动态计费，原issue的展示现象存在；不表示实际计费公式错误。

- [web/src/features/pricing/lib/dynamic-price.ts:65](/workspace/LemonHub/web/src/features/pricing/lib/dynamic-price.ts:65)：isDynamicPricingModel只检查tiered_expr和billing_expr存在。
- [web/src/features/pricing/components/model-billing-mode-badge.tsx:37](/workspace/LemonHub/web/src/features/pricing/components/model-billing-mode-badge.tsx:37)：所有表达式统一显示Dynamic Pricing，未按表达式单价/单位区分。

验证／触发边界：静态确认；不依赖尚未引入的/api/option/model_pricing/convert，可对已有tiered_expr模型观察。

建议：把执行方式与展示计费单位分离；简单表达式显示token/每次单位，真正条件价格再标动态。

### 066 · #7385 · /v1/audio/speech 的 stream_format=audio 未实现实时流式透传

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7385)

静态确认stream_format=audio落入完整缓冲分支，无法边生成边播放；直接改IsStream还会误走文本SSE scanner。

- [relaykit/dto/audio.go:44](/workspace/LemonHub/relaykit/dto/audio.go:44)：AudioRequest.IsStream仅StreamFormat==sse。
- [relay/channel/openai/audio.go:60](/workspace/LemonHub/relay/channel/openai/audio.go:60)：非IsStream分支io.ReadAll完整响应后写回。

验证／触发边界：静态确认；未调用付费TTS服务。

建议：区分binary audio和SSE，二进制逐块复制并flush，保留必要usage/音频计费处理。

### 067 · #7363 · [Bug] 使用日志中的 codex-* 模型未显示 OpenAI 图标

**确认存在** · P3 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7363)

静态确认codex-auto-review不命中任何provider分支并显示圆点；关闭的上游issue修复没有包含在本地。

- [web/src/features/usage-logs/components/model-badge.tsx:48](/workspace/LemonHub/web/src/features/usage-logs/components/model-badge.tsx:48)：OpenAI关键词列表缺codex-。
- [web/src/features/usage-logs/components/model-badge.tsx:132](/workspace/LemonHub/web/src/features/usage-logs/components/model-badge.tsx:132)：未识别provider时showDot=true。

验证／触发边界：静态确定性检查resolveModelProvider规则。

建议：为codex-添加OpenAI供应商识别，最好复用已有模型分类规则。

### 068 · #7361 · [BUG] v1.0.0-rc.37 常驻内存暴涨至 ~1.8GB（rc.36 仅 ~75MB），小内存机器触发 OOM

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7361)

issue仅描述rc36→rc37 RSS变化，评论要求pprof而未提供定位；本地有独立确认内存风险7394但不能证明它就是该20倍RSS回归，不能把版本或现象相似当同根因确认。

- [relay/helper/billing_expr_request.go:61](/workspace/LemonHub/relay/helper/billing_expr_request.go:61)：本地存在已确认的全量正文内存读取风险，参见7394。
- [controller/relay.go:137](/workspace/LemonHub/controller/relay.go:137)：本地已在无需统计/审核时避免构建巨大CombineText。

验证／触发边界：未做同流量rc36/rc37对照或采样pprof；需要可比较的请求与内存profile。

### 069 · #7360 · bug: Endpoint Type combobox in Test Channel Connection dialog auto-opens its dropdown

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7360)

本地已换成 Select，消除了报告中的 Combobox onFocus→setOpen(true) 触发链。

- [web/src/features/channels/components/dialogs/channel-test-dialog.tsx:1004](/workspace/LemonHub/web/src/features/channels/components/dialogs/channel-test-dialog.tsx:1004)：端点类型当前使用 SelectTrigger，不再用聚焦自动展开的 Combobox。

验证／触发边界：静态检查。

### 070 · #7354 · [Bug] 数据看板用户统计：周粒度下默认时间范围未选中

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7354)

本地周粒度与可见默认选项已一致，非原报告的 30/29 不匹配。

- [web/src/features/dashboard/constants.ts:37](/workspace/LemonHub/web/src/features/dashboard/constants.ts:37)：week 默认 29 天，与 TIME_RANGE_PRESETS 的 days=29 一致。
- [web/src/features/dashboard/components/users/user-charts.tsx:110](/workspace/LemonHub/web/src/features/dashboard/components/users/user-charts.tsx:110)：修改粒度时通过 getDefaultDays(g) 同步 selectedRange。

验证／触发边界：静态检查。

### 071 · #7348 · [Bug] 开启请求体透传的渠道上参数覆盖（param_override）静默失效

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7348)

静态确认：同时启用请求体透传与参数覆盖时覆盖不会执行，符合 issue 的静默失效。

- [relay/compatible_handler.go:97](/workspace/LemonHub/relay/compatible_handler.go:97)：透传分支直接创建原始 storage reader。
- [relay/compatible_handler.go:170](/workspace/LemonHub/relay/compatible_handler.go:170)：ApplyParamOverrideWithRelayInfo 仅存在非透传重建分支。
- [relay/claude_handler.go:167](/workspace/LemonHub/relay/claude_handler.go:167)：Claude 透传也直接回放原始 body。

验证／触发边界：type 8 渠道启用透传并配置 reasoning_effort=low，向 echo 上游发未携带该字段的 Chat 请求；本地分支将保持原始 body。

建议：在明确保留未知字段的前提下给透传应用 JSON override，或将互斥行为直接提示在配置 UI。

### 072 · #7338 · OAuth 敏感操作验证在 IdP 返回 Cross-Origin-Opener-Policy 时必定失败

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7338)

上游 OAuth 敏感操作证明流程没有移植至本项目，不能将 COOP 导致 verify 失败判成本项目同一缺陷；绑定流程仍依赖 opener 是另外的范围。

- [controller/secure_verification.go:16](/workspace/LemonHub/controller/secure_verification.go:16)：本地敏感验证仅声明 2fa/passkey。
- [controller/secure_verification.go:38](/workspace/LemonHub/controller/secure_verification.go:38)：UniversalVerify 不接收 oauth 验证方式。
- [web/src/features/auth/lib/oauth-callback-mode.ts:60](/workspace/LemonHub/web/src/features/auth/lib/oauth-callback-mode.ts:60)：OAuthCallbackMode 仅 login|bind，没有 verify。

验证／触发边界：静态检查版本边界。

### 073 · #7319 · audio没有兼容大小写

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7319)

本地已经移植音频扩展名大小写修复。

- [common/audio.go:24](/workspace/LemonHub/common/audio.go:24)：扩展名已先 strings.ToLower(ext)。
- [common/audio_case_test.go:37](/workspace/LemonHub/common/audio_case_test.go:37)：已有大小写音频扩展名回归测试。

验证／触发边界：静态检查，未在本次重新运行该已存在测试。

### 074 · #7309 · 模型广场 性能页面 会泄露 隐藏的分组

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7309)

本地前后端均过滤模型当前分组和查看者权限，隐藏分组不会仅因存在历史性能记录而暴露。

- [controller/perf_metrics.go:70](/workspace/LemonHub/controller/perf_metrics.go:70)：性能详情按当前模型允许分组过滤返回值。
- [controller/perf_metrics.go:90](/workspace/LemonHub/controller/perf_metrics.go:90)：可用分组来自当前用户 GetUserUsableGroups。
- [web/src/features/pricing/components/model-details-performance.tsx:184](/workspace/LemonHub/web/src/features/pricing/components/model-details-performance.tsx:184)：前端也按 availableGroups 过滤。

验证／触发边界：静态检查。

### 075 · #7307 · Claude→OpenAI conversion passes mid-conversation system messages through, breaking strict upstreams

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7307)

报告发送 messages[].role=system，而标准 Anthropic Messages 的 system 应是顶层字段。保持该非标准历史消息角色的实现存在，但不足以认定标准请求回归；上游维护者评论也明确将此归兼容配置。

- [relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:142](/workspace/LemonHub/relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:142)：转换确实按原顺序复制 claudeMessage.Role。

验证／触发边界：静态确认转换行为；未将非标准输入必须改写成 user 当作产品契约。

建议：特定兼容上游可配置参数覆盖或使用对应原生端点；如需支持该扩展应单独定义语义。

### 076 · #7296 · 请求日志中，阶梯计费模型的日志计费摘要误报「动态计费 · 无匹配结果」

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7296)

已直接执行本地 TypeScript parser：issue 原表达式返回 base 档，inputPrice=0.023127/outputPrice=0.44，未复现“无匹配结果”。

- [web/src/features/pricing/lib/billing-expr.ts:277](/workspace/LemonHub/web/src/features/pricing/lib/billing-expr.ts:277)：本地 parseTiersFromExpr 独立提取 tier，OR 条件链不会令 base 丢失。
- [web/src/features/usage-logs/lib/format.ts:339](/workspace/LemonHub/web/src/features/usage-logs/lib/format.ts:339)：日志摘要用解析结果按 matched_tier 匹配。

验证／触发边界：Node v24 stripTypeScriptTypes 执行生产函数，通过 issue 的完整四段 OR 表达式得到一个 base 档。

### 077 · #7290 · Claude 用量日志的 prompt_tokens 只记录未命中缓存的 fresh token，导致日志显示与 TPM／token 统计漏掉全部缓存输入

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7290)

静态确认统计/TPM漏计缓存输入。原始 Claude 日志保留 fresh 本身有语义理由，但将该列直接当总用量统计确实少算；本次不声称实际费用漏收。

- [service/billing_usage.go:161](/workspace/LemonHub/service/billing_usage.go:161)：Claude PromptTokens=fresh，InputTokens=fresh+cache read+cache create。
- [service/text_quota.go:528](/workspace/LemonHub/service/text_quota.go:528)：写日志依然只用 summary.PromptTokens。
- [model/log.go:454](/workspace/LemonHub/model/log.go:454)：统计 TokenUsed 仍是 params.PromptTokens+params.CompletionTokens。
- [model/log.go:695](/workspace/LemonHub/model/log.go:695)：TPM 查询直接 sum(prompt_tokens)+sum(completion_tokens)。

验证／触发边界：Claude usage input_tokens=2/cache_read=589733/cache_create=1088/output=1117，统计链只得到1119，遗漏590821缓存输入。

建议：保留原始计费语义，另传归一化总输入用于聚合、排行榜及TPM，避免直接改变计費所用 fresh。

### 078 · #7286 · “高级自定义”渠道配置路由内的”添加分流“按钮不见了

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7286)

本地“添加分流”入口已经恢复。

- [web/src/features/channels/components/dialogs/advanced-custom-editor-dialog.tsx:1263](/workspace/LemonHub/web/src/features/channels/components/dialogs/advanced-custom-editor-dialog.tsx:1263)：展开路由后仍渲染 onAddRoute 按钮 Add split。
- [web/src/features/channels/components/__tests__/advanced-custom-splits.test.tsx:26](/workspace/LemonHub/web/src/features/channels/components/__tests__/advanced-custom-splits.test.tsx:26)：已有展开分流并保存的交互回归测试。

验证／触发边界：静态检查。

### 079 · #7283 · [Bug] Gemini 渠道 :countTokens 被当作 generateContent 执行：无 totalTokens、延迟 24-44s、且按生成计费

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7283)

本地通过明确拒绝不支持的 countTokens 阻止误生成和误计费；这不表示支持完整 Gemini token counting 功能。

- [router/relay-router.go:72](/workspace/LemonHub/router/relay-router.go:72)：/v1 router 在认证/分配前安装 rejectGeminiCountTokens。
- [router/relay-router.go:198](/workspace/LemonHub/router/relay-router.go:198)：/v1beta 同样拒绝。
- [router/relay-router.go:214](/workspace/LemonHub/router/relay-router.go:214)：:countTokens 明确 RelayNotFound 并 Abort。

验证／触发边界：静态核对两个原生路由别名的拒绝边界。

### 080 · #7282 · [Bug] 模型广场 24 小时成功率柱状条间距不均匀

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7282)

本地卡片已经改为3柱摘要，原24小时24柱间距问题不适用；详情页另有独立 sparkline 不等同该卡片报告。

- [web/src/features/pricing/components/model-perf-badge.tsx:82](/workspace/LemonHub/web/src/features/pricing/components/model-perf-badge.tsx:82)：当前卡片仅保留最近3个状态条。
- [web/src/features/pricing/components/model-perf-badge.tsx:118](/workspace/LemonHub/web/src/features/pricing/components/model-perf-badge.tsx:118)：使用固定 gap-0.5/w-1，而非报告的24柱分配布局。

验证／触发边界：静态检查当前渲染结构。

### 081 · #7268 · Time-based billing tiers are not displayed correctly on the pricing page

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7268)

执行生产解析函数确认时间条件丢失：本地不同于上游“空数组”症状，会返回两档但 conditions 都为空，不能正确显示高峰时间范围，根因和影响同属时间阶梯未解析。

- [web/src/features/pricing/lib/billing-expr.ts:281](/workspace/LemonHub/web/src/features/pricing/lib/billing-expr.ts:281)：tier 条件正则仅识别 p/c/len，不能携带 hour 条件。
- [web/src/features/pricing/components/dynamic-pricing-breakdown.tsx:205](/workspace/LemonHub/web/src/features/pricing/components/dynamic-pricing-breakdown.tsx:205)：展示直接使用 parseTiersFromExpr 返回的条件。

验证／触发边界：将 issue 的 hour(Asia/Shanghai)>=9 && hour<18 ? tier(高峰...) : tier(空闲...) 传入生产函数，两档均 conditions=[]。

建议：在严格解析时间条件后展示明确时区/范围；无法保真时整体降级，避免将有条件档位呈现为无条件。

### 082 · #7267 · Users table quota display regressed after ea7cb0ba4

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7267)

本地余额表格未采用报告中的紧凑 Popover 控件，直接可见余额与进度条已恢复；无余额行使用 StatusBadge。

- [web/src/features/users/components/users-columns.tsx:169](/workspace/LemonHub/web/src/features/users/components/users-columns.tsx:169)：使用本地专有 UserQuotaCell。
- [web/src/features/users/components/user-quota-cell.tsx:62](/workspace/LemonHub/web/src/features/users/components/user-quota-cell.tsx:62)：正常余额直接占满宽度显示余额/总额和 Progress，详情使用 Tooltip。

验证／触发边界：静态检查结构，不声称覆盖所有浏览器像素表现。

### 083 · #7252 · fix(ollama): streaming tool_calls dropped when Ollama returns them in the final `done:true` frame (e.g. qwen3-coder)

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7252)

本地已移植终帧工具调用修复，done 帧文本和思考也保留。

- [relay/channel/ollama/stream.go:185](/workspace/LemonHub/relay/channel/ollama/stream.go:185)：done=true 帧先 buildOllamaStreamDelta 并发出 payload。
- [relay/channel/ollama/stream.go:198](/workspace/LemonHub/relay/channel/ollama/stream.go:198)：已有 tool call 则 finish_reason=tool_calls。

验证／触发边界：静态核对真实 done 分支。

### 084 · #7247 · Higress AI 网关使用 Transfer-Encoding: chunked 发送请求体给上游 New API 服务，但 New API 无法正确解析 chunked 请求体，导致返回 400 "invalid JSON request body"。

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7247)

正常 HTTP chunked 经 Go net/http 解码且本地读取到 EOF；issue 没有原始请求/字节证据，无法判定 Higress 的特定编码是否相同缺陷。

- [common/gin.go:69](/workspace/LemonHub/common/gin.go:69)：请求体由 Body reader 和 ContentLength 传给 CreateBodyStorageFromReader。
- [common/body_storage.go:328](/workspace/LemonHub/common/body_storage.go:328)：未知 ContentLength 仍 io.ReadAll(io.LimitReader)，不是读0字节。
- [middleware/distributor.go:248](/workspace/LemonHub/middleware/distributor.go:248)：400 由解块后的 body JSON 无效产生。

验证／触发边界：未搭建 Higress；静态检查未发现未知长度直接跳过 body 的问题。

建议：收集到达服务端的 Transfer-Encoding/Content-Encoding 和解块后脱敏 body，再定位不合法 JSON 的来源。

### 085 · #7235 · kimi-k3 动态工具调用异常

**疑似** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7235)

本地可确定动态消息 tools 扩展会丢；但 issue 正文顶层 tools=[]、评论示例非完整 JSON，单凭现有证据不能确认它声称的 required 错误在有合法顶层工具时必现。

- [relaykit/dto/openai_request.go:381](/workspace/LemonHub/relaykit/dto/openai_request.go:381)：Message DTO 没有 messages[].tools 字段。
- [relay/compatible_handler.go:109](/workspace/LemonHub/relay/compatible_handler.go:109)：非透传将解码 DTO 重新转换、序列化，未知消息字段会丢失。

验证／触发边界：静态确认字段缺失；尚未用支持 Kimi 动态 tools 的真实上游复现。

建议：若正式支持供应商 messages[].tools 扩展，应显式建模并保留；目前渠道透传可绕过。

### 086 · #7231 · 非流式请求客户端超时后仍继续上游请求，并在连接断开后扣费

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7231)

已对真实 doRequest 做离线HTTP复现：下游 context 已取消，仍收到上游200且上游调用1次，证实非流式请求未绑定下游取消；成功usage可继续进入结算。是否发生特定38分钟扣费须看部署日志。

- [relay/channel/api_request.go:320](/workspace/LemonHub/relay/channel/api_request.go:320)：DoApiRequest 使用 http.NewRequest 未继承 c.Request.Context。
- [relay/channel/api_request.go:533](/workspace/LemonHub/relay/channel/api_request.go:533)：doRequest 直接 relayClient.Do(req)，途中没有绑定客户端 context。
- [relay/compatible_handler.go:219](/workspace/LemonHub/relay/compatible_handler.go:219)：成功 usage 进入正常结算链。

验证／触发边界：Go overlay TestAudit7231CanceledDownstreamStillSendsUpstream，expected context.Canceled/0上游调用，actual err=nil/1调用。日志 /workspace/scratch/upstream-bug-audit/repros/batch3_cancel_output.txt；这是取消传播的局部生产链验证，未模拟数据库完整结算。

建议：将非流式请求生命周期绑定下游 context，并明确中断后的上游已用量结算策略，避免简单跳过所有扣费。

### 087 · #7229 · 严重的计费BUG-图像缓存命中重复计费

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7229)

已通过真实生产计费函数确定性复现：完全缓存的100图片token，图片倍率1/缓存倍率0.1，预期10但实际110。缓存图片同时进入缓存收费和图片收费。

- [service/billing_usage.go:190](/workspace/LemonHub/service/billing_usage.go:190)：Gemini cachedContentTokenCount 进入 CachedTokens。
- [service/billing_usage.go:192](/workspace/LemonHub/service/billing_usage.go:192)：原始 PromptTokensDetails 全部图片仍进入 ImageTokens。
- [service/text_quota.go:334](/workspace/LemonHub/service/text_quota.go:334)：缓存和图片分别从 base 扣除后分别乘倍率累加，重叠缓存图片没有扣除。
- [service/text_quota.go:351](/workspace/LemonHub/service/text_quota.go:351)：负 base 被归零，会保留缓存价与整张图片价双重收费。

验证／触发边界：Go overlay TestAudit7229FullyCachedImage 已运行且按预期失败：expected=10 actual=110。输出见 scratch/upstream-bug-audit/repros/batch3_billing_output.txt；没有修改生产源码。

建议：在 Gemini usage 归一化时分离缓存各模态交集；无缓存模态明细时采用保守、一致的计费策略。

### 088 · #7225 · 渠道测试 settleTestQuota 漏乘分组倍率

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7225)

静态确认后台测试倍率计费漏乘分组倍率，范围仅渠道测试，与用户正常中转结算分开。

- [controller/channel-test.go:576](/workspace/LemonHub/controller/channel-test.go:576)：传统倍率 settleTestQuota 仅乘 ModelRatio，不乘 GroupRatioInfo.GroupRatio。
- [controller/channel-test.go:561](/workspace/LemonHub/controller/channel-test.go:561)：表达式计费分支走统一 TryTieredSettle。

验证／触发边界：Go overlay TestAudit7225ChannelTestGroupRatio 已运行且按预期失败：100 prompt/ModelRatio=1/GroupRatio=1.5，expected=150 actual=100。输出见 scratch/upstream-bug-audit/repros/batch3_billing_output.txt。

建议：传统渠道测试结算应复用统一计费或至少乘分组倍率并保留安全舍入。

### 089 · #7224 · ParseContent() 丢失 cache_control

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7224)

本地实际执行复现：ParseContent 后 cache_control 为空，OpenAI→Claude 完整转换也不再包含该字段，缓存断点丢失。

- [relaykit/dto/openai_request.go:668](/workspace/LemonHub/relaykit/dto/openai_request.go:668)：JSON map text 内容仅拷贝 Type/Text，没有 CacheControl。
- [relaykit/relayconvert/internal/oai_chat/to_claude_messages_req.go:266](/workspace/LemonHub/relaykit/relayconvert/internal/oai_chat/to_claude_messages_req.go:266)：system 内容重建也没有 CacheControl。
- [relaykit/relayconvert/internal/oai_chat/to_claude_messages_req.go:335](/workspace/LemonHub/relaykit/relayconvert/internal/oai_chat/to_claude_messages_req.go:335)：普通text内容转换同样不拷贝 CacheControl。

验证／触发边界：运行 scratch/repros/batch3-relaykit.go：7224 parsed cache_control=""；converted_contains_cache_control=false。

建议：解析内容块及重建 Claude system/user 文本块时保留 CacheControl；覆盖原始JSON进入的真实路径。

### 090 · #7222 · [Bug] 模型广场卡片视图：切换分组后分页跳到最后一页，上一页按钮失灵

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7222)

静态确认：从page=5切换仅2页分组时显示2，但状态仍5，前三次上一页无视觉变化。

- [web/src/features/pricing/components/model-card-grid.tsx:44](/workspace/LemonHub/web/src/features/pricing/components/model-card-grid.tsx:44)：page state 独立保存；currentPage=Math.min(page,totalPages)。
- [web/src/features/pricing/components/model-card-grid.tsx:106](/workspace/LemonHub/web/src/features/pricing/components/model-card-grid.tsx:106)：上一页递减隐藏的page而非展示currentPage。
- [web/src/features/pricing/index.tsx:126](/workspace/LemonHub/web/src/features/pricing/index.tsx:126)：分组切换复用同一无key的ModelCardGrid。

验证／触发边界：5→2页分组：state5/display2，点击后state4/display2→state3/display2→state2/display2，第四次才display1。

建议：过滤分组改变时重置页码，或将页码与新的totalPages同步钳制。

### 091 · #7215 · 使用 Anthropic API 格式发起请求时，思考等级参数未正确映射至 OpenAI 上游的 reasoning_effort

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7215)

本地函数执行确认 adaptive thinking/output_config.effort=medium 转普通 OpenAI 时 reasoning_effort=""。后置条件override查不到已丢失的源协议字段也符合该转换顺序。

- [relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:25](/workspace/LemonHub/relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:25)：普通Claude→Chat构造只复制基础采样字段。
- [relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:43](/workspace/LemonHub/relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:43)：GetEfforts 仅在 OpenRouter 分支处理且写入 Verbosity；普通Chat无 ReasoningEffort 映射。

验证／触发边界：运行 scratch/repros/batch3-relaykit.go，7215 reasoning_effort=""。

建议：明确跨协议努力等级映射，并为条件override保留源请求上下文或在转换前应用相关条件。

### 092 · #7205 · [Bug] 原生 Gemini thinkingLevel 大写枚举被误判为模型不支持

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7205)

本地未引入上游发生误判的 ValidateGeminiThinkingConfig；普通模型原生 ThinkingLevel 不会经报告的大小写比较路径返回400。

- [relaykit/relayconvert/internal/shared/gemini/request.go:84](/workspace/LemonHub/relaykit/relayconvert/internal/shared/gemini/request.go:84)：本地旧 ApplyThinkingConfig 无 ValidateGeminiThinkingConfig 大小写比较。
- [relay/common/relay_info.go:469](/workspace/LemonHub/relay/common/relay_info.go:469)：已在日志提取处按大小写不敏感规范化努力等级，并保留原请求。

验证／触发边界：静态检查；未使用Google账号做网络请求。

### 093 · #7201 · qwen3.7-max/qwen3.8-max are incorrectly trimmed to qwen3.7/qwen3.8 after v1.0.0-rc.31

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7201)

报告中的 qwen3.7-max/qwen3.8-max 在本地不匹配 OpenAI 后缀解析，保持完整名称。评论后续 Gemini别名裁剪是另外的请求边界，本条只确认原始Qwen回归已不存在。

- [relaykit/relayconvert/reasoning/suffix.go:11](/workspace/LemonHub/relaykit/relayconvert/reasoning/suffix.go:11)：OpenAIEffortSuffixes 没有 -max。
- [relay/channel/openai/adaptor.go:338](/workspace/LemonHub/relay/channel/openai/adaptor.go:338)：Chat努力后缀仅在模型能力 UseMaxCompletionTokens 时应用。

验证／触发边界：静态核对 suffix 列表及 Chat 应用条件。

### 094 · #7194 · 最新版本视频模型生成成功后获取制品提示媒体预览失败，请重试，接口请问tasks状态为404

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7194)

本地未采用上游新的 /v1/tasks/.../artifacts/... 插件制品架构，报告迁移后的404和响应裁剪不属于当前实现。

- [router/video-router.go:16](/workspace/LemonHub/router/video-router.go:16)：本地制品代理路径是 /v1/videos/:task_id/content。
- [relay/relay_task.go:425](/workspace/LemonHub/relay/relay_task.go:425)：/v1/videos/:id 走既有适配器 ConvertToOpenAIVideo。
- [model/task.go:741](/workspace/LemonHub/model/task.go:741)：OpenAI Video 元数据仍包含 ResultURL。

验证／触发边界：静态确认旧video原生流程与返回URL；无真实供应商任务验证。

### 095 · #7185 · 插件抢占模型名后，模型映射无法切换到其他插件，导致请求失败

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7185)

本项目没有报告中的任务插件A、插件认领模型/插件过滤架构，不能出现同一“插件抢占前置过滤”根因。

- [relay/relay_adaptor.go:144](/workspace/LemonHub/relay/relay_adaptor.go:144)：本地任务适配器按渠道类型switch选择，无插件模型声明注册表。
- [controller/relay.go:639](/workspace/LemonHub/controller/relay.go:639)：任务渠道先按客户端ModelName走通用渠道选择。

验证／触发边界：静态检查任务路由架构。

### 096 · #7184 · fix: SSRF allowed_ports string-slice config is not parsed correctly

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7184)

本地已经实现字符串端口/范围的规范解析与错误拒绝，未沿用空整型列表导致放行的问题。

- [common/ssrf_protection.go:33](/workspace/LemonHub/common/ssrf_protection.go:33)：构建保护器时对[]string端口调用 parsePortRanges，错误直接返回。
- [common/ssrf_protection.go:45](/workspace/LemonHub/common/ssrf_protection.go:45)：解析出的整数端口写入 AllowedPorts。
- [common/ssrf_protection.go:194](/workspace/LemonHub/common/ssrf_protection.go:194)：出站端口逐一匹配allowlist。

验证／触发边界：静态检查实际构造和校验链。

### 097 · #7180 · 用户可越权添加其他用户组 api 令牌

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7180)

静态确认可创建绑定其他分组的token，符合“越权添加”现象；但本地relay仍403，不能报告为越权调用隐藏组或计费绕过。auto_groups有单独权限检查，普通group没有。

- [controller/token.go:308](/workspace/LemonHub/controller/token.go:308)：validateTokenGroups 仅校验非空和最多8个分组，没有权限检查。
- [controller/token.go:384](/workspace/LemonHub/controller/token.go:384)：普通token创建将任意token.Group写入数据库。
- [middleware/auth.go:478](/workspace/LemonHub/middleware/auth.go:478)：真正使用token时仍逐分组校验usableGroups并403。

验证／触发边界：已登录普通用户向POST /api/token传非空但无权分组，创建校验可通过；随后请求中转会在middleware返回403。

建议：创建/更新token普通group时复用IsUserSelectableGroup并保持使用时权限校验。

### 098 · #7177 · Rerank channel is incorrectly recognized as embedding, causing test failure

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7177)

静态确认bge-reranker-v2-m3会同时命中两个if，最终自动测试走embedding。显式选择rerank端点可避开；实际/v1/rerank中转不是本条问题。

- [controller/channel-test.go:142](/workspace/LemonHub/controller/channel-test.go:142)：自动检测先将rerank模型路径置为/v1/rerank。
- [controller/channel-test.go:147](/workspace/LemonHub/controller/channel-test.go:147)：后续独立embedding判断命中bge-又覆盖为/v1/embeddings。

验证／触发边界：自动检测测试bge-reranker-v2-m3，无endpointType：先rerank后bge覆盖embedding。

建议：先确立rerank优先级，embedding使用else if或统一分类函数。

### 099 · #7175 · [Bug] Claude Messages 转 OpenAI Chat 时单元素 stop_sequences 被转换为 string，导致部分上游 400

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7175)

本地函数执行确认单元素变为string；这是对只接受数组的兼容供应商的明确不兼容，string本身仍符合OpenAI规范，不能泛化为所有OpenAI上游失败。

- [relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:75](/workspace/LemonHub/relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:75)：单个stop_sequences直接取[0]写为string。

验证／触发边界：运行 scratch/repros/batch3-relaykit.go，stop_type=string，stop=</block>。

建议：保留非空StopSequences为数组可兼容两类供应商，不改变合法OpenAI语义。

### 100 · #7174 · 用户可创建空分组密钥，管理员修改用户分组后密钥会请求修改后的分组

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7174)

本地已阻止用户新建空分组密钥，符合报告主要诉求；历史已启用空分组token仍可能继承用户组，但不再可按复现步骤创建。

- [controller/token.go:308](/workspace/LemonHub/controller/token.go:308)：新建/编辑token强制显式非空group。
- [controller/token.go:340](/workspace/LemonHub/controller/token.go:340)：AddToken在生成key前调用validateTokenGroups。
- [controller/token.go:446](/workspace/LemonHub/controller/token.go:446)：启用旧无分组token也要求先修复。

验证／触发边界：静态检查新建、更新和启用校验。

### 101 · #7155 · 模型映射没有执行

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7155)

已读评论：维护者确认报告实例启用了全局透传，透传不处理模型是配置语义。非透传映射本地存在，不能由该issue认定映射普遍失效。

- [relay/helper/model_mapped.go:37](/workspace/LemonHub/relay/helper/model_mapped.go:37)：非透传映射支持链式查找并写入UpstreamModelName。
- [relay/helper/model_mapped.go:70](/workspace/LemonHub/relay/helper/model_mapped.go:70)：实际DTO.SetModelName为映射结果。
- [relay/compatible_handler.go:97](/workspace/LemonHub/relay/compatible_handler.go:97)：透传开关回放原body，因此不重写模型。

验证／触发边界：静态检查正常映射路径，并核对上游评论给出的全局透传原因。

建议：需要模型映射时关闭全局/渠道请求体透传；与#7348覆盖静默互斥问题区分。

### 102 · #7148 · 内联图片 Base64 被计入文本 Token：Claude tool_result 与 Responses compact 本地估算异常（模板重提）

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7148)

本地独立relaykit函数执行复现：Claude工具结果图片与compact输入图片均base64_as_text=true/files=0。可夸大本地预估和无usage回退计费。

- [relaykit/dto/claude.go:317](/workspace/LemonHub/relaykit/dto/claude.go:317)：tool_result.content 整体Marshal加入文本。
- [relaykit/dto/openai_responses_compaction_request.go:35](/workspace/LemonHub/relaykit/dto/openai_responses_compaction_request.go:35)：compact.Input完整JSON加入CombineText，Files为空。

验证／触发边界：运行 scratch/repros/batch3-relaykit.go：两种结构均base64_as_text=true files=0；不连接真实上游。

建议：Claude嵌套tool_result分解text/image；compact复用正常Responses的结构化媒体token元数据提取。

### 103 · #7144 · [Bug] 模型广场打开排序下拉菜单时顶部导航栏左右晃动

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7144)

已静态核对本地实际排序菜单，明确传modal=false；rc41回移记录亦列出2bfb89c1b。

- [web/src/features/pricing/components/pricing-toolbar.tsx:223](/workspace/LemonHub/web/src/features/pricing/components/pricing-toolbar.tsx:223)：排序DropdownMenu modal={false}，不锁定页面滚动。

### 104 · #7141 · [BUG] Traditional Chinese (zhTW) always flips back to Simplified Chinese on page reload

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7141)

本地convertDetectedLanguage已识别缓存形式zhtw并返回zhTW，刷新回简体的缺失分支已补。

- [web/src/i18n/languages.ts:72](/workspace/LemonHub/web/src/i18n/languages.ts:72)：lower === zhtw纳入繁体分支。

### 105 · #7136 · logs表upstream_request_id 值都是空的,不利于上游渠道排查问题

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7136)

本地字段并非完全不写；与上游维护者评论一致，目前只采集X-Oneapi-Request-Id。其他供应商返回x-request-id不能被记录属于当前支持范围限制，issue没有提供带支持响应头仍失败的样本。

- [relay/channel/api_request.go:553](/workspace/LemonHub/relay/channel/api_request.go:553)：读取common.RequestIdKey并写UpstreamRequestIdKey。
- [model/log.go:318](/workspace/LemonHub/model/log.go:318)：从上下文读取upstreamRequestId用于日志落库。

建议：如扩展供应商请求ID支持，应按供应商定义头部并补日志回归。

### 106 · #7134 · 客户端取消的流式请求被计为模型失败，持续污染性能成功率

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7134)

已用overlay复现：转换路径先发送hello，下游取消后返回nil usage及500 context canceled。最终错误按HTTP码记性能样本，默认空白名单将所有错误算失败，未豁免下游取消。

- [relay/channel/openai/chat_via_responses.go:233](/workspace/LemonHub/relay/channel/openai/chat_via_responses.go:233)：下游ObjectData失败转换成500 NewAPIError。
- [controller/relay.go:285](/workspace/LemonHub/controller/relay.go:285)：仅按状态码决定success并RecordRelaySample。
- [setting/perf_metrics_setting/config.go:59](/workspace/LemonHub/setting/perf_metrics_setting/config.go:59)：默认空白名单始终返回true，将取消产生的500算失败。

验证／触发边界：本地TestAudit7062And7134通过（断言确认现存缺陷），repro-4/openai.log。

建议：在确认下游取消且没有上游终态故障时将样本中性排除，保留审计；避免用状态码一概归因。

### 107 · #7130 · TTFT延迟 网关设置了 SSE 响应头但在首个数据帧之前从不 flush

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7130)

静态确认共享scanner只设置SSE头，不在收到已校验上游响应后主动flush；关闭ping且上游延迟首帧时，下游响应头仍等首帧。

- [relay/helper/stream_scanner.go:145](/workspace/LemonHub/relay/helper/stream_scanner.go:145)：SetEventStreamHeaders后启动读循环，未提前FlushWriter。
- [relay/helper/common.go:45](/workspace/LemonHub/relay/helper/common.go:45)：仅Header.Set，不WriteHeader/Flush。

验证／触发边界：关闭ping，mock上游立即回200/SSE头、延迟发首帧；静态证据，未运行浏览器网络复现。

建议：在上游响应通过校验后审慎提交头，兼顾首帧错误与重试语义；不能在Do请求前flush。

### 108 · #7127 · 多个 channel handler 设置了 SSE 头但从不 flush

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7127)

与#7130同一缺陷，issue本身步骤不足，但本地共享实现可静态独立确认头部延迟。

- [relay/helper/stream_scanner.go:145](/workspace/LemonHub/relay/helper/stream_scanner.go:145)：设置头部后未flush。
- [relay/helper/common.go:97](/workspace/LemonHub/relay/helper/common.go:97)：数据写入函数StringData写入后才flush。

验证／触发边界：参见#7130；两条应合并为一个修复项。

建议：同#7130。

### 109 · #7123 · 自定义首页 iframe 首屏语言/主题同步存在竞态，可能无法跟随中文

**确认存在** · P3 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7123)

静态确认自定义首页仅effect或onLoad单次postMessage，没有ready/ack监听或重发；iframe在load后延迟注册message监听就会错过初态。

- [web/src/features/home/index.tsx:40](/workspace/LemonHub/web/src/features/home/index.tsx:40)：syncIframePreferences发送一次主题和语言。
- [web/src/features/home/index.tsx:89](/workspace/LemonHub/web/src/features/home/index.tsx:89)：iframe仅onLoad触发同步。

验证／触发边界：使用issue给出的延迟2秒注册message监听HTML可触发；本次静态验证，未启动浏览器。

建议：增加校验来源的ready/ack协议并兼容旧页面，或有限次重发。

### 110 · #7113 · auto分组设置后无效果，令牌选择分组时无auto选项

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7113)

issue评论已确认需配置用户可用分组auto；本地GetUserGroups明确在可用分组含auto时返回auto。仅配置Auto组内顺序不会自动授权auto，不能据此认定相同bug。

- [controller/group.go:31](/workspace/LemonHub/controller/group.go:31)：读取用户可用分组。
- [controller/group.go:41](/workspace/LemonHub/controller/group.go:41)：允许auto时显式返回auto选项。

### 111 · #7106 · bug: 使用日志顶部用量显示 ¥0，但消费明细 quota 非零

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7106)

本地第二个RPM/TPM查询已扫描独立rateStat，仅回写Rpm/Tpm，不覆盖Quota；并且仍使用GORM1.25.2。

- [model/log.go:747](/workspace/LemonHub/model/log.go:747)：独立rateStat接收第二次Scan。
- [model/log.go:755](/workspace/LemonHub/model/log.go:755)：只赋回Rpm/Tpm。

### 112 · #7099 · 28版本无法启用

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7099)

报告针对rc28升级到GORM1.25.12的MySQL唯一约束迁移。当前项目固定GORM1.25.2，没有报告中的新增驱动迁移路径或DROP FOREIGN KEY脚本，不能将上游rc28启动崩溃套用本地。

- [go.mod:61](/workspace/LemonHub/go.mod:61)：gorm.io/gorm v1.25.2。
- [model/main.go:143](/workspace/LemonHub/model/main.go:143)：数据库初始化采用本地兼容分支；未引入报告的rc28依赖组合。

建议：未来升级GORM须覆盖既有唯一索引MySQL迁移。

### 113 · #7098 · rc.26~rc.28 启动崩溃循环：PostgreSQL(PgBouncer) 下 schema 自检报 "insufficient arguments"

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7098)

上游特定组合为GORM1.25.12+postgres1.5.2；本地保持GORM1.25.2，PostgreSQL连接还明确PreferSimpleProtocol。未具备issue原始升级触发条件；未连接真实PgBouncer验证其它故障。

- [go.mod:60](/workspace/LemonHub/go.mod:60)：postgres v1.5.2，但gorm为v1.25.2。
- [model/main.go:143](/workspace/LemonHub/model/main.go:143)：PreferSimpleProtocol:true关闭隐式prepared statement。

### 114 · #7094 · fix: Responses conversion writes empty function_call.name into history, breaking session replay

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7094)

overlay单测已复现非流式转换输出function_call且Name为空；流式也在首次工具delta时直接分配输出索引并发送空名added事件，没有等有效名称。

- [relaykit/relayconvert/internal/oai_chat/to_oai_responses_resp.go:183](/workspace/LemonHub/relaykit/relayconvert/internal/oai_chat/to_oai_responses_resp.go:183)：Name直接复制toolCall.Function.Name，无过滤。
- [relaykit/relayconvert/internal/oai_chat/to_oai_responses_stream_resp.go:204](/workspace/LemonHub/relaykit/relayconvert/internal/oai_chat/to_oai_responses_stream_resp.go:204)：即使name为空也分配OutputIndex，随后发added。

验证／触发边界：TestAudit7094通过：断言转换结果含空名function_call，确认缺陷。

建议：非流式丢弃空白函数名；流式缓存参数直到有效name后分配索引并补发，保持usage。

### 115 · #7089 · 游乐场无法使用

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7089)

issue仅Network Error截图，无具体接口/请求/Console；本地Playground走统一api客户端，不能仅凭症状归因为同一代码问题。需部署网络、请求响应及配置。

- [web/src/features/playground/api.ts:36](/workspace/LemonHub/web/src/features/playground/api.ts:36)：聊天使用api.post CHAT_COMPLETIONS。
- [web/src/features/playground/api.ts:47](/workspace/LemonHub/web/src/features/playground/api.ts:47)：模型列表使用api.get USER_MODELS。

### 116 · #7087 · bug: Responses SSE added item 缺少空数组时 Codex 丢失 active item

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7087)

本地原生Responses确会保留上游省略content/summary的JSON；但该issue是供应商产出不符合item schema，维护者明确要求供应商修复。不能把未做额外容错等同本地引入协议缺陷；本地转换器产物需另测。

- [relay/channel/openai/relay_responses.go:115](/workspace/LemonHub/relay/channel/openai/relay_responses.go:115)：仅重写model后转发原生data，未删减content/summary。
- [relay/channel/openai/relay_responses.go:121](/workspace/LemonHub/relay/channel/openai/relay_responses.go:121)：sendResponsesStreamData直接发送上游事件。

建议：可选增加窄范围added事件兼容补齐，但这是供应商容错。

### 117 · #7084 · Task-plugin (video) submits bypass channel model_mapping: submit body finalized before ModelMappedHelper (rc.27)

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7084)

本地未引入任务JS插件框架，仍使用原生Go Doubao适配器；BuildRequestBody在映射后构建且明确采用info.UpstreamModelName。不存在issue中validate阶段缓存submit descriptor的时序。

- [relay/relay_adaptor.go:164](/workspace/LemonHub/relay/relay_adaptor.go:164)：Doubao返回taskdoubao.TaskAdaptor。
- [relay/channel/task/doubao/adaptor.go:193](/workspace/LemonHub/relay/channel/task/doubao/adaptor.go:193)：IsModelMapped时body.Model=UpstreamModelName。
- [relay/relay_task.go:238](/workspace/LemonHub/relay/relay_task.go:238)：模型映射后才BuildRequestBody。

### 118 · #7063 · new-api-v1.0.0-rc.26.exe给我安装到哪里去了，我在仓库没找到任何关于如何卸载的字段

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7063)

这是将独立Go服务exe误认安装程序的运行咨询，没有安装器缺陷或卸载复现。本地main为直接启动服务，未引入安装器。

- [main.go:48](/workspace/LemonHub/main.go:48)：main直接初始化并启动服务。

### 119 · #7062 · 流式请求中 client_gone 导致 New API 计费为 0，但上游已正常扣费且客户端收到完整响应

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7062)

原生Responses路径已有基于已观察输出的usage估算，不能泛称所有断流都免费；但Chat→Responses→Chat路径仍可在已交付内容后因下游取消返回nil usage+500，controller最终错误统一退预扣，漏记已服务用量。overlay已复现这个返回结果，退款分支为静态调用链核验。

- [relay/channel/openai/chat_via_responses.go:315](/workspace/LemonHub/relay/channel/openai/chat_via_responses.go:315)：streamErr存在即return nil,streamErr，跳过usage恢复。
- [relay/helper/common.go:102](/workspace/LemonHub/relay/helper/common.go:102)：取消的Request.Context使StringData返回context canceled。
- [controller/relay.go:181](/workspace/LemonHub/controller/relay.go:181)：最终NewAPIError非空即Billing.Refund。
- [relay/channel/openai/responses_usage.go:129](/workspace/LemonHub/relay/channel/openai/responses_usage.go:129)：原生Responses路径有缺usage估算，已与转换路径区分。

验证／触发边界：TestAudit7062And7134：hello成功交付后取消，得到nil usage和HTTP500；见repro-4/openai.log。

建议：取消与计费解耦，结算已取得usage或已观察输出的估算；不要求无界继续请求上游。

### 120 · #7061 · token计费和上游差距较大

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7061)

只给总量截图和deepseek-v4-flash，没有成对请求/usage/协议。维护者指出messages输入语义及断流有关；本地Claude缓存token分字段记录而原生Responses有估算，统计口径或断流均可能，不能确认同一根因。

- [relay/channel/claude/relay-claude.go:237](/workspace/LemonHub/relay/channel/claude/relay-claude.go:237)：Claude input_tokens与cache_read/cache_creation分别记录。
- [relay/channel/openai/responses_usage.go:147](/workspace/LemonHub/relay/channel/openai/responses_usage.go:147)：无usage时根据观察文本估算，与供应商可能不同。

建议：采集同一请求原始usage、缓存token字段、入站协议及断流终态后对账。

### 121 · #7059 · Bug: upstream TCP reset leaves Responses SSE without a terminal event

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7059)

本地overlay以读到delta后返回TCP reset fixture复现：StreamStatus为scanner_error，但原生Responses handler无错误返回且无response.failed/incomplete终止事件。已输出内容后客户端仍只能见截断EOF。

- [relay/channel/openai/relay_responses.go:167](/workspace/LemonHub/relay/channel/openai/relay_responses.go:167)：scanner结束后直接return accumulator.finish(),nil，不生成失败终态。
- [relay/helper/stream_scanner.go:283](/workspace/LemonHub/relay/helper/stream_scanner.go:283)：scanner错误记录StreamStatus后结束。

验证／触发边界：TestAudit7059通过，断言hello事件存在、终态缺失、end_reason=scanner_error；repro-4/openai.log。

建议：已提交SSE时产生协议兼容失败终态，保留真实错误并禁止重试已输出内容。

### 122 · #7047 · chatgpt经newapi访问glm报错：tools[7].type:type is illegal

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7047)

上游维护者指出智谱原生Responses尚不支持；该缺口本地已经回移支持/api/v1/responses。但原issue只给tools[7].type报错无完整tools请求，无法证明当前路径是否仍受供应商工具类型兼容约束。

- [relay/channel/zhipu_4v/adaptor.go:71](/workspace/LemonHub/relay/channel/zhipu_4v/adaptor.go:71)：普通智谱Responses路由已实现；Coding Plan明确拒绝。

建议：以同一完整脱敏tools请求分别直连普通智谱Responses与本地验证；勿将Coding Plan外推。

### 123 · #7040 · [Bug] 多密钥渠道所有 Key 自动禁用后，定期测试无法恢复渠道

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7040)

静态调用链与issue一致：健康检查调用普通testChannel，普通setup选择enabled Key；全部自动禁用时提前返回no enabled keys，无法真正探测已恢复上游。

- [controller/channel-test.go:1031](/workspace/LemonHub/controller/channel-test.go:1031)：健康检查调用普通testChannel。
- [controller/channel-test.go:190](/workspace/LemonHub/controller/channel-test.go:190)：testChannel经SetupContextForSelectedChannel选key。
- [model/channel.go:238](/workspace/LemonHub/model/channel.go:238)：无enabledIdx直接返回no enabled keys。
- [controller/channel-test.go:1085](/workspace/LemonHub/controller/channel-test.go:1085)：恢复要求无本地错误且有成功context。

验证／触发边界：将多Key全置auto-disabled并启动周期测试即可；本次静态核对。

建议：只在恢复探测中显式逐个测试auto-disabled Key，跳过manually-disabled，成功恢复相应key。

### 124 · #7018 · v1.0.0-rc.25版本：配置额度的上限变小了

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7018)

本地token额度校验使用宿主int64总额度域与decimal历史业务上限；默认QuotaPerUnit下仍允许500000000000000，没有使用单次账单int32上限。

- [controller/token.go:274](/workspace/LemonHub/controller/token.go:274)：历史maxTokenQuotaAmount=1_000_000_000。
- [controller/token.go:286](/workspace/LemonHub/controller/token.go:286)：宿主int最大值结合QuotaPerUnit计算，而非int32饱和。

### 125 · #7011 · 【Bug】表达式模式不支持 weekday 函数，无法通过表达式实现周一至周五的高峰定价

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7011)

局部复现同报错：后端weekday/hour已定义，可保存复杂原始表达式；但前端CostEstimator调用new Function且环境没有时间函数，仍显示weekday is not defined。不能据此断言当前后端保存或计费也失败。

- [web/src/features/pricing/lib/tier-expr.ts:291](/workspace/LemonHub/web/src/features/pricing/lib/tier-expr.ts:291)：本地eval环境只有计费变量和数学函数，无weekday/hour。
- [web/src/features/system-settings/models/tiered-pricing-editor.tsx:1394](/workspace/LemonHub/web/src/features/system-settings/models/tiered-pricing-editor.tsx:1394)：CostEstimator使用evalExprLocally。
- [web/src/features/system-settings/models/tiered-pricing-editor.tsx:1465](/workspace/LemonHub/web/src/features/system-settings/models/tiered-pricing-editor.tsx:1465)：展示Expression error。
- [pkg/billingexpr/run.go:113](/workspace/LemonHub/pkg/billingexpr/run.go:113)：后端已提供weekday实现。
- [web/src/features/pricing/lib/billing-expr.ts:673](/workspace/LemonHub/web/src/features/pricing/lib/billing-expr.ts:673)：复杂嵌套时间条件无法分离为可视化rule，保留原始billingExpr进入估算器。

验证／触发边界：Bun调用实际splitBillingExprAndRequestRules(issue原始复杂weekday/hour表达式)后，billingExpr保留原文；继续evalExprLocally得到error=weekday is not defined。已验证真实预处理链，不仅孤立调用估算器。

建议：前端估算器支持与后端一致的时区时间函数或请求后端估算；保留原始复杂表达式。

### 126 · #7007 · [Bug] 渠道重试未按优先级选择，实际按渠道 ID 顺序处理

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/7007)

当前内存与DB路径均按priority层级选择，未发现按ID顺序重试的对应实现；报告缺少渠道配置/具体重试日志，无法证实标题所述。同priority内部权重随机不保证遍历每个Key/渠道，与按ID是不同问题。

- [model/channel_cache.go:155](/workspace/LemonHub/model/channel_cache.go:155)：唯一priority倒序排序并按retry选目标层级。
- [model/channel_cache.go:167](/workspace/LemonHub/model/channel_cache.go:167)：仅目标priority渠道进入权重选择。
- [model/ability.go:63](/workspace/LemonHub/model/ability.go:63)：DB路径getPriority选择对应优先级。

### 127 · #7005 · OaiStreamHandler 逐帧滞后转发,两级 new-api 串联时首字延迟被放大到「上游帧间隔」量级

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/7005)

静态确证每次回调先发lastStreamData，再将本帧存为lastStreamData；第一帧必须等第二帧到达或流结束后才发，慢工具帧会放大TTFT。

- [relay/channel/openai/relay-openai.go:133](/workspace/LemonHub/relay/channel/openai/relay-openai.go:133)：回调先HandleStreamFormat(lastStreamData)。
- [relay/channel/openai/relay-openai.go:146](/workspace/LemonHub/relay/channel/openai/relay-openai.go:146)：本帧仅赋给lastStreamData。

验证／触发边界：上游先role帧、停顿5秒再content帧；静态路径确认，未额外跑网络基准。

建议：普通文本/role/tool delta即时发送，仅保留确需尾部处理的usage-only候选帧并保持音频usage语义。

### 128 · #6995 · claude code 接入 ollama 的时候 会报错 Unable to validate model: undefined is not an object (evaluating 'U.usage.input_tokens')

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6995)

静态确证支持Claude请求转换到Ollama，但返回完全忽略RelayFormat，非流式输出OpenAITextResponse，流式也是OpenAI chunks+[DONE]，没有Claude message/usage.input_tokens协议；Claude handler未补转换。具体Claude Code版本报错文案未运行复现。

- [relay/channel/ollama/adaptor.go:26](/workspace/LemonHub/relay/channel/ollama/adaptor.go:26)：允许Claude→OpenAI→Ollama请求。
- [relay/channel/ollama/adaptor.go:93](/workspace/LemonHub/relay/channel/ollama/adaptor.go:93)：DoResponse只按流式/非流式选Ollama handler，未区分Claude。
- [relay/channel/ollama/stream.go:339](/workspace/LemonHub/relay/channel/ollama/stream.go:339)：非流式始终序列化dto.OpenAITextResponse。
- [relay/claude_handler.go:221](/workspace/LemonHub/relay/claude_handler.go:221)：直接交给adaptor.DoResponse输出，然后计费。

验证／触发边界：标准/v1/messages经Ollama渠道：代码确定返回OpenAI形状；本次静态验证。

建议：根据RelayFormat将Ollama响应转回Claude协议，或明确拒绝尚不支持的协议，覆盖流/非流usage。

### 129 · #6985 · Admin cannot unbind built-in OAuth/OIDC account bindings

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6985)

本地绑定配置已经分离key和field，OIDC操作key=oidc、数据字段oidc_id，并有六种内建provider契约测试。

- [web/src/features/users/components/dialogs/user-binding-dialog.tsx:126](/workspace/LemonHub/web/src/features/users/components/dialogs/user-binding-dialog.tsx:126)：key:oidc、field:oidc_id。
- [web/src/features/users/components/dialogs/__tests__/user-binding-dialog.test.tsx:111](/workspace/LemonHub/web/src/features/users/components/dialogs/__tests__/user-binding-dialog.test.tsx:111)：期望DELETE bindings/${type}而非数据库字段名。

### 130 · #6981 · StreamScannerHandler drops upstream SSE comment heartbeats during active streams

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6981)

overlay已复现upstream-heartbeat评论行在data帧之间被丢弃；scanner重置内部超时但仅接受data:/DONE，关闭自有ping时下游可能闲置断开。

- [relay/helper/stream_scanner.go:250](/workspace/LemonHub/relay/helper/stream_scanner.go:250)：每行重置ticker。
- [relay/helper/stream_scanner.go:257](/workspace/LemonHub/relay/helper/stream_scanner.go:257)：非data:/DONE行continue，含SSE注释。

验证／触发边界：TestAudit6981通过：收到data:first但无upstream-heartbeat；repro-4/root.log。

建议：仅在实际数据已提交后转发/生成注释保活，避免提前提交200影响错误和重试。

### 131 · #6978 · [Bug] 同一套餐未到期时续费，新订阅从购买时刻重新起算，提前续费损失剩余时长

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6978)

静态确认同套餐再次购买仍以当前时刻起算并创建并行active行，不衔接已有最晚EndTime。这里确认时间重叠行为，不声称额度丢失；是否改为续期需要明确产品语义。

- [model/subscription.go:873](/workspace/LemonHub/model/subscription.go:873)：nowUnix取当前时间，calcPlanEndTime(now,plan)。
- [model/subscription.go:920](/workspace/LemonHub/model/subscription.go:920)：StartTime=now.Unix，EndTime=endUnix。
- [model/subscription.go:1624](/workspace/LemonHub/model/subscription.go:1624)：余额购买仍调用同一创建函数。

验证／触发边界：允许同套餐多次购买，旧订阅还剩3天时再购买，新StartTime仍now；静态确认。

建议：提供明确续期模式并在并发保护下按现有最晚EndTime衔接；额度叠加/重置规则需一起定义。

### 132 · #6976 · 视频模型 配置按次计费后的前置余额扣费报错问题

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6976)

本地TaskErrorFromAPIError已将预扣错误标LocalError，controller跳过渠道错误处理且停止重试；不会因余额不足停用渠道。

- [service/error.go:288](/workspace/LemonHub/service/error.go:288)：TaskErrorFromAPIError设置LocalError:true。
- [controller/relay.go:693](/workspace/LemonHub/controller/relay.go:693)：仅非LocalError执行processChannelError。
- [relay/relay_task.go:233](/workspace/LemonHub/relay/relay_task.go:233)：预扣失败经TaskErrorFromAPIError转换。

### 133 · #6972 · 活跃登录会话数量已达上限。请在一台已登录的设备上打开“登录会话”，使用“退出其他登录会话”将其撤销。如果无法访问任何已登录设备，请重置密码以退出所有会话。

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6972)

本地也保留50活跃会话上限，但这是已定义策略；重启容器不删除DB会话符合设计。issue后续通过提高上限/撤销会话恢复，没有证明异常生成会话根因。

- [service/auth_session.go:68](/workspace/LemonHub/service/auth_session.go:68)：统计活跃session，达到上限返回ErrUserSessionLimit。
- [common/constants.go:43](/workspace/LemonHub/common/constants.go:43)：默认活跃上限50，可由USER_SESSION_ACTIVE_LIMIT配置。

建议：若要改善无法访问旧设备的恢复体验，可另设计认证后撤销最旧会话；不将政策行为算程序缺陷。

### 134 · #6971 · 无法开启在线支付

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6971)

无准确版本、支付配置或请求返回体，评论为配置咨询。本地在线支付状态依赖易支付可用配置而非只勾选合规，无法确认相同程序缺陷。

- [controller/topup.go:105](/workspace/LemonHub/controller/topup.go:105)：enable_online_topup取isEpayTopUpEnabled。
- [controller/topup.go:214](/workspace/LemonHub/controller/topup.go:214)：配置检查要求EpayId/EpayKey/PayAddress非空。

### 135 · #6963 · 账号密码暴露

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6963)

已将密码type遮罩替换成文本CSS遮罩并关闭autocomplete，不再触发同一原生password输入自动填充机制；rc41回移记录对应2d8e50bf3。未做真实Chrome自动填充E2E。

- [web/src/features/usage-logs/components/common-logs-filter-bar.tsx:256](/workspace/LemonHub/web/src/features/usage-logs/components/common-logs-filter-bar.tsx:256)：遮罩使用-webkit-text-security:disc。
- [web/src/features/usage-logs/components/logs-filter-toolbar.tsx:82](/workspace/LemonHub/web/src/features/usage-logs/components/logs-filter-toolbar.tsx:82)：autoComplete=off。

### 136 · #6959 · RC.25 的 docker 镜像，裸启动后没有自动进入 setup

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6959)

初始化状态只在模块内缓存，页面刷新后重置并调用getSetupStatus，未初始化明确跳转setup；旧浏览器持久化状态导致绕过的路径已回移修复。

- [web/src/routes/__root.tsx:111](/workspace/LemonHub/web/src/routes/__root.tsx:111)：setupStatusChecked仅模块变量。
- [web/src/routes/__root.tsx:129](/workspace/LemonHub/web/src/routes/__root.tsx:129)：每个新页面会话检查setup并在status=false跳转。

### 137 · #6957 · v1.0.0-rc.25 UI控制台权限控制无效：系统管理关闭侧边栏聊天区域后，个人用户仍显示这些配置项

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6957)

静态确认仅个人设置表单仍显示被管理员关闭的聊天配置；不能据此称后台权限绕过。issue附带的日志管理信息泄漏在本地已有过滤，不能整体照搬旧结论。

- [web/src/features/profile/components/sidebar-modules-card.tsx:222](/workspace/LemonHub/web/src/features/profile/components/sidebar-modules-card.tsx:222)：个人配置直接遍历全部 sectionDefs，未读取系统 SidebarModulesAdmin；chat 仍在固定列表
- [web/src/hooks/use-sidebar-config.ts:202](/workspace/LemonHub/web/src/hooks/use-sidebar-config.ts:202)：实际导航有 admin × user AND gate
- [model/log.go:149](/workspace/LemonHub/model/log.go:149)：普通用户日志已剥离 admin_info、is_model_mapped、upstream_model_name 等

验证／触发边界：管理员关闭chat后进入普通用户个人设置，Chat Area 开关仍在。

建议：个人设置卡使用管理员配置过滤section/module；保留实际导航AND gate。

### 138 · #6952 · 重大BUG："活跃登录会话数量已达上限"导致管理员彻底无法登录

**疑似** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6952)

上限拒绝登录行为静态确定；无可用旧设备且未绑定可用邮箱时会有管理员恢复困难。但issue未给会话数量，不能确认仅升级就触发，也不能称永久无法恢复（可重置/运维修复）。

- [service/auth_session.go:72](/workspace/LemonHub/service/auth_session.go:72)：达到 UserSessionActiveLimit 直接拒绝新会话，无角色豁免和自动撤销旧会话
- [common/constants.go:43](/workspace/LemonHub/common/constants.go:43)：默认活跃上限50
- [controller/misc.go:421](/workspace/LemonHub/controller/misc.go:421)：自助重置密码需要非空email及有效token

验证／触发边界：已存在50个未过期会话的管理员尝试新登录；是否无自助恢复还取决于邮箱配置。

建议：评估认证成功后安全撤销最旧会话，或提供明确管理员恢复流程；不可直接取消所有防滥用限制。

### 139 · #6947 · 转发请求缺少 ResponseHeaderTimeout：上游不返回响应头时 goroutine 与请求体永久驻留，累积至 OOM（rc.23 实测 172.9h 死亡）

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6947)

本地已设置独立响应头超时；可显式配置0关闭。定向 TestRelayResponseHeaderTimeoutConfiguration 已通过。

- [service/http_client.go:108](/workspace/LemonHub/service/http_client.go:108)：两种transport构造分支之后配置ResponseHeaderTimeout并做溢出钳制
- [common/init.go:119](/workspace/LemonHub/common/init.go:119)：RELAY_RESPONSE_HEADER_TIMEOUT默认1800秒

验证／触发边界：go test ./service -run TestRelayResponseHeaderTimeoutConfiguration -count=1

建议：无需修复；仅核查部署没有显式关闭超时。

### 140 · #6940 · v1.0.0-rc.25 充值确认支付失败

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6940)

结合issue评论，根因是账户余额+充值超过int32导致报价0/error；本地钱包已扩为2^53-1且报价入口与入账统一，不再受4294美元隐含上限。不是只凭标题判定。

- [controller/topup.go:405](/workspace/LemonHub/controller/topup.go:405)：充值额度使用WalletQuotaFromDecimalStrict额度域
- [controller/topup.go:475](/workspace/LemonHub/controller/topup.go:475)：报价/下单前检查充值及钱包余额上限
- [common/wallet_quota.go:12](/workspace/LemonHub/common/wallet_quota.go:12)：MaxWalletQuota=2^53-1，已非int32

验证／触发边界：静态核对 /api/user/amount 的 RequestAmount 与 RequestEpay 调用链。

建议：无需移植旧int32修复。

### 141 · #6939 · 调用deepseek-v4-flash会报错：400 The 'reasoning_content’ in the thinking mode must be passed back to the API.

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6939)

按issue最终评论，直连官方原生路径正常，混用腾讯tokenhub/转换才出现。当前本地原生DeepSeek路径保留reasoning_content，不存在其主张的同协议普遍丢字段；跨协议转换应另测。

- [relay/channel/deepseek/adaptor.go:85](/workspace/LemonHub/relay/channel/deepseek/adaptor.go:85)：原生OpenAI请求仅补充模型思考配置后直接return request
- [relaykit/dto/openai_request.go:386](/workspace/LemonHub/relaykit/dto/openai_request.go:386)：Message.ReasoningContent仍参与JSON序列化

验证／触发边界：对照静态DeepSeek ConvertOpenAIRequest，无Messages重建或ReasoningContent置空。

建议：若实际失败，保存最终转换链和上游请求，按对应转换器定位。

### 142 · #6937 · undefinedDockerfile.dev build fails because relaykit/go.mod is not copied before go mod downloadDockerfile.dev build fails because relaykit/go.mod is not copied before go mod download

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6937)

原issue缺少replace模块go.mod的问题已在Dockerfile.dev修复；未执行整镜像构建。

- [Dockerfile.dev:15](/workspace/LemonHub/Dockerfile.dev:15)：go mod download前已经ADD relaykit/go.mod ./relaykit/go.mod

建议：无需改动。

### 143 · #6936 · [Withdrawn]

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6936)

当前issue正文仅“Withdrawn by author.”，评论也没有原始故障或代码线索；不存在可可靠映射到本地源码的命题，不能虚构定位。

该项没有足够可映射的缺陷命题，因此未虚构源码定位。

建议：需原始被撤回内容才可审计。

### 144 · #6929 · 提示余额不足，但是账户余额$7.18，未限制令牌配额

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6929)

作者评论明确承认误投且与本开源项目无关；提供的是CC Switch本地代理/第三方站余额报错，没有本地项目可验证故障。

- [service/billing_session.go:258](/workspace/LemonHub/service/billing_session.go:258)：钱包不足会依据服务端用户余额返回403，但不能据第三方截图识别同一账户

建议：无需根据该报告改代码。

### 145 · #6923 · 阶梯计费的规则组中时间规则存在异常

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6923)

本地时间范围编译已区分跨日与同日，UI文案已改Time range。旧保存错误表达式仍需要用户重新保存，不是当前生成器仍错。

- [web/src/features/pricing/lib/billing-expr.ts:839](/workspace/LemonHub/web/src/features/pricing/lib/billing-expr.ts:839)：start>end用||，同日start<=end用&&
- [web/src/features/pricing/lib/__tests__/time-rule-expr.test.ts:188](/workspace/LemonHub/web/src/features/pricing/lib/__tests__/time-rule-expr.test.ts:188)：已覆盖9-12与14-18组合表达式

建议：无需移植。

### 146 · #6920 · v1.0.0-rc.25 充值页面 /api/user/amount 无法正常获取待支付金额

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6920)

结合issue评论，根因是账户余额+充值超过int32导致报价0/error；本地钱包已扩为2^53-1且报价入口与入账统一，不再受4294美元隐含上限。不是只凭标题判定。

- [controller/topup.go:405](/workspace/LemonHub/controller/topup.go:405)：充值额度使用WalletQuotaFromDecimalStrict额度域
- [controller/topup.go:475](/workspace/LemonHub/controller/topup.go:475)：报价/下单前检查充值及钱包余额上限
- [common/wallet_quota.go:12](/workspace/LemonHub/common/wallet_quota.go:12)：MaxWalletQuota=2^53-1，已非int32

验证／触发边界：静态核对 /api/user/amount 的 RequestAmount 与 RequestEpay 调用链。

建议：无需移植旧int32修复。

### 147 · #6919 · 订阅管理页面没有“手动绑定”按钮

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6919)

和上游维护者评论一致，功能在用户管理的管理订阅中，非订阅计划列表；属于文档/入口认知差异，不是手动绑定缺失。

- [web/src/features/users/components/data-table-row-actions.tsx:220](/workspace/LemonHub/web/src/features/users/components/data-table-row-actions.tsx:220)：用户菜单提供Manage Subscriptions
- [web/src/features/subscriptions/components/dialogs/user-subscriptions-dialog.tsx:281](/workspace/LemonHub/web/src/features/subscriptions/components/dialogs/user-subscriptions-dialog.tsx:281)：弹窗提供Add subscription
- [web/src/features/subscriptions/api.ts:87](/workspace/LemonHub/web/src/features/subscriptions/api.ts:87)：通过用户订阅管理API创建绑定

建议：若维护文档，可明确用户管理入口。

### 148 · #6911 · 订阅预扣成功但最终补扣溢出时未按 allow_wallet_overflow 扣除钱包差额

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6911)

已离线复现：allow_wallet_overflow=true，订阅预扣60000、总额100000、实际160000，Settle返回subscription used exceeds total；订阅停60000，钱包1000000未动，少扣100000。现有结算重构未修此溢出分摊。

- [service/billing_session.go:62](/workspace/LemonHub/service/billing_session.go:62)：结算资金来源失败直接返回，无钱包溢出处理
- [service/funding_source.go:114](/workspace/LemonHub/service/funding_source.go:114)：SubscriptionFunding.Settle只调用PostConsumeUserSubscriptionDelta
- [model/subscription.go:2406](/workspace/LemonHub/model/subscription.go:2406)：newUsed超过AmountTotal返回错误，事务不更新
- [service/billing_session.go:483](/workspace/LemonHub/service/billing_session.go:483)：allow_wallet_overflow仅用于预扣失败回退

验证／触发边界：overlay测试 TestAuditBatch5SubscriptionOverflow；证据 /workspace/scratch/upstream-bug-audit/batch5_repro.log。

建议：事务内按订阅余额分摊补扣，超出部分在允许时扣钱包，同时更新日志资金拆分/退款幂等。

### 149 · #6908 · API密钥编辑，提示 额度值超出有效范围，最大值为 2147483647

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6908)

本地明确区分单次账单int32限制和令牌总余额；9999美元额度不再因为2147483647拦截编辑。

- [controller/token.go:286](/workspace/LemonHub/controller/token.go:286)：token总余额校验使用宿主int上限与历史业务额度限制，不用MaxQuota/int32
- [controller/token.go:427](/workspace/LemonHub/controller/token.go:427)：编辑仍走已修复的validateTokenQuota

建议：无需改动。

### 150 · #6898 · [bug] 请求覆写规则似乎没有被执行

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6898)

评论中作者已确认使用original_model后规则生效；model条件匹配的是映射后的上游名，未定义use_magical在非透传中不保留。这是条件/协议使用限制，不能判定覆写未执行。

- [relay/compatible_handler.go:169](/workspace/LemonHub/relay/compatible_handler.go:169)：覆写明确在转换和剥离字段之后执行
- [relay/common/override.go:2118](/workspace/LemonHub/relay/common/override.go:2118)：提供original_model上下文保存原始别名
- [relaykit/dto/openai_request.go:114](/workspace/LemonHub/relaykit/dto/openai_request.go:114)：请求按定义DTO重序列化，不保留任意未定义use_magical字段

建议：使用original_model及受支持字段；自定义字段需明确透传策略。

### 151 · #6897 · claude对接new-api 输入缓存命中率低，但是最近版本new-api正常

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6897)

报告仅比较旧版0.0.5与rc.24缓存命中率，没有原始请求/usage/转换链；当前字段存在不代表保证上游命中，无法证实本地存在相同缓存率问题。

- [relay/channel/claude/adaptor.go:28](/workspace/LemonHub/relay/channel/claude/adaptor.go:28)：Claude适配路径按具体协议处理，不能由日志比例推断上游实际缓存命中
- [relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:165](/workspace/LemonHub/relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:165)：当前转换保留媒体CacheControl
- [service/billing_usage.go:173](/workspace/LemonHub/service/billing_usage.go:173)：Claude缓存读取量来自上游CacheReadInputTokens

建议：需同一上游同一序列请求与usage对照。

### 152 · #6894 · Playground 编辑消息时输入光标跳回文首，表现为从右往左插入

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6894)

原issue按键导致EditorView重建的依赖链已经消除；本地还更新重建时的最新文档值。

- [web/src/components/ai-elements/code-block.tsx:309](/workspace/LemonHub/web/src/components/ai-elements/code-block.tsx:309)：onKeyDown经ref读取，不再进入editorExtensions依赖
- [web/src/components/ai-elements/code-block.tsx:324](/workspace/LemonHub/web/src/components/ai-elements/code-block.tsx:324)：extensions仅依赖language/readOnly/showLineNumbers
- [web/src/components/ai-elements/code-block.tsx:363](/workspace/LemonHub/web/src/components/ai-elements/code-block.tsx:363)：initialValueRef持续更新为最新文档

建议：无需改动。

### 153 · #6887 · 突发500问题，无法进入playground

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6887)

第三方.pro/unknown版本且没有堆栈和可重现聊天数据，不能从通用500页面推断同一缺陷；本地已有存储限制及异常捕获，但不能据此声称所有Playground崩溃已修。

- [web/src/features/playground/lib/storage/storage.ts:56](/workspace/LemonHub/web/src/features/playground/lib/storage/storage.ts:56)：超大存储先做字节数限制
- [web/src/features/playground/lib/storage/storage.ts:341](/workspace/LemonHub/web/src/features/playground/lib/storage/storage.ts:341)：loadMessages校验schema、裁剪内容并catch解析错误

建议：需浏览器错误堆栈和脱敏历史数据。

### 154 · #6885 · 批量模式和标签模式同时启用时，批量模式无效

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6885)

已用本地@tanstack/table-core离线复现：选中标签子行1时，getFilteredSelectedRowModel().rows=[]而flatRows=[1]；因此操作栏不显示且ID收集为空。

- [web/src/features/channels/components/channels-table.tsx:298](/workspace/LemonHub/web/src/features/channels/components/channels-table.tsx:298)：标签模式将渠道聚合成父/子行
- [web/src/features/channels/components/channels-table.tsx:329](/workspace/LemonHub/web/src/features/channels/components/channels-table.tsx:329)：标签父行不可选择，真正选择的是子行
- [web/src/components/data-table/toolbar/bulk-actions.tsx:56](/workspace/LemonHub/web/src/components/data-table/toolbar/bulk-actions.tsx:56)：计数使用getFilteredSelectedRowModel().rows
- [web/src/features/channels/components/data-table-bulk-actions.tsx:71](/workspace/LemonHub/web/src/features/channels/components/data-table-bulk-actions.tsx:71)：批量操作IDs也读取.rows而不是.flatRows

验证／触发边界：node导入仓库已安装table-core，创建tag父行/children id=1，rowSelection={1:true}，输出rows=[]/flatRows=[1]。

建议：公共批量栏和渠道ID收集均使用flatRows，并确保过滤真实渠道ID/避免父行重复。

### 155 · #6883 · [Bug] Chat Completions → OpenAI Responses 转换中 cached_tokens 未参与结算

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6883)

本地缓存明细归一化已存在，定向 TestCalculateTextQuotaSummaryUsesOpenAIResponsesInputTokenDetails 实际通过。

- [service/billing_usage.go:121](/workspace/LemonHub/service/billing_usage.go:121)：Responses InputTokensDetails已合并到PromptTokensDetails
- [service/text_quota_test.go:356](/workspace/LemonHub/service/text_quota_test.go:356)：覆盖Responses缓存明细参与计费

验证／触发边界：go test ./service -run TestCalculateTextQuotaSummaryUsesOpenAIResponsesInputTokenDetails -count=1；日志batch5_fixed_checks.log。

建议：无需移植旧修复。

### 156 · #6877 · [bug?/feat req]管理后台无明确提示、若不配置worker易暴露源站IP

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6877)

未配置Worker时，对通过SSRF策略验证的用户公网URL由源站发起连接，目标可见出口IP，这是正常网络行为。后台增加出口IP说明、强制Worker选项属于产品增强；已有SSRF保护，不构成私网SSRF绕过或同类代码缺陷。

- [service/webhook.go:90](/workspace/LemonHub/service/webhook.go:90)：未配Worker时SSRF校验后仍由源站client.Do用户Webhook
- [service/download.go:67](/workspace/LemonHub/service/download.go:67)：未配Worker直接向通过SSRF验证的远程媒体URL发起GET
- [web/src/features/system-settings/integrations/worker-settings-section.tsx:134](/workspace/LemonHub/web/src/features/system-settings/integrations/worker-settings-section.tsx:134)：Worker说明只有转发说明，无源站IP提示

验证／触发边界：普通用户将Webhook指向自己公网服务器，未配Worker且白名单允许时触发通知观察来源IP。

建议：说明直连会暴露出口IP，可选强制Worker/公网域名白名单；是否默认禁用应另作产品决策。

### 157 · #6876 · 活跃登录会话数量已达上限。请在一台已登录的设备上打开“登录会话”，使用“退出其他登录会话”将其撤销。如果无法访问任何已登录设备，请重置密码以退出所有会话

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6876)

该issue要求取消会话上限，上游评论明确为设计限制；重复登录不复用会话触发限制是预期行为。与6952的无可用恢复入口风险分开评价。

- [service/auth_session.go:72](/workspace/LemonHub/service/auth_session.go:72)：活跃会话上限是明确实施的规则
- [common/init.go:150](/workspace/LemonHub/common/init.go:150)：可由USER_SESSION_ACTIVE_LIMIT调整
- [service/auth_session_test.go:128](/workspace/LemonHub/service/auth_session_test.go:128)：已有测试要求50个活跃会话后新会话被拒

建议：脚本复用会话并在退出时撤销，按容量配置上限。

### 158 · #6872 · 使用日志里模型标签的颜色都变成一样了，看着好累

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6872)

当前GPT/Claude/Gemini等常见模型标签统一灰底，依靠彩色提供商图标区分；未知模型仍会autoColor。这是可核对的设计行为，恢复旧配色属于视觉偏好/增强，不构成同类代码缺陷。

- [web/src/features/usage-logs/components/model-badge.tsx:134](/workspace/LemonHub/web/src/features/usage-logs/components/model-badge.tsx:134)：已识别provider的模型不传autoColor，统一bg-muted/30并显示彩色图标

验证／触发边界：日志列表比较已识别的gpt/claude/gemini模型。

建议：如希望恢复旧体验可按模型名着色或提供模式选择。

### 159 · #6864 · 阿里视频任务轮询不支持多密钥模式（multi-key）

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6864)

静态确认阿里多密钥任务提交使用选中单key，持久化却为空，后台轮询回退带换行渠道Key，Authorization将被Go拒绝；本地Gemini专用key保存修复未覆盖阿里。

- [model/task.go:263](/workspace/LemonHub/model/task.go:263)：InitTask仅Gemini/Vertex持久化实际选中API key，阿里未保存
- [service/task_polling.go:472](/workspace/LemonHub/service/task_polling.go:472)：轮询先取ch.Key，仅PrivateData.Key非空才覆盖
- [service/task_polling.go:478](/workspace/LemonHub/service/task_polling.go:478)：FetchTask实际收到多行整串key

验证／触发边界：阿里渠道multi-key至少两个换行分隔key，提交任务后检查private_data及轮询Authorization错误。

建议：所有多key异步任务在提交时持久化本次单key，轮询优先读取该key而非重新轮询选key。

### 160 · #6860 · 在游乐场内调用ollama渠道的模型输出一半终止后Ollama侧还在继续推理思考

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6860)

静态确认Playground停止请求不会经Ollama专用流循环主动取消上游：忽略写失败且不监听取消，要等上游done/EOF才CloseResponseBodyGracefully。通用SSE扫描器的取消处理不覆盖该路径。

- [relay/channel/api_request.go:320](/workspace/LemonHub/relay/channel/api_request.go:320)：出站请求用http.NewRequest，未绑定c.Request.Context
- [relay/channel/ollama/stream.go:161](/workspace/LemonHub/relay/channel/ollama/stream.go:161)：Ollama专用scanner循环不检查下游context
- [relay/channel/ollama/stream.go:180](/workspace/LemonHub/relay/channel/ollama/stream.go:180)：写客户端错误被忽略，循环继续读上游

验证／触发边界：Ollama长推理/长输出期间取消下游连接，当前handler仍可持续读取上游。

建议：为Ollama转发绑定取消上下文或退出流循环时关闭上游body，并处理写失败。

### 161 · #6859 · Bug: Chat→Claude Messages 转换静默丢弃无参数工具

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6859)

本地已保留无参数function工具且通过共享schema函数转换，原始不带ok的type字符串断言也已移除。

- [relaykit/relayconvert/internal/oai_chat/to_claude_messages_req.go:35](/workspace/LemonHub/relaykit/relayconvert/internal/oai_chat/to_claude_messages_req.go:35)：function类型不再因Parameters=nil跳过
- [relaykit/relayconvert/internal/shared/claude/schema.go:3](/workspace/LemonHub/relaykit/relayconvert/internal/shared/claude/schema.go:3)：统一schema转换补默认object/properties，安全处理类型

建议：无需移植。

### 162 · #6857 · Stripe 官方最近调整了 API 的兼容性规则导致stripe的sdk版本号报错,请更新最新版本中的sttipe的sdk版本号

**疑似** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6857)

本地缓存SDK api_version.go实际值仍为2025-02-24.acacia。若商户启用要求basil及以上的Managed Payments，同样不兼容；普通Stripe账户不一定受影响，未用真实Stripe商户凭据复现。

- [go.mod:44](/workspace/LemonHub/go.mod:44)：仍使用stripe-go/v81 v81.4.0
- [controller/topup_stripe.go:462](/workspace/LemonHub/controller/topup_stripe.go:462)：通过该SDK创建Checkout session，无Stripe-Version覆盖

验证／触发边界：启用Managed Payments的商户通过本地Stripe充值，需验证是否仍返回最低2025-03-31.basil要求。

建议：评估升级SDK及webhook兼容；或明确当前不支持Managed Payments。

### 163 · #6840 · 集群部署注册验证码提示错误

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6840)

静态确认即使配置Redis，两节点也不共享这份验证码；发信落A、注册落B时B必然查不到。影响非粘性会话集群注册，密码重置同类机制也受影响。

- [common/verification.go:38](/workspace/LemonHub/common/verification.go:38)：发送验证码仅写本进程verificationMap
- [common/verification.go:50](/workspace/LemonHub/common/verification.go:50)：验证只读本进程map，无Redis/DB共享
- [controller/misc.go:361](/workspace/LemonHub/controller/misc.go:361)：注册邮件调用上述RegisterVerificationCodeWithKey
- [controller/user.go:232](/workspace/LemonHub/controller/user.go:232)：注册校验调用上述VerifyCodeWithKey

验证／触发边界：节点A发验证码，再将注册请求指定节点B，正确验证码被拒。

建议：将验证码/用途/过期时间放共享Redis或数据库，并实现原子验证消费；粘性会话只能临时缓解。

### 164 · #6831 · [Bug] 单笔充值金额超过 int32 额度上限（默认 QuotaPerUnit 下约 $4,294.96）时回调被拒：客户已付款，但订单永久 pending、额度未入账

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6831)

本地已将充值和钱包额度独立扩容，并在下单前检查；10000美元/500000 QuotaPerUnit不会命中原int32上限，不再付完才拒入账。

- [model/topup.go:411](/workspace/LemonHub/model/topup.go:411)：易支付入账已用WalletQuotaFromDecimalStrict
- [common/wallet_quota.go:12](/workspace/LemonHub/common/wallet_quota.go:12)：钱包域2^53-1而非MaxInt32
- [controller/topup.go:475](/workspace/LemonHub/controller/topup.go:475)：支付前校验充值额度/余额域

建议：无需移植旧修复。

### 165 · #6822 · /v1/responses 流式快照 created_at 为浮点时三个快照事件被丢弃，usage 丢失、计费退化为估算

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6822)

已离线实际复现：ResponsesStreamResponse解析created_at:1786588600.0报cannot unmarshal number ... into ... int。影响经通用OpenAI Responses处理路径的兼容上游快照，包含completed/usage；不能保证所有通道都受影响（专用上游可能归一化）。

- [relaykit/dto/openai_response.go:299](/workspace/LemonHub/relaykit/dto/openai_response.go:299)：OpenAIResponsesResponse.CreatedAt仍为int
- [relay/channel/openai/relay_responses.go:94](/workspace/LemonHub/relay/channel/openai/relay_responses.go:94)：解析失败sr.Error后return，该快照不进入后续usage/事件处理

验证／触发边界：overlay TestAuditBatch5ResponsesFloatCreatedAt失败，日志batch5_repro.log。

建议：采用受界限保护的兼容数字时间戳解析，或在专用SGLang适配器归一化；覆盖真实快照及usage计费。

### 166 · #6817 · 测试 Anthropic 渠道(type=14)时因空 tools:[] 导致 400 错误

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6817)

无工具OpenAI请求转换后Tools保持nil，omitempty不再输出tools:[]；修复已在转换层覆盖渠道测试和常规调用。

- [relaykit/relayconvert/internal/oai_chat/to_claude_messages_req.go:94](/workspace/LemonHub/relaykit/relayconvert/internal/oai_chat/to_claude_messages_req.go:94)：只有len(claudeTools)>0时才赋值ClaudeRequest.Tools

建议：无需改动。

### 167 · #6805 · database is locked (SQLITE_BUSY) cannot start a transaction within a transaction

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6805)

本地仍存在SQLite并发写负荷风险，但issue缺版本、DSN、并发规模、事务栈，不能证明busy_timeout后嵌套事务错误是本地同一代码根因；也不能仅发现SQLite就标confirmed。

- [model/main.go:194](/workspace/LemonHub/model/main.go:194)：SQLite同样默认SQL_MAX_OPEN_CONNS=1000
- [model/subscription.go:2395](/workspace/LemonHub/model/subscription.go:2395)：订阅结算用DB.Transaction
- [model/main.go:149](/workspace/LemonHub/model/main.go:149)：SQLite使用glebarez/sqlite驱动

建议：需可控并发压测/失败SQL栈；确认后评估SQLite连接数、忙等待与事务长度。

### 168 · #6804 · [Bug] Auto-group API keys: group badge missing and "Cross-group" badge rendered unconditionally

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6804)

静态确认与上游报告完全相同：cross_group_retry=false的auto令牌也显示Cross-group，且无明确Auto徽章。

- [web/src/features/keys/components/api-key-group-cell.tsx:79](/workspace/LemonHub/web/src/features/keys/components/api-key-group-cell.tsx:79)：auto组仍无条件显示Cross-group且注释AutoGroupBadge
- [web/src/features/keys/components/api-key-group-cell.tsx:36](/workspace/LemonHub/web/src/features/keys/components/api-key-group-cell.tsx:36)：crossGroupRetry虽为props但未被渲染读取

验证／触发边界：auto令牌关闭跨组重试，查看密钥列表Group列。

建议：恢复crossGroupRetry条件和Auto组标识，并处理三徽章布局。

### 169 · #6797 · [Bug] Anthropic stream 空 body:上游无 "data:" 前缀的裸 JSON 帧被 StreamScannerHandler 丢弃(deepseek-v4-pro)

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6797)

对于报告中的无data前缀裸JSON，上述行为在本地仍存在；但这不是标准Anthropic SSE，上游维护者后续已确认OpenCode Go修正。不能把不兼容第三方非标准流直接算本项目标准协议bug。

- [relay/helper/stream_scanner.go:257](/workspace/LemonHub/relay/helper/stream_scanner.go:257)：通用SSE扫描器仅接受data:/DONE帧

验证／触发边界：如人为发送裸JSON行仍被过滤；标准data: SSE正常。

建议：若确需兼容该第三方，应做明确适配器，不宜放宽所有SSE扫描器。

### 170 · #6782 · task lease expired，日志无法删除

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6782)

评论定位的MySQL无变化UPDATE被当锁丢失问题已修复；零变更会复核租约，不再直接返回ErrSystemTaskLockLost。其他真实租约过期仍需运营日志判断。

- [model/system_task.go:326](/workspace/LemonHub/model/system_task.go:326)：RowsAffected==0后再次Count验证任务及lease仍有效
- [model/system_task.go:330](/workspace/LemonHub/model/system_task.go:330)：注释明确处理MySQL相同秒同state的no-op更新

建议：无需移植。

### 171 · #6781 · [使用] 使用日志不显示缓存以及invalid tool parameters

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6781)

正文混合工具参数错误与vLLM原生Anthropic缓存缺失，评论也要求提供vLLM原始usage。当前已映射标准Claude cache_read_input_tokens/cache_creation_input_tokens；缺少工具错误的具体请求/响应及vLLM版本，不能断定同一故障仍在。

- [relaykit/relayconvert/internal/claude_messages/to_oai_chat_resp.go:167](/workspace/LemonHub/relaykit/relayconvert/internal/claude_messages/to_oai_chat_resp.go:167)：Claude标准usage缓存字段转换。
- [relay/channel/claude/relay-claude.go:110](/workspace/LemonHub/relay/channel/claude/relay-claude.go:110)：message_delta补全缓存usage。

验证／触发边界：静态源码核对；未执行真实供应商或浏览器端到端复现。

建议：分别采集两种渠道的最小请求、原始SSE工具增量和usage再定位。

### 172 · #6775 · 模型元信息不准确，例如vendor

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6775)

模型同步按上游vendor_name解析供应商，没有随机分配vendor；本次只读抓取当前官方metadata中kimi-k3已为Moonshot AI。历史错误属于外部metadata，当前源已更正。

- [controller/model_sync.go:372](/workspace/LemonHub/controller/model_sync.go:372)：ensureVendorID依据up.VendorName建立映射。
- [controller/model_sync.go:26](/workspace/LemonHub/controller/model_sync.go:26)：官方metadata地址。

验证／触发边界：2026-10-06只读请求https://basellm.github.io/llm-metadata/api/newapi/models.json，kimi-k3.vendor_name=Moonshot AI；本地数据库旧记录未检查。

建议：已有错误存量模型可重新同步vendor字段；保留管理员手工覆盖。

### 173 · #6771 · 出现10%-20% 请求日志不扣费情况

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6771)

仅有网络不佳/请求不计费描述，无请求协议、模型、原始usage或日志完整内容。HTTP Responses缺usage估算已回移，能按已观察输出结算；若输出与usage均未收到则无法据现有信息证明应收金额。不能将原帖概率直接套到本地。

- [relay/channel/openai/responses_usage.go:115](/workspace/LemonHub/relay/channel/openai/responses_usage.go:115)：收集已观察文本、工具参数、推理和拒绝增量。
- [relay/channel/openai/responses_usage.go:147](/workspace/LemonHub/relay/channel/openai/responses_usage.go:147)：缺usage时基于实际输出估算并标记estimated。

验证／触发边界：静态源码核对；未执行真实供应商或浏览器端到端复现。

建议：保留脱敏SSE及断开时序，区分上游已计费但网关未收到内容与网关漏结算。

### 174 · #6763 · 启用 Turnstile 后密码错误会导致后续登录校验失败

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6763)

登录提交前保存本次captcha token后立即清空状态并增加widget key，下次请求不会复用已消耗token。

- [web/src/features/auth/sign-in/components/user-auth-form.tsx:157](/workspace/LemonHub/web/src/features/auth/sign-in/components/user-auth-form.tsx:157)：单次token立即清空并重建组件。
- [web/src/features/auth/sign-in/components/user-auth-form.tsx:170](/workspace/LemonHub/web/src/features/auth/sign-in/components/user-auth-form.tsx:170)：当前登录使用submittedCaptchaToken。

验证／触发边界：静态源码核对；未执行真实供应商或浏览器端到端复现。

建议：无须直接移植；按所述边界进一步核验。

### 175 · #6752 · IP白名单功能过滤的IP不对

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6752)

报告的100.127.*地址可能来自CGNAT/代理。当前IP白名单使用Gin ClientIP并支持显式TRUSTED_PROXIES；没有证据证明应用把公网IP改成该地址。真实地址恢复取决于代理转发头与信任配置。

- [middleware/auth.go:442](/workspace/LemonHub/middleware/auth.go:442)：IP白名单核对c.ClientIP()。
- [middleware/trusted_proxies.go:22](/workspace/LemonHub/middleware/trusted_proxies.go:22)：可配置真实反向代理IP/CIDR；默认不信任100.64/10。

验证／触发边界：静态源码核对；未执行真实供应商或浏览器端到端复现。

建议：核对代理出口、X-Forwarded-For及TRUSTED_PROXIES，不能无条件信任任意来源头。

### 176 · #6748 · Claude Code title generation requests can cause 429 retry amplification across channels

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6748)

这是要求对特定Claude Code标题prompt本地短路的功能请求。正常模型请求按管理员RetryTimes/状态码策略重试是现有语义；正文承认429来自上游。未发现超越配置上限的独立重试bug。

- [controller/relay.go:200](/workspace/LemonHub/controller/relay.go:200)：重试受common.RetryTimes约束。
- [controller/relay.go:367](/workspace/LemonHub/controller/relay.go:367)：shouldRetry集中处理可重试状态及跳过条件。

验证／触发边界：静态源码核对；未执行真实供应商或浏览器端到端复现。

建议：如确有需求，另设计可配置后台标题策略；先收紧429重试配置。

### 177 · #6746 · rerank模型配置后代理请求返回null

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6746)

原帖没有提供可读的原始上游响应JSON及确定渠道类型。通用rerank只解析顶层results，阿里适配器才解析output.results；仅改请求参数不能令专有响应结构自动兼容。评论亦指明此协议边界，不能确定为本地新回归。

- [relay/common_handler/rerank.go:63](/workspace/LemonHub/relay/common_handler/rerank.go:63)：通用rerank响应按标准dto.RerankResponse解码。
- [relay/channel/ali/rerank.go:63](/workspace/LemonHub/relay/channel/ali/rerank.go:63)：阿里单独从Output.Results映射。

验证／触发边界：静态源码核对；未执行真实供应商或浏览器端到端复现。

建议：使用与上游格式相符的渠道，或提供完整响应后补专用响应转换。

### 178 · #6745 · [Bug] CC Switch dialog model list not filtered by API key's group

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6745)

CC Switch查询无token分组参数，父组件也仅传key；后端空group明确返回用户所有可用分组模型。因此指定vip组的key可在导出下拉中选择仅default组支持的模型。

- [web/src/features/keys/components/dialogs/cc-switch-dialog.tsx:168](/workspace/LemonHub/web/src/features/keys/components/dialogs/cc-switch-dialog.tsx:168)：统一queryKey和无参getUserModels。
- [web/src/features/keys/components/api-keys-dialogs.tsx:36](/workspace/LemonHub/web/src/features/keys/components/api-keys-dialogs.tsx:36)：未传currentRow.group。
- [controller/user.go:775](/workspace/LemonHub/controller/user.go:775)：空group遍历所有用户可用分组。

验证／触发边界：静态确定：配置用户可访问default/vip，key仅vip，打开CC Switch下拉仍取GET /api/user/models全集。

建议：按key的实际group/auto_groups/model_limits筛选并把这些维度放入queryKey；本地支持多group，不能仅照搬上游单group补丁。

### 179 · #6744 · 容器部署下 monitor_memory_threshold 永不触发：system_monitor 读的是宿主机 /proc/meminfo 而非 cgroup 限额

**确认存在** · P1 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6744)

监控仍直接使用gopsutil mem.VirtualMemory().UsedPercent，未读cgroup；容器达到自身memory.max而宿主机使用率低时，不会触发所配置内存503保护。并非任何容器永远不触发，而是判定口径错误。CPU同样为宿主口径。

- [common/system_monitor.go:65](/workspace/LemonHub/common/system_monitor.go:65)：MemoryUsage来自宿主VirtualMemory UsedPercent。
- [middleware/performance.go:60](/workspace/LemonHub/middleware/performance.go:60)：用该MemoryUsage与MemoryThreshold比较。

验证／触发边界：静态证据确定，未制造OOM：例如宿主128GiB使用40%、容器限1GiB使用95%，阈值90仍不触发。

建议：按有限cgroup v1/v2实际限制计算容器水位，读取失败或无上限时回退宿主；同时明确CPU口径。

### 180 · #6732 · 设置 GLOBAL_API_RATE_LIMIT=1000000 时内存限流器资源占用高导致OOM

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6732)

作者评论用pprof纠正归因为高GLOBAL_API_RATE_LIMIT按IP预分配，而非最初猜测Responses builder。本地已改为每个实际接受请求一个链表节点，完全不按maxRequestNum预分配。长流累计仍可单独评估，但不是此issue最终确认根因。

- [common/rate-limit.go:9](/workspace/LemonHub/common/rate-limit.go:9)：按实际accepted request分配节点。
- [common/rate-limit.go:126](/workspace/LemonHub/common/rate-limit.go:126)：Request仅比较阈值后append一个节点。
- [common/rate_limit_test.go:11](/workspace/LemonHub/common/rate_limit_test.go:11)：百万逻辑上限不预分配回归。

验证／触发边界：本次go test定向TestInMemoryRateLimiterLargeLimitDoesNotPreallocate通过；日志fixed6-go.log。

建议：无须直接移植；按所述边界进一步核验。

### 181 · #6724 · 模型定价-上游价格同步崩溃

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6724)

缺崩溃堆栈/退出原因，评论维护者无法复现。本地价格抓取有8并发上限、请求context超时及10MB读取上限，代码查验未得到与所述502/容器重启对应的确定panic或OOM链。

- [controller/ratio_sync.go:197](/workspace/LemonHub/controller/ratio_sync.go:197)：8个并发抓取信号量。
- [controller/ratio_sync.go:251](/workspace/LemonHub/controller/ratio_sync.go:251)：每次抓取设置context timeout。
- [controller/ratio_sync.go:309](/workspace/LemonHub/controller/ratio_sync.go:309)：响应读取受10MB限制。

验证／触发边界：静态源码核对；未执行真实供应商或浏览器端到端复现。

建议：需要退出码、panic/OOM日志及脱敏上游返回体，不能凭502判定后端崩溃原因。

### 182 · #6715 · [Bug] Vertex AI Gemini 的 /v1/messages 请求未转换，渠道测试却误报转换成功

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6715)

Vertex的Gemini模型Init选择Gemini请求模式并构造Google generateContent URL，但ConvertClaudeRequest无模式分支，仍输出含anthropic_version/messages/max_tokens的VertexClaude体，生产/v1/messages调用不兼容。原帖渠道测试假阳性部分已修：本地测试现在用真实ClaudeRequest和ConvertClaudeRequest，因此会正确暴露相同错误。

- [relay/channel/vertex/adaptor.go:96](/workspace/LemonHub/relay/channel/vertex/adaptor.go:96)：无Gemini分支，直接copyRequest为VertexAIClaudeRequest。
- [relay/channel/vertex/adaptor.go:116](/workspace/LemonHub/relay/channel/vertex/adaptor.go:116)：非Claude/开源模型设RequestModeGemini。
- [relay/channel/vertex/adaptor.go:137](/workspace/LemonHub/relay/channel/vertex/adaptor.go:137)：Gemini模式URL指向Google publisher。
- [controller/channel-test.go:394](/workspace/LemonHub/controller/channel-test.go:394)：测试现按ClaudeRequest实际类型转换。

验证／触发边界：本次临时Go探针真实调用Vertex.Init+ConvertClaudeRequest：request_mode=2，却序列化出anthropic_version、messages、max_tokens。日志probe6.log；未调用真实Vertex。

建议：在Gemini模式复用Claude->Gemini请求转换并核对响应格式；用实际Vertex路径回归非流式、流式、工具和usage。

### 183 · #6712 · Claude Code/Desktop 直连 OpenGateway 需经 cc-headless 适配层(模型映射/格式整流/媒体降级)

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6712)

作者评论明确承认误发：属于OpenGateway内部部署仓库cc-headless适配层，已移至正确仓库。本项目没有该部署拓扑，不能据此外推共同bug。

- [router/relay-router.go:91](/workspace/LemonHub/router/relay-router.go:91)：本项目直接提供/v1/messages入口。

验证／触发边界：静态源码核对；未执行真实供应商或浏览器端到端复现。

建议：无需本仓库修复。

### 184 · #6710 · 渠道管理先清空模型再点击获取上游模型列表，清空前的模型依然被勾选

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6710)

渠道编辑器把当前表单模型数组作为existingModelsOverride传入获取模型弹窗，空数组使用??保留，不会回落到数据库旧模型。清空后再获取不再默认勾选旧值。

- [web/src/features/channels/components/drawers/channel-mutate-drawer.tsx:4929](/workspace/LemonHub/web/src/features/channels/components/drawers/channel-mutate-drawer.tsx:4929)：传当前表单currentModelsArray。
- [web/src/features/channels/components/dialogs/fetch-models-dialog.tsx:98](/workspace/LemonHub/web/src/features/channels/components/dialogs/fetch-models-dialog.tsx:98)：override空数组优先于activeChannel旧值。

验证／触发边界：静态源码核对；未执行真实供应商或浏览器端到端复现。

建议：无须直接移植；按所述边界进一步核验。

### 185 · #6700 · 加余额后，日志的user应是被操作的人，实际显示为操作管理员

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6700)

当前是有意设计的操作审计语义：日志user_id归属操作者，target_user_id记录被操作用户；并非充值金额加给错误账号。评论也确认actor语义，要求被操作用户额外流水属于另一个需求。

- [controller/user.go:1343](/workspace/LemonHub/controller/user.go:1343)：额度变更传入目标user.Id。
- [controller/audit.go:116](/workspace/LemonHub/controller/audit.go:116)：明确actor归属和target_user_id结构。

验证／触发边界：静态源码核对；未执行真实供应商或浏览器端到端复现。

建议：若需用户余额变化历史，可另增与操作审计关联的资金日志，避免改乱审计actor。

### 186 · #6696 · qwen3-rerank 模型Bearer认证失效

**不适用／非缺陷** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6696)

作者评论撤回并说明重启IDE后正常。本地/v1/rerank位于TokenAuth中间件之后，任意错误token会ValidateUserToken失败返回401，未发现认证绕过路径。

- [router/relay-router.go:75](/workspace/LemonHub/router/relay-router.go:75)：/v1共用TokenAuth。
- [router/relay-router.go:144](/workspace/LemonHub/router/relay-router.go:144)：rerank注册在受保护httpRouter。
- [middleware/auth.go:421](/workspace/LemonHub/middleware/auth.go:421)：token验证失败立即拒绝。

验证／触发边界：静态源码核对；未执行真实供应商或浏览器端到端复现。

建议：无需修复已撤回客户端问题。

### 187 · #6694 · anthropic 协议发送错误

**证据不足** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6694)

缺少渠道类型、透传设置及最小请求。标准OpenAI渠道的Claude转换明确把tools包装为type=function/function对象，与帖中裸Anthropic tools到OpenAI的报错相反；可能为原生Anthropic端点或透传上游不兼容，无法证明共同转换缺陷。

- [relay/channel/openai/adaptor.go:68](/workspace/LemonHub/relay/channel/openai/adaptor.go:68)：Claude入站转OpenAI请求。
- [relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:81](/workspace/LemonHub/relaykit/relayconvert/internal/claude_messages/to_oai_chat_req.go:81)：工具明确写入Function.Name/Description/Parameters。

验证／触发边界：静态源码核对；未执行真实供应商或浏览器端到端复现。

建议：补渠道设置及转换前后原始tools；区分原生上游协议与网关转换。

### 188 · #6689 · Bug: Advanced Custom渠道测试连接选择 /v1/messages 端点时构造 OpenAI chat 请求体，导致 Anthropic→OpenAI Chat 转换器报错 (convert_request_failed)

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6689)

渠道测试已按Anthropic/Gemini实际协议构造各自DTO，并依据Go类型调用ConvertClaudeRequest/ConvertGeminiRequest，不会再把Claude路由喂给ConvertOpenAIRequest。

- [controller/channel-test.go:781](/workspace/LemonHub/controller/channel-test.go:781)：Anthropic构造ClaudeRequest。
- [controller/channel-test.go:390](/workspace/LemonHub/controller/channel-test.go:390)：按实际请求类型分发适配器。

验证／触发边界：静态源码核对；未执行真实供应商或浏览器端到端复现。

建议：无须直接移植；按所述边界进一步核验。

### 189 · #6688 · [Bug] 非 root 管理员无法管理订阅套餐：合规状态读取依赖 root-only 的 /api/option/，前端将 403 误判为"未确认合规"

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6688)

订阅管理AdminAuth允许普通管理员，但前端读RootAuth保护的/api/option；请求拒绝后默认compliance_confirmed=false，禁用创建编辑操作，即便root已确认合规。

- [web/src/features/subscriptions/components/subscriptions-provider.tsx:52](/workspace/LemonHub/web/src/features/subscriptions/components/subscriptions-provider.tsx:52)：useSystemOptions失败/无数据默认为false。
- [router/api-router.go:291](/workspace/LemonHub/router/api-router.go:291)：optionRoute要求RootAuth。
- [router/api-router.go:223](/workspace/LemonHub/router/api-router.go:223)：subscription/admin仅要求AdminAuth。
- [web/src/features/subscriptions/components/subscriptions-primary-buttons.tsx:34](/workspace/LemonHub/web/src/features/subscriptions/components/subscriptions-primary-buttons.tsx:34)：未确认状态禁用创建。

验证／触发边界：静态确定权限不一致；root确认合规后以role=10新会话访问订阅管理将无法通过options读取，前端锁定。

建议：提供管理员可读的最小合规状态接口/订阅管理能力响应，勿放开所有Root-only系统配置。

### 190 · #6684 · 无法识别lobehub上智谱的模型图标

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6684)

锁定LobeHub icons现为5.14.0，安装包exports包含ZAI。图标加载器按导出名找组件，ZAI可显示；ZAI本身没有Color子组件时会回退基础图标，而不是完全无法解析。

- [web/bun.lock:361](/workspace/LemonHub/web/bun.lock:361)：@lobehub/icons锁定5.14.0。
- [web/src/lib/lobe-icon.tsx:129](/workspace/LemonHub/web/src/lib/lobe-icon.tsx:129)：不存在子变体时回退LobeIcons[baseKey]。

验证／触发边界：本次核对已安装包es/icons.js导出ZAI及ZAI/index.js基础组件；未跑浏览器像素验证。

建议：无须直接移植；按所述边界进一步核验。

### 191 · #6682 · /v1/responses 流式响应在 Codex 客户端中断流、EOF、JSON 解析失败

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6682)

确认其中usage兼容子问题：Chat->Responses转换只填CompletionTokenDetails，没有赋OutputTokensDetails，omitempty使response.completed缺少Responses规范位置的reasoning_tokens；即使上游提供标准completion_tokens_details也如此。不要把正文其他两点全称确证：Responses以response.completed结束，不必强制[DONE]；Chat created_at已转整数，原生Responses遇浮点created_at仍会解析失败。

- [relaykit/relayconvert/internal/oai_chat/to_oai_responses_resp.go:111](/workspace/LemonHub/relaykit/relayconvert/internal/oai_chat/to_oai_responses_resp.go:111)：UsageFromChatUsage新建Usage但未初始化OutputTokensDetails。
- [relaykit/relayconvert/internal/oai_chat/to_oai_responses_resp.go:145](/workspace/LemonHub/relaykit/relayconvert/internal/oai_chat/to_oai_responses_resp.go:145)：推理计数只复制到CompletionTokenDetails。
- [relaykit/dto/openai_response.go:239](/workspace/LemonHub/relaykit/dto/openai_response.go:239)：OutputTokensDetails为omitempty指针。

验证／触发边界：临时Go探针输入reasoning_tokens=3，实际UsageFromChatUsage序列化没有output_tokens_details；probe6.log。未联调Codex。

建议：映射并保证Responses output_tokens_details.reasoning_tokens存在（可为0），同时独立验证原生浮点created_at容错；不盲加[DONE]。

### 192 · #6681 · 兑换码的新值与修改页面的额度输入框，有值时，点击键盘的backspace键，删除到最后会强制显示为0

**确认存在** · P3 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6681)

兑换码金额输入onChange仍执行parseFloat(value)||0，清空字符串变NaN再强制0，因此Backspace不能保留空输入态。

- [web/src/features/redemption-codes/components/redemptions-mutate-drawer.tsx:301](/workspace/LemonHub/web/src/features/redemption-codes/components/redemptions-mutate-drawer.tsx:301)：空字符串被Number.parseFloat(...) || 0转换为0。

验证／触发边界：静态确定：输入200后全选删除，e.target.value为空，field被写0。

建议：表单保留空字符串作为编辑态，在提交/校验时转换为数字，避免每次按键归零。

### 193 · #6680 · 兑换码编辑页额度精度损失

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6680)

兑换码表单使用quotaUnitsToEditableAmount与列表同精度格式化，并在金额未改动时保留原始整数quota，已防止显示浮点噪声及编辑其他字段改变余额。

- [web/src/features/redemption-codes/lib/redemption-form.ts:97](/workspace/LemonHub/web/src/features/redemption-codes/lib/redemption-form.ts:97)：默认金额用可编辑精度格式化。
- [web/src/lib/format.ts:127](/workspace/LemonHub/web/src/lib/format.ts:127)：toFixed统一展示精度。
- [web/src/features/redemption-codes/components/redemptions-mutate-drawer.tsx:168](/workspace/LemonHub/web/src/features/redemption-codes/components/redemptions-mutate-drawer.tsx:168)：未改金额直接保留loadedRedemption.quota。

验证／触发边界：本次redemptions-mutate-drawer测试6项通过，含CNY噪声、原额度保留；fixed6-web.log。

建议：无须直接移植；按所述边界进一步核验。

### 194 · #6671 · Ali 渠道对未传 top_p 的请求强制注入 0.001，导致百炼部分模型 400 且静默改变采样行为

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6671)

Ali适配器仅在TopP非nil时处理边界；nil不注入，0/1边界为两位小数0.01/0.99，正文根因已修。

- [relay/channel/ali/text.go:28](/workspace/LemonHub/relay/channel/ali/text.go:28)：TopP!=nil才收敛，边界使用0.99/0.01。

验证／触发边界：本次TestRequestOpenAI2AliTopP通过；fixed6-go.log。

建议：无须直接移植；按所述边界进一步核验。

### 195 · #6661 · [Bug] API 密钥状态筛选把启用/过期当作互斥状态，导致「已过期」筛不全

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6661)

Token列表后端直接Find且mask响应不归一化过期状态；只有访问令牌且Redis关闭时才写status=3。前端status筛选不进queryKey/API参数，且仅按原始status过滤当前页，未访问或Redis启用的status=1过期令牌必然遗漏。

- [model/token.go:132](/workspace/LemonHub/model/token.go:132)：列表直接返回原始DB状态。
- [controller/token.go:47](/workspace/LemonHub/controller/token.go:47)：masked响应未派生过期状态。
- [model/token.go:257](/workspace/LemonHub/model/token.go:257)：过期写3仅在访问且Redis关闭时。
- [web/src/features/keys/components/api-keys-table.tsx:231](/workspace/LemonHub/web/src/features/keys/components/api-keys-table.tsx:231)：请求条件/queryKey遗漏status。
- [web/src/features/keys/components/api-keys-columns.tsx:132](/workspace/LemonHub/web/src/features/keys/components/api-keys-columns.tsx:132)：filter仅比较row原始status。

验证／触发边界：静态确定：存status=1且expired_time为过去的未访问token，列表仍status=1；选择Expired(3)过滤掉该行，跨页也不查询。

建议：后端按expired_time派生筛选并分页；前端传状态条件及更新queryKey，明确启停和可用性状态语义。

### 196 · #6659 · 额度计费显示问题

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6659)

本地任务计费有持久化账务阶段，refund以负Delta同步归还余额并调整已记账的用户/token/channel累计使用值；区别于原帖只恢复余额。历史未知账务状态受保护，不能追溯猜测扣减。

- [service/task_billing.go:445](/workspace/LemonHub/service/task_billing.go:445)：refund阶段Delta=-quota。
- [model/task_billing_ledger.go:1608](/workspace/LemonHub/model/task_billing_ledger.go:1608)：仅已计入累计的task应用usageDelta。
- [model/task_billing_ledger.go:1625](/workspace/LemonHub/model/task_billing_ledger.go:1625)：用户used_quota随负delta回滚。

验证／触发边界：本次TestAccountedTaskSettlesAndRefundsExactAggregateDelta通过；fixed6-go.log。

建议：无须直接移植；按所述边界进一步核验。

### 197 · #6657 · Sensitive word filter false positives on short substrings (agent sessions)

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6657)

相同机制重复报告：配置短词cch后先小写整个文本再做AC子串匹配，AgenticChat会被拦截；控制器构造默认500错误且不向客户端返回匹配词。属于启用自定义敏感词过滤后的匹配/错误体验问题，不是默认全站阻断。 与 #6656 同根因；本报告逐 issue 保留两条，不代表两个独立缺陷。

- [service/sensitive.go:47](/workspace/LemonHub/service/sensitive.go:47)：转小写并进行AC子串匹配。
- [controller/relay.go:145](/workspace/LemonHub/controller/relay.go:145)：匹配词仅写日志，响应创建通用敏感词错误。
- [relaykit/types/error.go:312](/workspace/LemonHub/relaykit/types/error.go:312)：NewError默认HTTP500。

验证／触发边界：临时Go探针实际调用SensitiveWordContains：AgenticChat=true words=[cch]；Agentic Chat=false。probe6.log。

建议：可配置单词边界/短词策略，并将内容拒绝返回适当4xx及可理解原因；按运营设置决定是否公开匹配词。

### 198 · #6656 · Sensitive word filter: short substring matches cause false positives and kill long agent sessions

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6656)

相同机制重复报告：配置短词cch后先小写整个文本再做AC子串匹配，AgenticChat会被拦截；控制器构造默认500错误且不向客户端返回匹配词。属于启用自定义敏感词过滤后的匹配/错误体验问题，不是默认全站阻断。

- [service/sensitive.go:47](/workspace/LemonHub/service/sensitive.go:47)：转小写并进行AC子串匹配。
- [controller/relay.go:145](/workspace/LemonHub/controller/relay.go:145)：匹配词仅写日志，响应创建通用敏感词错误。
- [relaykit/types/error.go:312](/workspace/LemonHub/relaykit/types/error.go:312)：NewError默认HTTP500。

验证／触发边界：临时Go探针实际调用SensitiveWordContains：AgenticChat=true words=[cch]；Agentic Chat=false。probe6.log。

建议：可配置单词边界/短词策略，并将内容拒绝返回适当4xx及可理解原因；按运营设置决定是否公开匹配词。

### 199 · #6650 · 「选择同步渠道」打开弹窗时页面卡死

**已修复／等效规避** · [上游 issue](https://github.com/QuantumNous/new-api/issues/6650)

上游同步页面已对channels空数组fallback做useMemo，endpoint effect仅实际变化才set状态，消除打开弹窗时新数组依赖反复触发渲染的问题。

- [web/src/features/system-settings/models/upstream-ratio-sync.tsx:143](/workspace/LemonHub/web/src/features/system-settings/models/upstream-ratio-sync.tsx:143)：channels引用按query data稳定缓存。
- [web/src/features/system-settings/models/upstream-ratio-sync.tsx:159](/workspace/LemonHub/web/src/features/system-settings/models/upstream-ratio-sync.tsx:159)：无变化时返回prev避免重复set。

验证／触发边界：静态源码核对；未执行真实供应商或浏览器端到端复现。

建议：无须直接移植；按所述边界进一步核验。

### 200 · #6649 · stream_status.end_reason shows "client_gone" for streams that completed normally (SSE scanner race)

**确认存在** · P2 · [上游 issue](https://github.com/QuantumNous/new-api/issues/6649)

Responses handler转发response.completed后仍等上游EOF；若客户端看到终止事件立即断开、上游连接仍开，StreamScanner主select写client_gone，sync.Once使后续正常终止无法纠正，最终成功响应记录为错误。正文原始[DONE]描述不精确，但评论中的completed先于EOF路径与本地代码一致。

- [relay/channel/openai/relay_responses.go:121](/workspace/LemonHub/relay/channel/openai/relay_responses.go:121)：先发送终止事件，completed分支仅记用量没有标记正常结束。
- [relay/helper/stream_scanner.go:298](/workspace/LemonHub/relay/helper/stream_scanner.go:298)：客户端取消直接写ClientGone并清理。
- [relay/common/stream_status.go:49](/workspace/LemonHub/relay/common/stream_status.go:49)：endOnce固定第一个终止原因。

验证／触发边界：本次临时Go探针走真实OaiResponsesStreamHandler：本地管道发送response.completed后保持上游未EOF，客户端flush终止事件后取消context。结果terminal_completed_delivered=true、end_reason=client_gone，handler无错误；probe6.log。无网络/真实供应商调用。

建议：在协议终止事件确认后记录正常完成，或引入明确可纠正的终止状态优先级；保留真实中途断开取消上游行为。

## 证据包与复核

- [200 条 CSV 筛选表](/workspace/scratch/upstream-bug-audit/results.csv)、[结构化结果 JSON](/workspace/scratch/upstream-bug-audit/results.json)。
- [原始 issue 快照](/workspace/scratch/upstream-bug-audit/issues.json)；200 份评论快照位于 `/workspace/scratch/upstream-bug-audit/comments`。
- [末帧与多图最小测试](/workspace/scratch/upstream-bug-audit/repro/root-repro.log)。
- [订阅溢出与浮点时间戳](/workspace/scratch/upstream-bug-audit/batch5_repro.log)。
- [图片缓存及渠道测试计费](/workspace/scratch/upstream-bug-audit/repros/batch3_billing_output.txt)。
- [拒绝响应计费](/workspace/scratch/upstream-bug-audit/repro-7551.log)。
- [工具／媒体／Schema转换](/workspace/scratch/upstream-bug-audit/repro-batch2.log)。
- [Responses终止与取消](/workspace/scratch/upstream-bug-audit/repro-4/openai.log)。
- [Vertex、usage与正常流结束原因](/workspace/scratch/upstream-bug-audit/probe6.log)。

数据覆盖已自动检查：200 个唯一 issue、6 批无遗漏／重复分配、200 份评论、全部引用文件存在且行号有效。原始 issue JSON 的 SHA-256：`7a3ee254eea11d6b060aa9dd18b51763e47f0e5af0ce11885d793e1472c4cb66`。

本报告是当前提交的静态审计与定向行为验证结果。未做所有部署、浏览器、数据库和真实供应商端到端复现；证据不足项需要补原始请求、usage、配置或日志才能进一步确认。修复代码尚未实施，不能把本次验证当作修复完成。
