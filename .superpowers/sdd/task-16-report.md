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

## 第二轮复审修复追加（2026-07-17）

### 范围与结论

- 修复基线：`373713874 fix(media): harden content and task contracts`。
- 第二轮 2 个 Critical、3 个 Important 和 1 个 Minor 均按独立 RED→GREEN 闭环；首轮安全、生命周期、DTO、Range、MIME 和大小上限行为保持。
- 本轮仍不修改 progress、生产 gateway、生成文件、Task 17 配置、真实 Adapter、UI 或取消接口。

### Critical 1：staged input 所有权

内部 `MediaCreateResult` 新增 `InputsAdopted bool`，不进入公开 DTO、DB schema 或队列 payload。Handler 不再从 `Create` 是否返回 error 推断所有权，而只按该字段决定 cleanup。

赋值语义：

- 新任务在第一条 queue message 成功且 ready CAS 成功或重读确认后为 `true`；之后同步 wait 的 SubscribeTerminal、DB read、subscription close、request cancel、fallback/timeout 写失败均返回 nonnil result + `true`。
- 输入 Artifact 和 durable spec 已成功写入、但 precharge 结果未知且任务保留为 fenced pending 时为 `true`，保证幂等恢复仍可读取对象。
- 创建前验证、确定性 policy/billing 拒绝、输入持久化失败、queue 失败、ready 失败为 `false`。
- 已有 ready/terminal 任务的幂等复用不接纳本次新 staged inputs，为 `false`；只有 stale initializer 实际使用本次 inputs 并推进到 ready 时才为 `true`。
- Accepted、Completed、FallbackAsync、ready 后 GatewayTimeout/Failed 均保留已接纳输入。

真实 Orchestrator fixture 覆盖 ready 后 cancel、terminal subscription close、后台 Artifact 可读、queue 前失败、幂等复用和 unknown-precharge durable recovery；Handler 覆盖 error+adopted 不清理、显式 false 清理、accepted/completed/fallback/gateway-timeout 保留。

RED：

```bash
go test ./internal/service ./internal/handler -run 'TestMedia(Orchestrator.*(AdoptsInputs|DoesNotAdopt|ZeroTimeout|ParentCancel|SyncSubscribe|SyncTimeoutFallback)|TaskHandler.*(AdoptedBeforeError|ExplicitlyRejectsOwnership))' -count=1
```

`InputsAdopted` 字段不存在，service/handler 编译失败。GREEN 同命令退出码 `0`：service `1.731s`、handler `0.934s`。扩大 Orchestrator 回归退出码 `0`，`0.884s`。

### Critical 2：redirect 完整链凭据

`CheckRedirect` 以 `via[0]` 为初始 origin，并扫描完整历史链。只要 current 非初始 origin，或任一历史 hop 曾离开初始 origin，就删除 `Authorization`、`Proxy-Authorization`、`Cookie`、`X-Api-Key`、`X-Goog-Api-Key` 和自动 `Referer`；因此 A→B→B 的相对 redirect、B→A 回跳都不会从初始 headers 复活凭据或带 signed query 的 Referer。隐式/显式默认端口仍视为同 origin，同源跳转保留凭据和 Referer。

RED：

```bash
go test ./internal/repository -run '^TestSecureHTTPUpstream(StripsCredentialsOnCrossOriginRedirect|NeverRestoresCredentialsAfterCrossOriginRedirect)$' -count=1
```

repository FAIL，`0.886s`：Referer 未删除，完整链规则缺失。GREEN（含同源默认端口反例）退出码 `0`，repository `0.863s`。

### Important 1 与 Minor：IANA special-purpose denylist

`ValidateIP` 改用显式 `netip.Prefix` denylist，并在匹配前 `Unmap` IPv4-mapped IPv6。IPv4 覆盖 0/8、RFC1918、CGNAT、loopback、link-local、协议分配、TEST-NET、6to4 relay、benchmark、multicast、reserved/broadcast；IPv6 覆盖 unspecified/loopback、IPv4 translation、discard-only、协议分配、benchmark/ORCHID、documentation、6to4、ULA、link-local、deprecated site-local 和 multicast。

公网纯函数正例使用 `8.8.8.8`、`::ffff:8.8.8.8`、`2606:4700:4700::1111`，不再把 `203.0.113.x` 当公网 fixture。`SecureHTTPUpstreamPolicy.AllowPrivate` 已删除，安全媒体 URL 和 Dial 固定禁止 private/special-purpose 地址，避免策略字段与 Dial 语义不一致。

RED：

```bash
go test ./internal/util/urlvalidator -run '^TestValidateIP(RejectsIANA|AcceptsPublic)' -count=1
```

退出码 `1`，`0.415s`；19 个 IPv4/IPv6 special-purpose 样本被错误接受。GREEN 联合命令退出码 `0`：urlvalidator `0.416s`、SecureHTTP/Reader repository `0.824s`。

