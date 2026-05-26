# 系统提示词配置设计

## 背景

FluxCode 需要支持按层级配置系统提示词，并在请求转发到不同上游平台前按规则注入。配置层级从高到低为：

1. APIKey 自定义系统提示词
2. 分组系统提示词
3. 系统设置中的平台级系统提示词

这个能力必须默认不改变现有请求行为，只有管理员或用户显式配置后才生效。

## 目标

- 系统设置支持按平台配置全局系统提示词和处理模式。
- 分组支持配置系统提示词和处理模式。
- APIKey 支持用户自定义系统提示词和处理模式。
- 转发链路按 `APIKey > 分组 > 系统平台配置` 解析最终规则。
- 支持四种状态：不配置、透传、覆盖、追加。
- 覆盖 Anthropic、OpenAI、Gemini、Antigravity 的主要请求入口。

## 非目标

- 不做多段提示词模板变量渲染。
- 不做按模型、账号、用户角色的系统提示词分支。
- 不改变现有身份补丁、Claude Code mimicry、Codex instructions template 的既有职责，只在它们进入上游前提供统一的业务提示词注入能力。

## 模式语义

新增枚举 `SystemPromptMode`：

- `inherit`：不配置。当前层不产生有效规则，继续向下一层查找；如果系统平台层也是不配置，则最终不注入。
- `passthrough`：透传。客户端已有系统提示词时保留客户端原值；客户端没有系统提示词时注入当前配置提示词。
- `override`：覆盖。用当前配置提示词替换客户端原系统提示词。
- `append`：追加。保留两者，当前配置提示词在前，客户端原系统提示词在后。

空白提示词不构成有效注入内容。保存配置时，`passthrough`、`override`、`append` 必须搭配非空 `system_prompt`；`inherit` 允许提示词为空。

默认值：

- APIKey：`system_prompt_mode = "inherit"`，`system_prompt = ""`
- 分组：`system_prompt_mode = "inherit"`，`system_prompt = ""`
- 系统平台配置：`system_prompt_mode_<platform> = "inherit"`，`system_prompt_<platform> = ""`

这样升级后所有现有请求默认保持原行为。

## 数据模型

### APIKey

在 `api_keys` 表新增：

- `system_prompt text not null default ''`
- `system_prompt_mode varchar(20) not null default 'inherit'`

同步修改：

- ent schema
- migration
- repository create/update/select mapper
- service `APIKey`
- APIKey auth cache snapshot，版本号递增
- DTO 和用户侧 API 请求/响应类型

### Group

在 `groups` 表新增：

- `system_prompt text not null default ''`
- `system_prompt_mode varchar(20) not null default 'inherit'`

同步修改：

- ent schema
- migration
- repository create/update/select mapper
- service `Group`
- APIKey auth cache 中的 group snapshot
- admin group DTO 和 create/update 请求
- group 更新后继续失效关联 APIKey 认证缓存

### System Settings

沿用当前 key/value settings 模式，新增 8 个设置键：

- `system_prompt_anthropic`
- `system_prompt_mode_anthropic`
- `system_prompt_openai`
- `system_prompt_mode_openai`
- `system_prompt_gemini`
- `system_prompt_mode_gemini`
- `system_prompt_antigravity`
- `system_prompt_mode_antigravity`

平台级模式允许 `inherit`、`passthrough`、`override`、`append`。字段缺省或空值按 `inherit` 处理；未知枚举返回 400。

这些平台级配置必须进入请求热路径缓存，不允许每次转发都查询 settings 表。缓存策略见“缓存与失效”。

## API 契约

### 用户 APIKey 接口

变更端点：

- `POST /api/v1/api-keys`
- `PUT /api/v1/api-keys/:id`
- `GET /api/v1/api-keys`
- `GET /api/v1/api-keys/:id`

新增请求字段：

```json
{
  "system_prompt": "你是一个严格遵守企业规范的助手。",
  "system_prompt_mode": "append"
}
```

新增响应字段：

```json
{
  "system_prompt": "你是一个严格遵守企业规范的助手。",
  "system_prompt_mode": "append"
}
```

兼容性：新增可选字段，旧客户端不传时等价于 `inherit`。

### 管理员分组接口

变更端点：

- `POST /api/v1/admin/groups`
- `PUT /api/v1/admin/groups/:id`
- `GET /api/v1/admin/groups`
- `GET /api/v1/admin/groups/:id`
- `GET /api/v1/admin/groups/all`

新增请求字段：

```json
{
  "system_prompt": "本分组统一使用中文回答。",
  "system_prompt_mode": "passthrough"
}
```

