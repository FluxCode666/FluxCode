# 媒体 Adapter 自动解析设计

## 1. 文档定位

本文是《统一媒体路由与存储设计》的增量修订，专门收敛媒体 Adapter 的归属、解析、管理端展示和运行时冻结规则。

本文生效后，原设计中由管理员维护 `default_adapter`、`default_async_mode` 的内容不再适用；其余独立媒体接口、账号隔离、分组调度、请求参数映射、异步任务和媒体存储设计保持不变。

## 2. 背景与问题

当前媒体模型注册表把以下字段作为管理配置：

```text
default_adapter
default_async_mode
```

但 Adapter 实际上是代码中的协议实现。管理员即使填写了一个格式正确的 Adapter key，也不能凭配置产生对应实现；填错后只会让模型在调度阶段找不到 Adapter。生产组合根目前也尚未注册真实媒体 Adapter，因此“允许填写”会制造系统已经支持该模型的错觉。

Adapter 还同时承载认证、请求转换、同步生成、原生异步提交、轮询和内容获取等行为。这些能力必须与代码实现一致，不能由数据库声明。

因此需要把 Adapter 从业务配置改为代码派生信息：管理员只注册公共模型的业务元数据，系统根据规范模型的厂商和模型 ID 自动解析 Adapter，并只读展示解析结果。

## 3. 已确认决策

- Adapter 是代码实现细节，管理员不能在媒体模型、账号或分组界面中输入 Adapter key。
- Adapter 只根据“模型厂商 + 规范模型 ID”解析；账号服务商、账号平台、Base URL 和上游模型 ID 不参与本期解析。
- 精确模型规则优先，模型家族规则兜底。
- 请求别名必须先解析成规范模型，Adapter 不能根据别名直接匹配。
- 无匹配、多匹配、实现未注册或能力不兼容时，模型不能启用。
- `default_async_mode` 不再属于全局媒体模型。上游协议选择继续由账号模型绑定声明，并与 Adapter 的代码能力取交集。
- 任务创建时冻结候选集合及每个候选的 Adapter、上游模型和异步模式；Worker 选中账号后再冻结当前执行元组。后续模型配置或代码规则变化不能重解释已冻结候选。
- 特殊“服务商 + 模型”协议覆盖如果未来确有需要，只能通过新的代码级设计扩展，不开放数据库或 UI 配置。本期不实现该维度。

## 4. 方案比较

### 4.1 方案一：继续由管理员配置 Adapter

优点是表面灵活，不需要修改当前字段结构。缺点是配置无法产生代码实现，容易形成拼写错误、能力虚报和运行时失败，且管理员必须理解内部协议类名。

该方案不采用。

### 4.2 方案二：为每个规范模型维护精确代码映射

每个 `(vendor, canonical_model_id)` 都在代码中映射到一个 Adapter。规则最明确，但同一家族的新版本会重复登记；模型版本较多时维护成本高，也容易遗漏仅名称后缀不同但协议一致的模型。

该方案适合作为特殊模型覆盖机制，但不单独作为完整方案。

### 4.3 方案三：精确模型覆盖 + 厂商内家族规则兜底

先查精确 `(vendor, canonical_model_id)` 规则；不存在精确规则时，再匹配同一厂商下的模型家族。精确规则处理协议例外，家族规则复用稳定协议。任何阶段都不允许用配置字符串决定 Adapter。

该方案兼顾确定性和扩展成本，为本设计采用的方案。

## 5. 术语与边界

### 5.1 规范公共模型

媒体模型注册表中的唯一 `model_id`。所有下游别名先映射到该 ID，分组白名单、账号模型绑定、计费和 Adapter 解析都使用规范 ID。

### 5.2 模型厂商

媒体模型定义的 `vendor`，表示模型归属，例如 xAI、Google 或字节跳动。它是 Adapter 解析输入之一。

`vendor` 不是账号实际采购渠道。管理员修改已启用模型的 `vendor` 时，系统必须重新解析并验证 Adapter；解析失败则拒绝保存。

### 5.3 账号服务商

`media_config.provider` 表示账号实际服务商或渠道。它可供 Adapter 内部认证、日志和请求处理使用，但不参与本期 AdapterResolver 的匹配键。