### Important 2：Wire opt-in 源图

全局 `handler.ProviderSet` 恢复使用旧 `ProvideHandlers`，不含 `NewMediaTaskHandler`、媒体 bindings 或 `MediaTaskProviderSet`。新增 opt-in `MediaTaskProviderSet`，只包含 `NewMediaTaskHandler` 和三个 interface binding；`ProvideHandlersWithMedia` 继续保留手工构造测试，Task 17 在完整 provider 链就绪后再显式合并 opt-in set 并切聚合构造器。

AST 源图契约 RED：

```bash
go test ./internal/handler -run '^TestMediaTaskWireProvidersRemainOptInUntilProductionDependenciesExist$' -count=1
```

handler FAIL，`0.862s`：全局 set 含媒体构造和新聚合，不含旧 `ProvideHandlers`。GREEN（连同手工聚合测试）退出码 `0`，handler `0.852s`。

Wire 实际验证：首次 `go generate ./cmd/server` 因本机缺少 `github.com/google/subcommands` 且 `proxy.golang.org` 网络超时退出 `1`，未进入源图分析；随后执行：

```bash
GOPROXY=https://goproxy.cn,direct go generate ./cmd/server
```

退出码 `0`，Wire 成功写入生成结果；`git diff --exit-code -- backend/cmd/server/wire_gen.go` 退出码 `0`，生成文件无变化、未提交。

### Important 3：内容 fallback 与稳定 502

`objectStore.Open` 只有 `ErrInvalidMediaRange`、`ErrMediaRangeNotSatisfiable` 直接返回；其他错误保留为内部 cause，并继续尝试 bounded Data URL 或 Adapter proxy。最终失败统一 `errors.Join(ErrMediaContentUnavailable, causes...)`，既保留内部诊断链，又让公开内容入口稳定映射 502。

覆盖：store 普通 error + Data URL 成功、store 普通 error + Adapter 成功、无 fallback、proxy network failure、`ErrSecureHTTPUpstreamProxyUnsupported`、`ErrMediaSecureUpstreamRequired`、nil content，以及 Range 仍为 416。`GetVideoContent` 使用专用错误映射：归属不存在 404、Range 416、其他内容读取错误 502；响应不输出内部 cause。

RED：

```bash
go test ./internal/service ./internal/handler -run 'TestMedia(ContentService(FallsBack|FinalFallback|NoFallback)|TaskHandlerContentOpenerFailures)' -count=1
```

退出码 `1`：service `1.633s`，普通 store error 阻断 fallback 且 error chain 缺少 unavailable；handler `0.842s`，secure/network opener error 返回 500。GREEN（含 Range 反例）退出码 `0`：service `0.842s`、handler `1.613s`。

### 第二轮 fresh 验证

全部 finding 联合回归：

```bash
go test ./internal/service ./internal/repository ./internal/handler ./internal/util/urlvalidator -run 'TestMedia(Orchestrator|TaskHandler|ContentService|TaskWire)|TestSecureHTTPUpstream|TestProvideHandlersWithMedia|TestValidateIP' -count=1
```

退出码 `0`：service `0.632s`、repository `0.795s`、handler `2.009s`、urlvalidator `1.181s`。

最终精确三包回归：

```bash
go test ./internal/service ./internal/repository ./internal/handler -run 'TestMedia(TaskHandler|Router|HTTPContent|Content|Orchestrator)|TestSecureHTTPUpstream|TestProvideHandlersWithMedia' -count=1
```

退出码 `0`：service `1.085s`、repository `0.553s`、handler `1.602s`。

扩大媒体回归：

```bash
go test ./internal/service ./internal/repository ./internal/handler -run '^TestMedia' -count=1
```

退出码 `0`：service `3.070s`、repository `3.779s`、handler `3.092s`。URL validator 全包退出码 `0`，`0.201s`。

精确 race：

```bash
go test -race ./internal/service ./internal/repository ./internal/handler -run 'TestMedia(TaskHandler|Router|HTTPContent|Content|Orchestrator)|TestSecureHTTPUpstream|TestProvideHandlersWithMedia' -count=1
```

退出码 `0`：service `3.403s`、repository `4.119s`、handler `2.097s`。

最终门禁全部退出码 `0`：

- `gofmt`（全部变更 Go 文件）与 `git diff --check`。
- `go vet ./...`。
- `go build ./...`。
- `go test ./... -run '^$' -count=1`，全仓编译型测试通过。
- `go mod verify`，输出 `all modules verified`。
- `git diff --exit-code c6f881a22 -- backend/internal/server/routes/gateway.go`。
- `git diff --exit-code -- backend/cmd/server/wire_gen.go backend/go.mod backend/go.sum`。
- 搜索确认无 Cancel/DELETE、无 DTO/Authorization/ObjectKey/UpstreamReference/ErrorMessage 泄露，测试文件无 Skip/Only。

