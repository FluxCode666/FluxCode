# OpenAI Codex Image Bridge 上游行为对齐设计

## 背景

当前本地分支为了解决 Codex TUI 普通请求触发 `image_generation` 的问题，临时把 `ensureOpenAIResponsesImageGenerationTool` 改成了 no-op。这能阻止普通文本请求被误改写，但与上游 `upstream/main` 的完整行为不一致。

上游当前语义是：Codex 生图 bridge 默认关闭；显式生图请求受 group 生图权限控制；bridge 打开且 group 允许时，才会为 Codex `/v1/responses` 自动补 `image_generation` 工具、`tool_choice:auto` 和桥接 instructions。上游还覆盖了账号/渠道 override、HTTP 与 WS 路径、compact 豁免、Spark 模型剥离生图工具等边界。

本设计目标是对齐这些外部行为，但不通过 `git merge`、`cherry-pick` 或直接搬运大段上游代码实现。上游只作为行为参考，最终实现应贴合本仓库当前结构。

## 目标

- Codex 普通文本请求默认不注入 `image_generation`。
- `gateway.codex_image_generation_bridge_enabled` 默认关闭，作为全局 bridge 开关。
- bridge 开关可被渠道和账号覆盖，优先级为账号 > 渠道 > 全局。
- group 未允许生图时，显式生图意图在本地返回 `403 permission_error`，错误文案为 `Image generation is not enabled for this group`。
- group 允许生图且 bridge 开启时，Codex 普通 `/v1/responses` 请求自动补齐 `image_generation` 工具、`tool_choice:auto` 和 bridge instructions。
- 显式携带 `image_generation` 工具的请求，即使 bridge 关闭，也继续保留并执行现有 normalize、计费和转发逻辑。
- HTTP `/v1/responses` 与 WS ingress 使用一致的策略。
- `/v1/responses/compact` 不注入 image tool 或 `tool_choice`。
- `gpt-5.3-codex-spark` 不向上游发送 `image_generation` tool，并继续拒绝 image input。

## 非目标

- 不直接复制上游实现代码，不做 git 合并，不做 cherry-pick。
- 不重构无关 OpenAI gateway 流程。
- 不改变 `/v1/images/generations`、`/v1/images/edits` 的既有生图接口语义，除非它们需要复用 group 生图权限 helper。
- 不在本设计中实现 Responses image account whitelist；该主题已有独立设计。

## 推荐架构

新增一组本地 helper，收束生图意图判断和 Codex bridge 策略，避免把判断散落到 gateway 多个分支。

### 生图意图 helper

职责：
- 判断 endpoint 是否为生图 endpoint。
- 判断请求 model 或 body model 是否为 image model。
- 判断 `tools[]` 或 `tool_choice` 是否显式选择 `image_generation`。
- 提供稳定错误文案和 group 权限判断。

建议接口语义：
- `ImageGenerationPermissionMessage() string`
- `GroupAllowsImageGeneration(group *Group) bool`
- `IsImageGenerationIntent(endpoint string, requestedModel string, body []byte) bool`
- `IsImageGenerationIntentMap(endpoint string, requestedModel string, reqBody map[string]any) bool`

### Codex bridge policy helper

职责：
- 解析全局配置 `gateway.codex_image_generation_bridge_enabled`。
- 解析渠道 `features_config.codex_image_generation_bridge`。
- 解析账号 `extra.codex_image_generation_bridge` 或 `extra.codex_image_generation_bridge_enabled`。
- 账号 override 优先于渠道 override，渠道 override 优先于全局配置。

该 helper 只回答“bridge 是否开启”，不直接修改请求体。请求体修改继续由 gateway/WS 调用已有 transform helper 完成。

### Transform helper

`ensureOpenAIResponsesImageGenerationTool` 恢复为纯工具函数：当调用方已经确认策略允许时，负责补齐 `tools:[{type:"image_generation", output_format:"png"}]`，并避免重复添加。

新增或恢复 `ensureOpenAIResponsesImageGenerationToolChoiceAuto`：当存在 image tool 且请求未指定 `tool_choice` 时设置为 `"auto"`。如果用户显式传了 `tool_choice`，保持不变。

`applyCodexImageGenerationBridgeInstructions` 继续只在请求已含 image tool 且非 Spark 时追加 bridge instructions，并通过 marker 保证幂等。

`stripCodexSparkImageGenerationTools` 用于 Spark 模型：移除请求中的 `image_generation` tool；移除后 tools 为空则删除 `tools` 字段。

## HTTP 数据流

1. 识别请求是否来自 Codex 官方客户端，保留现有 UA/originator 判断和 `ForceCodexCLI` 语义。
2. 从 context 中取 API key 和 group，计算 `imageGenerationAllowed`。
3. 用 raw body 判断原始 `imageIntent`。如果原始请求显式为生图意图但 group 不允许，立即返回 `403 permission_error`。
4. 执行现有模型映射和上游模型规范化后，再用映射后的 model 补充判断 image intent。若新增判断命中且 group 不允许，同样返回 `403`。
5. 若不是 compact 请求，并且出现以下任一情况，则展开 JSON map：
   - bridge 开启；
   - 请求 model 或 upstream model 是 image model；
   - 请求中 image tool 需要字段 normalize。
