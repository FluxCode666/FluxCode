# OpenAI OAuth 免费账号图片生成 — ChatGPT Web 链路设计

将 OAuth 免费账号（`plan_type` 为 `free` 或空值）的文生图与图生图请求从当前 Codex `/responses` image tool 链路切换到 ChatGPT Web 图片专用链路，解决 `gpt-image-2` 模型生图失败问题。

## 背景

- **问题**：OAuth 免费账号调用 `/v1/images/generations`（model: `gpt-image-2`）失败。
- **根因**：当前 OAuth 图片链路走 Codex `/responses` + `image_generation` tool（主模型 `gpt-5.4-mini`），对免费账号不兼容。
- **参考**：`chatgpt2api` 项目使用 ChatGPT Web 图片专用链路（model slug `gpt-5-3`）可正常工作。

## §1 分流逻辑

在 `forwardOpenAIImagesOAuth` 入口处按 `plan_type` 分流：

- `plan_type` 为 `free` 或空值 → 新链路 `forwardOpenAIImagesChatGPTWeb`
- 其他值（`plus`、`pro`、`team` 等）→ 保持原有 `/responses` image tool 链路

```go
func isOpenAIFreeAccount(account *Account) bool {
    planType := strings.ToLower(strings.TrimSpace(account.GetCredential("plan_type")))
    return planType == "" || planType == "free"
}
```

## §2 ChatGPT Web 图片管线

5 个阶段：

1. **access_token** — 复用已有 `GetAccessToken`
2. **Bootstrap** — `GET chatgpt.com/` 提取 PoW 脚本源
3. **Sentinel** — `POST /backend-api/sentinel/chat-requirements` 获取 sentinel token + proof token
4. **Prepare** — `POST /backend-api/f/conversation/prepare` 获取 `conduit_token`
5. **Generate** — `POST /backend-api/f/conversation` SSE 流 → 收集图片指针 → 下载 → 转换

模型映射：

| 输入模型 | 上游 slug |
|---------|----------|
| `gpt-image-2` | `gpt-5-3` |
| 其他/空 | `auto` |

可复用已有能力：
- `privacyClientFactory`（ImpersonateChrome HTTP 客户端）
- `collectOpenAIImagePointers` + `fetchOpenAIImageDownloadURL` + `downloadOpenAIImageBytes`

需新建能力：
- Bootstrap + PoW 解析
- Sentinel chat-requirements 请求构造（含 proof-of-work token 计算）
- Prepare payload 构造（`system_hints: ["picture_v2"]`、`conversation_mode.kind: primary_assistant`）
- Generate payload 构造（含 `supported_encodings: ["v1"]`、`client_contextual_info.app_name: chatgpt.com`）
- SSE 事件解析 → 提取 `conversation_id` + 图片指针

## §3 图片编辑（edits）支持 & 响应转换

### 图片编辑

在 Prepare 之前增加图片上传步骤：

1. `POST /backend-api/files`（`use_case: "multimodal"`）→ 获取上传 URL
2. `PUT` 到 Azure Blob 上传二进制
3. `POST /backend-api/files/{file_id}/uploaded` 确认
4. Generate payload 中以 `file-service://{file_id}` 作为 `image_asset_pointer`

### 响应转换

从 SSE 事件流收集：
- `conversation_id`
- `file_ids`（`file-service://` 指针）
- `sediment_ids`（`sediment://` 指针）
- text message（被拦截时的错误文本）

流结束后：
- 有图片指针 → 下载图片 → 输出 `{created, data: [{b64_json, revised_prompt}]}`
- 无图片且被拦截 → 透传上游错误文本

支持非流式（等待全部完成一次性返回）和流式（progress → result 事件格式）。

## §4 错误处理

| 阶段 | 失败 | 处理 |
|------|------|------|
| Bootstrap | 首页不可达 | 502 + 日志 |
| Sentinel | PoW 失败 / 429 | 透传上游状态码 |
| Prepare | conduit_token 为空 | 502 + 日志 |
| Generate | 内容安全拦截 | 透传错误文本（不伪装成功）|
| 图片下载 | 指针解析/超时 | 复用已有重试，最终 500 |
| 图片上传 | 上传失败 | 400/502 + 日志 |

原则：真实透传上游错误，不吞异常。

## §5 测试策略

单元测试（`openai_images_chatgpt_web_test.go`）：

1. 模型映射 — `gpt-image-2` → `gpt-5-3`，空值 → `auto`
2. Prepare payload 构造 — 验证关键字段
3. Generate payload 构造 — 文生图 vs 图生图差异
4. SSE 事件解析 — mock 响应，验证指针提取
5. 分流判断 — `isOpenAIFreeAccount` 行为

不含集成测试（需真实 token），手动验证走实际账号。

## §6 文件组织

**修改**：
- `openai_images_responses.go` — 加 ~5-10 行分支

**新建**：
- `openai_images_chatgpt_web.go` — 新链路全部实现（~400-500 行）
- `openai_images_chatgpt_web_test.go` — 单元测试（~200-300 行）

**不改动**：
- `openai_images.go`、`openai_gateway_service.go`、所有前端文件

## 验收标准

- OAuth 免费账号 `/v1/images/generations`（model: `gpt-image-2`）返回兼容 Images API 响应
- OAuth 免费账号 `/v1/images/edits`（model: `gpt-image-2`）返回兼容 Images API 响应
- 付费账号行为不变
- API Key 行为不变
- 单元测试全部通过
- 错误日志反映真实上游原因