### 第二轮 code-audit

高风险审计范围包含外部 URL/DNS/Dial/redirect、认证凭据、任务/对象所有权、幂等/恢复/queue/ready 状态、内容 fallback、公开错误、Wire 可达性和生成结果。输入所有权仅为内部返回契约，不涉及公开字段、DB、迁移、缓存 key 或队列 schema；special-purpose denylist 不受全局媒体宽松配置绕过；内容内部 causes 只参与 `errors.Is` 和服务端诊断，不进入 DTO 或日志。

审计结论：通过，无已知阻断或非阻断代码风险。未执行项仍是无过滤完整 `go test ./internal/service` / `go test ./...` 功能套件，原因继续为既有 OpenAI stub concurrent-map fatal；本轮没有把编译型全仓 PASS 表述为完整功能 PASS。生产媒体路由和 Task 17 provider 链仍不可达，符合任务边界。

## 第三轮复审修复追加（2026-07-17）

### 范围与结论

- 修复基线：`b2dae388f fix(media): harden input ownership and content fallback`。
- 第三轮 1 个 Critical、2 个 Important、1 个 Minor 均完成独立 RED→GREEN；所有权、内容公开错误、URL 安全和 cleanup 生命周期契约保持向后兼容。
- 本轮只修改 Orchestrator/Content/Handler/URL validator 及其测试；仍未修改 progress、生产 gateway、`wire_gen.go`、Task 17 配置、真实 Adapter、UI 或取消接口。

### Critical：durable enqueue 后的输入所有权

第一条 durable `Queue.Enqueue` 成功即为输入所有权转移点。该点之后即使 ready CAS 返回不确定错误、原 request context 的紧接重读失败，detached 补偿也可能通过旧 version CAS 失败后重读确认任务已经 ready；此时 `Create` 即使仍返回内部错误，也必须返回 nonnil `MediaCreateResult` 且 `InputsAdopted=true`，避免 Handler 删除后台任务仍在读取的对象。第一条 enqueue 明确失败仍保持 `false`。

新增 fixture 可注入一次 `GetByID` 错误，组合 `readyWriteAppliedErrors=1` 覆盖“ready 写已提交但结果未知、原上下文重读失败、detached 重读确认 ready”。测试同时验证后台 task 为 queued/precharged、lease 已清除，且输入 Artifact 仍可读取。

RED：

```bash
go test ./internal/service -run '^TestMediaOrchestratorDurableEnqueueConservativelyAdoptsInputsWhenReadyOutcomeIsUncertain$' -count=1
```

service FAIL，`0.931s`，失败点为 `InputsAdopted` 实际 `false`。GREEN（同时包含 queue failure 反例和 ready 正常重读）退出码 `0`，service `0.851s`。

### Important 1：RFC 9780 IPv6 Dummy Prefix

出站 IP denylist 新增 RFC 9780 Dummy IPv6 Prefix `100:0:0:1::/64`，拒绝 `100:0:0:1::1`。公网反例 `8.8.8.8`、`::ffff:8.8.8.8`、`2606:4700:4700::1111` 保持通过。

RED：IPv6 精确用例退出码 `1`，`0.401s`，dummy 地址被错误接受。GREEN：URL validator 全包退出码 `0`，`0.462s`。

### Important 2：OpenVideo task repository 错误分类

`OpenVideo` 仅在 repository error 满足 `errors.Is(err, ErrMediaTaskNotFound)` 时返回隐藏式 404。repository 返回 nil task，以及 task 非 video/非 completed/非当前用户时仍统一隐藏为 404；DB/infra、`context.DeadlineExceeded`、`context.Canceled` 则保留原 cause，并通过 `errors.Join` 同时携带 `ErrMediaContentUnavailable`。Handler 内容入口继续稳定返回安全 502，不输出数据库、网络、上下文或凭据 cause。

RED：service 分类表中 DB/deadline/cancel 三例均只得到 `ErrMediaTaskNotFound`，命令退出码 `1`，`0.892s`。GREEN：service 分类与不合格任务状态测试退出码 `0`，`0.848s`；Handler 的 502/不泄露回归退出码 `0`，`0.884s`。

### Minor：staged input cleanup error 可观测

Handler 构造签名不变，新增可选 `MediaInputCleanupObserver` 与 setter；默认 observer 写结构化 warning，但只记录 operation、input_count 和固定 `discard_failed` classification，不接收或记录 raw cleanup error、ObjectKey、URL、Authorization 或凭据。

Create 应用错误改为延迟输出：若输入未被接纳，先执行 detached cleanup，再用 `errors.Join` 合并 cleanup error，因此原 `ErrMediaInputNotRecoverable` 仍可通过 `errors.Is` 映射为公开 400 `invalid_media_input`，响应不泄露 cleanup cause。若 Accepted/Completed/GatewayTimeout 等响应已经写出，cleanup 失败不会二次写响应，但仍触发安全 observer。

