# Task 16 实施报告

## 结论

已完成未挂生产路由的媒体 API 契约、安全视频内容读取、输入暂存端口、内容持久化策略、standalone Gin Handler 与 Handler Wire 聚合。没有修改生产媒体路由，没有实现真实 Provider Adapter，没有修改 UI、取消 API 或文本链路。

基线：`c6f881a22d675dd1a6dcf4d5aa36c995ab03324e`

计划提交信息：`feat(media): define task and video content api contracts`

## TDD 证据

### 初始 RED

命令：

```bash
cd backend && go test ./internal/service ./internal/repository ./internal/handler -run 'TestMedia(TaskHandler|Router|HTTPContent|Content)' -count=1
```

退出码：`1`

失败符合预期：

- service 缺少 `MediaContent`、`MediaHTTPContentRequest`、`NewMediaContentService`、disabled object store 与 Range 错误；
- repository 缺少 `NewMediaHTTPContentReader`；
- handler 缺少 `NewMediaTaskHandler` 和公开内容类型。

### 后续 RED → GREEN 缺陷循环

1. 输入对象存储只返回 ObjectKey 时，暂存结果丢失本地 SHA-256。
   - RED：`TestMediaContentServiceStageUploadClearsDataAndKeepsChecksum`，实际 checksum 为空。
   - GREEN：合并本地 size/checksum/content-type，仍保证 `Data=nil`。
2. 无 Content-Length 响应超过流式上限后没有立即释放生命周期。
   - RED：`TestMediaHTTPContentReaderLimitsBodyWithoutContentLengthAndCancelsOnClose`，请求上下文未取消。
   - GREEN：超限读立即幂等关闭上游 Body 并 cancel；显式 Close 仍安全。
3. 扩展名为 `.mp4` 的任意二进制曾可按 octet-stream 放行。
   - RED：`TestMediaTaskHandlerRejectsVideoWhoseDetectedTypeDoesNotMatchExtension`，实际返回 202。
   - GREEN：扩展名与 `http.DetectContentType` 必须同时匹配受支持视频类型。
4. 已完成任务缺少输出 Artifact 曾被映射为 404。
   - RED：`TestMediaTaskHandlerMissingCompletedArtifactReturns502WithoutLeak`，实际返回 404。
   - GREEN：任务归属/不存在保持统一 404；已完成内容暂不可用稳定返回 502。
5. 对象存储输出只返回 ObjectKey 时会写入不完整 Artifact。
   - RED：`TestMediaContentServiceStoredOutputKeepsInputMetadata`，MediaType 为空。
   - GREEN：写库前补回媒体类型、Content-Type、size、checksum 与维度元数据。
6. Multipart `async=TRUE`、带 `?sig=` 的图片 URL、已完成图片查询 shape 与契约不一致。
   - RED：对应三项测试分别得到 202、泄露 signed URL、返回顶层 OpenAI create shape。
   - GREEN：Multipart 只接受空串/`true`/`false`；公开图片 URL 禁止 query/fragment/userinfo；图片查询返回 `media.task`，其 `result.data` 复用 OpenAI 数据数组。
7. 256 字节 Idempotency-Key 曾在外部 URL 暂存后才拒绝。
   - RED：`TestMediaTaskHandlerRejectsLongIdempotencyKeyBeforeStagingExternalInput` 的 stage 调用数为 1。
   - GREEN：所有创建入口在解析外部输入/上传暂存前完成 255 字节边界校验。
8. 同一个视频 multipart 同时上传 image/video 时曾忽略一侧并建单。
   - RED：`TestMediaTaskHandlerRejectsAmbiguousImageAndVideoUpload` 返回 202。
   - GREEN：拒绝混合上传、未知文件字段和全局文件数超限。

### 最终 GREEN

精确命令：

```bash
cd backend && go test ./internal/service ./internal/repository ./internal/handler -run 'TestMedia(TaskHandler|Router|HTTPContent|Content)' -count=1
```

退出码：`0`

