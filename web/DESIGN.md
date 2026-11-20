# LemonHub 界面规范

## 产品与实施范围

开发者从首页了解网关协议，在模型广场比较价格，然后登录管理密钥、额度和调用记录。运营人员通过相同的工作台管理用户、渠道、模型和站点设置。公开页面重在介绍产品及接入方法，控制台重在查阅数值、定位记录和执行操作。

重构基线为 `4751f44e3`。原首页有轮播终端及演示数字，公开导航滚动时改变宽度，控制台卡片在悬停时浮起，数据表每次更新都会触发行入场动效。实施顺序为主题变量与通用组件、导航骨架、首页及目录、用户和管理工作流、真实浏览器回归。

沿用 React 19、Rsbuild、Tailwind 4 与 Base UI。TanStack Router 负责路由及守卫，React Query 和 Axios 负责请求，Zustand 保存应用状态，i18next 管理七种界面语言。英文回退文本随入口加载，其余语言各自分包，首次渲染等待所选语言就绪。

## 两种工作场景

| 场景     | 排版和布局                                                                           | 操作                                                                                             |
| -------- | ------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------ |
| 公开站点 | 首页主标题与等宽字符地球并列。窄屏按阅读顺序堆叠。标题、导航和内容沿同一条起始线对齐 | 首屏提供进入平台与文档的路径。示例明确区分协议与响应。接入地址取自站点配置                       |
| 控制台   | 导航、页面标题、页内筛选及内容依次排列。相关指标共享一块表面，以分隔线区分           | 保留用户选定的列、分页、过滤器与表格操作。窄屏展开筛选或使用列表卡片。长内容的滚动由对应容器承接 |

## 视觉变量

色值定义在 `src/styles/theme.css`，主题预设、圆角、密度和字体选择由 `src/styles/theme-presets.css` 覆盖。组件优先使用语义类。

| 用途                    | 默认浅色              | 默认深色              |
| ----------------------- | --------------------- | --------------------- |
| 页面 / 内容表面         | `#f8f9f6` / `#ffffff` | `#181d19` / `#202621` |
| 正文 / 次级说明         | `#20251f` / `#566054` | `#e9eee6` / `#b2bdaf` |
| 主操作 / 按钮文字       | `#33452d` / `#ffffff` | `#c6dc9e` / `#202719` |
| 细分隔线 / 控件边框     | `#d9ded5` / `#b5bdb0` | `#3b453b` / `#586450` |
| 品牌点色 / 点色上的墨色 | `#d9f46b` / `#20271c` | 同浅色                |

- **字体。** 管理界面沿用用户选定的系统字体、Public Sans 或 Lora。使用等宽字体区分端点、模型标识和代码，金额与计数采用等宽数字。正文通常为 14–16px，标签和表格辅助信息为 12–14px。管理页标题为 20–24px，首页标题随视口缩放。长说明使用约 1.5 倍行高。
- **空间。** 使用 Tailwind 的 4px 间距单位。公共内容的最大外宽为 76rem，典型左右留白为 20–32px。控制台内容留白为 16–24px。组件默认圆角基准为 8px，阴影用于弹出层，静态数据区用细边界划分。
- **表单与列表。** 默认输入、选择器和按钮高度为 36px，表格工具可使用紧凑尺寸。筛选条件与结果靠近，状态同时使用文字和语义色。表头、行操作与分页保持独立可聚焦。
- **反馈。** 进行中的操作提供加载文字或具可访问名称的图标。空状态说明结果，加载失败提供错误信息与适用的重试操作。破坏性操作继续使用已有确认流程。弹窗支持键盘关闭、焦点管理与内容滚动。
- **动效。** 常规反馈使用约 140ms 的颜色过渡。表格记录保持静止。首页可暂停的 ASCII 动画限制在 10fps，低性能或偏好减少动态效果的设备使用静态画面。
- **断点。** 以 390、768、1024、1440px 检查长中文、移动导航、工具栏折行、表内滚动与深浅模式。功能性文本、控件和焦点必须在图形之上清楚可见。

## 配置与依赖

站点下发的名称、Logo、文档地址、公开内容、导航开关及主题优先。路由授权、数据库、计费、支付和设置持久化沿用原有代码。新增运行依赖为 MIT 许可的 `ascii.rest` 0.4.0，首页仅按需加载地球场景。

## 设计工具复现

以下工具安装在本工作区的 `.agents/skills/`，供 Codex 处理设计与审查，应用构建不依赖这些工具。当前会话直接读取各自的 `SKILL.md`；新会话可重新发现它们。

| 工具                      | 核对的版本            | 在本项目的用途                                       |
| ------------------------- | --------------------- | ---------------------------------------------------- |
| UI/UX Pro Max             | 2.15.0，MIT           | 生成设计系统建议，检查焦点、表格密度、颜色语义与交互 |
| Impeccable                | 4.10.0，Apache-2.0    | 审视公开页的信息层次和工作台的操作效率               |
| Design Taste Frontend     | 2.5.1，`f6b7657`，MIT | 审查首页版式、实际内容、对齐和断点                   |
| Anthropic Frontend Design | `e4bb5d9`，Apache-2.0 | 审查字体层次、品牌识别和主视觉                       |
| ascii-rest                | 0.4.0，`9ba5ab1`，MIT | 查证官方场景与挂载方式、核对动画降级                 |

安装来源见 [UI/UX Pro Max](https://github.com/nextlevelbuilder/ui-ux-pro-max-skill)、[Impeccable](https://github.com/pbakaus/impeccable)、[Taste](https://github.com/Leonxlnx/taste-skill)、[Anthropic Frontend Design](https://github.com/anthropics/skills/tree/main/skills/frontend-design) 和 [ascii.rest](https://ascii.rest/docs/)。

从仓库根目录可按已核对的版本安装。

```sh
npm install -g --ignore-scripts ui-ux-pro-max-cli@2.15.0
uipro init --ai codex
npx --yes impeccable@4.10.0 install --providers=codex --scope=project --no-hooks --yes
npx --yes skills@1.9.0 add https://github.com/Leonxlnx/taste-skill/tree/f6b76575f949707b4e3c37cd86cb44b0b98643e8/skills/taste-skill --skill design-taste-frontend --agent codex --yes
npx --yes skills@1.9.0 add https://github.com/anthropics/skills/tree/e4bb5d960d87126ff3b3a8af1a104b7db67560ba/skills/frontend-design --skill frontend-design --agent codex --yes
npx --yes skills@1.9.0 add https://github.com/bas3line/ascii/tree/9ba5ab14a1946dc1ab875d5c456203079854bdcf/skills/ascii-rest --skill ascii-rest --agent codex --yes
```

## 本地预览与变更记录

在 `web/` 运行 `bun install --frozen-lockfile` 和 `bun run build:check` 生成生产产物。开发预览可连接已启动的本地测试后端。

```sh
VITE_REACT_APP_SERVER_URL=http://127.0.0.1:3000 bun run dev --host 127.0.0.1 --port 5173
```

在仓库根目录运行 `git diff 4751f44e3..redesign/lemonhub-interface -- web` 可查看本次前端改动。`work` 分支保存了改动前的版本。