RED：新增 observer API 尚不存在，Handler 编译失败，退出码 `1`。GREEN：业务错误和已写响应两条 cleanup failure 路径连同既有 ownership 回归均通过，handler `0.850s`。

### 第三轮 fresh 验证

精确媒体回归：

```bash
go test ./internal/service ./internal/repository ./internal/handler -run 'TestMedia(TaskHandler|Router|HTTPContent|Content|Orchestrator)|TestSecureHTTPUpstream|TestProvideHandlersWithMedia' -count=1
```

退出码 `0`：service `0.672s`、repository `1.799s`、handler `2.341s`。

扩大媒体回归：

```bash
go test ./internal/service ./internal/repository ./internal/handler -run '^TestMedia' -count=1
```

退出码 `0`：service `2.156s`、repository `1.952s`、handler `1.307s`。URL validator 全包退出码 `0`，`0.462s`。

精确 race：

```bash
go test -race ./internal/service ./internal/repository ./internal/handler -run 'TestMedia(TaskHandler|Router|HTTPContent|Content|Orchestrator)|TestSecureHTTPUpstream|TestProvideHandlersWithMedia' -count=1
```

退出码 `0`：service `2.408s`、repository `4.037s`、handler `3.013s`。

其余最终门禁全部退出码 `0`：

- `GOPROXY=https://goproxy.cn,direct go generate ./cmd/server`，Wire 成功生成，`wire_gen.go` 零 diff。
- `gofmt`、`git diff --check`、`go vet ./...`、`go build ./...`。
- `go test ./... -run '^$' -count=1`，全仓编译型测试通过。
- `go mod verify`，输出 `all modules verified`。
- `git diff --exit-code c6f881a22 -- backend/internal/server/routes/gateway.go`。
- `git diff --exit-code -- backend/cmd/server/wire_gen.go backend/go.mod backend/go.sum`。
- 搜索确认无 Cancel/DELETE、无新公开 DTO 或 ObjectKey/URL/Authorization/ErrorMessage 泄露，变更测试无 Skip/Only。

### 第三轮 code-audit

高风险复审覆盖 queue/ready/补偿 CAS 的输入所有权、后台 Artifact 生命周期、task repository 错误链、内容公开 404/502、IPv6 special-purpose denylist，以及 cleanup error 的响应前合并和响应后安全可观测性。默认日志不接触 raw cleanup error；内部 repository causes 只参与服务端 error chain，不进入公开响应。

审计结论：通过，无已知阻断或非阻断代码风险。仍未执行无过滤完整 service/全仓功能套件，原因继续为既有 OpenAI stub concurrent-map fatal；本轮仅将 `go test ./... -run '^$'` 表述为全仓编译型 PASS。

## 第四轮复审修复追加（2026-07-17）

### 范围与结论

- 修复基线：`e754d10f5 fix(media): close ownership and cleanup gaps`。
- 第四轮 2 个 Important、1 个 Minor 均完成 RED→GREEN；前三轮所有权、内容 fallback、URL 安全和公开 DTO 契约保持。
- 生产代码仅修改 `backend/internal/handler/media_task_handler.go`；另修改 Handler 测试和 Content 的 `.MOV` URL 回归测试。未修改 progress、生产 gateway、生成文件、Task 17 配置、UI、取消接口或真实 Adapter。

### Important 1：cleanup 错误分类隔离

所有 create、partial-stage 和 staged-result 验证失败路径均把 primary 业务错误与 cleanup error 分离。公开 `writeServiceError` 只接收原 primary error；cleanup sentinel 不再通过 `errors.Is` 抢占 status/code。cleanup error 仅决定是否调用安全 observer，raw error、ObjectKey、URL、Authorization 和凭据均不进入 HTTP mapper 或 observer 参数。

覆盖矩阵包含 primary `ErrMediaModelNotFound`、`ErrInvalidMediaSpec`、`ErrMediaInputNotRecoverable`，以及 cleanup `ErrMediaArtifactObjectStoreDisabled`、`ErrMediaContentUnavailable`。create 与 partial-stage 的全部组合仍分别返回 400 `invalid_request` 或 400 `invalid_media_input`，响应不泄露 cleanup cause。

RED：

```bash
go test ./internal/handler -run '^TestMediaTaskHandler(CleanupFailureCannotOverride.*|InvalidStagedResultReportsCleanupFailureOnce)$' -count=1
```

handler 退出码 `1`，`0.627s`：model/spec 的 create 与 partial-stage 组合被 cleanup sentinel 改为 502；partial-stage 和 invalid staged-result 的 observer 调用数为 0。GREEN：同组连同既有响应前/响应后 cleanup 测试退出码 `0`，handler `0.855s`。

### Important 2：严格 bounded QuickTime/MOV 检测

`.mov` multipart 上传不再依赖扩展名或 `http.DetectContentType` 的宽松结果。新增 ISO-BMFF 顶层 box 解析：