- service：PASS，`0.720s`
- repository：PASS，`5.940s`
- handler：PASS，`5.114s`

精确 race：

```bash
cd backend && go test -race ./internal/service ./internal/repository ./internal/handler -run 'TestMedia(TaskHandler|Router|HTTPContent|Content)' -count=1
```

退出码：`0`

- service：PASS，`7.040s`
- repository：PASS，`8.063s`
- handler：PASS，`9.453s`

扩大媒体回归：

```bash
cd backend && go test ./internal/service ./internal/repository ./internal/handler -run '^TestMedia' -count=1
```

退出码：`0`，三包均 PASS。

## 文件与接口

新增：

- `backend/internal/service/media_content.go`
- `backend/internal/service/media_content_test.go`
- `backend/internal/repository/media_http_content.go`
- `backend/internal/repository/media_http_content_test.go`
- `backend/internal/handler/media_task_handler.go`
- `backend/internal/handler/media_task_handler_test.go`

修改：

- `backend/internal/service/media_adapter.go`
  - 新增 `MediaContent`、`MediaHTTPContentRequest`；
  - 新增 `MediaHTTPContentReader`、`MediaContentFetcher`、`MediaArtifactObjectStore`、`MediaInputStager`；
  - `MediaArtifactInput` 增加 size/checksum 元数据。
- `backend/internal/service/media_orchestrator.go`
  - `GetForUser` 按跨任务决议改为 `(*MediaTask, []MediaArtifact, error)`；
  - 仍在 service 边界清除 Account/Adapter/Upstream Task/Poll/Billing/ObjectKey/UpstreamReference。
- `backend/internal/service/media_orchestrator_test.go`
  - 同步唯一精确调用方及清洗断言。
- `backend/internal/handler/handler.go`
  - 增加 `MediaTask *MediaTaskHandler` 聚合字段。
- `backend/internal/handler/wire.go`
  - 增加 Handler provider；
  - `MediaTaskApplication -> *service.MediaOrchestrator`；
  - 内容读取/暂存端口绑定到 `*service.MediaContentService`。

没有手改或重新生成 Wire/Ent 生成文件。当前生产初始化图仍不实例化该 standalone Handler；Task 17 可在专用配置和生产路由就绪后完成完整生产图。

## 安全审计

### SSRF / DNS / Redirect

- `ValidateURL` 复用 `urlvalidator.ValidateHTTPURL`，始终 `AllowPrivate=false`；Scheme、host allowlist 和私网字面量在请求前校验。
- `Open` 只把验证后的 URL 交给现有 `HTTPUpstream`。
- `TestMediaHTTPContentReaderRejectsPrivateAddress`：静态阻断 `127.0.0.1`，断言 upstream 零调用。
- `TestMediaHTTPContentReaderRejectsRedirectToPrivateResolvedAddress`：直接通过现有 `httpUpstreamService.redirectChecker -> validateRequestHost -> ValidateResolvedIP` 阻断重定向到私网。
- `TestMediaHTTPContentReaderUsesHTTPUpstreamDNSValidationBeforeDial`：通过真实 `NewHTTPUpstream` 验证 DNS 解析失败在网络 Dial 前终止。
- 每次重定向继续使用现有 `HTTPUpstream.CheckRedirect` 安全链；未新增一套不完整 SSRF 检查。

### Method / Header / Range / 生命周期

- Reader 只能构造 GET；没有公开 method 输入。
- 请求 Header 仅允许 `Authorization`、`Accept`、`User-Agent`、`X-Api-Key`、`X-Goog-Api-Key`；`Connection`、`Cookie` 等 hop-by-hop/用户态敏感 Header 不透传。
- Range 只能是单段 `bytes=`；多 Range 在 upstream 调用前拒绝。
- 上游 416 映射为稳定 Range 错误并立即关闭响应体；Data URL Range 支持闭区间、开放末端和 suffix，非法/不可满足统一由 Handler 返回 416。
- 只接受上游 200/206；声明超限立即 close+cancel；无 Content-Length 用流式硬上限，超限也立即 close+cancel。
- 正常响应 Body 由 Handler defer Close；Reader 的 Close 幂等并取消 timeout context。

