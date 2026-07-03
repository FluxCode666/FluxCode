# Task 6 Report: Wire WS Ingress Behavior

## Summary
- 在 `backend/internal/service/openai_ws_forwarder.go` 的首包 ingress 处理阶段补齐了与 HTTP `/v1/responses` 一致的 image generation policy。
- 当 Codex bridge 开启且 group 允许时，首包会注入 `image_generation` tool、`tool_choice:auto` 与 bridge instructions。
- 当 group 禁用且请求存在显式 image intent 时，WS 入口会返回 `StatusPolicyViolation`，reason 包含 `ImageGenerationPermissionMessage()`。
- 保持 `firstPayload.rawForHash` 不变，sticky session hash 继续基于原始客户端 payload。
- 对 Spark 上游请求补充复用了 `stripCodexSparkImageGenerationTools` 和 `stripCodexSparkImageGenerationToolChoice`，避免残留 image tool / tool_choice。

## Tests Added
- `TestOpenAIGatewayService_Forward_WSv2_CodexImageBridgeInjectsWhenEnabled`
- `TestOpenAIGatewayService_Forward_WSv2_DeniesImageIntentWhenGroupDisabled`

## TDD Evidence
1. 先新增上述两个测试。
2. 运行：
   - `cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/service -run 'TestOpenAIGatewayService_Forward_WSv2_.*Image|Test.*WS.*Image' -count=1`
3. 初次结果为 FAIL：
   - bridge 注入测试未看到 `image_generation` tool。
   - deny 测试未得到 `OpenAIWSClientCloseError`，而是继续尝试连上游。
4. 实现 WS ingress policy 后再次运行同一命令，结果 PASS。

## Files Changed
- `backend/internal/service/openai_ws_forwarder.go`
- `backend/internal/service/openai_ws_forwarder_success_test.go`

## Verification
- `cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/service -run 'TestOpenAIGatewayService_Forward_WSv2_.*Image|Test.*WS.*Image' -count=1`
- 结果：`ok  	github.com/Wei-Shaw/sub2api/internal/service	0.041s`

## Commit
- `feat: apply codex image bridge policy to ws ingress`
