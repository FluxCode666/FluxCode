# 统一媒体路由与存储设计

## 1. 背景

FluxCode 已具备图片/视频媒体任务基础设施，包括统一任务状态机、Redis Worker、租约与恢复扫描、媒体 Adapter 小接口、计费结算和安全内容交付。本设计在此基础上重新定义媒体账号、模型路由、下游模型映射、上游参数映射和媒体文件存储。

本期的媒体接口是独立调用域，不附着在 Chat、Responses、Anthropic Messages 或 Gemini 文本接口中。现有文本 Gateway、文本调度器和文本账号行为保持不变。

## 2. 目标

- 通过独立图片/视频接口承载所有已注册的媒体模型。
- 按“下游请求模型 ID + 分组 + 媒体能力”选择媒体账号。
- 媒体账号使用现有 Account 实体，但通过 `platform=media` 与文本账号完全隔离。
- 一个媒体账号可以配置多个图片和视频模型。
- 同一个公共模型可以绑定多个媒体账号，并复用账号级优先级、负载、并发、冷却和粘性策略。
- Adapter 默认按“模型厂商 + 模型”解析，不把服务商/渠道加入 Adapter 主键。
- 支持系统级下游模型别名和账号级下游模型到上游模型映射。
- 支持账号级声明式请求参数映射，例如 `size → chicun`。
- 下游通过 `async=true` 选择异步调用；同步和异步共用同一任务内核。
- 新媒体数据默认写入本地路径，可选 MinIO；历史 DB/Qiniu 数据不迁移、不改写。
- Local 和 MinIO 均通过 FluxCode 签名代理向下游交付，不暴露本地路径、Bucket、对象 Key 或凭证。

## 3. 非目标

- 本期不重构 Chat、Responses、Anthropic Messages、Gemini 的文本调度器。
- 本期不把媒体账号加入任何文本候选池。
- 本期不增加模型绑定级优先级或权重；媒体模型绑定继承账号级调度参数。
- 本期不按“服务商 + 厂商 + 模型”维护 Adapter 规则。
- 本期不执行历史图片/视频的批量搬迁、重编码或 DB/Qiniu 清理。
- 本期不提供异步任务取消接口。
- 本期不允许请求参数映射执行脚本或任意表达式。

## 4. 术语和边界

### 4.1 Account 与服务商

现有 `Account` 仍是调度的最终选择单位。媒体账号使用：

```text
account.platform = media
```

账号中的 `provider` 只表示实际服务商或渠道，例如 `xai`、`volcengine`、`google` 或 `third-party-relay`，用于管理、日志和展示，不参与本期 Adapter 主键解析。

### 4.2 公共模型、上游模型和 Adapter

- 公共模型 ID：下游调用的统一模型 ID，例如 `seedance-1.5-pro`。
- 规范模型 ID：系统注册表中的唯一模型定义 ID；请求别名先解析到规范模型 ID。
- 上游模型 ID：某个媒体账号实际发送给上游的模型名，例如 `doubao-seedance-1-5-pro`。
- Adapter：负责协议、认证、请求转换、同步/异步提交、状态查询和结果下载的实现。

默认解析规则：

```text
vendor + public_model_id → adapter
```

服务商不参与默认 Adapter 选择。未来如果出现真正的渠道协议差异，可以增加显式 Adapter 覆盖，但不在本期实现渠道维度的规则。

## 5. 总体架构

```text
/v1/images/generations
/v1/images/edits
/v1/images/{task_id}
/v1/images/{task_id}/content
/v1/videos
/v1/videos/{task_id}
/v1/videos/{task_id}/content
        │
        ▼
Media Gateway
  - 鉴权
  - 解析 API Key、分组和请求模型
  - 解析图片/视频能力
  - 解析 async=true/false
        │
        ▼
Model Registry
  - 请求别名 → 规范模型 ID
  - 校验模型能力
  - 获取模型厂商和默认 Adapter
        │
        ▼
Media Scheduler
  - 只查询 platform=media 账号
  - 校验分组媒体模型白名单
  - 校验账号模型绑定
  - 复用账号级调度选择器
        │
        ▼
Media Route Target
  - Account
  - upstream_model_id
  - vendor
  - adapter
  - request_mapping
  - upstream async mode
        │
        ▼
Request Mapping Pipeline
        │
        ▼
Media Adapter
  - 同步生成
  - 原生异步提交
  - 状态查询
  - 内容下载
        │
        ▼
MediaTask / Worker / Artifact Store / Billing
```

文本 Gateway 不调用 Media Scheduler。文本调度器的候选平台使用显式白名单，不能使用“排除若干平台后全部放行”的反向逻辑，从而保证 `platform=media` 永不进入文本链路。