6. bridge 开启时，调用 `ensureOpenAIResponsesImageGenerationTool` 与 `ensureOpenAIResponsesImageGenerationToolChoiceAuto`。
7. 对显式或注入的 image tool 执行 normalize、model 校验、日志和 bridge instructions。
8. Spark 模型路径在最终上游转发前剥离 image tool，并继续对 image input 返回模型不支持错误。

## WS 数据流

WS ingress 在 payload 已解析、`previous_response_id` 等基础校验之后执行同一策略：

1. 获取 API key group，计算 group 生图权限。
2. 如果 payload 显式为生图意图但 group 不允许，关闭 WS，原因使用 `ImageGenerationPermissionMessage()`。
3. 如果 Codex bridge 策略开启，展开 payload map，补 image tool、`tool_choice:auto`、normalize 和 bridge instructions。
4. Spark 模型剥离 image tool，并保留 image input 拒绝逻辑。
5. 重新 marshal payload 后继续现有 WS 转发流程。

## 配置与数据结构

### 全局配置

在 gateway 配置中增加：

```yaml
gateway:
  codex_image_generation_bridge_enabled: false
```

默认值必须为 `false`，避免普通 Codex 文本请求被默认改写。

### group 权限

复用或补齐 `Group.AllowImageGeneration`，作为所有生图路径的本地权限 gate。未绑定 group 的 API key 保持兼容行为：允许生图。

### 渠道 override

渠道 `FeaturesConfig` 支持：

```json
{
  "codex_image_generation_bridge": true
}
```

或按平台：

```json
{
  "codex_image_generation_bridge": {
    "openai": true
  }
}
```

### 账号 override

账号 `Extra` 支持：

```json
{
  "codex_image_generation_bridge": true
}
```

也支持嵌套在 `openai` 下的同名键或 `codex_image_generation_bridge_enabled`，用于兼容已有配置风格。

## 行为矩阵

| 场景 | 期望行为 |
| --- | --- |
| 普通 Codex 文本请求，bridge 默认关闭 | 不注入 image tool |
| 普通 Codex 文本请求，group 允许且 bridge 开启 | 注入 image tool、`tool_choice:auto`、bridge instructions |
| 普通非 Codex 请求，bridge 开启 | 不按 Codex bridge 注入 |
| 显式 `tools[].type=image_generation`，group 允许 | 保留并 normalize |
| 显式 `tools[].type=image_generation`，group 禁用 | 本地 403 |
| image model 请求，group 禁用 | 本地 403 |
| 用户显式传 `tool_choice` | 不覆盖 |
| compact 请求，bridge 开启 | 不注入 image tool 或 `tool_choice` |
| Spark 请求携带 image tool | 移除 image tool |
| Spark 请求携带 image input | 返回模型不支持 image input |

## 测试计划

- transform 单测：
  - no tools 时 helper 可补 image tool。
  - 已有 image tool 时不重复补。
  - 已有普通 tools 时追加 image tool。
  - 有 image tool 且无 `tool_choice` 时设置 `"auto"`。
  - 已有 `tool_choice` 时不覆盖。
  - Spark 剥离 image tool。
- policy 单测：
  - 默认全局关闭。
  - 全局开启。
  - 渠道 override 覆盖全局。
  - 账号 override 覆盖渠道和全局。
- HTTP gateway 测试：
  - 默认关闭时普通 Codex 请求不注入。
  - group 允许且 bridge 开启时普通 Codex 请求注入。
  - group 禁用时显式生图请求返回 403。
  - explicit image tool 在 bridge 关闭时仍可转发。
  - compact 请求不注入。
- WS 测试：
  - bridge 开启时 Codex payload 注入。
  - group 禁用时显式生图 payload 被拒绝。
  - Spark payload 剥离 image tool。

## 风险与缓解

- 风险：开启 bridge 后，普通 Codex 请求仍会变成携带 image tool 的请求，部分上游账号可能拒绝。
  - 缓解：全局默认关闭；账号和渠道可细粒度控制；显式生图走 group gate。
- 风险：HTTP 与 WS 策略不一致导致行为漂移。
  - 缓解：共用 intent 和 policy helper，分别补测试。
- 风险：本地实现与上游未来继续分叉。
  - 缓解：以行为矩阵和测试作为对齐契约，而不是依赖源码形态。

## 验收标准

- 不使用 git merge、cherry-pick 或直接搬运上游大段代码。
- 普通 Codex TUI 请求在默认配置下不再触发生图工具。
- bridge 开启后的注入行为与上游行为矩阵一致。
- group 禁用生图时，显式生图请求在本地被拒绝，错误文案稳定。
- 相关 HTTP、WS、transform、policy 测试通过。