- 仅在结构有效的 `ftyp` box 中识别 major brand 或 compatible brand `qt  `，并统一映射 `video/quicktime`。
- 校验 32-bit size、64-bit large size、header 最小值、box 边界、`ftyp` 最小 payload 和 compatible brand 四字节对齐。
- 扫描上限固定为 4 KiB/32 boxes，且 `ftyp` box 自身不得越过 sniff 上限，避免大 payload brand 扫描。
- 拒绝任意 `ftypqt` 子串、截断 box、错 brand、undersized box、compatible brand 不对齐和超出 bounded scan 的 `ftyp`。

真实最小 MOV major-brand 和 compatible-brand 两种 header 均通过 Handler、进入 Stage，并以 `video/quicktime` 传给应用。MP4/WebM 上传检测保持通过；外部 URL `.MOV` 继续由 Content Stage 映射为 `video/quicktime`。

首个 RED：

```bash
go test ./internal/handler -run '^TestMediaTaskHandler(AcceptsStrictQuickTimeMOVUpload|RejectsMalformedOrNonQuickTimeMOVUpload)$' -count=1
```

handler 退出码 `1`，`0.901s`：真实 `qt  ` MOV 返回 400，错 brand `.mov` 返回 202。初次 GREEN 退出码 `0`，`0.865s`。

自审补充 bounded RED：结构边界自洽但 `ftyp` size 超过 4 KiB 的样本仍返回 202，handler 退出码 `1`，`0.948s`。将 box 自身纳入 sniff 上限后，MOV 正反例及 MP4/WebM 回归全部退出码 `0`，handler `0.862s`。

### Minor：所有 cleanup failure 统一安全 observer

新增单一内部 `cleanupStagedInputs` helper，统一处理 partial Stage 失败、staged-result 验证失败、应用拒绝以及响应已写后的 cleanup。helper 只在至少一个输入需要 cleanup 且 `Discard` 返回错误时调用一次 observer，operation、input_count 和固定 `discard_failed` classification 保持一致；不传递 raw cleanup error，避免重复 observer 和敏感信息日志。

invalid staged-result 测试验证两次 Discard 失败只产生一次 observer，operation 为 `image_edit`、input_count 为 2；partial-stage 测试验证 input_count 为 1 且不会进入应用 Create。

### 第四轮 fresh 验证

精确媒体回归：

```bash
go test ./internal/service ./internal/repository ./internal/handler -run 'TestMedia(TaskHandler|Router|HTTPContent|Content|Orchestrator)|TestSecureHTTPUpstream|TestProvideHandlersWithMedia' -count=1
```

退出码 `0`：service `1.856s`、repository `2.321s`、handler `2.852s`。

扩大媒体回归：

```bash
go test ./internal/service ./internal/repository ./internal/handler -run '^TestMedia' -count=1
```

退出码 `0`：service `2.265s`、repository `1.972s`、handler `3.348s`。URL validator 全包退出码 `0`，`0.432s`。

精确 race：

```bash
go test -race ./internal/service ./internal/repository ./internal/handler -run 'TestMedia(TaskHandler|Router|HTTPContent|Content|Orchestrator)|TestSecureHTTPUpstream|TestProvideHandlersWithMedia' -count=1
```

退出码 `0`：service `3.440s`、repository `2.128s`、handler `4.084s`。

其余最终门禁全部退出码 `0`：

- `GOPROXY=https://goproxy.cn,direct go generate ./cmd/server`，Wire 成功生成，`wire_gen.go` 零 diff。
- `gofmt`、`git diff --check`、`go vet ./...`、`go build ./...`。
- `go test ./... -run '^$' -count=1`，全仓编译型测试通过。
- `go mod verify`，输出 `all modules verified`。
- `git diff --exit-code c6f881a22 -- backend/internal/server/routes/gateway.go`。
- `git diff --exit-code -- backend/cmd/server/wire_gen.go backend/go.mod backend/go.sum`。
- 搜索确认无 Cancel/DELETE、无 cleanup error 进入公开 mapper/observer、无新公开 DTO 或 Authorization/ErrorMessage 泄露，变更测试无 Skip/Only。

### 第四轮 code-audit

高风险复审覆盖 primary/cleanup error chain 隔离、partial object lifecycle、observer 去重与安全字段、MOV 文件扩展与内容一致性、ISO-BMFF size/整数/截断/scan 上限、MP4/WebM 和 URL `.MOV` 反例。box size 在转换为 `int` 前已限制为不超过当前 `[]byte` 剩余长度；brand payload 只可能来自 4 KiB sniff 范围。

审计结论：通过，无已知阻断或非阻断代码风险。仍未执行无过滤完整 service/全仓功能套件，原因继续为既有 OpenAI stub concurrent-map fatal；本轮仅将 `go test ./... -run '^$'` 表述为全仓编译型 PASS。