## 6. API 契约

### 6.1 图片

```text
POST /v1/images/generations
POST /v1/images/edits
GET  /v1/images/{task_id}
GET  /v1/images/{task_id}/content
```

`/v1/images/{task_id}/content` 用于本地或 MinIO 产物的签名内容交付；多图任务通过签名参数选择产物位置。

### 6.2 视频

```text
POST /v1/videos
GET  /v1/videos/{task_id}
GET  /v1/videos/{task_id}/content
```

### 6.3 模型路由

所有媒体创建请求必须包含下游模型 ID。Handler 先查询媒体模型注册表：

- 已注册模型：进入统一 Media Gateway 和 Media Scheduler；
- 未注册模型：若仍存在旧图片兼容链路，则由兼容链路处理，并记录迁移日志；
- 新增模型必须先注册，不能通过任意请求值绕过模型能力和 Adapter 校验。

现有 `gpt-image-*` 等媒体模型可以注册到新的媒体模型注册表。注册后，新请求进入统一媒体任务内核并使用新的媒体存储策略；历史图片记录继续走旧读取链路。

### 6.4 同步和异步参数

`async` 是 FluxCode 控制字段：

- 未传或 `false`：同步等待；
- `true`：立即返回任务 ID。

只有 Adapter 明确声明需要转换时，才把异步语义转换为上游参数；不能盲目透传 `async`。

## 7. 数据模型

### 7.1 MediaModelDefinition

全局媒体模型注册表保存：

```text
public_model_id       // 唯一规范模型 ID
vendor                // 模型厂商
capabilities          // image、video 或 both
default_adapter       // vendor + model 对应 Adapter
default_async_mode    // unsupported、native、emulated
enabled
```

### 7.2 MediaModelAlias

系统级别的下游请求别名映射：

```text
requested_model_id → public_model_id
```

别名全局生效，不支持分组级别别名。所有分组都使用统一的公共模型语义。

### 7.3 Account.media_config

媒体账号继续复用现有 Account，并在 `extra.media_config` 中保存版本化配置：

```json
{
  "version": 1,
  "provider": "volcengine",
  "models": {
    "seedance-1.5-pro": {
      "enabled": true,
      "upstream_model_id": "doubao-seedance-1-5-pro",
      "async_mode": "native",
      "request_mapping": {
        "rules": [
          {
            "source": "size",
            "target": "chicun",
            "operation": "rename"
          },
          {
            "source": "quality",
            "target": "quality",
            "operation": "enum",
            "values": {
              "standard": "basic",
              "hd": "high"
            }
          }
        ]
      }
    }
  }
}
```

一个账号可以同时配置多个图片和视频模型。账号模型绑定本期不增加独立优先级或权重，继承 Account 的调度参数。

### 7.4 Group Media Model Scope

分组增加媒体模型白名单，逻辑关系为：

```text
group_media_model_scopes
  - group_id
  - public_model_id
  - enabled
```

分组不定义模型别名、不覆盖模型厂商、不覆盖 Adapter。

### 7.5 MediaTask 快照

任务保存以下路由快照：

```text
public_model_id
upstream_model_id
vendor
adapter
account_id
request_mapping
resolved_request
downstream_async
upstream_async_mode
pricing_snapshot
```

账号映射、模型配置和价格修改不会改变已创建任务。

### 7.6 MediaArtifact 存储字段

媒体产物至少保存：

```text
task_id
direction
position
media_type
content_type
size_bytes
checksum_sha256
storage_provider     // local 或 minio
object_key
public_url           // 仅保存经过校验的交付地址
upstream_reference   // 敏感字段
expires_at
```

数据库只保存任务和文件索引，不保存媒体大文件。

历史 DB/Qiniu 记录继续由原实体和原读取链路处理，不被强制转换为新的 `MediaArtifact` 存储记录。

## 8. 媒体调度

### 8.1 RouteRequest

调度器的核心输入为：

```text
group_id
public_model_id
capability       // image 或 video
request_mode     // sync 或 async
session_hash
```

### 8.2 候选过滤

候选账号必须满足：

```text
account.platform = media
账号状态可用
账号允许调度
账号属于当前分组
分组允许 public_model_id
账号 media_config 启用了 public_model_id
模型能力匹配当前图片/视频接口
账号未被排除、冷却或并发耗尽
```

### 8.3 账号选择

复用现有账号级选择策略：

- 优先级；
- 实时负载与并发槽位；
- RPM、窗口额度和冷却；
- 粘性会话；
- 最近使用；
- 失败排除和安全回退。

同一公共模型的多个媒体账号进入同一候选池，但每个候选保留自己的 `upstream_model_id` 和参数映射。

