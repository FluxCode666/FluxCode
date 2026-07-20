# 统一媒体路由与存储实施计划

> 供代理执行者：必须使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans，逐任务执行本计划。所有步骤使用 - [ ] 跟踪。

**Goal:** 不修改 Chat、Responses、Anthropic Messages、Gemini 文本链路，把图片/视频入口接入按模型 ID+分组调度的独立媒体域，并为新媒体数据提供 Local 默认、MinIO 可选存储。

**Architecture:** 复用现有 Account、分组、账号选择器和 MediaTask Worker；platform=media 账号只由 Media Scheduler 使用。全局模型 Registry 保存公共模型、厂商、能力、Adapter 和全局别名；账号 media_config 保存上游模型、原生异步能力和声明式请求映射。媒体产物使用 MediaArtifactObjectStore，经 FluxCode 签名代理交付；历史 DB/Qiniu 记录保持旧链路。

**Tech Stack:** Go 1.26、Gin、Ent、PostgreSQL、Redis Streams、AWS SDK v2 S3、Google Wire、Vue 3、TypeScript、Vitest、Docker Compose。

---

## 范围

- 依据 docs/superpowers/specs/2026-07-20-media-unified-routing-and-storage-design.md。
- 当前分支已含 CAS、租约、Redis Worker、Fake Adapter 和媒体 Handler，本计划实现生产化路由和存储。
- 文本调度、OAuth、Chat/Responses/Messages/Gemini 行为不改。
- 新注册媒体模型走统一 Media Gateway；未注册模型暂留旧图片兼容链路。
- 历史 DB/Qiniu 不搬迁；新请求切入统一媒体域后产生新 Artifact。
- 不做取消、模型绑定级权重、服务商级 Adapter 规则。
- 真实 Grok/Seedance/Nano Banana/Veo Adapter 需要上游 fixture；本计划先抽取 OpenAI Images Adapter 和生产注册边界。

---

### Task 1：持久化与 Ent

Files: 修改 backend/ent/schema/media_model_definition.go、backend/ent/schema/media_artifact.go；新建 backend/ent/schema/media_model_alias.go、backend/ent/schema/group_media_model_scope.go、backend/migrations/131_media_unified_routing_storage.sql、backend/migrations/media_unified_routing_storage_migration_test.go。

- [ ] 写迁移失败测试：模型 vendor/default_adapter/default_async_mode，alias 请求 ID 唯一，group scope 复合唯一，artifact storage_provider。
- [ ] 写迁移：旧 artifact provider 为 legacy；创建 alias、group scope、索引和外键。
- [ ] 运行生成和测试：

    cd backend
    go generate ./ent
    go test ./migrations ./internal/repository -run 'Media.*(Migration|Model)' -count=1
    git commit -am "feat(media): add unified routing storage schema"

### Task 2：模型 Registry、别名和 RouteTarget

Files: 修改 backend/internal/service/media_model_registry.go、backend/internal/repository/media_model_repo.go；新建 backend/internal/service/media_route.go、backend/internal/repository/media_model_alias_repo.go；测试 backend/internal/service/media_model_registry_test.go。

- [ ] 写别名命中/禁用/目标不存在/重复、缺少 vendor/Adapter、非法异步模式和能力不匹配测试。
- [ ] 增加类型：

    type MediaRouteRequest struct {
        GroupID int64
        RequestedModel string
        Operation MediaOperation
        Capability MediaType
        SessionHash string
        ClientAsync bool
    }

    type MediaRouteTarget struct {
        AccountID int64
        PublicModelID string
        UpstreamModelID string
        Vendor string
        Adapter string
        NativeAsyncMode NativeAsyncMode
        RequestMapping MediaRequestMapping
    }

- [ ] Registry 原子刷新模型和 alias，刷新失败保留旧快照；刷新阶段只校验 Adapter 名称格式，实际 Adapter 是否已注册在路由启用时校验，避免生产 Adapter 注册任务之前阻塞 Registry 启动。

    cd backend
    go test ./internal/service ./internal/repository -run 'MediaModel|MediaRoute' -count=1
    git commit -am "feat(media): add model routing metadata"