## 第五轮复审修复追加（2026-07-17）

### 范围与结论

- 修复基线：`91a48ac32 fix(media): isolate cleanup errors and validate mov`。
- 本轮仅修复 1 个 Important：multipart Stage 的瞬态存储引用不得影响 Idempotency-Key 请求指纹。
- 实际代码改动文件为 `backend/internal/service/media_orchestrator.go` 和 `backend/internal/service/media_orchestrator_test.go`；本报告为第三个改动文件。未修改 progress、生产 gateway、生成文件、Adapter、取消 API、文本链路、nil Body fallback 或 Range 数值语义。

### Important：上传输入的稳定指纹身份

原 `mediaCreateFingerprint` 直接序列化完整 `MediaArtifactInput`，因此 Stage 每次随机生成的 `ObjectKey` 或 `UpstreamReference` 都进入 SHA-256 指纹。同一 multipart 内容使用同一 Idempotency-Key 重试时，即使 checksum、size、position 和媒体元数据完全相同，只要临时对象引用变化就被错误判为冲突。

新增稳定输入身份，仅包含：

- position、MediaType、ContentType；
- SizeBytes、ChecksumSHA256；
- Width、Height、DurationSeconds、Resolution、FPS；
- 已由 Stage 规范化的 ExternalURL。

`ObjectKey`、`UpstreamReference`、Direction 和原始 Data 均不进入指纹。Direction 已在 `normalizeMediaInputs` 固定为 `input`；Data 在同一层明确拒绝并清空。输入仍按 position 排序，因此仅切换 slice 物理顺序不会改变指纹，而 position 与内容的对应关系变化仍会改变指纹。

RED：

```bash
go test ./internal/service -run '^TestMediaOrchestratorIdempotencyRetryReusesUploadWhenObjectKeyChanges$' -count=1
```

退出码 `1`，service `0.854s`。关键失败为第二次请求返回：

```text
media idempotency key conflicts with the original request
```

失败原因与 finding 一致：唯一变化是 `ObjectKey` 从 `staged/random-object-first` 变为 `staged/random-object-second`，checksum、size、position 和媒体元数据相同。

最小实现后的 GREEN：同一命令退出码 `0`，service `0.853s`；第二次请求复用首个 PublicID、`InputsAdopted=false`、task create 与 billing precharge 均只有一次，后台仅保留首个输入 Artifact。

补充守护测试覆盖：

- 相同 checksum 但不同 `UpstreamReference` 同样复用；
- checksum 真正变化仍返回 `ErrMediaIdempotencyConflict`；
- position 对应关系、size 元数据和规范化 ExternalURL 变化仍产生不同指纹；
- 输入 slice 物理顺序变化但 position 不变时指纹保持相同；
- 不符合 operation 的 MediaType 仍被 `ErrMediaInputNotRecoverable` 拒绝。

联合直接测试：

```bash
go test ./internal/service -run '^(TestMediaOrchestratorIdempotencyRetryReusesUploadWhen(ObjectKey|UpstreamReference)Changes|TestMediaCreateFingerprint(PreservesStableInputDifferences|DistinguishesNormalizedExternalURLs))$' -count=1
```

退出码 `0`，service `0.912s`。

### 第五轮扩大验证

直接 Orchestrator 与 fingerprint 回归：

```bash
go test ./internal/service -run 'TestMedia(Orchestrator|CreateFingerprint)' -count=1
```

退出码 `0`，service `0.622s`。

扩大三包媒体回归：

```bash
go test ./internal/service ./internal/repository ./internal/handler -run '^TestMedia' -count=1
```

退出码 `0`：service `1.560s`、repository `1.960s`、handler `1.593s`。

直接相关 race：

```bash
go test -race ./internal/service -run 'TestMedia(Orchestrator|CreateFingerprint)' -count=1
```

退出码 `0`，service `2.133s`。

其余门禁：`gofmt`、`git diff --check` 均退出码 `0`；`git diff --exit-code c6f881a22 -- backend/internal/server/routes/gateway.go` 以及 `git diff --exit-code -- backend/cmd/server/wire_gen.go backend/go.mod backend/go.sum` 均退出码 `0`。搜索确认无 Cancel/DELETE、无测试 Skip/Only，稳定身份结构不含 Data、ObjectKey 或 UpstreamReference。

### 第五轮 code-audit

自审重点覆盖 canonical JSON 字段、输入排序、存储引用排除、内容差异反例、外部 URL、MediaType 验证与公开任务 JSON。实现只改变内部 RequestFingerprint 的输入子结构，不改变任务 RequestSpec、durable Artifact、队列 payload、公开 DTO、数据库 schema 或依赖。

历史 fingerprint 兼容性不构成当前生产迁移风险：Task 16 的生产媒体路由/provider 链仍按既定边界保持 opt-in 且不可达，Task 17 尚未启用。除此之外无已知 concerns。两个已登记 Minor（nil Body fallback、Range 数值语义）按控制器要求未在本轮处理。