### 5.4 上游模型 ID

账号模型绑定中的 `upstream_model_id`。它只决定该账号向上游发送的模型名，不能用于猜测 Adapter，也不能改变规范模型的协议归属。

### 5.5 Adapter key

代码内注册的稳定标识，用于查找 Adapter 实现并写入任务快照。它可以在管理端只读显示，但不是管理配置。

## 6. 总体解析链路

```text
下游请求 model 或系统别名
        │
        ▼
MediaModelRegistry
  别名 → 规范公共模型
  读取 vendor、model_id、operations
        │
        ▼
MediaAdapterResolver（代码内规则）
  1. 精确 vendor + canonical_model_id
  2. 同 vendor 下唯一 family matcher
        │
        ▼
MediaAdapterRegistry（代码内实现）
  校验 Adapter key 已注册
  校验同步/原生异步接口能力
  校验支持的媒体操作
        │
        ▼
Media Scheduler
  分组白名单
  账号公共模型绑定
  upstream_model_id
  账号上游异步模式
  request_mapping
        │
        ▼
不可变候选快照 / MediaTask
```

请求参数映射仍发生在统一请求校验之后、Adapter 调用之前。映射可以把 `size` 改名为某个上游字段，但不能覆盖 Adapter key、模型厂商、规范模型 ID 或账号身份。

## 7. 代码内组件

### 7.1 MediaAdapterResolver

Resolver 只接受已经规范化的：

```text
vendor
canonical_model_id
operations
```

Resolver 不读取数据库 Adapter 字段，不读取账号 `provider`，也不读取 `upstream_model_id`。

代码规则分为两类：

```text
exact rule:  vendor + canonical_model_id → adapter_key + model_capabilities
family rule: vendor + family_matcher     → adapter_key + model_capabilities
```

每条家族规则还包含稳定 `family_id`，用于日志、诊断和测试。`model_capabilities` 明确声明 `supported_operations`、`allows_sync_upstream` 和 `allows_native_async_upstream`。家族 matcher 和能力声明都是受版本控制的代码，不接受管理员提供的正则、脚本或表达式。

### 7.2 MediaAdapterRegistry

Registry 继续按稳定 Adapter key 保存实际实现。每个实现通过代码中的 `MediaAdapterRegistration` 注册，不从数据库加载：

```text
key
adapter implementation
supported_operations
exact rules
family rules
```

`supported_operations` 的来源就是该注册描述，不新增管理员配置或模型表字段。Registry 同时根据实际 Go 接口得到只读的实现能力上限：

```text
adapter_key
supported_operations
supports_sync_upstream
supports_native_async_upstream
supports_content_fetch
```

同步和原生异步能力必须与实现的 Go 接口一致：

- `supports_sync_upstream=true` 时必须实现 `MediaSyncGenerator`；
- `supports_native_async_upstream=true` 时必须同时实现 `MediaAsyncSubmitter` 和 `MediaAsyncPoller`；
- 声明内容回源能力时必须实现对应内容读取接口。

Resolver 的 exact/family 目录只从这些注册描述构建，不再维护第二份中心映射表，避免 Adapter 实现与匹配规则分离漂移。

规范 key 注册时必须与实现 `Name()` 规范化后一致。每条模型规则至少允许上游同步或原生异步中的一种，且规则允许的执行协议必须是实现接口能力的子集。注册阶段发现重复规范 key、规则引用未注册实现、规则操作超出注册描述、规则执行协议超出实现能力、规则没有任何执行路径、规范 key 与实现名称不一致，或声明能力与接口不一致时，实例启动失败，不能带着不完整 Adapter 表继续提供媒体请求。

### 7.3 解析结果

Resolver 返回不可变结果：

```text
adapter_key
matched_by          // exact 或 family
matched_family      // exact 时为空
capabilities
```

最终 `capabilities` 中的媒体操作、上游同步和原生异步能力，是匹配规则的模型能力与 Adapter 实际能力上限的交集；内容回源能力直接取 Adapter 实际接口能力。这样同一个实现即使服务多个家族，也不能因为实现了某个 Go 接口，就错误地宣称每个家族都支持该操作或上游协议。业务层只消费解析结果，不允许再被账号配置覆盖。

