# 上游选择性同步与代码审查 — 2026-10-16

本次以 LemonHub `b8d952a40` 为本地基线，对 `QuantumNous/new-api` 在上次审查快照 `1a4166d8e8ba9802d2ca56fe8ecf0ed5404e80d5` 之后、截至 `45fbb149f0c89ff48d805335f22419be392d0350` 的 **90 个提交**逐项审计，按本地兼容性选择移植。**不是整体合并 rc.44 或上游 main**，也不以缺失上游 SHA 判断本地缺失功能。最终覆盖 **37 项兼容移植、12 项部分兼容**；其余按已有等效、明确冲突、实验性或不适用分类保留/跳过，详见逐提交表。

完整共同祖先仍是 rc.25 `f116414284162ad15d8925f7bca494c109b83e93`；之后已有 [rc.41 兼容移植](upstream-rc41-backports.md)及 [10 月 7 日的 74 个代码修复单元](upstream-bug-fixes-2026-10-07.md)。本次先对照实际源码及这些后续修复，再决定是否移植。`git merge-tree --write-tree` 在两个固定基线上返回冲突，记录 327 条 `CONFLICT` 消息；没有对工作区执行整体 merge，也没有使用整文件接受上游覆盖定制。

## 发布范围

时间使用 Asia/Shanghai。下表提交数从上次固定审查快照开始计算，不是 release 自身完整 changelog 数量。