### Task 3：账号媒体模型绑定

Files: 修改 backend/internal/service/media_account_config.go、backend/internal/service/account.go、backend/internal/service/account_service.go；测试 backend/internal/service/media_account_config_test.go。

- [ ] 写未知字段、空模型、空上游模型、非法异步模式、重复模型和多模型账号测试。
- [ ] 实现 version=1 配置：Version、Provider、Models；每个绑定包含 Enabled、UpstreamModel、NativeAsyncMode、RequestMapping。
- [ ] 新配置不生成账号级 Adapter；Adapter 由 vendor+model 决定。
- [ ] 旧 adapter/model_overrides 只读转换，保存时升级；增加 ResolveMediaModelBinding 和 HasMediaModel，媒体调度不调用文本 IsModelSupported。
- [ ] 运行测试并提交。

    cd backend
    go test ./internal/service -run 'MediaAccountConfig|AccountIsModelSupported|GatewayService' -count=1
    git commit -am "feat(media): version account media bindings"

### Task 4：声明式请求映射

Files: 新建 backend/internal/service/media_request_mapping.go、backend/internal/service/media_request_mapping_test.go；修改 backend/internal/service/media_account_config.go、backend/internal/service/media_orchestrator.go。

- [ ] 写 rename/copy/default/enum/cast、嵌套路径、缺失、枚举未命中、类型转换失败、目标冲突测试。
- [ ] 实现 MediaMappingRule、MediaRequestMapping.Validate 和 Apply；路径只允许安全字段段，rename 删除源字段，禁止脚本。
- [ ] 账号选定后保存转换后的请求、上游模型和映射快照；Worker 重试只读快照。
- [ ] 运行并提交。

    cd backend
    go test ./internal/service -run 'MediaRequestMapping|MediaOrchestrator' -count=1
    git commit -am "feat(media): add declarative request mapping"

### Task 5：分组媒体白名单和候选过滤

Files: 修改 backend/internal/service/group.go、backend/internal/service/media_scheduler.go、backend/internal/repository/group_repo.go；新建 backend/internal/repository/group_media_model_scope_repo.go；测试 backend/internal/service/media_scheduler_test.go。

- [ ] 写媒体/文本账号互斥、未授权无候选、同模型多账号同池和能力过滤测试。
- [ ] 实现 ListEnabledMediaModelIDs 和 ReplaceMediaModelScopes，保存时验证模型并去重。
- [ ] 候选必须同时满足 platform=media、分组允许模型、账号绑定模型和 operation 能力；保留账号优先级/负载/并发/冷却/粘性，不加绑定权重。
- [ ] 运行 go test 和 go test -race，提交 feat(media): route by media model scope。

### Task 6：独立媒体生产路由

Files: 修改 backend/internal/server/routes/gateway.go、backend/internal/server/router.go、backend/internal/handler/media_task_handler.go、backend/internal/handler/handler.go、backend/internal/handler/wire.go；测试 backend/internal/server/routes/gateway_test.go 和 backend/internal/handler/media_task_handler_test.go。

- [ ] 写图片/视频创建、查询、内容鉴权测试，断言文本 Handler 未变化。
- [ ] 注册独立媒体路由，静态 content 先于 id，不调用 isOpenAICompatibleGroup。
- [ ] 已注册模型走 Media Gateway，未注册模型暂走旧 OpenAI Images，并记录 legacy 路由指标。
- [ ] 增加 GET /v1/images/{task_id}/content，支持签名和 Range。
- [ ] 运行媒体路由测试，提交 feat(media): enable independent media routes。

### Task 7：Local 对象存储

Files: 新建 backend/internal/service/media_storage.go、backend/internal/service/media_storage_local.go；修改 backend/internal/service/media_content.go、backend/internal/service/media_orchestrator.go；测试 backend/internal/service/media_storage_local_test.go。