上游任务 ID 一旦取得，后续查询和下载固定使用原账号。只有提交阶段明确失败且 Adapter 支持安全重试时，才允许切换账号。

## 9. 请求参数映射

参数映射发生在统一请求校验之后、Adapter 调用之前。Adapter 接收已经按账号配置转换过的上游请求。

第一期支持以下声明式操作：

- `rename`：字段重命名；
- `copy`：复制字段到上游路径；
- `default`：源字段缺失时填充默认值；
- `enum`：枚举值映射；
- `cast`：`string`、`integer`、`number`、`boolean` 基础类型转换。

规则使用 JSON 路径，支持嵌套字段。默认行为：

- `rename` 成功后不再发送源字段；
- 未配置映射的标准字段交给 Adapter；
- 上游专属字段必须显式配置；
- 目标字段冲突时保存配置失败，运行时再次校验；
- 源字段缺失且无默认值时按字段必选性处理；
- 不执行脚本、模板或任意表达式。

管理端提供转换预览，展示统一下游请求和转换后的上游请求。

## 10. 同步与异步执行

同步和异步使用同一个 `MediaTask` 状态机。

### 10.1 下游异步

```text
创建任务
→ 预扣
→ 持久化任务和输入引用
→ 投递 Redis
→ 立即返回 task_id
```

### 10.2 下游同步

```text
创建任务
→ 高优先级执行
→ 在系统等待时长内等待完成
```

同步等待超时：

- 自动转异步开关关闭：停止本地后续动作，按超时退款策略结算；
- 自动转异步开关开启：任务继续进入异步执行并返回 task_id；
- 转换不重复预扣、不重复创建上游任务；
- 转换后的任务不可取消。

### 10.3 上游能力矩阵

| 下游调用 | 上游能力 | 执行方式 |
| --- | --- | --- |
| 同步 | 同步 | 同步等待 |
| 同步 | 原生异步 | 提交并轮询，直到同步等待结束 |
| 异步 | 同步 | Worker 后台等待同步响应 |
| 异步 | 原生异步 | Worker 后台提交并轮询 |

异步任务创建后不提供取消接口。

## 11. 错误、重试和计费

### 11.1 请求前错误

模型不存在、分组未授权、能力不匹配、没有候选账号、参数映射无效或 Adapter 缺失，都在预扣前返回，不产生扣费。

### 11.2 提交阶段

- 上游明确拒绝且没有任务 ID：按 Adapter 的可重试标记排除账号并选择下一个候选；
- 重试耗尽：任务失败并全额退款；
- 提交超时且无法确认是否创建任务：不盲目重提，进入恢复处理；
- 已取得任务 ID 后固定原账号。

### 11.3 执行和交付阶段

上游生成失败、Worker 系统错误或存储失败进入失败状态，并按系统设置执行退款或比例结算。结算状态与执行状态分离，防止重复退款或重复扣费。

任务创建时保存价格快照，后续模型价格变化不影响在途任务。

## 12. 媒体存储

### 12.1 后端选择

```text
media_storage_provider = local | minio
默认值：local
```

Local 和 MinIO 实现同一个 `MediaArtifactObjectStore`：

```text
Put
Open（支持 Range）
Discard
```

没有启用 MinIO 时使用本地文件；启用 MinIO 后新输入和新输出都写入 MinIO。

### 12.2 Local

默认路径：

```text
Docker：/app/.fluxcode/generated
非 Docker：./data/generated
```

可以通过系统配置或环境变量覆盖：

```text
MEDIA_LOCAL_STORAGE_PATH=/absolute/path/to/generated
```

Local 实现要求：

- 使用系统生成的随机对象 Key；
- 防止路径穿越；
- 临时文件写入后原子重命名；
- 校验 MIME、大小和 SHA-256；
- 支持 Range 读取；
- 删除幂等；
- 启动时检查目录可读写。

Dockerfile 和 entrypoint 创建目录并修复非 root 运行用户权限；Docker Compose 为 `/app/.fluxcode/generated` 增加持久化 volume 或 bind mount。

### 12.3 MinIO

MinIO 使用独立于数据库备份的私有 Bucket。系统设置页配置：

```text
Endpoint
Bucket
Access Key
Secret Key
Region
Use SSL
Path Style
Object Prefix
```

凭证加密保存并脱敏返回。启用后，启动或保存配置时执行 Endpoint、Bucket 和读写测试。

MinIO 上传失败不自动回退本地；任务按存储失败规则结束并结算。只有管理员把后端切换为 Local 后，后续新数据才写入本地。

### 12.4 存储快照和历史数据

每个新 `MediaArtifact` 保存 `storage_provider`。切换 Local/MinIO 后，历史新媒体产物仍按写入时的后端读取。