新增响应字段同上。

兼容性：新增可选字段，旧管理端不传时等价于 `inherit`。

### 管理员系统设置接口

变更端点：

- `GET /api/v1/admin/settings`
- `PUT /api/v1/admin/settings`

新增字段：

```json
{
  "system_prompt_anthropic": "",
  "system_prompt_mode_anthropic": "inherit",
  "system_prompt_openai": "",
  "system_prompt_mode_openai": "inherit",
  "system_prompt_gemini": "",
  "system_prompt_mode_gemini": "inherit",
  "system_prompt_antigravity": "",
  "system_prompt_mode_antigravity": "inherit"
}
```

兼容性：新增可选字段，旧管理端不传时保留或写入默认 `inherit`。

## 解析流程

新增集中解析函数，例如：

```go
ResolveEffectiveSystemPrompt(ctx, apiKey, platform) EffectiveSystemPrompt
```

返回结构：

```go
type EffectiveSystemPrompt struct {
    Prompt string
    Mode   SystemPromptMode
    Source SystemPromptSource
}
```

解析规则：

1. 如果 APIKey `system_prompt_mode != "inherit"`，使用 APIKey 规则。
2. 否则如果分组 `system_prompt_mode != "inherit"`，使用分组规则。
3. 否则读取系统设置中的平台规则。
4. 如果最终模式是 `inherit`，不注入。
5. 如果某一层模式是 `passthrough`、`override`、`append` 但提示词为空，跳过该层并继续向下一层查找，同时记录调试日志。

系统中不提供“当前层显式阻断下层配置”的独立开关。需要不注入时，将对应层级保持为 `inherit`，并确保更低层级也未配置有效提示词。

平台来源：

- Anthropic `/v1/messages` 使用当前 APIKey 的有效分组平台；fallback group 生效后使用 fallback 后的分组。
- OpenAI Responses / Messages / WebSocket 使用 OpenAI 兼容分组解析出的目标平台。
- Gemini 和 Antigravity 使用对应账号或分组平台。

## 注入规则

统一 helper 负责根据请求格式注入，避免 handler 中散落逻辑。

### Anthropic Messages

目标字段：顶层 `system`。

- 无客户端 `system`：写入配置提示词。
- `passthrough` 且已有客户端 `system`：不修改。
- `override`：替换为配置提示词。
- `append`：配置提示词在前，原 `system` 在后。

兼容 `system` 为 string 或 content block array。追加时优先输出 array，保留原有 block。

### OpenAI Responses

目标字段：顶层 `instructions`。

- 无客户端 `instructions`：写入配置提示词。
- `passthrough` 且已有客户端 `instructions`：不修改。
- `override`：替换为配置提示词。
- `append`：`配置提示词 + "\n\n" + 原 instructions`。

如果请求只有 `input` 中的 `role: system`，在转换链路中也应保证配置提示词位于最终 `instructions` 或第一个 system item 前。

### Chat Completions

目标字段：`messages` 中的 `role: "system"`。

- 无 system message：在 `messages` 开头插入一条 system message。
- `passthrough` 且已有 system message：不修改。
- `override`：移除原 system message，插入配置 system message。
- `append`：在所有原消息前插入配置 system message，保留原 system message。

### Gemini / Antigravity

目标字段：`systemInstruction.parts[].text`。

- 无 `systemInstruction`：创建 `parts`，写入配置提示词。
- `passthrough` 且已有 `systemInstruction`：不修改。
- `override`：替换为配置提示词。
- `append`：配置提示词作为第一个 text part，原 parts 后移。

Antigravity 现有 identity patch 仍保留。业务系统提示词应作为通用系统提示词进入请求，身份补丁继续按现有逻辑处理；两者都存在时，身份补丁保持最前，业务提示词排在其后。

## 前端设计

### 系统设置

在管理后台设置页的网关相关配置区域增加“全局系统提示词”配置：

- 平台选择或四个平台独立 textarea
- 每个平台一个模式选择：不配置、透传、覆盖、追加
- 当选择透传/覆盖/追加时提示词必填

### 分组管理

在创建和编辑分组表单增加：

- 系统提示词模式：不配置、透传、覆盖、追加
- 分组系统提示词 textarea

默认“不配置”。选择透传/覆盖/追加时提示词必填。

### 用户 APIKey

在用户 APIKey 创建和编辑表单增加：

- 系统提示词模式：不配置、透传、覆盖、追加
- APIKey 系统提示词 textarea

默认“不配置”。选择透传/覆盖/追加时提示词必填。

## 缓存与失效

