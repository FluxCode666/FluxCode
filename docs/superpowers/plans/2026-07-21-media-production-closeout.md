# 媒体生成生产收口实施计划

> 执行方式：使用 `superpowers:executing-plans`，按测试驱动拆分实施；每个阶段完成后运行定向门禁。

**目标：** 在不改动 Chat、Responses、Anthropic Messages 与 Gemini 文本链路的前提下，让独立媒体入口具备 GPT Image、Nano Banana、同步/异步、本地/MinIO 存储和真实计费能力。

**边界：** Adapter 只按 `vendor + canonical model/family` 自动解析；账号只配置凭证、上游模型、原生异步能力和声明式请求映射。不实现取消、历史 DB/Qiniu 迁移、服务商级 Adapter 覆盖或文本调度器重构。

## 阶段 1：Artifact 输入与存储快照

- [x] 给 `MediaArtifact`、`MediaArtifactInput` 增加 `StorageProvider`，贯通 Ent Repository 创建、读取及幂等比较。
- [x] 增加受限的输入物化能力：只读取当前任务 `input_artifact_ids`，按 position 保序，拒绝跨任务、错误 direction/type、重复 ID、非法 MIME 和超限内容。
- [x] 修复 HTTP 媒体读取器对图片 Content-Type 的错误视频归一化。
- [x] 为 Local、MinIO、PublicURL 和 data URL 输入补测试。

## 阶段 2：GPT Image Adapter

- [x] 新增 `openai-images` Adapter，使用媒体账号的通用 `base_url`/`api_key` 和冻结的 `upstream_model`。
- [x] 文生图使用 `/v1/images/generations` JSON；图片编辑使用 `/v1/images/edits` multipart。
- [x] 支持 prompt、n、size、quality、response_format、output_format，解析 URL 与 `b64_json`。
- [x] 消费账号映射后的 `ResolvedRequest`，但禁止覆盖认证、URL 和冻结模型。
- [x] 对 4xx、429、5xx、超时、坏响应、内容策略错误建立稳定分类；注册 exact/family 规则并补契约测试。

## 阶段 3：Nano Banana Adapter

- [x] 新增 `nano-banana` Adapter，调用 Gemini 原生 `POST /v1beta/models/{model}:generateContent`。
- [x] 文生图构造 text part；图片编辑构造有序 `inlineData` parts。
- [x] 固定 `responseModalities=[TEXT,IMAGE]`，支持受控的 aspect ratio/image size 映射与 provider 扩展字段。
- [x] 解析 camelCase/snake_case `inlineData`、usage 与安全拦截；只实现同步上游，下游 `async=true` 由本地队列执行。
- [x] 为 Google vendor 的已知 Nano Banana/Gemini Image canonical models 注册 exact 规则并补契约测试。

## 阶段 4：Local/MinIO 对象存储

- [x] 实现 provider 路由：Put 使用当前系统配置；Open/Discard 使用 artifact 自身 provider 快照；未知 provider 明确失败。
- [x] Local 使用安全根目录、原子写入、校验和、Range 与幂等删除；开发默认 `./data/generated`，Docker 默认 `/app/.fluxcode/generated`。
- [x] MinIO 使用 S3-compatible client，支持 endpoint、bucket、region、TLS、path-style、prefix 和 Range；失败不得静默回退 Local。
- [x] 新增加密媒体存储设置和保存前连接测试接口；Secret 只脱敏回传，空值更新保留旧值。
- [x] 管理端增加 Local/MinIO 可视化设置、测试按钮和 Local 多实例提示。
- [x] Docker 镜像创建目录并为 Compose 增加持久化挂载；集群文档说明 MinIO 或共享 RWX/NFS 要求。

## 阶段 5：真实媒体定价与账务

- [x] 基于模型定价配置冻结单价、计费模式、用户/分组倍率和候选上下文，替换 `ZeroMediaPricing`。
- [x] 新增事务型媒体账务操作表，以 `{task_public_id}:{precharge|success|failure}` 唯一幂等。
- [x] 预扣记录余额、赠送余额、订阅等资金来源分配；成功按真实 usage 退款或补扣；失败支持全退或系统配置的惩罚比例（默认扣 80%）。
- [x] 覆盖“账务已成功但任务 CAS 失败后重试”、并发重复调用、赠送余额与订阅回滚。
- [x] 补媒体渠道定价 UI，并确保免费/未知定价不会被生产静默接受。

## 阶段 6：请求映射与接入文档

- [x] 新增请求映射预览 API，复用后端 Validate/Apply 语义。
- [x] 将账号媒体配置中的原始 JSON textarea 升级为规则编辑器，支持 rename/copy/default/enum/cast 和样例预览。
- [x] 文档补齐图片/视频同步、`async=true`、任务查询和 `/content` 下载；修正文档中 model/prompt 必填与实际支持字段。

## 阶段 7：生产接线与验证

- [x] Wire 注册两个真实 Adapter、对象存储、真实 Pricing/Billing，重新生成 `wire_gen.go`。
- [x] 覆盖下游同步/异步 × 上游同步/原生异步、图片编辑、Local/MinIO 切换、失败结算与文本隔离。
- [x] 运行媒体定向 Go/Vitest、migration、race、`go vet`、前端 typecheck/lint/build、Docker Compose config 和全仓测试。
- [x] 区分并记录与本次媒体改造无关的既有失败，确认无新增回归。

## 验证记录

- `go generate ./cmd/server` 连续生成哈希一致；`wire_gen.go` 已注入真实 Adapter、存储、Pricing、Billing 及存储一致性 Repository。
- `go test ./...`、`go vet ./...` 及媒体 Billing/Storage PostgreSQL 集成测试通过；Adapter、存储一致性和计费关键路径 race 测试通过。
- 外链大媒体使用固定缓冲区流式写入 Local/MinIO；真实 `aws s3.Client` 已验证未知长度 chunked 上传、服务端失败和取消后的删除补偿。
- Nano Banana 映射会规范化 `generationConfig` 别名并拒绝任何 `responseModalities` 覆盖；其测试已纳入普通 `go test ./...`。
- `go test -tags=unit ./...` 仍被仓库既有 unit-tag 测试桩老化、旧断言和后台 warmup stub panic 阻塞；失败不涉及本期媒体代码。
- 前端 122 个测试文件、604 条用例、typecheck、build 以及本次 17 个变更文件 ESLint 全部通过。
- 全仓 `lint:check` 的 15 个错误和 1 个警告均位于非本期文件，未扩大修复。
- 6 份 Docker Compose 配置、Shell 语法、`git diff --check` 和补丁级敏感信息扫描通过。

## 发布判定

- 两个 Adapter 均能被 Registry 自动解析，管理员无需填写 Adapter。
- 新 Artifact 的 `storage_provider` 非空，存储切换后旧产物仍按原 provider 可读。
- 生产请求不再经过 Disabled Store、Zero Pricing 或 Disabled Billing。
- 同一账务操作无论 Worker/CAS 如何重试，只影响余额一次。
- 图片/视频独立路由与现有文本协议保持隔离。