改造上线前已经存在的 DB/Qiniu 数据不迁移、不重写、不删除，继续使用原有读取链路。

### 12.5 签名代理

图片 URL 和视频内容统一通过 FluxCode 签名代理交付：

```text
Local/MinIO
  → FluxCode 签名代理
  → 下游
```

代理负责签名过期、任务/用户归属、内容类型、长度、Range、大小限制和访问审计。下游不看到本地绝对路径、MinIO Endpoint、Bucket 或对象 Key。

## 13. 多实例部署

应用无法可靠推断多实例是否共享文件系统，因此由部署者手动控制拓扑：

```text
单实例：Local 或 MinIO
多实例 + 独立磁盘：MinIO
多实例 + 共享 NFS/PVC：Local 可用，但必须挂载同一目录
```

管理端选择 Local 时显示风险提示。健康检查只验证当前实例，不声称跨实例共享验证通过。

## 14. 管理端配置

### 14.1 全局媒体模型

管理端维护公共模型 ID、厂商、图片/视频能力、默认 Adapter、默认异步能力和启用状态，并维护系统级请求模型别名。

### 14.2 媒体账号

账号编辑页选择 `Media` 平台，配置服务商名称、Base URL、凭证和多个模型绑定。每个模型绑定配置上游模型 ID、原生异步能力和请求参数映射。

### 14.3 分组

分组独立维护媒体模型白名单和媒体账号绑定。文本模型权限与媒体模型权限分离。

### 14.4 存储

系统设置页提供：

- Local/MinIO 后端选择；
- Local 路径；
- MinIO 连接配置；
- MinIO 连接测试；
- 存储健康状态；
- Local 多实例风险提示。

## 15. 测试策略

### 15.1 路由隔离

- 媒体账号不会被 Chat、Responses、Anthropic Messages、Gemini 选中；
- 媒体请求不会进入文本 Gateway；
- 文本请求回归行为不变；
- 分组媒体权限不影响文本权限。

### 15.2 模型与 Adapter

- 模型 ID、别名和能力校验；
- `vendor + model` Adapter 解析；
- 多账号同模型进入同一候选池；
- 未注册模型不能绕过 Registry。

### 15.3 参数映射

- 字段重命名、复制、默认值、枚举映射和类型转换；
- 嵌套路径；
- 缺失字段、非法路径、转换失败和目标冲突；
- 脚本和任意表达式拒绝。

### 15.4 调度和任务

- 分组模型白名单和账号模型绑定；
- 图片/视频能力过滤；
- 账号优先级、负载、并发、冷却、粘性和失败切换；
- 获取上游任务 ID 后固定原账号；
- 同步/异步四种组合；
- 超时转异步开关和结算规则；
- Redis 重复消息、租约、恢复扫描和 CAS。

### 15.5 存储

- Local Put/Open/Range/Discard；
- MinIO S3-compatible 集成测试；
- MIME、大小、哈希和路径安全；
- MinIO 配置失败不回退 Local；
- 存储后端切换后按 `storage_provider` 读取历史新数据；
- 签名代理过期、归属、Range 和审计；
- 历史 DB/Qiniu 数据读取回归；
- Docker 权限、volume 持久化和非 Docker 默认路径。

## 16. 分阶段交付

1. 增加媒体平台隔离、全局媒体模型/别名注册和分组媒体白名单。
2. 完成账号媒体模型配置、上游模型映射和声明式请求参数映射。
3. 将媒体入口接入统一 Media Scheduler；注册现有和首批媒体模型。
4. 接入 Local 存储、签名代理和图片/视频内容交付。
5. 接入 MinIO、系统设置、连接测试和 Docker 持久化挂载。
6. 接入真实 Adapter，例如 Grok、Seedance、Nano Banana、Veo、Z-Image 和其他模型。
7. 完成文本链路隔离回归、媒体同步/异步矩阵和多实例部署验收。

## 17. 验收标准

- Chat、Responses、Anthropic Messages、Gemini 路由和调度行为不变。
- 图片和视频使用独立媒体入口、统一模型注册和媒体调度。
- `platform=media` 账号永不进入文本候选池。
- 下游使用统一模型 ID；同一模型可在多个媒体账号之间调度。
- 账号可以把统一请求参数声明式映射为上游字段和枚举值。
- 下游同步/异步和上游同步/原生异步组合均可执行。
- 新媒体数据默认写入 Local，可选 MinIO；MinIO 故障不静默回退 Local。
- Local/MinIO 产物通过 FluxCode 签名代理交付。
- 历史 DB/Qiniu 数据不迁移且继续可读。
- 单实例和多实例部署的存储拓扑约束在管理端和部署文档中明确可见。
