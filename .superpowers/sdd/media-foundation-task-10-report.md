# Task 10 实施报告：分组媒体权限与跨平台媒体账号候选

## 固定范围与起点

- 固定起点：`c7686b0f6b41c1a9db65aa08f64408567c9975b7`
- 生产改动仅覆盖 Task 10 指定的 9 个前端文件；本报告为 SDD 流程记录。
- 未修改生产文本 Gateway/调度链、媒体生产路由、认证/授权逻辑或 Task 11+ 文件。
- Vue 项目画像：Vue 3、Composition API、Vite、TypeScript、Vitest、Vue Test Utils、SPA；本任务没有新增 route/store/API endpoint。

## 状态与字段映射

三个字段均由 `GroupsView` 的 create/edit reactive form 持有，`GroupMediaSettings` 使用一个 `GroupMediaConfig` 本地快照合并连续输入，再通过单一 `update:modelValue` 契约回写父表单。没有为三个字段建立分散的 ref/store。

| 字段 | API response / 旧数据 | 前端类型 | create/edit form | 组件 v-model | API request | 默认/空值策略 |
| --- | --- | --- | --- | --- | --- | --- |
| `allow_image_generation` | `AdminGroup` 必填 bool；编辑边界兼容 missing/null | `GroupMediaConfig` bool；Create/Update 可选 bool | 父表单字段 | 同名字段 | create/update 显式写入 | create/reset=false；missing/null=false |
| `allow_video_generation` | 同上 | 同上 | 同上 | 同上 | 同上 | 同上 |
| `media_cross_platform_enabled` | 同上 | 同上 | 同上 | 同上 | 同上 | 同上 |

这三个字段没有独立展示列、文本校验或字段级错误结构；提交错误继续复用 GroupsView 现有表单级反馈。typed legacy fixture 覆盖 missing/null，不用 `any` 掩盖旧数据契约。

## RED

先新增行为测试，再运行：

```bash
cd frontend
pnpm test:run src/components/admin/group/__tests__/GroupMediaSettings.spec.ts src/components/common/__tests__/GroupSelector.media.spec.ts src/views/admin/__tests__/GroupsView.imageGeneration.spec.ts
```

结果：退出码 1；3 个测试文件失败，4 个测试失败、1 个原逻辑回归测试通过，`GroupMediaSettings` 因生产组件不存在而 0 test。失败证据包括：

- 标准父级 `ref + v-model` 无法加载新组件；
- 不同平台且开启媒体跨平台的分组未出现在账号候选中；
- GroupsView 创建/编辑表单找不到三个媒体控件，因而无法验证水合、重置和 API payload。

## GREEN 与生产实现

- 新增 `GroupMediaConfig`，并贯通 `AdminGroup`、`CreateGroupRequest`、`UpdateGroupRequest`。
- 新增 `GroupMediaSettings`：一个本地配置快照按字段合并，避免连续更新基于旧 props 丢失先前值；外部 props 变化时显式水合。
- GroupsView 将原 OpenAI 图片开关替换为通用媒体权限组件；create/edit 默认值、computed setter、关闭重置、旧数据水合和 create/update payload 均显式覆盖三个字段。
- GroupSelector 保留无 platform 时全部候选、codex2api→openai 映射和 Antigravity Mixed Scheduling 三平台候选；仅额外并入 `media_cross_platform_enabled === true` 的账号管理候选。
- 中英文提示均明确：跨平台只扩大账号与分组管理中的媒体账号候选，不改变文本请求的平台边界。

定向 GREEN 最终结果：3 个测试文件、7 个测试全部通过。

## 验证证据

### 相关回归

```bash
cd frontend
pnpm test:run \
  src/components/admin/group/__tests__/GroupMediaSettings.spec.ts \
  src/components/common/__tests__/GroupSelector.media.spec.ts \
  src/views/admin/__tests__/GroupsView.imageGeneration.spec.ts \
  src/views/admin/__tests__/GroupsView.fallback.spec.ts \
  src/views/admin/__tests__/groupsMessagesDispatch.spec.ts \
  src/components/account/__tests__/CreateAccountModal.spec.ts \
  src/components/account/__tests__/EditAccountModal.spec.ts \
  src/components/account/__tests__/BulkEditAccountModal.spec.ts \
  src/utils/__tests__/apiKeyGroupSelection.spec.ts
```

结果：9 个测试文件、59 个测试全部通过。

覆盖点：

- 标准父级 v-model 连续三字段更新；
- create 默认 false、连续修改、真实 create request、提交后重置；
- edit 从全 true 切换到 missing/null 旧记录时覆盖先前状态并归一为 false；
- 真实 update request 显式包含三个 bool；
- OpenAI 编辑表单中图片权限控件仅一份，旧 OpenAI Messages 图片控件不再渲染；
- GroupSelector 同平台、跨平台媒体候选、codex2api 映射、Antigravity Mixed Scheduling；
- GroupsView fallback/messages 和三个账号 Modal、API Key 分组选择回归。

### 工程门禁

- `pnpm typecheck`：通过。
- 9 个 Task 10 文件的定向 ESLint：通过，无新增 `any`。
- `pnpm build`：通过，1178 modules transformed；仅有既有 Browserslist 数据陈旧、静态/动态 import 和 chunk size 警告。
- `git diff --check`：通过。

## 自审

- 范围：GroupSelector 的带 platform 调用方仅位于账号 Create/Edit/Bulk 管理 Modal；公告编辑器不传 platform，行为不变。未修改 Gateway 或后端文本选择逻辑。
- 状态：三个字段的持久 owner 是 GroupsView form；子组件只维护一个交互快照并 emits，不直接修改 props，不存在 watcher 自写回触发源。
- 类型：新增生产/fixture 代码没有 `any`；旧数据用明确的 legacy wire fixture 表达 missing/null。
- 安全：没有 `v-html`、动态 URL/style、敏感日志、token/session、本地持久化或认证绕过；后端权限边界未改变。
- 性能：只增加一个小型表单组件和一个布尔过滤条件，无新请求、定时器、全局监听或深层列表状态。

## 遗留风险

- 没有可用的已认证 Admin 浏览器会话/实时后端，因此未绕过认证进行真实浏览器手测；行为由组件测试、实际 API mock request、typecheck、lint 和生产构建覆盖。
- build 的既有 chunk/import/Browserslist 警告与本任务无关，未在 Task 10 范围内处理。
- 本任务只配置并提交媒体权限元数据；实际媒体 Scheduler 对跨平台候选的消费属于后续任务。