- [ ] 写目录创建、默认路径、原子写入、路径穿越、MIME/大小/hash、Range、幂等删除和权限测试。
- [ ] 实现 provider local/minio/legacy 和 Put/Open/Discard。
- [ ] 对象 Key 使用 tasks/{task-id}/{direction}/{random}.{ext}；临时文件 fsync 后 Rename；新 artifact 写 provider=local。
- [ ] 运行 go test ./internal/service -run 'MediaStorageLocal|MediaContent'，提交 feat(media): add local artifact storage。

### Task 8：MinIO 存储

Files: 新建 backend/internal/service/media_storage_minio.go、backend/internal/repository/media_storage_settings.go；测试 backend/internal/service/media_storage_minio_test.go 和 backend/internal/repository/media_storage_integration_test.go。

- [ ] 写 endpoint/bucket/access/secret 缺失、Bucket/读写失败测试，禁止静默 Local fallback。
- [ ] 复用 AWS SDK v2 S3，配置自定义 Endpoint、静态 credentials、UsePathStyle；Put 写 Content-Type/Length/hash，Get 透传 Range。
- [ ] HeadBucket 后执行临时 Put/Get/Delete 健康检查。
- [ ] 运行 go test ./internal/service ./internal/repository -run 'Media.*MinIO'，提交 feat(media): add optional minio storage。

### Task 9：签名代理和存储快照

Files: 新建 backend/internal/service/media_storage_proxy.go；修改 backend/internal/service/media_content.go、backend/internal/handler/media_task_handler.go、backend/internal/repository/media_task_repo.go；测试 backend/internal/service/media_storage_proxy_test.go 和 backend/internal/handler/media_task_handler_test.go。

- [ ] 写有效/过期/篡改 token、跨用户、非法 Range、未知 provider 和不泄露凭证测试。
- [ ] claims 包含 task public ID、artifact ID、position、expires_at；使用现有加密/签名服务生成短期 token。
- [ ] 图片 URL/b64_json 和视频 content 统一经 FluxCode 代理；按 artifact provider 读，legacy 继续旧链路。
- [ ] 运行代理测试，提交 feat(media): deliver artifacts through signed proxy。

### Task 10：媒体存储系统设置

Files: 修改 backend/internal/service/domain_constants.go、backend/internal/service/setting_service.go、backend/internal/service/settings_view.go、backend/internal/handler/admin/setting_handler.go、backend/internal/handler/dto/settings.go、backend/internal/server/routes/admin.go；新建 backend/internal/handler/admin/media_storage_handler.go；测试 backend/internal/service/setting_service_media_test.go 和 backend/internal/handler/admin/setting_handler_media_test.go。

- [ ] 写默认 provider=local、Docker 默认 /app/.fluxcode/generated、非 Docker 默认 ./data/generated、MinIO 必填字段测试。
- [ ] 增加 provider、Local path、MinIO endpoint/bucket/access/secret/region/SSL/path-style/prefix、签名 TTL；secret 保留旧值并脱敏。
- [ ] 增加 POST /admin/settings/media-storage/test，临时执行目录或 Bucket Put/Get/Delete，不先保存、不回显 secret。
- [ ] 运行设置测试，提交 feat(media): add storage settings。

### Task 11：管理端模型、账号、分组和存储 UI

Files: 新建 backend/internal/handler/admin/media_model_handler.go、frontend/src/views/admin/MediaModelsView.vue、frontend/src/components/admin/media/MediaModelEditor.vue、frontend/src/components/admin/media/RequestMappingEditor.vue；修改 frontend/src/api/admin/accounts.ts、frontend/src/api/admin/groups.ts、frontend/src/api/admin/settings.ts、frontend/src/types/index.ts、frontend/src/components/account/MediaConfigEditor.vue、frontend/src/components/admin/group/GroupMediaSettings.vue、frontend/src/views/admin/GroupsView.vue、frontend/src/views/admin/SettingsView.vue、frontend/src/router/index.ts、frontend/src/i18n/locales/zh.ts、frontend/src/i18n/locales/en.ts。