### 7.4 历史 key 别名

Adapter key 改名不能通过把同一实现重复注册为不同规范 key 处理。Registry 提供独立的代码级 `RegisterAlias(old_key, canonical_key)`：

- `canonical_key` 必须已经注册实际实现；
- alias 不能覆盖规范 key 或已有 alias；
- alias 不能指向另一个 alias，不允许链和环；
- alias 解析后继承规范 key 的同一实现与能力；
- Resolver 的新规则只能返回规范 key，只有历史任务可以继续携带旧 key。

这样规范 key 仍与实现 `Name()` 一致，而冻结旧 key 的任务仍可恢复。

## 8. 匹配算法与歧义处理

解析按以下固定顺序执行：

1. 对 `vendor` 和规范模型 ID 使用系统统一规范化函数。
2. 查询精确 `(vendor, canonical_model_id)` 映射。
3. 若精确命中，直接采用该结果，不再运行家族规则。
4. 若没有精确命中，只运行相同 `vendor` 下的家族 matcher。
5. 家族规则必须恰好命中一条。
6. 根据 AdapterRegistry 验证实现存在，并验证模型 `operations` 是规则模型能力与 Adapter 实现能力交集的子集。
7. 验证最终能力至少包含 `sync_upstream` 或 `native_async_upstream`；两者都为 `false` 时返回 `capability_mismatch`，不能标记为 `ready`。

结果处理如下：

| 情况 | 结果 |
| --- | --- |
| 一条精确规则命中 | 使用精确 Adapter |
| 无精确规则且一条家族规则命中 | 使用家族 Adapter |
| 无规则命中 | `unresolved`，启用失败 |
| 多条家族规则命中 | `ambiguous`，启用失败 |
| 规则指向未注册 key | `implementation_missing`，启用失败 |
| Adapter 不支持模型操作 | `capability_mismatch`，启用失败 |
| 最终没有同步或原生异步执行路径 | `capability_mismatch`，启用失败 |

重复精确键在代码注册阶段直接报错，因此不会以“后注册覆盖先注册”的方式运行。家族多匹配也不能依赖注册顺序取第一条。

## 9. 模型启用与运行时行为

### 9.1 保存规则

- 所有模型无论是否启用，都必须先通过基础校验，包括模型 ID、厂商、媒体类型、操作、约束、计费单位、别名格式和别名唯一性。
- `enabled=false`：允许保存尚未支持的模型业务元数据，便于先准备别名和约束；响应必须显示具体解析状态。当前分组白名单只接受已启用模型，因此分组配置必须在模型成功启用后完成。
- `enabled=true`：写入前必须得到唯一且能力兼容的 Adapter 解析结果，否则拒绝保存。
- `model_id` 继续保持不可修改；需要新 ID 时创建新模型。已启用模型修改 `vendor` 或 `operations` 时按新值重新解析；失败时整次更新不落库。
- 从禁用切换为启用：执行与新建启用模型相同的完整校验。

实现上必须拆分“模型基础校验”和“Adapter resolution 校验”。禁用只放宽 Adapter 必须 `ready` 的要求，不能绕过任何基础校验；任务中的冻结模型也只复用基础校验，最终 Adapter 始终校验候选快照的 `resolved_model.adapter`。

### 9.2 既有异常数据

升级前已经处于 `enabled=true`、但新代码无法解析的记录不能继续进入媒体候选池。实例刷新模型快照时对该模型故障关闭，记录结构化错误，并在管理 API 中返回非 `ready` 状态；不得回退使用旧 `default_adapter`。

每次刷新在数据库模型和别名读取成功后构造一份全新快照。单个模型基础非法、`unresolved`、`ambiguous` 或 `capability_mismatch` 时，该模型及所有指向它的别名从可路由索引剔除，并分别写入只读 unavailable tombstone 索引；其他模型和别名继续刷新。新快照必须替换旧快照，不能因为单模型错误而继续服务旧路由。

