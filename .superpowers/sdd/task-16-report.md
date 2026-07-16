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

## 首轮复审修复追加（2026-07-17）

### 复审范围与结论

- 修复基线：`ceb4513c7 feat(media): define task and video content api contracts`。
- 首轮复审指出的 SSRF、URL MIME、Range 错误传播、稳定错误映射、inline Data URL 上限、staging 生命周期、Wire 聚合和三个 Minor 均已按 RED→GREEN 闭环。
- 自审另外补齐 URL `userinfo` 的暂存入口拒绝、CGNAT/全局 multicast 阻断，以及跨 origin redirect 自定义 API key 剥离。
- 仍未挂生产媒体路由，未增加真实媒体 Adapter/对象存储 Provider、进度/UI、取消 API 或 Task 17 专用配置。

### Critical：SSRF 与 DNS rebinding

新增显式小端口：

```go
type SecureHTTPUpstreamPolicy struct {
	AllowedHosts      []string
	RequireAllowlist  bool
	AllowInsecureHTTP bool
	AllowPrivate      bool
}

type SecureHTTPUpstream interface {
	DoSecure(...)
}
```

Reader 对未实现 `SecureHTTPUpstream` 的普通 upstream fail closed。媒体策略固定 `AllowPrivate=false`，不继承全局 `AllowPrivateHosts` 的宽松值；初始请求和每次 redirect 都重新校验 scheme、allowlist、userinfo 和私网字面量，并拒绝 HTTPS→HTTP downgrade。账号配置代理时返回 `ErrSecureHTTPUpstreamProxyUnsupported`，不回退到不安全代理路径。

安全 transport 每个请求独立创建。`DialContext` 对目标 hostname 只解析一次，逐个调用 `urlvalidator.ValidateIP`，跳过不允许的地址，只把通过校验的同一个 IP 用 `net.JoinHostPort` 绑定到实际 socket dial；Request URL/Host 保留原 hostname，因此标准 `http.Transport` 的 TLS SNI 和证书验证仍针对原 hostname。Body Close 时关闭该 transport 的 idle connections。

复审 RED 是缺少安全策略、`DoSecure`、redirect 校验和解析结果绑定导致编译/行为失败；初次 GREEN：

```bash
go test ./internal/repository -run 'Test(MediaHTTPContentReaderFailsClosed|MediaHTTPContentReaderUsesSecurePolicy|SecureHTTPUpstream)' -count=1
```

退出码 `0`，repository PASS，`0.814s`。原 Reader 回归：

```bash
go test ./internal/repository -run 'TestMediaHTTPContent' -count=1
```

退出码 `0`，repository PASS，`0.561s`。

追加安全 RED→GREEN：

- `TestMediaHTTPContentReaderRejectsURLUserinfoDuringValidation`：RED 时 `ValidateURL("https://user:pass@media.example/video.mp4")` 返回 nil error；修复后 GREEN，repository PASS，`0.805s`。
- `TestValidateIPRejectsNonPublicAddressSpace`：RED 时 `100.64.0.1`、`239.1.1.1`、`ff0e::1` 均被接受；修复后 CGNAT 和所有 multicast 同时用于 DNS 结果与 IP 字面量校验，GREEN，`0.404s`。
- `TestSecureHTTPUpstreamStripsCredentialsOnCrossOriginRedirect`：RED 时跨 Host redirect 仍携带 `Authorization`；修复后跨 origin 剥离 `Authorization`、`Proxy-Authorization`、`Cookie`、`X-Api-Key`、`X-Goog-Api-Key`，同 origin（包括隐式/显式默认端口）保留，GREEN，repository PASS，`0.814s`。

### Important：内容与错误契约

1. 外部 URL MIME
   - `MediaContentService.Stage` 只从 URL path 严格映射 png/jpeg/webp 与 mp4/webm/quicktime；query 保留在内部引用但不参与扩展名判断。
   - 无扩展或媒体类别不匹配返回 `ErrInvalidMediaInput`。
   - RED 时合法 `.png` 的 ContentType 为空，错类/无扩展返回 202；GREEN 命令：

```bash
go test ./internal/service ./internal/handler -run 'TestMedia(ContentServiceStageExternalURL|TaskHandlerExternalURL)' -count=1
```

退出码 `0`：service `1.541s`，handler `0.730s`。

2. object store Range 错误传播
   - `OpenVideo` 对 `ErrInvalidMediaRange` 和 `ErrMediaRangeNotSatisfiable` 直接传播，只有 disabled/unavailable 才允许代理 fallback；其他存储错误用 `%w` 返回。
   - RED 时 Range 错误被忽略并从 Data URL 返回 206；GREEN：

