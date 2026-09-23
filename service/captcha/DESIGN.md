# 人机验证与 GeeTest v4 二次校验

`middleware.CaptchaCheck` 从兼容参数 `turnstile` 读取验证凭据，交给本包按管理员配置的渠道验证。浏览器组件显示成功只表示拿到了凭据，登录、注册等受保护操作仍须经过服务端校验。

## GeeTest 的信任边界

- 浏览器提交 `lot_number`、`captcha_output`、`pass_token`、`gen_time`；四项均必填。若同时提交 `captcha_id`，它必须与服务端配置一致，不能用客户端 ID 选择服务端密钥。
- 服务端去除配置 ID/Key 首尾的复制空白，拒绝包含 `*` 的脱敏 Key。完整 Key 只用于服务端 HMAC-SHA256 签名，不下发到公开状态接口或写入日志。
- 通过 HTTPS 表单请求极验 `/validate`；明确拒绝结果和 `status=error` 均阻止业务请求。日志记录 `status/code/msg/result/reason`，不记录原始凭据、签名或 Key。
- 保留现有容灾策略：网络错误、HTTP 5xx 和无法解析的响应会记录日志并放行。这是可用性取舍，不等价于极验已确认用户通过；本次修复不扩大其范围。缺失参数、过时 ID 和无效配置在请求极验之前拒绝。

## 配置变更与排查

- 前端保存机器人保护开关、渠道或验证码公开 ID 后，应使 `status` 查询失效，避免登录页继续初始化旧渠道或旧 ID。
- 控制台展示的 `***` 是脱敏内容；使用复制按钮获取同一业务模块的完整 ID 和 Key。设置页重载后 Key 留空属于敏感设置不回传的正常行为。
- 收到“配置已更新”时刷新页面，重新完成人机验证；旧凭据不能复用。
- 若仍被拒绝，查看服务端 `geetest validate rejected` 日志。`-50304 / lot_number not match` 应检查流水号、签名和 ID/Key 配对；`pass_token expire` 表示需重新验证；`pass_token error` 应检查前后端 ID 是否一致。

接口依据：[服务端 API](https://docs.geetest.com/gt4/apirefer/api/server)、[快捷集成指南](https://docs.geetest.com/gt4/quick_integration_guide)、[服务端 FAQ](https://docs.geetest.com/gt4/faq/server)。

## 回归验证

`go test ./service/captcha` 覆盖签名表单、首尾空白、旧版凭据兼容、过时 ID、脱敏 Key、缺失参数、极验明确拒绝及现有断网容灾行为。前端设置测试覆盖保存配置后刷新公开状态。
