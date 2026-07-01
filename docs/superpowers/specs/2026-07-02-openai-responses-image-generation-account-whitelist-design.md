# OpenAI Responses Image Generation 账号白名单设计

## 背景

`POST /openai/v1/responses` 当前会先解析请求体中的 `model`、`stream` 和 `previous_response_id`，再调用 `SelectAccountWithSchedulerForPlatform` 选择账号，最后进入 `OpenAIGatewayService.Forward`。service 层已经支持识别和规范化 Responses 官方 `image_generation` 工具，但这发生在选中账号之后。

现有 `/v1/images` 路径使用 `OpenAIImagesCapabilityBasic` / `OpenAIImagesCapabilityNative` 表达图片链路能力。这个能力只说明账号能否走图片接口链路，不表达账号级 `model_mapping` 是否显式允许 `gpt-image-*` 模型。因此不能直接复用来约束 `/v1/responses` 中的官方 `image_generation` 工具。

## 目标

- 仅约束 `/openai/v1/responses` 请求中显式携带 OpenAI 官方 `tools[].type == "image_generation"` 的场景。
- 当账号级 `model_mapping` 没有显式允许 `gpt-image-*` 时，该账号视为不支持 Responses 官方生图工具。
- 所有账号都不满足时，返回 `400 invalid_request_error`，语义按“不支持该模型”处理。
- 账号级 `model_mapping` 是唯一白名单来源；渠道级模型映射不算账号支持 `gpt-image-*`。

## 非目标

- 不改变 `/v1/images/generations`、`/v1/images/edits` 的现有能力分类和调度语义。
- 不改变 ChatGPT Web 生图链路。
- 不改变普通文本 Responses 请求的账号选择规则。
- 不把空 `model_mapping` 解释为支持 Responses 官方生图工具；空映射对普通模型仍保持现有“允许所有”的语义。

## 推荐方案

新增一个 Responses 官方生图工具专用的账号调度约束。

handler 在账号选择前从原始请求体中识别 `image_generation` 工具。如果请求需要该工具，则调用账号调度时附带一个新约束，要求候选账号具备账号级 `gpt-image-*` 白名单。scheduler 在 previous response 粘性、session 粘性、负载均衡候选、DB fresh recheck 和等待计划兜底路径中统一应用该约束。

## 账号白名单判定

账号支持 Responses 官方生图工具需同时满足：

- 账号属于 OpenAI 兼容平台，并满足现有图片能力基础要求。
- 账号级 `model_mapping` 非空。
- `model_mapping` 的 key 中存在精确或通配规则可匹配 `gpt-image-*` 系列，例如：
  - `gpt-image-2`
  - `gpt-image-*`
  - 更宽泛的 `gpt-*` 也可匹配，但建议管理端配置使用 `gpt-image-*`，避免误放过宽。

只检查账号级 `model_mapping`，不检查分组默认模型、渠道级映射或请求中的文本模型。这样可以避免渠道映射替账号背书。

## 数据流

1. `OpenAIGatewayHandler.Responses` 读取并校验 JSON 请求体。
2. handler 调用 service helper 识别请求是否显式包含官方 `image_generation` 工具。
3. 如果不包含，维持现有 `SelectAccountWithSchedulerForPlatform` 调度行为。
4. 如果包含，调度请求携带“需要 Responses image_generation 账号白名单”的约束。
5. scheduler 过滤掉账号级 `model_mapping` 不支持 `gpt-image-*` 的账号。
6. 如果没有任何候选账号满足约束，handler 返回 `400 invalid_request_error`，并使用不支持模型语义。
7. 如果选中账号满足约束，后续 `Forward` 继续执行已有的工具规范化、模型映射和上游转发逻辑。

## 错误处理

新增一个明确的模型不支持错误分支，避免把账号能力问题伪装为 `503 Service temporarily unavailable`。

建议错误形态：

```json
{
  "error": {
    "type": "invalid_request_error",
    "message": "Model gpt-image-* is not supported by this account for Responses image_generation"
  }
}
```

非流式请求返回普通 JSON。流式尚未开始时同样返回 JSON；如果未来在流开始后才触发同类错误，沿用现有 SSE error 机制。

## 测试计划

- 单元测试账号白名单判定：
  - 空 `model_mapping` 不支持 Responses 官方生图工具。
  - `model_mapping: {"gpt-image-*":"gpt-image-2"}` 支持。
  - `model_mapping: {"gpt-image-2":"gpt-image-2"}` 支持。
  - `model_mapping: {"gpt-5.1":"gpt-5.1"}` 不支持。
- scheduler 测试：
  - 带新约束时跳过无 `gpt-image-*` 白名单账号，选择有白名单账号。
  - 粘性账号无白名单时不会被复用，会回退到其他满足条件账号。
- handler/service 测试：
  - `/openai/v1/responses` 请求带 `tools:[{"type":"image_generation"}]` 且无账号满足白名单时，返回 `400 invalid_request_error`。
  - 普通 Responses 请求不受影响。

## 风险和回滚

风险主要是已有客户端通过 `/v1/responses` 直接带 `image_generation` 工具，但账号未配置 `gpt-image-*` 白名单。该行为会从“尝试转发到上游”变为“本地 400 拒绝”。这符合目标语义，但上线前需要提醒运维为需要官方生图工具的账号补充账号级 `model_mapping`。

回滚方式是撤销新调度约束和 handler 错误映射，恢复现有 `/v1/responses` 调度行为。由于不涉及数据库迁移，回滚风险低。
