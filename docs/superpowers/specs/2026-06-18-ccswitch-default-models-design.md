# CC-Switch 默认模型导入设计

## 背景

用户密钥页面的“导入到 CCS”按钮会生成 `ccswitch://v1/import` deep link，将当前 API key 导入到 CC-Switch。当前链接只包含 provider 基本信息、endpoint、apiKey 和用量查询配置，没有传递默认模型。CC-Switch provider deep link 支持 `model` 字段，Claude provider 还支持 `opusModel` 字段。

## 目标

调整 CC-Switch 导入链接的默认模型：

- OpenAI/Codex 默认导出 `gpt-5.5`。
- Claude 默认导入 `claude-opus-4-7`。
- Antigravity 选择 Claude 客户端时同样使用 Claude 默认模型。
- Gemini 导入逻辑保持现状，不新增默认模型。

## 方案

采用前端 deep link 参数补充方案，不改后端接口、数据库或 CC-Switch 安装检测逻辑。

- 新增一个小型工具函数负责生成 CC-Switch provider deep link，集中处理参数和默认模型。
- OpenAI 平台映射到 `app=codex` 时，写入 `model`。模型值优先使用后台公开设置中的 `openai_use_key_model_id`，为空时 fallback 到现有默认值 `gpt-5.5`。
- Claude 平台映射到 `app=claude` 时，写入 `model=claude-opus-4-7` 和 `opusModel=claude-opus-4-7`。
- Gemini 平台映射到 `app=gemini` 时不写入模型字段。
- 保留现有 `usageScript`、`usageEnabled` 和 `usageAutoInterval` 参数。

## 数据流

1. `KeysView.vue` 根据 API key 所属分组平台决定 CC-Switch app 和 endpoint。
2. 页面将 provider 名称、endpoint、apiKey、用量脚本和可选 OpenAI 模型配置传入工具函数。
3. 工具函数返回完整 `ccswitch://v1/import?...` 链接。
4. 页面继续用 `window.open(deeplink, '_self')` 触发协议处理。

## 测试

新增工具函数单元测试：

- OpenAI/Codex 链接包含 `model=gpt-5.5`。
- OpenAI/Codex 链接在传入自定义模型时使用自定义值。
- Claude 链接包含 `model=claude-opus-4-7` 和 `opusModel=claude-opus-4-7`。
- Gemini 链接不包含 `model` 或 `opusModel`。

## 非目标

- 不改变普通“一键配置”弹窗的 Claude Code 环境变量。
- 不改变 OpenAI/Codex 普通配置文件生成逻辑。
- 不改变 CC-Switch 协议失败提示和客户端选择弹窗。