每个 tombstone 只保存规范模型 ID、别名到规范模型的关系、解析状态和稳定错误码。`Resolve`、`CanonicalModelID` 遇到 tombstone 时返回 typed unavailable error，使网关可以实现第 9.3 节的专用 503；tombstone 绝不能返回模型定义或进入 Scheduler 候选池。

数据库读取失败、别名读取失败、重复规范模型 ID、重复别名、别名与规范 ID 冲突，或 `implementation_missing` 等代码注册完整性错误属于全局错误，此时不发布半成品并保留上一份快照。实例首次启动时若发生全局错误则 readiness 失败；只有上一段列出的单模型错误可以带着其余有效模型启动。

异常记录仍可在管理端查看和禁用；基础非法记录必须由管理员修正，Adapter 非 `ready` 记录必须由开发者补充代码规则并完成部署，恢复前均不可调度。

### 9.3 请求阶段

所有 Adapter 解析错误必须发生在预扣费和上游请求之前。未启用模型继续返回现有“模型不存在/不可用”错误；数据库标记为启用但解析非 `ready` 的异常模型返回 `503 MEDIA_MODEL_ADAPTER_UNAVAILABLE`，便于区分客户端模型名错误和系统部署错误。若异常状态是在任务创建与 Worker 执行之间因部署差异暴露，任务按系统故障处理并全额释放预扣，不尝试猜测其他 Adapter。

## 10. 异步能力归属

### 10.1 删除全局默认异步模式

媒体模型注册表不再保存或编辑 `default_async_mode`。一个模型能否通过某个账号走原生异步协议，取决于以下两项的交集：

```text
Adapter 代码能力
∩
账号公共模型绑定的 async_mode
```

账号绑定继续使用：

```text
unsupported  // 该绑定走上游同步协议
native       // 该绑定走上游原生异步提交与轮询协议
```

新配置不提供 `optional`。旧账号配置在尚未保存迁移前仍可为新任务产生 `optional`，保持当前语义：Adapter 必须同时支持同步、提交和轮询；下游同步调用走上游同步协议，下游异步调用走上游原生异步协议。旧账号一旦通过当前编辑页保存，继续按既有迁移规则把非 `unsupported` 模式写成 v1 的 `native`。

已经冻结为 `optional` 的在途任务必须始终按上述旧语义执行，不能在读取时改写成 `native` 或 `unsupported`。新 UI 不允许直接创建 `optional`。

### 10.2 候选过滤

- 账号绑定为 `unsupported` 时，解析出的 Adapter 必须支持 `MediaSyncGenerator`。
- 账号绑定为 `native` 时，解析出的 Adapter 必须同时支持提交和轮询。
- 历史绑定为 `optional` 时，解析出的 Adapter 必须同时支持同步、提交和轮询。
- 不满足交集的账号从候选池排除，并记录可诊断原因。

### 10.3 与下游 sync/async 的关系

下游 `async` 仍是调用方式，不是 Adapter 配置：

| 下游调用 | 账号上游模式 | 执行方式 |
| --- | --- | --- |
| 同步 | `unsupported` | 同步调用上游并等待 |
| 同步 | `native` | 提交上游任务并轮询，在同步等待窗口内返回结果 |
| 异步 | `unsupported` | Worker 后台执行上游同步调用 |
| 异步 | `native` | Worker 后台提交并轮询 |
| 同步 | 历史 `optional` | 同步调用上游并等待 |
| 异步 | 历史 `optional` | Worker 后台提交并轮询 |

因此删除 `default_async_mode` 不会削弱“所有媒体模型均可由下游选择同步或异步调用”的能力。

## 11. 管理 API 契约

### 11.1 写入请求

媒体模型创建和更新的正式请求契约删除：

```text
default_adapter
default_async_mode
```

这两个字段不再具有写入语义。为允许全部新后端上线后继续服务尚未刷新页面的旧前端，首个兼容版本的解码 DTO 暂时保留同名可选字段作为废弃输入槽：服务层必须丢弃其值，不校验、不持久化，也不允许影响解析结果。除这两个明确的废弃槽外，管理 API 继续严格拒绝未知字段。