| 版本 | 发布时间 | 本次新增提交 |
| --- | --- | ---: |
| [v1.0.0-rc.42](https://github.com/QuantumNous/new-api/releases/tag/v1.0.0-rc.42) | 2026-10-08 18:10 | 19 |
| [v1.0.0-rc.43](https://github.com/QuantumNous/new-api/releases/tag/v1.0.0-rc.43) | 2026-10-10 14:11 | 11 |
| [v1.0.0-rc.44](https://github.com/QuantumNous/new-api/releases/tag/v1.0.0-rc.44) | 2026-10-12 21:42 | 41 |
| main 至 `45fbb149f` | 固定抓取快照 | 19 |

GitHub 将这三个 release 标为 `prerelease=false`，但名称仍是 RC，不能据此把其中每个功能认定为适合生产。上游 [rc.27 的说明](https://github.com/QuantumNous/new-api/releases/tag/v1.0.0-rc.27)明确将任务插件系统称为实验版，并写明“不推荐生产环境使用”；本次不引入该架构及其后续插件更新。

## 兼容边界

- 保留 `site_id` 隔离、合法 `role=5`、身份 claim 与授权事务、会话撤销及面板/API token 的不同权限。
- 保留子站采购钱包、支付核单、返佣流水、订阅履约快照与溢出结算 receipt；不替换现有数据库迁移或重算历史账单。
- 保留单次费用 int32 饱和审计、钱包 `2^53-1` 数值域、token 64 位余额、请求数量上限及预扣/退款的幂等边界。
- 保留本地 SSE 提交、重试、终态与取消的处理；权威 usage、缓存用量及本地拒绝免单开关按现有合同兼容。
- 保留紧凑定价卡片、隐藏原始表达式、Usage Activity、邮件活动进度、七语言和子站入口。
- `relaykit` 继续独立构建；不引入根模块依赖、任务插件、moejs、插件 PGO、Responses WebSocket 或新 tokenkit 架构。
- 不新增跳过 TLS 验证开关，不放宽错误信息中的上游地址/密钥遮罩，不新增 `thinking.block_binding` beta 功能。

## 更新后的可见行为与注意事项

- 请求与供应商：Moonshot Responses、Vertex Project ID 与 us/eu 主机名、已有 anthropic-beta 多值转发、正则参数条件，以及重复读取请求体时保留原始错误。
- 协议转换：Gemini Schema/真实媒体、并行工具结果附件、Claude 同帧思考与正文、effort 开关及 Responses 末尾历史兼容；保留严格工具约束和脱敏诊断。
- 费用与权限：Claude 纯缓存计费、Vidu metadata 不得覆盖已校验时长/分辨率、分组独立限流、只读 token 的 IP 白名单、Root 关闭广场后仍可管理定价、Sub2API 渠道余额及任务日志标记。
- 界面：Playground 账号隔离及 reasoning 别名、权限一致的搜索、用户组筛选、按价格类型排序、历史计费档位、日期秒和语言、主题文字颜色、轻量品牌图标；原始定价表达式继续隐藏。
- 依赖：go-epay 0.0.5、icons 5.22.0、ai 7.0.136、stream-markdown-parser 1.2.18、yace 1.1.1，以及 Electron 使用的 undici 补丁。独立 relaykit 的 x/text 从 0.39.0 升至根模块已用的 0.42.0，处理最终扫描发现的 GO-2026-6629 公告。保留 Go 1.26.9、Electron 44.4.5 和现有安全 overrides；跳过 peer/格式约定冲突的 router-plugin 与 oxfmt 更新。

本次没有新增数据库迁移，没有更改历史账单或部署生产实例。需要了解的行为变化：

1. **Playground 旧记录没有可验证的账号归属。** 原始 legacy 键保留以便恢复，但不自动分配给登录账号，也不显示在新账号会话中。新记录按账号保存；迟到回调和卸载保存仍绑定原 owner。
2. **只读 token 查询也遵守已有 IP 白名单。** 已过期或额度耗尽的 token 仍可在允许 IP 查询其使用数据；禁用 token 继续拒绝。没有新增读取用户钱包余额的接口。
3. **静态资源不再消耗页面访问预算。** 可选 `GLOBAL_STATIC_RATE_LIMIT_ENABLE=false`、`GLOBAL_STATIC_RATE_LIMIT=1000`、`GLOBAL_STATIC_RATE_LIMIT_DURATION=180` 已写入 `.env.example`；HTML 入口和缺失资源仍使用页面预算。
4. **敏感词检查只调整未配置时的默认值。** 不覆盖数据库里已经保存的开关或词表。
5. **Gemini v1 不能无损表达的 Schema 返回明确 400。** 不静默删掉 enum、引用或其他约束；支持 JSON Schema 的版本保留完整输入。未配置 Project ID 的 Vertex API Key 保持原 Express 模式。
6. 价格排序按基础 USD 比较；无法可靠计算的复杂或固定按次表达式保持在无价格项末尾，不用旧倍率猜算。旧排序链接继续兼容，明确为零的已配置价格不视为缺失。
7. go-epay 0.0.5 拒绝含 `&` 的歧义签名值及含 `&`/`=` 的参数名；自定义订阅名称等支付元数据需要符合这一签名约束。严格支付核单仍保留。
8. 配置了独立限流的分组开始使用用户+分组计数键，切换时不会把旧共享键的历史计数猜分到各组；未配置独立额度的组继续共用原用户预算。

## 独立 Codex 代码审查

实现后由未参与对应模块实现的 Codex agent 交叉检查变更、调用方和本地保护合同，并另行核对 90 条提交处置范围。审查发现的问题已纳入修复及回归：

- Playground 延迟保存/卸载回调在账号切换后写错 owner；改为会话 owner 绑定并按账号重建页面。
- Claude 缓存修复的初稿扩大了输入估算条件；收窄后保留成功响应的明确零输入，并测试纯缓存预扣、结算和退款。
- 静态压缩缓存初稿丢失 HTTP 条件请求及文件尾斜线重定向；改用标准内容服务并补 304/412/HEAD/重定向测试。
- Gemini typed schema 同时有 nullable、type、anyOf 时，null 仍被交集拒绝；修正所有相关约束，并对仅展示字段的转换提供固定脱敏诊断。
- 新价格排序初稿未兼容旧 URL，并把已配置的零价格与缺失价格混同；补 URL 规范化及显式价格字段识别，保持未知表达式不猜算。

独立审查证据在工作区的 `review-backend.md`、`review-relay.md`、`review-relaykit.md`、`review-root-relay-backend-by-relaykit.md`，前端及最终范围核对另见 `review-frontend.md`、`scope-review.md`。审查和测试覆盖本次修改及相关调用链，不代表真实供应商或任意生产环境的无缺陷保证。

## 逐提交处置

“兼容移植”指目标行为按本地架构实现；“部分兼容”只包含说明中的子项。已有等效不是本轮新增功能，跳过也不代表该上游功能本身均为实验性。

| 提交 / 首次版本 | 处置 | 实际范围与冲突理由 |
| --- | --- | --- |
| [`b48b74ab7`](https://github.com/QuantumNous/new-api/commit/b48b74ab77cf289ccaaa8d99c067ce50cced4a82) / rc.42 | 排除 | 实验性 JS 插件结果解码，不引入 moejs。 |
| [`973cf8ef4`](https://github.com/QuantumNous/new-api/commit/973cf8ef4600947a4270e95ada7916740fa8264c) / rc.42 | 保留本地 | 本地 GroupBadge 已有内边距、Auto 外环及换行保护，上游零内边距布局不存在。 |
| [`78bd5b1cb`](https://github.com/QuantumNous/new-api/commit/78bd5b1cbf7bffc462a515a6c6e27567ace7a4d8) / rc.42 | 部分兼容 | 补 Responses→Gemini 工具结果媒体；累计 usage 本地逐帧快照不累加。保留本地 Claude effort 显式语义，不自动推断默认档位。 |
| [`8ca26d81d`](https://github.com/QuantumNous/new-api/commit/8ca26d81dfff9d0454e0f5d06394c93689fe69b1) / rc.42 | 兼容移植 | 静态文件独立限流及压缩缓存；保留动态 HTML、Host、条件请求、HEAD 和重定向。 |
| [`956019109`](https://github.com/QuantumNous/new-api/commit/956019109bb09eb94417fd69ec0a5af0d897ac5a) / rc.42 | 兼容移植 | 用户组筛选接现有带站点范围的后端，保留旧 URL 与行选择契约。 |
| [`c6bac617d`](https://github.com/QuantumNous/new-api/commit/c6bac617d46ad2d63fa7cb692942fe6d21e264f7) / rc.42 | 兼容移植 | thinking 开关覆盖 effort 后缀，保留禁用时模型名和原始参数。 |
| [`0908d9e7b`](https://github.com/QuantumNous/new-api/commit/0908d9e7b91124e5808f20d2d4b9402c7f2f5fae) / rc.42 | 兼容移植 | go-epay 0.0.5，保留严格商户/金额/订单核实及站点资金流程。 |
| [`bb28f2cca`](https://github.com/QuantumNous/new-api/commit/bb28f2ccad3f6286a9dc4a0ad9e20f571f57533b) / rc.42 | 冲突跳过 | tokenkit 抽离同时重标定估算、图片规则和回退计费，与本地资金及多模态合同交叉；保留现有模块。 |
| [`0a532f7f7`](https://github.com/QuantumNous/new-api/commit/0a532f7f7d1b507cc6657d6bc140f73d4298e46a) / rc.42 | 排除 | xAI Grok Imagine 实验性任务插件。 |
| [`0a03088e0`](https://github.com/QuantumNous/new-api/commit/0a03088e0e737e99bc7ca936d3d2276e8d636fb8) / rc.42 | 部分兼容 | 兼容 Gemini assistant 文本与工具调用共存等转换；保留本地更完整的终态、取消、观察用量和退款逻辑。 |
| [`acb347a19`](https://github.com/QuantumNous/new-api/commit/acb347a197ff0e22f651be9813b4a1a867342941) / rc.42 | 部分兼容 | 仅取独立 Moonshot Responses 透传；新的搜索编码/用量计费架构与本地结算冲突。 |
| [`45094bdf4`](https://github.com/QuantumNous/new-api/commit/45094bdf41fb13a5d24bd3e8eb3fdfec6241c59f) / rc.42 | 兼容移植 | OriginOS 动画避免零 blur 并释放 filter，保留减少动态效果偏好。 |
| [`04c597bf6`](https://github.com/QuantumNous/new-api/commit/04c597bf6cce1eea76b9e67a4936e15fb54a5612) / rc.42 | 兼容移植 | 正则覆盖条件与稳定编辑草稿；保留本地审计、敏感字段权限及 Select。 |
| [`d10f11fff`](https://github.com/QuantumNous/new-api/commit/d10f11ffff20688b8e3e745268cc0036e896d068) / rc.42 | 冲突跳过 | Google Search 新查询/提示计费单位依赖未合入的搜索计费架构。 |
| [`0cce21db0`](https://github.com/QuantumNous/new-api/commit/0cce21db0283304d139bf22588f4dc31bdc5ae67) / rc.42 | 排除 | JS 插件请求体重建优化。 |
| [`bc8235792`](https://github.com/QuantumNous/new-api/commit/bc823579294fbca0064b622c22477d3d04ca1bd8) / rc.42 | 保留本地 | 不跟随上游删除转换测试，保留本地行为回归覆盖。 |
| [`b23471112`](https://github.com/QuantumNous/new-api/commit/b23471112d8f7e5848f70f30b798ee417d1ec75c) / rc.42 | 排除 | moejs alpha.6。 |
| [`192802ee0`](https://github.com/QuantumNous/new-api/commit/192802ee0691900575a3c7a0cdb90b2d7e703035) / rc.42 | 排除 | moejs PGO 及相应发布构建接入。 |
| [`6370b2942`](https://github.com/QuantumNous/new-api/commit/6370b29424168039e94d40d610191e7d2e65dbf4) / rc.42 | 排除 | 实验性插件 json-order 能力。 |
| [`7aa3531ef`](https://github.com/QuantumNous/new-api/commit/7aa3531ef4c247ad4891c06cc8ed9d0ffb73fecc) / rc.43 | 排除 | 实验性插件 multipart 请求头；保留本地内置适配器。 |
| [`e0483122c`](https://github.com/QuantumNous/new-api/commit/e0483122c7d3693629f0e84473380a8c9506c56a) / rc.43 | 兼容移植 | Responses 末尾已完成 assistant 文本转换时避免被当作 prefill；保留未结束工具调用及 assistant-only 输入，记录脱敏诊断。 |
| [`128089fa7`](https://github.com/QuantumNous/new-api/commit/128089fa7ddf41b575206a0feb28940d33793833) / rc.43 | 冲突跳过 | 288 文件消息/i18n 架构重写与本地认证、支付、审计和错误协议冲突；七语言与独立日期语言修复保留。 |
| [`1d4328e97`](https://github.com/QuantumNous/new-api/commit/1d4328e97417a043a161a0dd30a5b129be3ace49) / rc.43 | 冲突跳过 | README 描述上游独有转换接口，不替换本地独立模块文档。 |
| [`b66674ac2`](https://github.com/QuantumNous/new-api/commit/b66674ac23d79bfd35a90797ca1ac64fc74a2d8f) / rc.43 | 排除 | 插件自动时长 -1，与本地时长校验和计费边界不同。 |
| [`29509776e`](https://github.com/QuantumNous/new-api/commit/29509776ef855965fb2cff3978578d891005945e) / rc.43 | 排除 | 插件 utils.fetch 及域名访问能力。 |
| [`66c3e1428`](https://github.com/QuantumNous/new-api/commit/66c3e142820e513a328def88f2771f73479275bc) / rc.43 | 已有等效 | 本地 Hailuo 已用 files/retrieve 的 DownloadURL，保留任务凭据快照，不引入插件 fetch。 |
| [`3f0224478`](https://github.com/QuantumNous/new-api/commit/3f02244783e0bb96b1398601f156625936be2f15) / rc.43 | 排除 | 插件上传域名权限 UI。 |
| [`ba3b2c015`](https://github.com/QuantumNous/new-api/commit/ba3b2c015ad83e6710c18592b0e3e3ba9df9a67e) / rc.43 | 排除 | 插件能力声明重构。 |
| [`c8d234baa`](https://github.com/QuantumNous/new-api/commit/c8d234baa18b9090b394fc1b8f384eec94f8bfef) / rc.43 | 排除 | xAI 插件视频编辑/延展计费；本地没有对应内置适配器。 |
| [`c6741c36a`](https://github.com/QuantumNous/new-api/commit/c6741c36a9a1a729554ad94ee99f21602a214a23) / rc.43 | 排除 | 不引入每渠道跳过上游 TLS 证书验证。 |
| [`1ca79d57e`](https://github.com/QuantumNous/new-api/commit/1ca79d57e77d91c855371b43bb26b151429e18a1) / rc.44 | 冲突跳过 | 24 个历史样本条与本地紧凑三状态卡片设计不同；保留渠道与模型分组过滤，不新增无消费者 API。 |
| [`0f38a14a5`](https://github.com/QuantumNous/new-api/commit/0f38a14a5e8e322ea741478b0e49878be3130e1d) / rc.44 | 兼容移植 | Auto 使用浏览器 sans-serif，保留显式字体、主题及布局定制。 |
| [`036ba376d`](https://github.com/QuantumNous/new-api/commit/036ba376da7d572a4db3e0be8553aadc28c3616a) / rc.44 | 部分兼容 | 兼容 Gemini 2.5 effort→预算；不覆盖本地明确禁止预算推断 effort、未经请求默认 Claude 档位的合同。 |
| [`13c8e90b8`](https://github.com/QuantumNous/new-api/commit/13c8e90b8f4b929aa88c2e9616efc462e0068d72) / rc.44 | 已有等效 | 本地 Responses 图片转 Chat 已采用 image_url 对象并有工具附件回归。 |
| [`5ac756284`](https://github.com/QuantumNous/new-api/commit/5ac75628409de7e3adde7d36771700714af50853) / rc.44 | 兼容移植 | Chat→Claude 同一帧 reasoning 与正文均保留。 |
| [`977450a3e`](https://github.com/QuantumNous/new-api/commit/977450a3e810ee9c7d7e98acce1dc4efad90b491) / rc.44 | 兼容移植 | 无 Redis 时模型限流也返回说明性 429，保留失败释放及并发 reservation。 |
| [`b892a6b2e`](https://github.com/QuantumNous/new-api/commit/b892a6b2e44df572698f6cf731ff381f1977a1f2) / rc.44 | 已有等效 | 本地按 session/build 限制自动刷新，存储异常允许手动恢复；保留更完整方案。 |
| [`1fb6d07fe`](https://github.com/QuantumNous/new-api/commit/1fb6d07fee0171d5f4a0bd2d2ea2660572b08389) / rc.44 | 兼容移植 | 钱包、OAuth、绑定页面使用直接品牌 SVG，保留本地安全验证行为。 |
| [`06dd76a9d`](https://github.com/QuantumNous/new-api/commit/06dd76a9d38eb0eaf8e900b1264b500250156c4d) / rc.44 | 兼容移植 | Playground 按账号隔离，延迟写入绑定原 owner；旧无归属数据保留但不自动载入。 |
| [`96275d224`](https://github.com/QuantumNous/new-api/commit/96275d224bfcf6b5545ca7e57b3bf90f7c3ad396) / rc.44 | 兼容移植 | 日历及相对时间跟随界面语言，保留本地 profile/公告结构。 |
| [`93ea55f3c`](https://github.com/QuantumNous/new-api/commit/93ea55f3c3102f7bc11cd005570c7965f0f22a03) / rc.44 | 兼容移植 | 日期时间及日志时间范围支持秒，保留响应式布局和空输入保护。 |
| [`4b13d9388`](https://github.com/QuantumNous/new-api/commit/4b13d9388ed6f43118707534daad4903e5d22745) / rc.44 | 保留更完整实现 | 不删除非字符串 enum；与 c107 的无损 Schema 保留一起处理。 |
| [`801456a06`](https://github.com/QuantumNous/new-api/commit/801456a061fb9988957341c4eb5aa8ec20c40de1) / rc.44 | 不适用 | 本地没有 Responses WebSocket，不为关闭帧修复引入整套架构。 |
| [`c10780e05`](https://github.com/QuantumNous/new-api/commit/c10780e054e6e518c9d0d8a903526663c9b57d75) / rc.44 | 部分兼容 | 完整 Schema 与明确 API 能力适配；v1 可无损表示的使用 typed schema，不支持的明确拒绝，避免上游静默删约束。 |
| [`42e694eb6`](https://github.com/QuantumNous/new-api/commit/42e694eb60fe3191795c00d416571ced716eb9c4) / rc.44 | 兼容移植 | Meta Muse Spark 在通用 spark 之前识别，保留本地供应商映射。 |
| [`3827fd830`](https://github.com/QuantumNous/new-api/commit/3827fd83076b9cbaa3f1d9f9d2957ae653573ed7) / rc.44 | 兼容移植 | 显示表达式缓存读取价格，同时保留原始表达式隐藏。 |
| [`3f926c3ef`](https://github.com/QuantumNous/new-api/commit/3f926c3ef49745be273cd8f753fb691abdd7b836) / rc.44 | 已有等效 | 本地 StreamOutcome 已让已接受终态优先于晚到取消，并有统计回归。 |
| [`bbf60b49f`](https://github.com/QuantumNous/new-api/commit/bbf60b49f6a4cab709e49e57d45f7911fc7dccae) / rc.44 | 已有等效 | 本地徽章已居中，混合钱包/订阅有独立布局，保留双资金源展示。 |
| [`b22a43b6e`](https://github.com/QuantumNous/new-api/commit/b22a43b6e89e5349d8cdbddc88d570b16e15fb1f) / rc.44 | 兼容移植 | 任务差额及退款日志补 is_task，不改结算状态机。 |
| [`34b194212`](https://github.com/QuantumNous/new-api/commit/34b19421237fc4695e919cdd799c1b7813023085) / rc.44 | 兼容移植 | 模型广场关闭时，仅真实 RootAuth 仍可管理定价。 |
| [`ba0e478e0`](https://github.com/QuantumNous/new-api/commit/ba0e478e061ee0c824922d89c05620de878103cf) / rc.44 | 已有等效 | 本地 Claude→Chat 已保留 assistant 文本与工具调用，保留回归。 |
| [`fc08b76f0`](https://github.com/QuantumNous/new-api/commit/fc08b76f0fb3f95e7cb81667e7bf350200147fb3) / rc.44 | 兼容移植 | 不可解析的计费预览保留历史 matched_tier 或特殊计费标签，不暴露公式。 |
| [`4bfcf2812`](https://github.com/QuantumNous/new-api/commit/4bfcf2812198913156a3e88a43e746b61a7b6ef5) / rc.44 | 兼容移植 | icons 锁定 5.22.0，保留安全 overrides。 |
| [`9948b1ef1`](https://github.com/QuantumNous/new-api/commit/9948b1ef14ea17fb23539386c0cb09018671c955) / rc.44 | 不适用 | 本地不存在上游 pricing conversion API，不为数组解析补丁引入新接口。 |
| [`2ed058795`](https://github.com/QuantumNous/new-api/commit/2ed058795bf0ce968134aa8db18b0dd86b54c30e) / rc.44 | 部分兼容 | 新增 Bun Dependabot；CI event SHA 本地已有，不引入 tokenkit/插件 jobs 或替换本地测试。 |
| [`2b85a8fc6`](https://github.com/QuantumNous/new-api/commit/2b85a8fc67bdb6d8d41a25f60f734dd3b9917618) / rc.44 | 兼容移植 | 登录按钮允许触发现有法律同意提醒，验证仍先于 CAPTCHA 和凭据提交。 |
| [`767a82b87`](https://github.com/QuantumNous/new-api/commit/767a82b8764778b847462ebecdc231eacd1e282c) / rc.44 | 部分兼容 | 转发现有多个 anthropic-beta 头并规范 Bedrock 列表；排除新增 block_binding beta 字段。 |
| [`ba29f0645`](https://github.com/QuantumNous/new-api/commit/ba29f06453942d9b5c0b32330340b8a000692c4f) / rc.44 | 已有等效 | 本地 xmldom 已 0.8.15。 |
| [`b8c2dc126`](https://github.com/QuantumNous/new-api/commit/b8c2dc12660dceb2a905008a672f8bc0260c98c8) / rc.44 | 兼容移植 | yace 1.1.1。 |
| [`c730add95`](https://github.com/QuantumNous/new-api/commit/c730add954e1b565656a9320444cac5fee12cfe4) / rc.44 | 兼容移植 | stream-markdown-parser 1.2.18。 |
| [`82306b3c3`](https://github.com/QuantumNous/new-api/commit/82306b3c35945bc0ed2c567688afe28d8a281300) / rc.44 | 冲突跳过 | router-plugin 1.168.42 要求更高 runtime peer；保留现有配套版本，避免两份 router-core。 |
| [`bf4e1ebfb`](https://github.com/QuantumNous/new-api/commit/bf4e1ebfb88579930d75d852d2d2bca74cbd5f62) / rc.44 | 冲突跳过 | oxfmt 0.72 要求 tinypool 2.2，与本地 2.1.2 override 及格式约定冲突。 |
| [`7e25797d1`](https://github.com/QuantumNous/new-api/commit/7e25797d14681e4e16c2b46831450e7d6a5ccc74) / rc.44 | 已有更高版本 | 本地 Electron 44.4.5，不降到上游 41.10.6。 |
| [`7f51a9722`](https://github.com/QuantumNous/new-api/commit/7f51a97226c43bc082a065bdc8b40b719c4bdcce) / rc.44 | 已有等效 | 本地 js-yaml 已 4.3.2。 |
| [`1c58f3d16`](https://github.com/QuantumNous/new-api/commit/1c58f3d161fa32755b372d9449f2d5e077d50a5d) / rc.44 | 已有更高版本 | 本地 fast-uri 修复版本更高，不降级。 |
| [`3fe394cdb`](https://github.com/QuantumNous/new-api/commit/3fe394cdba280c00bc566de493b5fc8399b0a7d1) / rc.44 | 已有等效 | 本地 gorilla/websocket 已 1.5.3。 |
| [`9f048107e`](https://github.com/QuantumNous/new-api/commit/9f048107edbba2156691718126de7c4577614533) / rc.44 | 兼容移植 | ai 7.0.136，使用的类型契约经过验证，不新增实验 API。 |
| [`3c9fb2a0d`](https://github.com/QuantumNous/new-api/commit/3c9fb2a0de168d88ce5bffbf44acecdc1cdcdd39) / rc.44 | 已有等效 | 保留本地 brace-expansion 安全修复。 |
| [`93cf4ba6f`](https://github.com/QuantumNous/new-api/commit/93cf4ba6fb92e3f755fdeb6af9a9484bfa748816) / rc.44 | 空提交 | 无文件变化，且本地 js-yaml 修复已有。 |
| [`dd5985405`](https://github.com/QuantumNous/new-api/commit/dd59854050f26f4f67bb3dac5274af5e17013462) / rc.44 | 兼容移植 | Electron undici 6.29.0/7.30.0，保留局部 override 与代理/TLS 行为。 |
| [`633b4c5b1`](https://github.com/QuantumNous/new-api/commit/633b4c5b190dc1af99a2814391354d86948e7943) / rc.44 | 冲突跳过 | 日志导出和默认时间窗口缺本地站点/审计/统计合同，暂不引入接口与 UI，不截断原有历史。 |
| [`b4be2a784`](https://github.com/QuantumNous/new-api/commit/b4be2a7847c4db4a956c3e71ae4bc1f31bc71b60) / main | 兼容移植 | 新增定价分组置顶，保留本地倍率及自动分配优先级。 |
| [`63cecab82`](https://github.com/QuantumNous/new-api/commit/63cecab824bad1a421d19950004c309bb2054c09) / main | 兼容移植 | Sub2API 渠道余额，保留明确零、负债、失败不覆盖和权限；不修改用户钱包。 |
| [`d10eafb57`](https://github.com/QuantumNous/new-api/commit/d10eafb57e289e971453eb68e882addaa29f8cc9) / main | 兼容移植 | 请求体读取失败保留真实原因，超限仍为 413。 |
| [`ef60ef27f`](https://github.com/QuantumNous/new-api/commit/ef60ef27f36e418143fa4abf0c9025ba6f17fba7) / main | 兼容移植 | 重复读取保持首个失败，不用已关闭 body 的错误掩盖原因。 |
| [`7de10f2c6`](https://github.com/QuantumNous/new-api/commit/7de10f2c602b50122017556e8a09cb7c488c273c) / main | 部分兼容 | 补只读 token IP 限制；不引入默认披露用户余额的新 schema/API。 |
| [`4013729f6`](https://github.com/QuantumNous/new-api/commit/4013729f6a2a14b9b5988d3c1a529ea97d2b2d8d) / main | 兼容移植 | 仅初始默认关闭敏感词检查、移除测试词，不清除已保存设置。 |
| [`a0bf6a8a4`](https://github.com/QuantumNous/new-api/commit/a0bf6a8a412764bc296a8e27cf8c4aaa3f3f0ed9) / main | 兼容移植 | Vertex API Key 模式可配置 Project ID，保留空配置 Express 路径及敏感字段保护。 |
| [`c9f7b6208`](https://github.com/QuantumNous/new-api/commit/c9f7b6208131089bcf3c36ac554432967b57156d) / main | 兼容移植 | 按基础 USD 输入/输出/按次价排序；无法确定的表达式价格置后，不猜算。 |
| [`1b63c28dc`](https://github.com/QuantumNous/new-api/commit/1b63c28dcc999c4445c70f21ec90103ab49e8be5) / main | 部分兼容 | Claude 缓存输入不重复估算、纯缓存仍计费；明确零输入和本地 refusal 开关合同保留，验证实际预扣/结算/退款。 |
| [`76cd5114e`](https://github.com/QuantumNous/new-api/commit/76cd5114eae2fd933ea0da08f41984f1ff52cd85) / main | 排除 | moejs alpha.7。 |
| [`150ab3cd4`](https://github.com/QuantumNous/new-api/commit/150ab3cd439441e246be7bca9a1ab25e339e1865) / main | 部分兼容 | 仅更新本次实际依赖许可证条目，不复制插件/tokenkit 清单。 |
| [`bfdad293c`](https://github.com/QuantumNous/new-api/commit/bfdad293cdd4fd93c4c80b46ce1baa7a3a593e92) / main | 部分兼容 | 内置 Vidu 独立修复 metadata 覆盖已校验时长/分辨率；原有模型保护及计费方式保留，不导入插件。 |
| [`f196934ce`](https://github.com/QuantumNous/new-api/commit/f196934ce6908623cb0ee302cd4ec9a6e11f0228) / main | 排除 | Hailuo/Vidu/Veo 插件与 New API 渠道绑定。 |
| [`718a4e7f9`](https://github.com/QuantumNous/new-api/commit/718a4e7f9751c5bf1958747cd0c2c895e2b937d4) / main | 兼容移植 | Vertex us/eu 官方多区域主机名，保留 global、单区域及自定义地址。 |
| [`19d435e7f`](https://github.com/QuantumNous/new-api/commit/19d435e7fdfdaa2b9a734a76393f112cf980b815) / main | 部分兼容 | 保留更严格地址脱敏，仅补 IPv6 等漏遮罩；不限制为只遮自定义地址。 |
| [`908f94d65`](https://github.com/QuantumNous/new-api/commit/908f94d65d4f60729db3d8870f505ca3fba943c5) / main | 兼容移植 | 控制台搜索复用本地侧栏权限过滤，覆盖主站/子站角色及用户隐藏项。 |
| [`c9179d38b`](https://github.com/QuantumNous/new-api/commit/c9179d38b780c1d6635bcbc50f77b5074f8b49bb) / main | 兼容移植 | Playground 支持 reasoning 别名，保留 reasoning_content 明确空值优先级及同帧顺序。 |
| [`6549a17ed`](https://github.com/QuantumNous/new-api/commit/6549a17eda83eaa270fd8cbb0c03623836b351d6) / main | 兼容移植 | Flow 标签随主题颜色，保留零 token 标签和图表数据语义。 |
| [`45fbb149f`](https://github.com/QuantumNous/new-api/commit/45fbb149f0c89ff48d805335f22419be392d0350) / main | 兼容移植 | 显式配置分组各用独立限流；未配置组继续共享用户预算，使用已认证主分组。 |

## 验证范围

验证使用 Go 1.26.9、Bun 1.3.14、Node 24.19.0。Go 模块均以 `GOWORK=off`、只读依赖模式验证；独立模块在 x/text 0.42.0 更新后重新运行全部检查。

| 检查 | 本轮结果 | 原始记录 |
| --- | --- | --- |
| 根模块 `go test -p 2 -mod=readonly -timeout=180s ./...` | 通过，包含认证、支付、计费、站点隔离、SSE 及本次新增回归 | `final-root-test-green.log` |
| 根模块 `go vet -p 2 ./...` / `go build -p 2 ./...` | 均通过 | `final-root-vet.log` / `final-root-build.log` |
| 独立 relaykit `go test` / `go vet` / `go build` | 均通过，不依赖根模块工作区 | `final-relaykit-checks.json` |
| 前端锁文件安装 / `bun run typecheck` | 均通过 | `final-frontend-install.log` / `final-frontend-typecheck-green.log` |
| 前端 `bun run test --maxWorkers=2` | 146 个文件、881 项测试全部通过 | `final-frontend-test.log` |
| 前端 `bun run build` / `bun run format:check` | 生产构建通过；完整 1287 文件格式检查通过 | `final-frontend-build.log` / `final-frontend-format.log` |
| 嵌入新前端包的根应用构建 | 通过；产物保存在审计目录，未运行或部署 | `final-application-build.result.json` / `final-application-artifact.json` |
| 前端本次改动文件 lint | 通过；一条非阻塞 JSX 风格 warning | `frontend-lint-final.log` |
| 前端全库 `bun run lint` | **未通过：284 errors、95 warnings**。全部报错涉及的 151 个文件与 HEAD 字节相同，oxlint 版本及配置未变；属于本次同步前已有问题，未扩展为全库风格改写 | `final-frontend-lint.log` / `final-frontend-lint-baseline-evidence.json` |
| Electron 代理、TLS 及监听地址回归 | 9 项通过，0 失败、0 跳过 | `electron-tests.log` |
| Bun / npm 依赖公告扫描 | 均未发现公告 | `frontend-audit.json` / `electron-audit.json` |
| 根模块 `govulncheck -show verbose ./...` | 0 可达漏洞、0 导入包漏洞；存在 1 项未导入 openpgp 包的模块公告，见下文 | `final-root-govulncheck.log` |
| 独立 relaykit `govulncheck` | 更新 x/text 后未发现漏洞 | `final-relaykit-govulncheck.log` |
| Go 格式 / `git diff --check` | 67 个变更 Go 文件格式通过；差异检查通过 | `final-source-format.json` |

根模块扫描中的 GO-2026-5932 指向 `golang.org/x/crypto/openpgp` 停止维护；本仓库未导入或调用该包，公告没有修复版本。保留其他功能需要的 x/crypto 0.57.0，并如实记录这一模块级提示，不能将结果描述为所有依赖公告均为零。

本轮数据库相关自动化使用测试夹具；没有执行真实 MySQL/PostgreSQL 环境、真实付费供应商请求、浏览器端到端验收或生产部署。没有新增数据库结构或迁移。历史文档中的三数据库、Docker、浏览器或供应商验收不充作本轮结果。

本轮原始证据保存在 `/workspace/scratch/upstream-sync-2026-10-16/`，包括固定提交清单、release 响应、整体合并冲突模拟、模块审计、回归和最终验证日志。该目录为工作区附件，不会随本 Markdown 文件自动复制。