```bash
go test ./internal/service -run 'TestMediaContentServicePropagatesObjectStoreRangeErrors' -count=1
```

退出码 `0`，service PASS，`0.943s`。Handler 组合直接测试确认 opener 包装后的不可满足 Range 最终为 416 且不泄露内部错误。

3. Handler 稳定错误映射
   - `ErrMediaIdempotencyConflict` → 409；`ErrMediaContentRejected` / `ErrMediaGenerationNotAllowed` → 403；`ErrMediaInputNotRecoverable` → 400；`ErrMediaTaskInitializing` → 409；`ErrGroupNotFound` → 404。
   - RED 时全部返回 500；GREEN：

```bash
go test ./internal/handler -run 'TestMediaTaskHandlerMapsStableServiceErrors' -count=1
```

退出码 `0`，handler PASS，`0.676s`；测试使用含 Authorization/upstream secret 的包装错误并断言响应不泄露。

4. Inline Data URL 上限
   - 统一上限 `maxInlineMediaDecodedBytes = 1 << 20`；PersistOutputs 在 Artifact 写入前校验。
   - 使用 `base64.NewDecoder` 与 `io.LimitReader(max+1)` bounded decode，避免先整体解码分配；OpenVideo 使用相同路径。
   - 精确 1 MiB 接受，1 MiB+1 返回 `ErrMediaContentTooLarge` 且 Artifact 零写入。
   - RED 时超限仍写 DB，Open 返回 1,048,577 字节 Body；GREEN：

```bash
go test ./internal/service -run 'TestMediaContentServiceBoundsInlineData' -count=1
```

退出码 `0`，service PASS，`0.955s`。

### Important：staging 生命周期与 Wire

新增生命周期端口：

```go
type MediaInputDiscarder interface {
	Discard(ctx context.Context, userID int64, input MediaArtifactInput) error
}

type MediaInputLifecycle interface {
	MediaInputStager
	MediaInputDiscarder
}
```

`MediaArtifactObjectStore` 同步增加 `Discard`。外部 URL 或无 durable key 的输入 cleanup 为 no-op；ObjectKey/内部引用交给 object store，错误用 `%w` 保留。

Handler 在任何 Stage 前读取并验证全部上传；部分 Stage 失败时用 `errors.Join` 保留原错误并逆序 cleanup。Create 返回错误、nil result、GatewayTimeout 或 Failed 时清理；Accepted、FallbackAsync、Completed 保留。JSON 外部 URL 经真实 ContentService cleanup 为 no-op。直接测试覆盖了两个输入在应用拒绝时按 input-2、input-1 逆序清理，以及 GatewayTimeout 清理。

复审 RED 分别表现为第二文件非法时已 Stage 第一文件、第二次 Stage 失败时 cleanup 为零、应用拒绝时 cleanup 为零；GREEN：

```bash
go test ./internal/handler -run 'TestMediaTaskHandler(ValidatesEveryUpload|CleansStagedInputs|KeepsStagedInputs)' -count=1
```

退出码 `0`，handler PASS，`0.643s`。新增 Discard/416/逆序直接覆盖组合命令退出码 `0`：service `1.651s`、handler `0.852s`；GatewayTimeout cleanup 单测 PASS，`0.855s`。

Wire 保留旧 `ProvideHandlers(...)` 供现有 `wire_gen.go` 使用；新增 `ProvideHandlersWithMedia(...)` 设置 `Handlers.MediaTask`，`ProviderSet` 使用新聚合并绑定 `service.MediaInputLifecycle -> *MediaContentService`。未手改生成文件，完整生产依赖链仍留给 Task 17。

Wire RED 为 `ProvideHandlersWithMedia` 未定义；GREEN：

```bash
go test ./internal/handler -run 'TestProvideHandlersWithMedia' -count=1
```

退出码 `0`，handler PASS，`0.871s`。

### Minor

- Create 要求 `AuthSubject.UserID == APIKey.UserID`，事实源使用 `APIKey.UserID`。
- Multipart `n`、`duration_seconds`、`fps` 省略时使用默认；显式非法、零或负数在 Stage 前返回 400。
- `sanitizeMediaTaskForUser` 额外清除 ID/User/APIKey/Group、ErrorMessage、RequestSpec、Stage、async/fallback、Billing/金额/Retry、Version、执行时间和 UpdatedAt 等内部字段。