兼容版本部署到全部后端实例且新前端上线后，再在后续清理版本删除废弃输入槽；届时继续发送旧字段才返回字段未知错误。该兼容窗口不改变正式契约，也不能成为隐藏的 Adapter 配置入口。

管理员仍维护：

```text
model_id
vendor
media_type
operations
constraints
billing_unit
enabled
aliases
```

### 11.2 读取响应

列表、详情、创建和更新响应增加同一只读对象：

```json
{
  "adapter_resolution": {
    "status": "ready",
    "resolved_adapter": "bytedance-seedance",
    "matched_by": "family",
    "matched_family": "seedance",
    "capabilities": {
      "operations": ["text_to_video", "image_to_video"],
      "sync_upstream": true,
      "native_async_upstream": true,
      "content_fetch": true
    },
    "reason_code": ""
  }
}
```

`status` 只允许：

```text
ready
invalid_definition
unresolved
ambiguous
implementation_missing
capability_mismatch
```

非 `ready` 时，`resolved_adapter` 可以为空；`reason_code` 使用稳定机器码，前端负责本地化，不把内部堆栈或凭证写入响应。

各状态的字段规则固定为：

| status | resolved_adapter | match 字段 | capabilities | reason_code |
| --- | --- | --- | --- | --- |
| `ready` | 最终 key | 实际匹配来源 | 实际能力对象 | 空字符串 |
| `invalid_definition` | 空字符串 | 空字符串 | `null` | `MEDIA_MODEL_DEFINITION_INVALID` |
| `unresolved` | 空字符串 | 空字符串 | `null` | `MEDIA_ADAPTER_UNRESOLVED` |
| `ambiguous` | 空字符串 | 空字符串 | `null` | `MEDIA_ADAPTER_AMBIGUOUS` |
| `implementation_missing` | 规则指向的 key | 实际匹配来源 | `null` | `MEDIA_ADAPTER_IMPLEMENTATION_MISSING` |
| `capability_mismatch` | 已解析 key | 实际匹配来源 | 实际能力对象 | `MEDIA_ADAPTER_CAPABILITY_MISMATCH` |

API 不提供 Adapter 规则增删改接口，也不提供 Adapter 列表供管理员选择。

首个兼容版本可以同时返回标记为废弃的旧响应字段，供旧前端完成滚动过渡：`default_adapter` 只能取 `resolved_adapter`，`default_async_mode` 只能由最终解析能力（规则模型能力与实现能力的交集）按下表换算，二者都不能读取旧数据库列。

| 最终解析能力 | 兼容 `default_async_mode` |
| --- | --- |
| 仅上游同步 | `unsupported` |
| 仅上游原生异步 | `required` |
| 同时支持二者 | `optional` |
| 非 `ready` | `unsupported` |

新前端不得消费或回传这两个字段；后续清理版本与废弃输入槽一起删除。

## 12. 管理端交互

媒体模型编辑器删除 Adapter 输入框和默认异步模式选择框。列表与编辑详情增加只读“系统适配”区域，展示：

- 解析状态；
- 最终 Adapter key；
- 精确匹配或家族匹配；
- 上游同步、原生异步和内容获取能力；
- 非就绪状态的可操作提示。

新建模型尚未保存时显示“保存后由系统解析”。保存为禁用状态后展示服务端结果；本期不增加单独的解析预览 API。

非就绪模型的提示必须说明“需要部署对应代码适配”，不能引导管理员填写一个 Adapter 字符串。禁用模型可以保存；管理员尝试启用非就绪模型时，界面展示后端稳定错误码对应的说明。

账号媒体模型绑定继续配置：

- 规范公共模型；
- `upstream_model_id`；
- 上游原生异步模式；
- 动态请求参数映射。

账号界面可以只读显示该公共模型解析到的 Adapter，但不能覆盖它。Adapter 不支持原生异步时禁用 `native` 选项；不支持同步时禁用 `unsupported` 选项。后端仍执行最终能力校验，不能依赖前端约束。

账号模型选择器和分组媒体白名单只提供同时满足 `enabled=true` 且解析状态为 `ready` 的模型。升级前遗留的非就绪绑定或白名单不参与调度；管理端保留告警和删除入口，但不能新增同类绑定。

