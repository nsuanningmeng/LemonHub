# v0.4.45 依赖审计修复说明

## 问题来源

2026-10-07，联系页修复提交的 CI 中，前端类型检查、78 个文件的 461 项测试、后端检查和数据库兼容测试通过，但依赖审计失败：`bun audit` 报告 10 项告警，`npm audit` 报告 9 项受影响的桌面依赖记录。

审计依据锁定版本与漏洞公告匹配，并不表示已确认网站存在可利用路径。Seroval 和 KaTeX 属于前端依赖；其余主要来自开发工具及 Electron 下载、打包工具链。桌面审计中的多个记录来自同一条 `global-agent → roarr → sprintf-js` 传递依赖链。

## 修复方式

| 依赖或依赖链 | 修复 | 处理原因 |
| --- | --- | --- |
| KaTeX | 统一为 `0.18.2`，覆盖直接和嵌套依赖 | 修复继承原型属性绕过渲染信任配置的问题。 |
| Seroval | 锁定 `1.6.3` | 同时覆盖反序列化调用风险和 TypedArray 内存耗尽问题；`1.6.2` 尚未覆盖后者。 |
| source-map-js | 锁定 `1.2.2` | 修复恶意索引 source map 导致事件循环阻塞的问题。 |
| tinypool | 锁定 `2.1.2` | 覆盖工作线程配置中的两项原型污染相关公告。 |
| shadcn CLI 的工具依赖 | 移除 CLI，完整保留其静态 Tailwind CSS | 项目只导入该包的样式，没有执行 CLI。移除后不再引入 `braces`、MCP SDK、proxy-addr 和 postcss-selector-parser 的受影响链。 |
| http-cache-semantics | `4.2.0 → 4.3.0`，保持原 `^4` 兼容范围 | 新锁定版本不再命中审计的受影响范围；该公告未明确列出修复版本，且维护者对报告存在异议，不将此次升级表述为已复现并验证的缓存漏洞修复。 |
| global-agent 的日志依赖 | 仅覆盖 `global-agent@3` 下的 `roarr@6.0.0` | `sprintf-js` 无已发布补丁；新版日志库移除了它，并保留消费者使用的日志 API。 |

保留 Electron、electron-builder、`@electron/get` 和 global-agent 的既有版本。下载器兼容性测试发现直接升级 global-agent 会改变自定义 CA 和证书主机名校验行为，因此最终只调整日志依赖。没有禁用审计、添加漏洞忽略项或更改漏洞版本标识。

## 样式和许可

`web/src/styles/shadcn.css` 的样式正文与 `shadcn@4.13.1` 的 `dist/tailwind.css` 完全一致，SHA-256 为 `bc7d83425702955b4cb67cb14ede9d603f9d912376d57a2d81d661094d2a782a`。文件附带完整 MIT 许可，并对该文件单独跳过格式化，以便持续核对上游内容。

构建会移除 CSS 注释，因此另外分发 `web/public/shadcn-LICENSE.txt`；生产产物包含同名文件。第三方许可清单也记录了来源和版权。现有 UI 组件、状态变体及动画样式继续保留。

## 验证范围

- 使用冻结锁文件重新安装后，前端 `bun audit` 与桌面 `npm audit` 均报告 0 项漏洞。
- 前端类型检查、78 个文件的 461 项测试和生产构建通过；涉及文件的格式与版权检查通过。
- 原 CSS 与新 CSS 的编译产物去除注释后完全一致；38 个实际 DOM 状态匹配场景通过，覆盖 Base UI / Radix 的状态及方向变体。
- KaTeX 数学渲染通过；在原型 `trust` 被污染时，恶意 JavaScript 链接仍被拒绝。
- Node.js 22 下 9 项桌面测试通过。测试调用 electron-builder 实际使用的下载器，验证认证代理、HTTPS CONNECT、环境与请求级自定义 CA、不可信证书和错误主机名拒绝、NO_PROXY；启用日志以覆盖替换后的真实日志实现。
- 当前源码的后端构建及 Linux Electron `--dir --publish never` 打包通过；打包内的后端和许可证与输入文件逐字节一致，内嵌前端包含 shadcn 的分发许可。
- 公开的测试证书和私钥仅供本机回归测试，不用于任何生产连接。

全库 lint 仍报告 368 项既有错误，涉及的 172 个源码文件与修复前完全一致；此次没有通过修改无关源码来消除这些结果。上述“通过”不包含全库 lint。

主要公告：[Seroval 调用风险](https://github.com/advisories/GHSA-p6vx-979v-rg4c)、[Seroval 内存耗尽](https://github.com/advisories/GHSA-jp82-f5mq-hwhp)、[KaTeX](https://github.com/advisories/GHSA-238p-pmpm-9mq7)、[sprintf-js](https://github.com/advisories/GHSA-hp3w-g68c-fv3c)、[http-cache-semantics](https://github.com/advisories/GHSA-ch52-4w7c-c8xp)。