RED 时身份不一致返回 202、非法数字返回 202、sanitize 后 ID 仍为 1；GREEN：

```bash
go test ./internal/handler ./internal/service -run 'TestMedia(TaskHandlerRejectsMismatched|TaskHandlerRejectsInvalidMultipartNumbers|OrchestratorGetForUserSanitizes)' -count=1
```

退出码 `0`：handler `0.642s`，service `0.832s`。

### 复审后最终验证

精确三包回归（含安全 upstream 和 Wire）：

```bash
go test ./internal/service ./internal/repository ./internal/handler -run 'TestMedia(TaskHandler|Router|HTTPContent|Content)|TestSecureHTTPUpstream|TestProvideHandlersWithMedia' -count=1
```

退出码 `0`：service `1.084s`、repository `0.543s`、handler `1.568s`。

最终精确 race：

```bash
go test -race ./internal/service ./internal/repository ./internal/handler -run 'TestMedia(TaskHandler|Router|HTTPContent|Content)|TestSecureHTTPUpstream|TestProvideHandlersWithMedia' -count=1
```

退出码 `0`：service `2.146s`、repository `2.191s`、handler `2.835s`。

扩大媒体回归：

```bash
go test ./internal/service ./internal/repository ./internal/handler -run '^TestMedia' -count=1
```

退出码 `0`：service `1.529s`、repository `2.210s`、handler `1.560s`。URL validator 全包回归退出码 `0`，`0.406s`。

最终门禁：

- `gofmt`：执行全部变更 Go 文件。
- `git diff --check`：退出码 `0`。
- `go vet ./...`：退出码 `0`。
- `go build ./...`：退出码 `0`。
- `go test ./... -run '^$' -count=1`：退出码 `0`，全仓编译型测试通过。
- `go mod verify`：退出码 `0`，输出 `all modules verified`。
- `git diff --exit-code c6f881a22 -- backend/internal/server/routes/gateway.go`：退出码 `0`。
- 搜索确认 standalone Handler 仍只有六个公开方法，没有 Cancel/DELETE；DTO 只从安全字段构造，没有输出 ObjectKey、UpstreamReference、Authorization 或内部 ErrorMessage。

### 已知基线与边界

- 按任务账本，本轮仍未运行无过滤条件的完整 `go test ./internal/service` / `go test ./...` 功能套件；既有 OpenAI stub concurrent map fatal 与本任务无关。本轮执行了精确/扩大媒体功能回归、race 和全仓编译型测试，没有把编译型 PASS 表述为完整功能套件 PASS。
- 未修改 `backend/internal/server/routes/gateway.go`、progress、UI、取消接口、真实 Adapter 或 Task 17 配置。

### 提交前 code-audit 追加

高风险外部输入与对象生命周期审计发现：客户端在已 Stage 后断开时，原实现把已经取消的 request context 直接传给 `Discard`，cleanup 会立即失败并遗留对象。

- RED：`TestMediaTaskHandlerCleansStagedInputAfterRequestCancellation` 中 Discard 零调用成功，期望 1，handler FAIL，`0.853s`。
- GREEN：cleanup 使用 `context.WithoutCancel` 保留 request values、脱离客户端取消，再统一增加 10 秒 timeout；逆序和 `errors.Join` 语义不变。直接测试 handler PASS，`0.865s`。

最后一次生产代码变更后的 fresh 验证：

- 精确三包回归退出码 `0`：service `0.876s`、repository `2.156s`、handler `2.681s`。
- 扩大 `^TestMedia` 退出码 `0`：service `2.359s`、repository `4.342s`、handler `3.681s`。
- 精确 race 退出码 `0`：service `2.379s`、repository `3.129s`、handler `3.720s`。
- URL validator 全包退出码 `0`，`0.436s`。
- `gofmt`、`git diff --check`、`go vet ./...`、`go build ./...`、`go test ./... -run '^$' -count=1`、`go mod verify` 均再次退出码 `0`。
- `git diff --exit-code c6f881a22 -- backend/internal/server/routes/gateway.go` 与 `git diff --exit-code -- backend/cmd/server/wire_gen.go` 均退出码 `0`，生产 gateway 与生成文件零改动。

最终 code-audit 结论：高风险审计通过，无已知阻断或非阻断代码风险。字段对账仅涉及既定内部任务清洗和公开错误映射，已有直接契约测试；不涉及 DB schema、迁移、依赖、配置、缓存、队列 topic、编辑/删除链路或真实生产路由。测试文件无 Skip/Only；未执行的无过滤完整功能套件及原因继续按上文列为证据边界。
