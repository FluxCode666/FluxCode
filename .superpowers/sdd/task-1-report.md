状态：DONE

修改文件列表：
- backend/ent/group.go
- backend/ent/group/group.go
- backend/ent/group/where.go
- backend/ent/group_create.go
- backend/ent/group_update.go
- backend/ent/migrate/schema.go
- backend/ent/mutation.go
- backend/ent/runtime/runtime.go
- backend/ent/schema/group.go
- backend/internal/handler/admin/group_handler.go
- backend/internal/handler/dto/mappers.go
- backend/internal/handler/dto/types.go
- backend/internal/repository/api_key_repo.go
- backend/internal/repository/group_repo.go
- backend/internal/service/admin_service.go
- backend/internal/service/admin_service_group_test.go
- backend/internal/service/api_key_auth_cache.go
- backend/internal/service/api_key_auth_cache_impl.go
- backend/internal/service/group.go
- backend/migrations/125_add_group_allow_image_generation.sql

提交 hash：
- d232062f1

测试命令和结果：
- `cd /Volumes/T7/project/new/FluxCode/backend && go test -tags=unit ./internal/service -run 'TestAdminService_(CreateGroup_WithImagePricing|UpdateGroup_WithImagePricing|ListGroups_PassesSortParams)$' -count=1`
  - 结果：PASS
- `cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/repository -run 'Test(GroupEntityToService_PreservesMessagesDispatchModelConfig|APIKeyRepository_GetByKeyForAuth_PreservesMessagesDispatchModelConfig_SQLite)$' -count=1`
  - 结果：PASS
- `cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/handler/... -run '^$' -count=1`
  - 结果：PASS（仅编译检查，无匹配测试）

自检结果与 concerns：
- 已确认未修改或暂存 `backend/internal/service/openai_codex_transform.go`、`backend/internal/service/openai_codex_transform_test.go`、`docs/plans/2026-06-22-openai-image-generation-api.md`。
- 已完成 TDD：先在 `admin_service_group_test.go` 添加失败断言并确认 RED，再补实现并回归到 GREEN。
- concerns：无功能性 concerns；仅记录并行执行多个 `go test` 会因大型 Ent 包重复编译显著拖慢验证，因此最终采用串行验证。