## 13. 数据库兼容策略

现有 `media_model_definitions.default_adapter` 和 `default_async_mode` 列本期暂时保留，避免破坏性迁移和生成代码的大范围联动，但从发布后不再作为业务真值：

- Resolver、Scheduler、Worker 和管理响应均不得读取它们决定新任务路由；
- 新建记录使用数据库默认值；
- 更新记录不根据解析结果回写这两个列；
- 旧值原样保留或保持默认值，不迁移、不解释、不回退；
- 后续在独立清理版本中删除列和生成代码。

账号 `media_config.version=1` 本身已经不保存 Adapter，继续作为新配置格式。旧账号配置中的顶层 `adapter` 字段仅保留历史读取兼容，不得覆盖新媒体注册表链路的 Resolver 结果。兼容转换即使暂时用旧 `adapter` 填充账号 `provider`，该值也只能作为服务商标签，不能变成 Adapter 解析输入。

历史任务上的 Adapter key 不是上述废弃配置列，必须继续保留并使用。

## 14. 两阶段任务冻结与 Adapter key 稳定性

第一阶段在任务创建时冻结候选集合。每个候选分别保存：

```text
canonical_model_id
candidate.account_id
candidate.adapter_key
candidate.upstream_model_id
candidate.native_async_mode
candidate.request_mapping snapshot
```

Worker 只能在该冻结候选集合中调度。提交前出现 Adapter 明确标记为可安全重试、且没有上游任务或未知提交风险时，可以排除当前候选并选择另一个冻结候选；此过程不能查询当前账号模型映射或重新运行 AdapterResolver。

第二阶段在 Worker 选中账号时把当前执行元组写入任务列：

```text
selected account_id
selected adapter_key
selected upstream_model_id
selected native_async_mode
```

安全的提交前跨账号重试可以原子替换当前执行元组。一旦取得上游任务 ID、提交结果处于未知状态，或进入必须固定账号的轮询/恢复阶段，执行元组不得再切换。后续提交恢复、轮询和内容回源只使用任务列中的已选 Adapter key，不重新按当前模型规则解析。

这样既保留现有提交前安全重试，也保证管理员修改模型元数据或新版本代码调整家族规则时，不会改变任务可用候选和已绑定的上游协议。

当前持久化名称分别是候选快照中的 `resolved_model.adapter` 和任务表中的 `adapter`；本文的 `adapter_key` 是对两者的统一概念称呼，不要求本期重命名数据库字段。

旧候选快照是顶层 JSON 数组，包含 `MediaModelDefinition.DefaultAdapter`、`DefaultAsyncMode` 等 Go 字段名，并使用严格 JSON 解码。本期保持现有 v1 wire format，不引入 v2 envelope：

- 新旧快照继续写顶层候选数组，所有读取和持久化输入重写点继续使用同一 v1 codec；
- v1 兼容 DTO 保留旧字段的准确 JSON 名称和类型，不能直接删除或改名；
- 新任务写快照时，兼容 `DefaultAdapter` 使用系统解析出的最终 key，兼容 `DefaultAsyncMode` 使用第 11.2 节的代码能力换算值，保证旧 Worker 的严格基础校验可通过；
- 新代码读取 v1 时只对业务模型字段执行基础校验，忽略两个兼容字段的路由语义，最终 Adapter 只信 `resolved_model.adapter`；
- 候选快照格式升级和旧字段物理删除需要独立设计及两阶段 reader/writer 发布，不属于本期。

Adapter key 是持久化协议标识，不能因为 Go 类型重命名而随意修改。确需更名时，新版本必须通过第 7.4 节的 `RegisterAlias` 保留旧 key；只有数据库扫描确认不存在非终态、待恢复或仍需上游内容回源的记录引用旧 key，并且旧版本实例已经全部退出后，才可删除旧 alias。

## 15. 多实例发布顺序

Resolver 和 AdapterRegistry 是每个实例本地代码。旧后端仍把数据库旧列当作调度真值，因此本迁移的混合版本窗口采用“只读预检 + 冻结媒体模型写入”，不声称废弃 API 输入槽可以解决后端混跑。