## 第六轮复审修复追加（2026-07-17）

### 范围与结论

- 修复基线：`42ea27fe0 fix(media): stabilize staged input fingerprints`。
- 本轮仅修复 1 个 Important：被排除瞬态引用的非 URL 输入必须具备稳定内容身份，不能因 checksum/size 缺失而静默合并。
- 实际代码改动文件为 `backend/internal/service/media_orchestrator.go` 和 `backend/internal/service/media_orchestrator_test.go`；本报告为第三个改动文件。未修改生产 gateway、生成文件、Adapter、取消 API、文本链路、RequestSpec/Artifact/queue payload shape，也未处理已登记的 nil Body 与 Range 两个 Minor。

### Important：非 URL 输入的稳定内容身份契约

第五轮 canonical 正确排除了随机 `ObjectKey`/`UpstreamReference`，但此前 `normalizeMediaInputs` 未要求非 URL 输入携带 checksum/size。两个真实内容不同、只靠不同对象引用区分且 checksum/size 均为空的内部请求因此得到相同指纹，并错误复用首任务。

本轮在请求创建和指纹计算共用的 normalize 阶段增加严格契约：

- `ExternalURL` 输入继续以 Stage 已规范化的 URL 为稳定身份，不要求上传 checksum/size。
- 非 `ExternalURL` 输入必须满足 `SizeBytes > 0`。
- checksum 必须可解码为恰好 32 bytes，即有效 64 位十六进制 SHA-256；通过后规范为小写，与 `MediaContentService.Stage` 的 `hex.EncodeToString` 真实产出一致。
- 缺失 size/checksum、短 checksum 或非十六进制 checksum 均返回现有安全领域错误 `ErrMediaInputNotRecoverable`，且发生在 task create、precharge 和 Artifact 持久化之前。
- 正常 canonical 仍不加入随机 `ObjectKey`/`UpstreamReference`，也不加入原始 Data。

RED：

```bash
go test ./internal/service -run '^TestMediaOrchestratorRejectsUnidentifiedNonURLInputsBeforeIdempotentReuse$' -count=1
```

退出码 `1`，service `0.856s`。ObjectKey 与 UpstreamReference 两个子例的关键失败均为：

```text
Expected error with "media input is not recoverable" in chain but got nil.
```

测试在断言前实际执行首次请求和不同瞬态引用的第二次请求；当前实现两次均无错误并复用同一任务，失败原因与 finding 一致。

最小实现后的 GREEN：同一命令退出码 `0`，service `0.856s`；两个请求均在 task create 前返回 `ErrMediaInputNotRecoverable`，create/precharge 调用数均为 0。

补充守护覆盖：

- 缺 size、缺 checksum、63 位 checksum、64 位非十六进制 checksum 均拒绝。
- 有效相同 checksum + 不同 ObjectKey/UpstreamReference 继续复用。
- 不同 checksum 继续返回 `ErrMediaIdempotencyConflict`。
- ExternalURL 不携带上传 checksum/size 仍能创建并幂等复用。
- 仅对语义上代表已 Stage 上传的既有 fixture 补充正 size 和真实 64 位十六进制 checksum，没有放宽输入契约。

聚焦联合测试：

```bash
go test ./internal/service -run '^(TestMediaOrchestrator(IdempotencyRetryReusesUploadWhen(ObjectKey|UpstreamReference)Changes|RejectsUnidentifiedNonURLInputsBeforeIdempotentReuse|ExternalURLDoesNotRequireUploadContentIdentity)|TestMediaCreateFingerprint(RejectsInvalidNonURLContentIdentity|PreservesStableInputDifferences|DistinguishesNormalizedExternalURLs))$' -count=1
```

退出码 `0`，service `0.921s`。

### 第六轮扩大验证

直接 Orchestrator 与 fingerprint 回归：

```bash
go test ./internal/service -run 'TestMedia(Orchestrator|CreateFingerprint)' -count=1
```

退出码 `0`，service `0.829s`。

扩大三包媒体回归：

```bash
go test ./internal/service ./internal/repository ./internal/handler -run '^TestMedia' -count=1
```

退出码 `0`：service `1.824s`、repository `2.044s`、handler `1.672s`。

直接相关 race：

```bash
go test -race ./internal/service -run 'TestMedia(Orchestrator|CreateFingerprint)' -count=1
```

退出码 `0`，service `2.209s`。

其余门禁：`gofmt`、`git diff --check` 均退出码 `0`；`git diff --exit-code c6f881a22 -- backend/internal/server/routes/gateway.go` 以及 `git diff --exit-code -- backend/cmd/server/wire_gen.go backend/go.mod backend/go.sum` 均退出码 `0`。搜索确认无 Cancel/DELETE、无测试 Skip/Only。

### 第六轮 code-audit

