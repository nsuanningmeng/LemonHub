# LemonHub 界面规范

## 产品与实施范围

开发者从首页了解网关协议，在模型广场比较价格，然后登录管理密钥、额度和调用记录。运营人员通过相同的工作台管理用户、渠道、模型和站点设置。公开页面重在介绍产品及接入方法，控制台重在查阅数值、定位记录和执行操作。

本轮从 `6aa5a3008` 开始。上一版首页把介绍和接口端点放在左右两栏，概览以多个并列数据区开场，筛选和行内说明里还有装饰分隔符。本轮先确定导航、色彩和文案用途，再调整页面主操作的位置。使用本地示例接口巡检路由，并在工单、模型、密钥和钱包等路径检查筛选或异常反馈。

沿用 React 19、Rsbuild、Tailwind 4 与 Base UI。TanStack Router 负责路由及守卫，React Query 和 Axios 负责请求，Zustand 保存应用状态，i18next 管理七种界面语言。英文回退文本随入口加载，其余语言各自分包，首次渲染等待所选语言就绪。

## 两种工作场景

| 场景               | 排版和布局                                                                                                 | 操作                                                                           |
| ------------------ | ---------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| 产品首页           | 标题与操作覆盖在横贯版心的月面字符场景上方。较窄屏幕缩小场景高度，下方的接入工作台与厂商目录按阅读顺序排列 | 可选择协议、填写模型名、复制请求，也可按供应商进入模型目录                     |
| 公开目录和账户入口 | 模型目录先选供应商再比较价格。账户表单、初始化步骤和法律正文各自使用适合阅读长度的版心                     | 手机仍可筛选、查看模型详情和使用外部身份验证                                   |
| 控制台             | 深色导航栏贯穿视窗。工作区标题后呈现余额、活动趋势或当前任务；次级信息置于明细区。数字、单位和标签分别排列 | 保留原有权限与菜单选择。窄屏可打开导航和次级筛选，表格与长表单在各自容器内滚动 |
| 运营与支持         | 工单把状态筛选放在记录上方，奖励区分待领取金额与记录，设置页将标签说明与控件对齐                           | 加载失败时提供重试；筛选无结果时可以清除条件。编辑已有子站前先读取该站点的数据 |

标题和按钮采用短语。正文、警告和合同保留语义所需的标点；并列字段优先用间距、行和定义列表区分。协议名、API 路径、代码、掩码、价格单位及用户输入原样展示。

## 视觉变量

色值定义在 `src/styles/theme.css`，主题预设、圆角、密度和字体选择由 `src/styles/theme-presets.css` 覆盖。组件优先使用语义类。

| 用途                    | 默认浅色              | 默认深色              |
| ----------------------- | --------------------- | --------------------- |
| 页面 / 内容表面         | `#f7f7f3` / `#ffffff` | `#151e2d` / `#1c2839` |
| 正文 / 次级说明         | `#182437` / `#566377` | `#edf1f5` / `#b2bfd0` |
| 主操作 / 按钮文字       | `#253e62` / `#ffffff` | `#b7cff3` / `#14243c` |
| 细分隔线 / 控件边框     | `#d5dbe1` / `#a5b0be` | `#354559` / `#60738c` |
| 品牌点色 / 点色上的墨色 | `#ffe45e` / `#17253a` | 同浅色                |

- **字体。** 管理界面沿用用户选定的系统字体、Public Sans 或 Lora。使用等宽字体区分端点、模型标识和代码，金额与计数采用等宽数字。正文通常为 14–16px，标签和表格辅助信息为 12–14px。管理页标题为 20–24px，首页标题随视口缩放。长说明使用约 1.5 倍行高。
- **空间。** 使用 Tailwind 的 4px 间距单位。公共内容的最大外宽为 76rem，典型左右留白为 20–32px。控制台内容留白为 16–24px。组件默认圆角基准为 10px，阴影用于弹出层，静态数据区用细边界划分。
- **表单与列表。** 默认输入、选择器和按钮高度为 36px，表格工具可使用紧凑尺寸。筛选条件与结果靠近，状态同时使用文字和语义色。表头、行操作与分页保持独立可聚焦。
- **反馈。** 进行中的操作提供加载文字或具可访问名称的图标。空状态说明结果，加载失败提供错误信息与适用的重试操作。破坏性操作继续使用已有确认流程。弹窗支持键盘关闭、焦点管理与内容滚动。
- **动效。** 常规反馈使用约 140ms 的颜色过渡。表格记录保持静止。首页的 ASCII 月面场景在独立 worker 中以每秒 5 帧绘制，低性能或偏好减少动态效果的设备使用静态画面。
- **断点。** 以 390、768、1024、1440px 检查长中文、移动导航、工具栏折行、表内滚动与深浅模式。功能性文本、控件和焦点必须在图形之上清楚可见。

## 配置与依赖

站点下发的名称、Logo、文档地址、公开内容、导航开关及主题优先。路由授权、数据库、计费、支付和设置持久化沿用原有代码。新增运行依赖为 MIT 许可的 `ascii.rest` 0.4.0，首页按需加载官方 earthrise 场景，静态图与动画第一帧一致。

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

在仓库根目录运行 `git diff 6aa5a3008..redesign/lemonhub-workspace -- web` 可查看本次前端改动。`redesign/lemonhub-interface` 保存上一版，`4751f44e3` 是最初的前端基线。