推荐顺序：

1. 待发布版本提供只读 preflight，逐个输出 `model_id`、解析状态、规范 Adapter key、旧 `default_adapter`、key 是否一致以及旧 `default_async_mode` 是否能被旧版本读取；preflight 不修改数据库。
2. 对非 `ready` 模型先在旧系统中禁用；对 key 不一致的模型，优先让新 Resolver 沿用现有稳定 key，否则改用停止旧实例后的受控切换。
3. 在运维层冻结媒体模型的创建、更新、删除和启停；账号绑定与分组白名单也不新增模型。本期不为这次性发布冻结增加业务设置开关。媒体请求只有在 preflight 全部通过且既有 Adapter 行为向后兼容时才可继续。
4. 滚动部署全部后端与 Worker。新旧实例都读写同一 v1 候选快照，且对现有启用模型解析到同一 Adapter key。
5. 确认所有实例启动、Adapter 注册检查和旧 key alias 检查通过后，解除管理写入冻结。
6. 部署删除编辑项、只消费 `adapter_resolution` 的新前端。
7. 再创建或启用新媒体模型，并配置账号公共模型绑定和分组白名单。

如果无法完成 preflight、无法冻结写入，或本次必须改变既有模型的 Adapter 行为，则不能滚动混跑，必须采用停止旧 API/Worker 后再启动新版本的受控切换。不得先启用模型再等待部分实例补齐 Adapter，也不能依赖数据库旧字段让不同版本实例临时选择不同实现。

## 16. 错误与可观测性

结构化日志至少包含：

```text
canonical_model_id
vendor
adapter_resolution_status
adapter_key（已解析时）
matched_by
matched_family（家族匹配时）
account_id（进入候选过滤后）
native_async_mode（进入候选过滤后）
```

禁止记录 API Key、Authorization、完整上游响应体和请求中的敏感媒体内容。

需要区分以下指标：

- 模型解析失败次数，按状态和规范模型统计；
- 因 Adapter 协议能力与账号异步模式不匹配而排除的候选数；
- Worker 使用冻结旧 Adapter key 的任务数；
- 规则解析成功但实现未注册的实例错误数。

管理员保存时的解析错误返回 4xx；代码注册表不完整导致的启动检查失败属于部署错误；运行中任务因实例能力缺失失败属于系统错误并执行系统故障退款路径。`implementation_missing` 保留为防御性诊断状态，正常完成启动校验的实例不应出现该状态。

## 17. 测试策略

### 17.1 Resolver 单元测试

- 厂商和规范模型 ID 规范化；
- 精确规则优先于家族规则；
- 单一家族匹配成功；
- 无匹配返回 `unresolved`；
- 多家族匹配返回 `ambiguous`；
- 重复精确键在注册阶段失败；
- 规则引用未注册 Adapter 返回 `implementation_missing`；
- 模型操作超出 Adapter 能力返回 `capability_mismatch`；
- 最终同步和原生异步能力都为 `false` 时返回 `capability_mismatch`；
- 注册描述是 `supported_operations` 的唯一代码来源，且声明不能超出实现能力；
- 历史 key alias 禁止覆盖、链和环，并解析到规范 key 的同一实现；
- 账号 provider、Base URL 和 upstream model 的变化不影响解析结果。

### 17.2 管理 API 测试

- 正式写入契约不再包含 `default_adapter`、`default_async_mode`，兼容输入槽即使收到不同值也不影响持久化和解析；
- 禁用模型允许保存非就绪状态；
- 禁用模型仍执行全部基础字段和别名校验；
- 启用非就绪模型时原子拒绝且数据库不变；
- 所有读取响应包含一致的只读解析对象；
- 非就绪响应只暴露稳定错误码；
- 单模型失效会发布剔除该模型及其别名的新快照，不会继续沿用旧路由或阻断其他模型；
- unavailable tombstone 对规范模型和别名返回 typed unavailable error，但不能进入 Scheduler；
- 旧数据库列中的任意值不会改变响应解析结果。

### 17.3 调度与 Worker 测试

