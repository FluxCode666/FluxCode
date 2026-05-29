# OpenAI Responses Long Session 502 Bug Design

## 背景

生产环境中，Codex Desktop 新开会话可以正常请求 `POST /v1/responses`，但持续运行一段时间后，尤其上下文变大并进入工具调用续链后，会出现客户端报错：

```text
unexpected status 502 Bad Gateway
```

Cloudflare 页面显示 Browser 和 Cloudflare 正常，Host Error。生产日志进一步定位到这不是容器 OOM 或进程重启：

- 容器 `fluxcode-backend` 未重启，`oom=false`。
- 命中请求为 `POST /v1/responses`，trace_id 为 `089d2ac0822c1e6034e81e0275c7831d`。
- 后端日志中的真实上游错误是 `400 message=No tool output found for tool search call ...`。
- 当前后端最终向客户端写出的是 `502`。

因此这次问题的核心不是 Cloudflare 或 Nginx 源站不可达，而是 OpenAI Responses 长会话工具续链被网关破坏后，上游 400 又被网关包装成 502。

## 根因

### 1. `previous_response_id` 校验与转发语义不一致

`OpenAIGatewayHandler.Responses` 的 `validateFunctionCallOutputRequest` 会把非空 `previous_response_id` 视为 `function_call_output` 的合法关联上下文。也就是说，请求只要携带 `previous_response_id`，即使 input 中没有本轮 `tool_call/function_call` 或完整 `item_reference`，也会通过 handler 预校验。

但随后 `OpenAIGatewayService.Forward` 在非 WSv2 上游路径中会无条件删除 `previous_response_id`。HTTP 入站请求即使账号开启了 WSv2，也会被强制解析为 HTTP 上游路径，所以该删除逻辑会实际生效。

这会形成断链：

1. 客户端发送 `function_call_output` + `previous_response_id`。
2. Handler 认为可依赖历史 response 关联工具调用，放行。
3. Service 删除 `previous_response_id` 后再发给上游。
4. 上游只看到孤立的 `function_call_output`，找不到对应工具调用，返回 `400 No tool output found ...`。

### 2. OpenAI 上游 400 被映射成客户端 502

非 passthrough OpenAI 错误处理的默认策略会把上游错误改写为 `502 upstream_error`。这让用户看到 Bad Gateway，而不是能直接判断会话状态/请求体失效的 400 invalid request。

结果是客户端会把一个可诊断的请求状态错误理解成网关坏掉，`/compact` 也无法自救，因为 compact 仍依赖已损坏的工具续链上下文。

### 3. 系统提示注入存在 nil 依赖 panic

当前未提交改动中，`OpenAIGatewayService.Forward` 和 passthrough 路径都会调用 `applyResolvedSystemPromptToJSON(..., s.settingService)`。当测试或某些手工构造服务没有注入 `SettingService` 时，`*SettingService(nil)` 作为接口值传入后并不等于 nil，`ResolveEffectiveSystemPrompt` 会继续调用 `GetSystemPromptSettings`，最终在 `s.settingRepo.GetMultiple` 空指针 panic。

这不是生产 502 的主根因，但属于同一热路径上的回归，需要一起修复。

## 修复目标

1. `function_call_output` 依赖 `previous_response_id` 的请求，转发阶段不得无条件删除 `previous_response_id`。
2. OpenAI 上游 400 应以 400 和清晰的 `invalid_request_error` 返回客户端，不应默认包装成 502。
3. 系统提示注入在 `SettingService` 未注入时必须安全跳过，保持继承模式，不得 panic。
4. 保持 WSv2 既有保护：WSv2 在 `function_call_output` 场景中继续保留 `previous_response_id`，在非工具输出场景仍可按现有逻辑做锚点恢复。

## 非目标

- 不重构 OpenAI Gateway 的整体转发架构。
- 不改变 passthrough 模式原样返回上游错误的既有行为。
- 不调整 Cloudflare、Nginx、Docker 或生产部署拓扑。
- 不扩大系统提示功能范围，只修复 nil 依赖安全性。

## 验收标准

- 新增测试能复现：HTTP 入站、请求含 `function_call_output` + `previous_response_id` 时，上游收到的 body 保留 `previous_response_id`。
- 新增测试能复现：OpenAI 上游返回 400 时，客户端响应为 400，错误类型为 `invalid_request_error`，消息包含上游错误摘要。
- 新增测试能复现：`OpenAIGatewayService` 未设置 `SettingService` 时，Forward 不 panic。
- 相关定向测试通过：
  - `go test ./internal/service -run 'TestOpenAIGatewayService_Forward_HTTPIngressPreservesPreviousResponseIDForFunctionCallOutput|TestOpenAIHandleErrorResponse_Upstream400ReturnsInvalidRequest|TestOpenAIGatewayService_Forward_HTTPIngressStaysHTTPWhenWSEnabled|TestOpenAIGatewayService_OAuthPassthrough_UpstreamErrorIncludesPassthroughFlag' -count=1`
  - `go test ./internal/handler -run 'TestOpenAIGatewayHandler.*previous_response_id|TestReadRequestBodyWithPrealloc' -count=1`