自审覆盖校验顺序、SHA-256 长度/十六进制语义、size 与真实 Stage 输出、ExternalURL 豁免、指纹 canonical、Artifact 生命周期和错误公开映射。`hex.DecodeString` 的结果必须恰好为 `sha256.Size`，避免仅检查字符串长度；规范化后的 checksum 同时进入 stable fingerprint 与后续 durable Artifact metadata。

实现不改变 RequestSpec、Artifact/queue payload shape、公开 DTO、数据库 schema 或依赖。无新增 concerns；两个登记 Minor 按要求未处理。

## 第七轮复审修复追加（2026-07-17）

### 范围与结论

- 修复基线：`6a29a1d2f fix(media): require stable upload identity`。
- 本轮仅修复 1 个 Important：已验证/规范化的上传 size/checksum 必须进入 durable input Artifact。
- 实际代码改动文件为 `backend/internal/service/media_orchestrator.go` 和 `backend/internal/service/media_orchestrator_test.go`；本报告为第三个改动文件。未修改生产 gateway、生成文件、Adapter、取消 API、文本链路、RequestSpec/queue payload shape，也未处理已登记的两个 Minor。

### Important：durable Artifact 保留稳定内容元数据

原 `persistInputs` 创建 `MediaArtifact` 时复制了 ContentType、对象引用和媒体尺寸元数据，但遗漏 `SizeBytes` 与 `ChecksumSHA256`。第六轮 normalize 已验证的稳定内容身份因此只存在于 fingerprint，最终 durable Artifact 仍为 size=0、checksum=""，削弱恢复、审计和完整性校验。

本轮最小生产修复仅在 `MediaArtifact` 初始化中增加：

```go
SizeBytes: input.SizeBytes, ChecksumSHA256: input.ChecksumSHA256
```

输入来自 `validateRequest` 返回的 normalized inputs，因此大写 SHA-256 已在持久化前规范为小写。没有改变 Artifact 类型、repository 接口、DB schema 或 payload shape。

RED：

```bash
go test ./internal/service -run '^TestMediaOrchestratorPersistsResolvedCandidateAndDurableInputSnapshot$' -count=1
```

退出码 `1`，service `0.927s`。测试传入 `SizeBytes=128` 和大写 64 位十六进制 SHA-256，关键失败为：

```text
expected: 128
actual  : 0
```

失败原因与 finding 一致：真实持久化路径创建 Artifact 时未复制 size；checksum 也会因同一遗漏保持为空。

GREEN：同一命令退出码 `0`，service `0.907s`；durable input Artifact 的 size 为 128，checksum 为 normalize 后的小写 64 位 SHA-256。

守护测试同时验证：

- RequestSpec 仍只保存 input Artifact ID，不包含 checksum 或 size_bytes。
- queue enqueue 调用次数保持 2。
- ObjectKey 继续写入 ObjectKey；UpstreamReference 继续写入 UpstreamReference；ExternalURL 继续写入 PublicURL。
- ExternalURL 输入仍保持 size=0/checksum=""，不被误当作上传内容身份。
- UpstreamReference staged 上传同时保留原引用及 size/checksum。

守护联合命令：

```bash
go test ./internal/service -run '^TestMediaOrchestrator(PersistsResolvedCandidateAndDurableInputSnapshot|IdempotencyRetryReusesUploadWhenUpstreamReferenceChanges|ExternalURLDoesNotRequireUploadContentIdentity)$' -count=1
```

退出码 `0`，service `0.846s`。

### 第七轮扩大验证

直接 Orchestrator 与 fingerprint 回归：

```bash
go test ./internal/service -run 'TestMedia(Orchestrator|CreateFingerprint)' -count=1
```

退出码 `0`，service `0.612s`。

扩大三包媒体回归：

```bash
go test ./internal/service ./internal/repository ./internal/handler -run '^TestMedia' -count=1
```

退出码 `0`：service `1.582s`、repository `1.961s`、handler `1.585s`。

直接相关 race：

```bash
go test -race ./internal/service -run 'TestMedia(Orchestrator|CreateFingerprint)' -count=1
```

退出码 `0`，service `2.143s`。

其余门禁：`gofmt`、`git diff --check` 均退出码 `0`；`git diff --exit-code c6f881a22 -- backend/internal/server/routes/gateway.go` 以及 `git diff --exit-code -- backend/cmd/server/wire_gen.go backend/go.mod backend/go.sum` 均退出码 `0`。搜索确认无 Cancel/DELETE、无测试 Skip/Only。

### 第七轮 code-audit

自审覆盖 normalize→fingerprint→persistInputs 数据流、大小写规范化、Artifact 引用字段、RequestSpec 和 queue 行为。生产差异仅为 durable Artifact 初始化新增两个已有字段赋值，不改变任何外部契约、schema 或接口。

无新增 concerns；两个登记 Minor 按要求未处理。
