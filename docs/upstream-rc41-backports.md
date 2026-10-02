# rc.41 及后续上游低冲突修复移植记录

2026-10-02，在 LemonHub 基线 `996a55bee0fc3ace7106805418d2488bfa205ab3` 上完成 **28 个上游提交的全部或局部移植**：原审查优先推荐的 13 项，加上 15 项可小范围适配的修复。发布前追加的数据库迁移保护、依赖安全更新及最终验收另见 [v0.4.44 审查记录](security-release-audit-v0.4.44.md)。

上游固定审查快照为 `QuantumNous/new-api` 的 `1a4166d8e8ba9802d2ca56fe8ecf0ed5404e80d5`，覆盖 rc.26–rc.41 及其后 7 项提交。完整 234 项审查及 release 对照见 [合并前审查快照](upstream-review-2026-10-02.md)。本记录描述实际实现和最新验证；不表示整体合并了 rc.41，也不表示上游所有前置架构均已引入。

## 第一组：原推荐 13 项

| 上游提交 | 本项目实际行为与适配 |
| --- | --- |
| [`2d8e50bf3`](https://github.com/QuantumNous/new-api/commit/2d8e50bf36e94200b809dfb39e73624ec48b1e23) | 日志筛选字段使用文本遮罩并关闭自动填充，保留显示切换、筛选值和 URL 查询行为。 |
| [`8f6961c67`](https://github.com/QuantumNous/new-api/commit/8f6961c675932f406260ff0c218bc2aa0603e9b2) | 透传 vLLM `thinking_token_budget`，显式零值不会丢失。 |
| [`cae3676ec`](https://github.com/QuantumNous/new-api/commit/cae3676ec6f46ee5ef596443256f78c4e9b34ceb) | 普通智谱 BaseURL 支持 Responses 转发；已有 CodingPlan 虚拟地址没有 Responses 端点，明确返回不支持，保留 Chat/Claude 原路由。 |
| [`98d50d538`](https://github.com/QuantumNous/new-api/commit/98d50d5383a33432ff6c30b129461b170e5cbffc) | 页面刷新后重新检查服务初始化状态；同一页面会话复用成功结果，失败后可重试。 |
| [`2cf177ac4`](https://github.com/QuantumNous/new-api/commit/2cf177ac487e62c627c7d423b65735ba2481ef4f) | 批量复制 RawMessage，保持请求及重试间的深拷贝隔离。 |
| [`76f7dafd2`](https://github.com/QuantumNous/new-api/commit/76f7dafd2b82fc305533ce6a3065b3de2d338bae) | 音频扩展名不区分大小写，覆盖真实 WAV 解码和无效文件。 |
| [`8e5e09166`](https://github.com/QuantumNous/new-api/commit/8e5e09166cf83731df1819fd1cb9c50ea2576ed6) | Ollama 流末尾 done 帧保留工具、文本和思考内容；保留本地公开模型名处理及旧协议回退路径。 |
| [`62f8db775`](https://github.com/QuantumNous/new-api/commit/62f8db775bcdaa5da144d9c4a36f5ead82de3ac5) | DeepSeek 渠道余额优先采用 USD，否则按汇率换算 CNY；拒绝非法或非有限值，不修改用户钱包。 |
| [`3e8c358da`](https://github.com/QuantumNous/new-api/commit/3e8c358da09b4598a946712db8219ff44f197475) | 智谱请求转换保留 `reasoning_effort`。 |
| [`c76452d22`](https://github.com/QuantumNous/new-api/commit/c76452d22412be45eb708f2618748f7aac46703d) | Playground 长连续文本换行；修复所在文件原有 8 项 lint error，并处理附件 blob 转换失败时的未捕获拒绝，保留草稿供重试。 |
| [`c2b7a9a9e`](https://github.com/QuantumNous/new-api/commit/c2b7a9a9e0b548c2051a949fceabb59029adcb49) | 原生 Claude 每条消息保留 `output_config`；测试合入本地现有文件。 |
| [`789c97019`](https://github.com/QuantumNous/new-api/commit/789c970199ea527e6a26e071915f4a4cd2c64178) | 原生 Claude 请求保留 `safeguards` 及其零值、false 和省略语义。 |
| [`811212067`](https://github.com/QuantumNous/new-api/commit/8112120673bf593eb62e3a297dafca32a4605eb6) | 渠道自动禁用原因的长文本在提示框内换行。 |

## 第二组：追加 15 项

| 上游提交 | 本项目实际行为与适配 |
| --- | --- |
| [`2506e1b98`](https://github.com/QuantumNous/new-api/commit/2506e1b980ecf67c829ef20255a16fc61c47cb55) | 创建用户检查本地角色白名单；保留合法 role=5、role=0 的创建默认值、上下级权限和站点/身份/授权事务。 |
| [`feefe09f2`](https://github.com/QuantumNous/new-api/commit/feefe09f2781429e039c3f7ba88da7f29492e98c) | 5 处 HTTP transport 克隆 TLS 配置，保留支付的 `strictTLS`；另外落实同一提交中的 Waffo Pancake StoreID 校验，见升级说明。 |
| [`057f71c23`](https://github.com/QuantumNous/new-api/commit/057f71c2336c3981187b732a9d06f65490e9a946) | **局部移植**用户日志 `Other` 的读侧隐私过滤，去除管理员/审计/渠道等敏感元数据；RawMessage 保留账单整数精度、零值及公共字段，不引入新 LogOther 写入架构。 |
| [`49ec46966`](https://github.com/QuantumNous/new-api/commit/49ec4696682530781a036eab1ac195f0b04706c0) | 最小引入 OpenAI Chat 模型能力判定，按映射后的模型和实际推理参数处理 token、采样及 developer role；保留显式 0、现有双 token 字段优先级和额度校验。 |
| [`d0cb7347c`](https://github.com/QuantumNous/new-api/commit/d0cb7347c07bc935187c077b960435f3f3b08595) | 支持 GPT-6 Sol/Luna 的 `max_completion_tokens` 和无推理时的采样参数；与能力判定一起验证别名、日期快照、变体及未知模型。 |
| [`fa90b2312`](https://github.com/QuantumNous/new-api/commit/fa90b2312cfb686aaa6010f9280bffcf0bf6d391) | **局部移植** Gemini 已知 thinkingLevel 的规范化日志及重试恢复；本地原生请求已接受大小写变化，保留请求原文和未知值，不引入上游新验证器/Intent 框架。 |
| [`d1c79d728`](https://github.com/QuantumNous/new-api/commit/d1c79d7288221768a5f7270649d691ac5a137def) | 仪表盘 week 默认滚动窗口从 30 调整为 29，与已有 29 Days 选项一致；不是修改自然日端点算法。 |
| [`2bfb89c1b`](https://github.com/QuantumNous/new-api/commit/2bfb89c1b99d5eb7a799ba5706813614bfb6189a) | 价格排序菜单使用非模态交互，避免背景滚动锁导致布局偏移。 |
| [`dfd3cd893`](https://github.com/QuantumNous/new-api/commit/dfd3cd8930447780a5733eaaba9a306df595251d) | 系统设置菜单与现有 root 路由权限保持一致；子站管理员自己的管理入口保留。 |
| [`54eee488b`](https://github.com/QuantumNous/new-api/commit/54eee488bed3ec6b9659b3a792194b18c2dfad27) | 主题偏好迁到 localStorage，按 origin 隔离；保留清缓存时的主题键，存储异常时界面仍可操作。 |
| [`521cebf58`](https://github.com/QuantumNous/new-api/commit/521cebf585efc2e782dd9fb93d0f66752c8d3c32) | 精简完成初始化后的仪表盘引导，保留本地公告、主界面及子站入口。 |
| [`2035a82ae`](https://github.com/QuantumNous/new-api/commit/2035a82aeb5414253a728bd937d4b8f97aa99b9b) | 渠道表增加刷新按钮，沿用当前筛选、排序及页码，请求期间显示忙碌状态。 |
| [`da2540dda`](https://github.com/QuantumNous/new-api/commit/da2540dda1328040193f279a58c3e315083e416c) | 调整渠道卡片指标布局和状态徽标间距，保留敏感信息遮罩。 |
| [`c9a110190`](https://github.com/QuantumNous/new-api/commit/c9a110190c5241c24d9d66de340431f5e9873db6) | **局部移植**渠道默认 BaseURL 占位提示，复用本地默认地址配置；不改实际保存值，不新增后端接口。 |
| [`2d7aef741`](https://github.com/QuantumNous/new-api/commit/2d7aef7414e8671b1ac7db336bc170eeb7cc5e2b) | CC Switch 模型选择使用现有 Portal 组件，修复抽屉内点击、键盘及加载交互，不改共享选择器或导入凭据逻辑。 |

## 本地兼容边界

- 用户日志只在现有 self/token 读路径过滤 `Other`，保留统一错误掩码和管理员完整日志；顶层 `ChannelId` 仍遵循原接口约定，不宣称移除了所有渠道标识。坏 JSON、null 或空 `Other` 返回 `{}`。
- OpenRouter 能力判断以最终 nested reasoning 为准，覆盖其与顶层字段、模型后缀的冲突。已提供但无法确定 effort 的 nested reasoning（包括 `{}`）保守关闭新增采样支持，不改既有出站 `reasoning` / `reasoning_effort` 的组装和优先级，也不伪造日志 effort。既有 `openai/gpt-*` 供应商前缀识别行为未扩展。
- OpenAI 渠道探测仍先解析模型映射，再经实际适配器决定参数。没有在映射前按公开别名转换 token 字段，也没有引入 `@` 模型修饰符体系。
- 上述 28 项移植本身不改变 schema、迁移、依赖或锁文件；不覆盖本地 site_id、身份 claim、权限事务、预扣/结算/退款及钱包数值域规则。relaykit 保持独立构建。随后发布审查另行加固迁移检查并更新安全依赖，具体范围见 v0.4.44 审查记录。
- Claude 字段支持限于原生透传；普通智谱 Responses 的实际可用性取决于供应商端点。Ollama 原生 Claude/Responses 切换未引入，保留旧版服务兼容路径。

## 升级注意事项

**Waffo Pancake 必须配置正确的 `WaffoPancakeStoreID`。** 新建支付会话拒绝空配置；回调在 SDK 验签成功后，还必须匹配当前配置的店铺。空配置、错店铺或切换店铺前的旧店铺未结订单回调会被拒绝。升级或切换店铺时需核对配置及未结订单归属。这一变更没有跳过验签、放宽签名时窗或增加生产公钥注入入口；仓库另一套旧版 Waffo 支付接口不在此次修改范围内。

主题首次读取不继承旧 cookie，会采用默认主题；之后的偏好按当前 origin 保存。

## 验证

| 检查 | 最终结果 |
| --- | --- |
| 根模块全量测试 | `go test -mod=readonly -timeout=180s ./...` 通过，包含最终 OpenRouter 修正。 |
| 根模块静态检查及构建 | `go vet ./...`、`go build -mod=readonly ./...` 通过。Go 检查在前端产物生成后执行，以满足 `go:embed`。 |
| relaykit 独立验收 | `GOWORK=off go test -mod=readonly -timeout=180s ./...`、`go vet ./...`、`go build ./...` 全部通过。 |
| 前端全量测试 | `bun run test --maxWorkers=2`：**77 个文件、453 项测试通过**；随后仅给路由测试名增加 TanStack 忽略前缀，重跑该文件 4 项测试通过。 |
| 前端类型与构建 | `bun run typecheck`、`bun run build` 通过；最终构建无路由测试扫描警告。 |
| 前端涉及文件 lint | 32 个源码/测试文件检查通过，**0 error、4 项既有 warning**；重命名后的路由测试单独检查也通过。 |
| 格式与补丁 | 32 个前端文件按项目保护版权头方式检查格式通过；31 个新增/修改 Go 文件符合 gofmt；`git diff --check` 通过。 |

保留的 lint warning 为 CC Switch 模板字符串、价格菜单自闭合标签和 prompt 输入的两处事件绑定写法。旧审查中 prompt 文件的 8 项 lint error 已全部处理。前端测试日志含 JSDOM 的 `scrollTo` 未实现提示，无失败测试或未处理错误。

关键新增回归覆盖角色和站点归属、TLS 对象隔离、Waffo Pancake 真签名及钱包/订阅结算、日志精度和错误掩码、实际映射后的模型参数、请求重试隔离、流式末帧、主题和菜单权限、筛选分页与下拉交互。Waffo Pancake 仅在测试中通过 `t.Setenv` 使用 SDK 已有公钥环境变量覆盖机制，以临时 RSA 密钥走真实验签、控制器和 SQLite 结算链，验证正确店铺可入账、错误店铺不变更资金或权益；生产验签调用未变，未调用线上支付。

移植阶段环境为 Go 1.26.8、`GOWORK=off`、`CGO_ENABLED=0`，前端使用 Bun 和当时锁定的依赖。上述阶段尚未执行 MySQL/PostgreSQL 实机验收，后续发布阶段已另行验证，见 v0.4.44 审查记录。本次仍未执行真实供应商联调、浏览器 E2E 或 race detector；组件布局测试验证渲染契约，不等同真实浏览器像素或自动填充行为验收。

## 保留为独立后续工作的范围

数据库驱动/事务默认值变更仍需三数据库兼容验证。任务插件、scoped access token、Responses WebSocket、表达式定价重构和跨协议 custom tools 涉及本地认证、站点和资金链路，继续按原审查结论单独适配。其余候选的原始冲突数、依赖和建议测试保留在完整审查表中。

本地证据目录：`/workspace/scratch/upstream-audit/`。`implementation-commits.json` 对应上述 28 个源提交；`implementation-*.txt` 记录各组适配和红绿回归；`final-*.log` 记录最终组合验证。审查快照中的 13 项临时验证和旧 lint 结果仅作历史证据，以本记录为准。