- 下游别名先解析到规范模型再选择 Adapter；
- 同一规范模型的多个媒体账号复用当前账号级调度逻辑；
- 账号只能覆盖 upstream model、上游异步模式和请求映射；
- 同步/原生异步能力与账号绑定取交集；
- 不兼容候选被排除，其他兼容账号仍可调度；
- 候选快照冻结每个候选的路由元组，安全的提交前重试只能在冻结集合内切换；
- 执行元组绑定后，产生上游任务或进入恢复边界的任务不能再切换账号或 Adapter；
- 在任务创建后修改 Resolver 规则，旧任务仍只使用快照中的 key；
- 旧 key alias 可恢复历史非终态任务；
- 旧任务和新任务的 v1 顶层数组快照都能被严格解码，兼容默认字段不参与新代码路由；
- 历史 `optional` 账号和任务保持“下游同步走上游同步、下游异步走上游原生异步”的语义；
- 发布 preflight 能发现非就绪模型和新旧 Adapter key 不一致。

### 17.4 前端测试

- 创建和编辑表单不存在 Adapter 与默认异步模式输入；
- 提交载荷不包含废弃字段；
- 列表和详情正确展示解析状态与能力；
- 非就绪模型的启用失败提示明确指向代码适配；
- 账号模型绑定仍能配置上游模型 ID、原生异步和请求映射，但不能编辑 Adapter。

## 18. 非目标

- 本设计不实现 Grok、Seedance、Nano Banana、Veo、Z-Image、Agens Video 或其他真实 Adapter。
- 本设计不改变独立图片/视频下游接口及其请求格式。
- 本设计不重构 Chat、Responses、Anthropic Messages 或 Gemini 文本路由。
- 本设计不增加管理员可编辑的 Adapter 规则、脚本、正则或插件上传能力。
- 本设计不增加“账号服务商 + 模型”的运行时配置覆盖。
- 本设计不删除旧数据库列，也不迁移历史任务。
- 本设计不升级候选快照 wire format；格式版本化另行设计。
- 本设计不改变媒体计费、存储或同步超时退款策略。

## 19. 验收标准

1. 管理员在任何新媒体配置界面都不能输入 Adapter key 或全局默认异步模式。
2. 任意下游别名均先得到规范模型，再由规范模型的 `vendor + model_id` 唯一解析 Adapter。
3. 精确规则覆盖家族规则；家族无匹配或多匹配均故障关闭。
4. 非 `ready` 模型可以禁用保存，但不能启用或进入调度候选池。
5. 管理 API 和 UI 能只读展示最终 Adapter、匹配来源、能力与稳定状态码。
6. 账号仍可配置公共模型到上游模型 ID 的映射、上游原生异步模式和动态请求参数映射。
7. 下游继续可对所有已就绪媒体模型选择同步或异步调用。
8. 旧请求字段、兼容响应字段和数据库中的 `default_adapter`、`default_async_mode` 都不能影响新任务路由。
9. 在途任务只能使用创建时冻结候选中的 Adapter key；执行元组越过提交边界后保持固定，规则变化不会重解释历史任务。
10. 新模型家族只有在代码实现、规则和测试部署完成后才能启用。
11. 现有 v1 候选快照格式保持可读写，旧任务不会因移除配置语义而无法恢复。
12. 多实例升级在解除写入冻结前完成 preflight，并确认全部 API 与 Worker 实例使用同一规则集。

## 20. 对原设计的替代关系

本文替代原《统一媒体路由与存储设计》中以下语义：

- 第 4.2 节的 `vendor + public_model_id → adapter` 从数据库配置改为代码内 Resolver；
- 第 5 节 Model Registry 的“获取默认 Adapter”改为“取得规范模型后调用 Resolver”；
- 第 7.1 节删除 `default_adapter`、`default_async_mode` 业务字段；
- 第 8 节 Route Target 的 Adapter 来源改为 Resolver，不允许账号覆盖；
- 第 10.3 节的上游能力由 Adapter 代码能力与账号绑定共同决定；
- 第 14.1 节删除管理员维护默认 Adapter 和默认异步能力；
- 第 15.2 节补充唯一解析、歧义、代码能力和任务快照测试。

其他章节继续有效。