### 暂存 / 内容 / 归属

- Disabled object store 的 Put/Open 均返回 `ErrMediaArtifactObjectStoreDisabled`。
- 上传图片/视频必须先 Stage；返回值清除 Data，只保留内部 ObjectKey/reference、size/checksum 和媒体元数据。
- 上传视频在对象存储不可用时返回 `ErrMediaVideoObjectStorageRequired`，Handler 测试断言 Create 零调用。
- 任务 JSON 只由 Orchestrator 注入 Artifact ID，Multipart 字节不进入 RequestSpec。
- Data URL 只接受 `video/*;base64` 且服务端解码；非法 Base64 明确拒绝。
- `OpenVideo` 先用 `GetByPublicIDForUser` 做归属检查，只允许已完成视频；越权/不存在/非视频/未完成统一隐藏为任务 404。
- 完成任务缺少内容或代理内容不可用返回 502，不暴露 Artifact/上游引用。

### 公开 DTO / 错误泄露

- DTO 不序列化领域对象；只映射 public ID、四种公开状态、operation/model/progress/time、安全 result/error。
- 视频完成结果只输出 `/v1/videos/{public_id}/content`，不重定向上游。
- 图片只输出无 userinfo/query/fragment 的 HTTPS PublicURL；signed URL 不输出。
- 不读取或输出 `UpstreamReference`、`ObjectKey`、`UpstreamTaskID`、Poll/Billing metadata、Authorization、账号信息或内部 ErrorMessage。
- GatewayTimeout 固定返回 504 和安全错误码，响应中无 Task ID。
- 查询用 `AuthSubject.UserID`；任务不存在与越权均为同一 404。
- 没有 `Cancel` 方法，也没有 DELETE 路由。

## Handler 契约

仅定义六个方法：

- `CreateImageGeneration`
- `CreateImageEdit`
- `GetImageTask`
- `CreateVideo`
- `GetVideoTask`
- `GetVideoContent`

创建接口从 API Key Context 获取 User/API Key/Group。JSON `async` 使用 `*bool`，nil/false 同步；Multipart 只接受空串、`true`、`false`。Idempotency-Key TrimSpace 后按 UTF-8 字节计数，255 接受、256 拒绝。上传同时执行请求体上限、文件总数、字段、扩展名和 DetectContentType 校验。

## 最终门禁

- `gofmt`：已执行全部变更 Go 文件。
- `git diff --check`：退出码 `0`。
- `go vet ./...`：退出码 `0`。
- `go build ./...`：退出码 `0`。
- `go test ./... -run '^$' -count=1`：退出码 `0`，全仓编译型测试通过。
- `go mod verify`：退出码 `0`，输出 `all modules verified`。
- `git diff --exit-code c6f881a22 -- backend/internal/server/routes/gateway.go`：退出码 `0`，生产 gateway route 零改动。

## 未执行项与已知基线

- 未运行不带过滤条件的完整 `go test ./internal/service` / `go test ./...` 功能测试。任务账本已记录完整 service 测试存在既有 OpenAI stub concurrent map fatal；本任务没有修改该无关基线。
- 没有真实对象存储 Provider、真实媒体 Adapter、生产媒体 Worker/路由或 Task 17 专用 MediaTasks 配置；这是本任务明确的非生产边界，不是测试遗漏。

## 自审

- 所有新增错误包装保留 `%w`；HTTP 响应不直接输出内部 error 文本。
- 所有创建路径在建单前完成 async/idempotency/body/file/MIME 校验，上传视频存储失败时 Create 零调用。
- 所有取得的 HTTP response/body、multipart 临时文件、timer/cancel 均有明确收口。
- context 沿 Handler → Service → Reader → HTTPUpstream 透传。
- 没有日志新增，因此没有 URL、Authorization、账号凭证日志泄露面。
- 搜索确认生产 Handler 仅有六个方法，未出现 Cancel；生产 gateway 文件零 diff。
