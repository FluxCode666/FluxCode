## Task 0 报告：backend/internal/service 单测编译基线修复

### 实现内容
- 仅修改 `backend/internal/service` 下的测试文件，未改任何生产业务代码。
- 将三个重名测试 helper 重命名并同步更新本地调用点：
  - `float64Ptr` -> `float64PtrForTest`
  - `ptrInt64` -> `ptrInt64ForTest`
  - `timePtr` -> `timePtrForTest`
- 为测试桩补齐了缺失的 no-op / panic-on-unexpected 方法，覆盖：
  - `stubConcurrencyCacheForTest`
  - `userRepoStubForGroupUpdate`
  - `userRepoStub`
  - `mockUserRepo`
  - `mockConcurrencyCache`
  - `snapshotHydrationCache`
- 所有变更文件都已执行 `gofmt`。

### 测试命令与结果
- 运行命令：`cd backend && go test -tags unit ./internal/service -count=1`
- 结果：编译层面的 baseline 已修复，命令已推进到测试运行阶段；当前最终失败点是既有运行时测试失败，不再是 helper 重名或 stub 缺方法。
- 观察到的失败包括：
  - `TestAdminService_BulkUpdateAccounts_MixedChannelPreCheckBlocksOnExistingConflict`
  - `setting_service.go` 后台 warm cache 触发的 `unexpected GetMultiple call` panic

### 变更文件
- `/Users/duegin/.codex/worktrees/b8b6/FluxCode/backend/internal/service/admin_service_apikey_test.go`
- `/Users/duegin/.codex/worktrees/b8b6/FluxCode/backend/internal/service/admin_service_create_user_test.go`
- `/Users/duegin/.codex/worktrees/b8b6/FluxCode/backend/internal/service/admin_service_delete_test.go`
- `/Users/duegin/.codex/worktrees/b8b6/FluxCode/backend/internal/service/concurrency_service_test.go`
- `/Users/duegin/.codex/worktrees/b8b6/FluxCode/backend/internal/service/gateway_multiplatform_test.go`
- `/Users/duegin/.codex/worktrees/b8b6/FluxCode/backend/internal/service/pool_monitor_service_test.go`
- `/Users/duegin/.codex/worktrees/b8b6/FluxCode/backend/internal/service/sales_commission_service_test.go`
- `/Users/duegin/.codex/worktrees/b8b6/FluxCode/backend/internal/service/scheduler_snapshot_hydration_test.go`
- `/Users/duegin/.codex/worktrees/b8b6/FluxCode/backend/internal/service/user_service_test.go`

### 自审结论
- 本次修改严格限制在测试基线修复范围内，没有触碰 `backend/internal/service/concurrency_service.go`、`backend/internal/service/user_service.go` 或其他生产文件。
- 变更性质是机械性的接口对齐与 helper 去重，符合预检 Task 0 的要求。

### 疑虑
- 目标命令现在已经不再卡在编译失败，但仍有两个既有测试运行时失败；它们看起来属于更上层的测试行为问题，不在本次基线修复范围内。
- 如果后续任务希望把 `go test -tags unit ./internal/service -count=1` 变成全绿，需要单独处理这些现有测试失败。