- [ ] 写模型能力/别名、账号多模型、规则增删/预览、分组白名单、Local 多实例提示、MinIO 脱敏测试。
- [ ] 实现模型/别名管理和账号绑定；不提供服务商级 Adapter 选择。
- [ ] 实现分组媒体模型多选、Local 路径、MinIO 字段和连接测试。
- [ ] 运行 Vitest、typecheck，提交 feat(media): add media routing admin UI。

### Task 12：OpenAI Images Media Adapter

Files: 新建 backend/internal/service/openai_media_adapter.go、backend/internal/service/openai_media_adapter_test.go；修改 backend/internal/service/media_adapter.go、backend/internal/service/wire.go；复用 backend/internal/service/openai_images_official_params_test.go fixture。

- [ ] 建立 JSON/multipart、prompt、size、quality、n、response_format、URL/b64_json、错误码契约测试。
- [ ] 实现 Name、MediaSyncGenerator、必要的 MediaContentFetcher；Adapter 处理 Base URL/API Key、上游模型和产物解析。
- [ ] 生产 Wire 显式注册，Fake Adapter 只用于测试；模型只有在 Adapter 存在时启用。
- [ ] 运行 Adapter 测试，提交 feat(media): register openai images adapter。

### Task 13：生产 Wire、Docker 持久化和部署文档

Files: 修改 backend/internal/service/wire.go、backend/internal/repository/wire.go、backend/internal/handler/wire.go、backend/cmd/server/wire.go、backend/cmd/server/wire_gen.go、Dockerfile、deploy/Dockerfile、deploy/backend/Dockerfile、deploy/docker-entrypoint.sh、deploy/docker-compose.yml、deploy/docker-compose.dev.yml、deploy/docker-compose.local.yml、deploy/backend/docker-compose.yml、deploy/DOCKER.md、deploy/README_CLUSTER.md；测试 backend/internal/service/wire_test.go。

- [ ] Provider 测试：Local 注入 Local Store，MinIO 注入 MinIO Store，配置不完整明确报错且不注入 Disabled Store。
- [ ] 运行 go generate ./cmd/server，确认 Handlers.MediaTask、Registry、Scheduler、Worker、ContentService、Storage Store 非 nil。
- [ ] 镜像创建并授权 /app/.fluxcode/generated；Compose 增加持久化挂载；文档说明单实例 Local、多实例 MinIO 或共享 RWX/NFS。
- [ ] 运行 go test 和 docker compose config，提交 feat(media): wire production storage and deployment。

### Task 14：端到端回归和收口

Files: 新建 backend/internal/integration/media_unified_routing_test.go、backend/internal/integration/media_storage_switch_test.go；修改 backend/internal/handler/media_task_handler_test.go、backend/internal/service/media_worker_test.go、backend/internal/server/routes/gateway_test.go、deploy/README.md、deploy/README_CLUSTER.md。

- [ ] 同步/异步四象限：下游同步/异步 × 上游同步/原生异步，验证状态、预扣、结算、超时转异步和内容 URL。
- [ ] 存储切换：Local 创建后切 MinIO 仍读旧 Local；MinIO 创建后切 Local 仍读旧 MinIO；MinIO 故障不写 Local。
- [ ] 历史兼容：旧 DB/Qiniu 继续旧链路；新 Artifact provider 为 Local/MinIO；legacy 不被新 Store 打开。
- [ ] 文本隔离：Chat、Responses、Anthropic Messages、Gemini 文本候选快照不包含媒体账号。
- [ ] 运行全仓门禁：

    cd backend
    go test ./...
    go test -race ./internal/service ./internal/repository
    go vet ./...
    cd ../frontend
    pnpm test -- --run
    pnpm typecheck
    pnpm build

- [ ] 检查 git status 干净并提交 test(media): verify unified routing and storage。

## 计划自检

- 覆盖模型注册/别名、账号隔离、分组白名单、上游映射、请求映射、同步/异步、Local、MinIO、签名代理、历史兼容、多实例、管理端和测试。
- Task 5、6、14 覆盖文本隔离；Task 7–10 覆盖存储安全；Task 2、12 约束 Adapter 按 vendor+model 解析。
- 无 TODO/TBD/待定或空泛步骤；每项都有文件、测试和提交点。
