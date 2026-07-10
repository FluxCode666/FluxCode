# Task 5: GPT-5.6 Reasoning Effort Max And Candidate Models

## 实现内容

- 新增 `normalizeOpenAIReasoningEffortForModel` 与 `deriveOpenAIReasoningEffortFromModelCandidates`；`max` 仅在候选中存在已知 GPT-5.6 模型时有效。
- `extractOpenAIReasoningEffortFromBody` 和 `extractOpenAIReasoningEffort` 改为接收 variadic 模型候选；HTTP、passthrough 与两条 WebSocket 返回路径均传入可用候选。
- HTTP Forward 在 OAuth compact 请求体被变换前提取 reasoning effort 元数据，令 `OpenAIForwardResult.ReasoningEffort` 维持请求原始的 `max` 语义。
- OAuth compact GPT-5.6 请求在发送上游前将 `reasoning.effort: max` 降级为 `xhigh`，仅影响 outbound payload。
- OpenAI compat model splitting 支持 `gpt-5.6-max`，规范化为 `gpt-5.6-sol`，并将 `max` 映射为 Claude output effort `max`。

## 测试命令和结果

### TDD RED 证据

```bash
cd backend && go test -tags unit ./internal/service -run 'TestExtractOpenAIReasoningEffortFromBody_GPT56Max|TestExtractOpenAIReasoningEffortFromBody_MaxRejected|TestExtractOpenAIReasoningEffortFromBody_UsesModelCandidates|TestSplitOpenAICompatReasoningModel_GPT56Max|TestOpenAIOAuthCompactDowngradesGPT56MaxReasoningEffort' -count=1
```

- 结果：FAIL（编译失败）。
- 关键报错：`extractOpenAIReasoningEffortFromBody` 尚未支持第二个模型候选参数，证明新增测试约束的 variadic 接口尚未实现。

### TDD GREEN / 最终定向验证

```bash
cd backend && go test -tags unit ./internal/service -run 'TestExtractOpenAIReasoningEffortFromBody|TestSplitOpenAICompatReasoningModel|TestOpenAIOAuthCompactDowngradesGPT56MaxReasoningEffort' -count=1
```

- 结果：PASS，`ok github.com/Wei-Shaw/sub2api/internal/service 0.065s`。
- 覆盖：GPT-5.6 显式 `max`、非 GPT-5.6 拒绝 `max`、模型候选推导、compat model split、OAuth compact 上游降级和返回元数据保真。

## 变更文件

- `backend/internal/service/openai_gateway_service.go`
- `backend/internal/service/openai_ws_forwarder.go`
- `backend/internal/service/openai_compat_model.go`
- `backend/internal/service/openai_gateway_service_hotpath_test.go`
- `backend/internal/service/openai_oauth_passthrough_test.go`
- `backend/internal/service/openai_compat_model_test.go`

## 自审结果

- 已搜索并核对 `extractOpenAIReasoningEffort` / `extractOpenAIReasoningEffortFromBody` 的调用点，HTTP、passthrough、WebSocket 单请求与 ingress turn 路径均已覆盖。
- 已执行 `git diff --check`，未发现空白符错误。
- 未新增依赖、配置、持久化、缓存或 API schema 变更。

## 疑虑

- 无。