系统提示词配置必须缓存，避免请求转发热路径反复读取数据库。

### APIKey / Group 缓存

- APIKey 和 Group 的 `system_prompt`、`system_prompt_mode` 进入现有 APIKey auth cache snapshot。
- auth cache snapshot 版本号递增，旧缓存自动回源。
- APIKey 更新后继续调用 `InvalidateAuthCacheByKey`，确保该 APIKey 的提示词配置立即失效。
- Group 更新后继续调用 `InvalidateAuthCacheByGroupID`，确保分组下所有 APIKey 的分组提示词配置立即失效。
- 认证缓存 L1/L2 继续使用现有 key、TTL、Pub/Sub 失效机制，不新增独立 Redis key。

### 系统平台配置缓存

新增 `SettingService` 级别的系统提示词缓存，例如：

```go
type cachedSystemPromptSettings struct {
    byPlatform map[string]EffectiveSystemPrompt
    expiresAt  int64
}
```

缓存行为：

- 缓存内容包含四个平台的 `system_prompt_<platform>` 和 `system_prompt_mode_<platform>`。
- 使用进程内 `atomic.Value` 保存快照，读取路径无锁。
- 缓存 TTL 默认 60 秒，DB 读取错误时使用 5 秒短 TTL 的空配置快照。
- 缓存 miss 或过期时使用 `singleflight` 合并回源。
- 回源查询使用独立 DB timeout，避免请求取消导致缓存无法刷新。
- `UpdateSettings` 写入任一系统提示词字段或模式字段后，必须清理或刷新系统提示词缓存。
- 多实例环境允许最多一个 TTL 窗口的一致性延迟；如果后续已有 settings 级 Pub/Sub 失效能力，可复用它做跨实例即时失效。

解析函数只从缓存读取系统平台层规则：

```go
GetSystemPromptSettings(ctx) map[string]EffectiveSystemPrompt
```

APIKey 和 Group 层来自认证缓存快照；系统平台层来自 `SettingService` 缓存。最终解析不直接访问数据库。

## 错误处理与校验

- 不接受未知模式枚举。
- APIKey / Group 的 `inherit` 只表示当前层不配置，不会清空下层配置。
- `passthrough`、`override`、`append` 搭配空提示词时，写接口返回 400；系统设置保存时也返回 400。
- 转发注入失败时返回原始请求并记录 warn，不应导致业务请求失败，除非 JSON 本身已无法解析。

## 测试计划

### 后端单元测试

- 有效规则解析：APIKey 优先于分组，分组优先于系统设置。
- `inherit` 继续向下查找。
- 系统平台层为 `inherit` 时最终不注入。
- 空 prompt 搭配注入模式被拒绝。
- APIKey auth cache snapshot 能保留 APIKey 和 Group 的系统提示词字段。
- 系统平台提示词缓存命中时不重复读取 settings 表。
- 系统平台提示词缓存过期后通过 `singleflight` 合并回源。
- `UpdateSettings` 修改任一系统提示词字段或模式字段后清理系统提示词缓存。
- settings 回源失败时返回短 TTL 的空配置快照，热路径不因配置读取失败中断。

### 注入 helper 测试

- Anthropic：无 system、string system、array system。
- OpenAI Responses：无 instructions、有 instructions、空白 instructions。
- Chat Completions：无 system message、单个 system message、多个 system message。
- Gemini / Antigravity：无 systemInstruction、有 systemInstruction、有身份补丁。

### API 测试

- settings get/update 返回并保存 8 个新增字段。
- group create/update/list/get 返回新增字段。
- api key create/update/list/get 返回新增字段。
- 非法模式和空提示词返回 400。

### 前端验证

- SettingsView 保存 payload 包含平台提示词和模式。
- GroupsView 创建/编辑 payload 包含系统提示词字段。
- KeysView 创建/编辑 payload 包含系统提示词字段。
- TypeScript 类型检查通过。

## 兼容性与回滚

兼容性：

- 数据库新增字段有默认值，不影响旧记录。
- API 新增可选字段，旧客户端无需修改。
- 默认模式保证升级后请求不发生提示词注入。

回滚：

- 所有系统平台配置改回 `inherit`。
- 分组和 APIKey 配置改回 `inherit`。
- 如需代码回滚，新增字段留在数据库中不影响旧代码读取。

## 验收标准

- APIKey、分组、系统平台三层都能保存和读取系统提示词配置。
- 转发请求时按 `APIKey > 分组 > 系统平台配置` 生效。
- 四种模式行为符合本文定义。
- 默认升级后不改变既有请求。
- 后端测试、前端类型检查和关键构建命令通过。
