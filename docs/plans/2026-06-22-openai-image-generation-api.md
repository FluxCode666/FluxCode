# FluxCode 生图接口接入文档

更新时间：2026-06-23

本文说明如何通过 FluxCode 的 OpenAI 兼容 Images API 接入生图能力，并提供可直接复制的 `curl` 示例。调用方只需要使用 FluxCode 平台生成的 API Key，不需要把 OpenAI 上游 Key 暴露给客户端。

## 1. 接入方式选择

FluxCode 当前提供 OpenAI 兼容代理接口：

| 方式 | 适用场景 | FluxCode 端点 |
| --- | --- | --- |
| Images API | 单次文生图、改图、需要直接指定 GPT Image 模型 | `POST /v1/images/generations`、`POST /v1/images/edits` |
| Images API 别名 | 不带 `/v1` 前缀的兼容路径 | `POST /images/generations`、`POST /images/edits` |

推荐策略：

- 调用方优先使用 `/v1/images/generations`，方便直接复用 OpenAI SDK 或 OpenAI 兼容客户端。
- 只需要“输入 prompt，返回图片”：使用 `POST /v1/images/generations`。
- 需要图生图或带 mask 的编辑：使用 `POST /v1/images/edits`。
- 4K 图片请显式传 `size: "3840x2160"` 或 `size: "2160x3840"`，系统会将该请求记为 `4K` 图片计费层级。

官方参考：

- [Image generation guide](https://developers.openai.com/api/docs/guides/image-generation)
- [Create image API reference](https://developers.openai.com/api/reference/resources/images/methods/generate)
- [Create response API reference](https://developers.openai.com/api/reference/resources/responses/methods/create)

## 2. 鉴权

所有请求都使用 FluxCode 平台 API Key：

```bash
export FLUXCODE_BASE_URL="https://your-fluxcode.example.com"
export FLUXCODE_API_KEY="sk-..."
```

请求头：

```http
Authorization: Bearer $FLUXCODE_API_KEY
Content-Type: application/json
```

若请求返回 `Images API is not supported for this platform`，说明该 API Key 所属分组不是 OpenAI 兼容渠道，或分组未配置可用的 OpenAI 图片上游账号。

## 3. FluxCode Images API：文生图

### 3.1 请求

```http
POST {FLUXCODE_BASE_URL}/v1/images/generations
Content-Type: application/json
Authorization: Bearer $FLUXCODE_API_KEY
```

常用 JSON 字段：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 建议填 | 推荐 `gpt-image-2`。系统要求图片接口使用 `gpt-image-*` 模型。 |
| `prompt` | string | 是 | 图片描述。GPT Image 模型的 prompt 最大长度为 `32000` 字符。 |
| `n` | number | 否 | 生成图片数量，`1-10`。默认通常为 `1`。 |
| `size` | string | 否 | 推荐使用下方 1K、2K、4K 分辨率；`gpt-image-2` 还支持满足约束的自定义尺寸。 |
| `response_format` | string | 否 | `b64_json` 或 `url`。API Key 上游账号链路会透传上游返回；OAuth/ChatGPT Web 链路会按该字段构造响应。 |
| `quality` | string | 否 | `auto`、`low`、`medium`、`high`。草稿可用 `low`，最终图可用 `medium` 或 `high`。 |
| `output_format` | string | 否 | `png`、`jpeg`、`webp`。默认 `png`。低延迟场景可优先 `jpeg`。 |
| `response_format` | string | 否 | `b64_json` 或 `url`。默认`b64_json` |
| `output_compression` | number | 否 | `0-100`，仅对 `jpeg`/`webp` 有意义。 |
| `background` | string | 否 | `auto` 或 `opaque`。注意：`gpt-image-2` 当前不支持 `transparent`。 |
| `moderation` | string | 否 | `auto` 或 `low`。默认 `auto`。 |
| `stream` | boolean | 否 | 是否启用流式返回，默认 `false`。 |
| `partial_images` | number | 否 | 流式模式下返回的中间图数量，`0-3`。 |
| `user` | string | 否 | 代表终端用户的稳定标识，便于安全监控与滥用检测。 |

推荐分辨率：

| 档位 | 横图 | 竖图 | 方图 |
| --- | --- | --- | --- |
| 1K | `1024x576` | `576x1024` | `1024x1024` |
| 2K | `2048x1152` | `1152x2048` | `2048x2048` |
| 4K | `3840x2160` | `2160x3840` | 不推荐方图 4K，优先使用横图或竖图 |

`gpt-image-2` 自定义尺寸约束：

- 最长边不超过 `3840px`。
- 宽高都必须是 `16px` 的倍数。
- 长短边比例不超过 `3:1`。
- 总像素数在 `655,360` 到 `8,294,400` 之间。
- 超过 `2560x1440` 总像素的输出属于实验性 2K 以上范围。

### 3.2 响应

非流式响应中，GPT Image 模型默认返回 base64 图片数据：

```json
{
  "created": 1713833628,
  "data": [
    {
      "b64_json": "..."
    }
  ],
  "output_format": "png",
  "quality": "medium",
  "size": "1024x1024",
  "usage": {
    "input_tokens": 0,
    "output_tokens": 0,
    "total_tokens": 0
  }
}
```

应用侧通常只需要读取 `data[0].b64_json`，base64 解码后写入文件或上传对象存储。

### 3.3 curl：生成 4K 图片并保存

```bash
curl -s "$FLUXCODE_BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $FLUXCODE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "一张 4K 电影感科技产品海报，未来感工作台，显示器上展示 AI 代码分析界面，真实光影，高细节，干净构图",
    "n": 1,
    "size": "3840x2160",
    "quality": "high",
    "output_format": "jpeg",
    "output_compression": 85
  }' \
  | jq -r '.data[0].b64_json' \
  | base64 --decode > fluxcode-4k.jpg
```

macOS 如果 `base64 --decode` 不可用，可改成：

```bash
  | base64 -D > fluxcode-4k.jpg
```

### 3.4 curl：生成普通 PNG 并保存

```bash
curl -s "$FLUXCODE_BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $FLUXCODE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "一张干净的 SaaS 产品官网首屏插图，展示 AI 代码助手正在分析代码仓库，现代扁平插画风格，浅色背景",
    "n": 1,
    "size": "1024x1024",
    "quality": "medium",
    "output_format": "png"
  }' \
  | jq -r '.data[0].b64_json' \
  | base64 --decode > fluxcode-image.png
```

### 3.5 curl：流式生成

流式模式返回 Server-Sent Events。可先接收中间图，再接收最终图。

```bash
curl -N -s "$FLUXCODE_BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $FLUXCODE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "一张电影感城市海报，雨夜街道，霓虹灯，中心是一位拿着透明伞的人",
    "size": "1024x1536",
    "quality": "medium",
    "output_format": "png",
    "stream": true,
    "partial_images": 2
  }'
```

典型事件类型：

- `image_generation.partial_image`：中间图，包含 base64 图片片段字段。
- `image_generation.completed`：最终图，包含最终 base64 图片和 usage。

## 4. FluxCode Images API：图生图/改图

### 4.1 请求

```http
POST {FLUXCODE_BASE_URL}/v1/images/edits
Authorization: Bearer $FLUXCODE_API_KEY
```

图生图/改图支持两种请求体：

| 请求体 | 适用场景 | 说明 |
| --- | --- | --- |
| `application/json` | 图片已在公网 URL、CDN URL，或已有 `data:image/...;base64,...` | 使用 `images[].image_url` 传入参考图。 |
| `multipart/form-data` | 本地文件上传 | 使用 `image` 或 `image[]` 上传一张或多张图片，可选 `mask`。 |

### 4.2 JSON 请求字段

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 建议填 | 推荐 `gpt-image-2`。系统要求图片接口使用 `gpt-image-*` 模型。 |
| `prompt` | string | 是 | 编辑指令，例如“保留主体，将背景替换为雪山”。 |
| `images` | array | 是 | 参考图数组。当前系统 JSON 方式只支持 `images[].image_url`。 |
| `images[].image_url` | string | 是 | 图片 URL 或 `data:image/...;base64,...`。 |
| `mask.image_url` | string | 否 | 可选蒙版图片 URL 或 data URL。当前系统 JSON 方式不支持 `mask.file_id`。 |
| `n` | number | 否 | 生成图片数量，`1-10`。默认通常为 `1`。 |
| `size` | string | 否 | 同文生图 `size` 字段，推荐使用 1K、2K、4K 分辨率。 |
| `response_format` | string | 否 | `b64_json` 或 `url`。默认`b64_json` |
| `quality` | string | 否 | `auto`、`low`、`medium`、`high`。 |
| `output_format` | string | 否 | `png`、`jpeg`、`webp`。 |
| `output_compression` | number | 否 | `0-100`，仅对 `jpeg`/`webp` 有意义。 |
| `background` | string | 否 | `auto` 或 `opaque`。 |
| `input_fidelity` | string | 否 | 图像参考保真度，是否支持取决于上游模型。 |
| `stream` | boolean | 否 | 是否启用流式返回，默认 `false`。 |
| `partial_images` | number | 否 | 流式模式下返回的中间图数量，`0-3`。 |

注意：

- 当前系统 JSON 图生图不支持 `images[].file_id`，传入会返回 `images[].file_id is not supported (use images[].image_url instead)`。
- 当前系统 JSON mask 不支持 `mask.file_id`，传入会返回 `mask.file_id is not supported (use mask.image_url instead)`。
- API Key 上游账号链路会把 JSON body 原样转发给上游，除模型映射/重写外不改写其它字段；OAuth/ChatGPT Web 链路会将输入图转换为对应上游格式。

### 4.3 multipart 请求字段

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | form field | 建议填 | 推荐 `gpt-image-2`。 |
| `prompt` | form field | 是 | 编辑指令。 |
| `image` / `image[]` | file | 是 | 输入图片；多图建议使用多个 `image[]=@...`。 |
| `mask` | file | 否 | 可选蒙版图片。 |
| `n` | form field | 否 | 生成图片数量。 |
| `size` | form field | 否 | 输出尺寸。 |
| `response_format` | form field | 否 | `b64_json` 或 `url`。 |
| `quality` | form field | 否 | `auto`、`low`、`medium`、`high`。 |
| `output_format` | form field | 否 | `png`、`jpeg`、`webp`。 |
| `output_compression` | form field | 否 | `0-100`。 |
| `stream` | form field | 否 | `true` 或 `false`。 |

单个上传 part 当前最大读取 `20MB`。如果图片较大，建议先压缩或改用 URL 方式。

### 4.4 响应

非流式响应与文生图一致，通常读取 `data[0].b64_json`：

```json
{
  "created": 1713833628,
  "data": [
    {
      "b64_json": "..."
    }
  ],
  "output_format": "png",
  "quality": "medium",
  "size": "1024x1024"
}
```

API Key 上游账号链路会透传上游成功响应。如果上游按 `response_format: "url"` 返回 `data[].url`，下游也会收到 `url`。

### 4.5 curl：JSON 图生图

```bash
curl -s "$FLUXCODE_BASE_URL/v1/images/edits" \
  -H "Authorization: Bearer $FLUXCODE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "保留人物主体和姿势，将背景替换为干净的未来感办公室，真实光影，高细节",
    "images": [
      {
        "image_url": "https://example.com/source-image.png"
      }
    ],
    "size": "1024x1024",
    "quality": "medium",
    "output_format": "png"
  }' \
  | jq -r '.data[0].b64_json' \
  | base64 --decode > fluxcode-edit.png
```

### 4.6 curl：multipart 上传本地图生图

```bash
curl -s "$FLUXCODE_BASE_URL/v1/images/edits" \
  -H "Authorization: Bearer $FLUXCODE_API_KEY" \
  -F "model=gpt-image-2" \
  -F "image[]=@source-image.png" \
  -F "prompt=将这张图改成高级产品海报风格，保留主体，背景换成浅色摄影棚，高细节" \
  -F "size=1024x1024" \
  -F "quality=medium" \
  -F "output_format=png" \
  | jq -r '.data[0].b64_json' \
  | base64 --decode > fluxcode-edit.png
```

### 4.7 curl：带 mask 的局部编辑

```bash
curl -s "$FLUXCODE_BASE_URL/v1/images/edits" \
  -H "Authorization: Bearer $FLUXCODE_API_KEY" \
  -F "model=gpt-image-2" \
  -F "image=@source-image.png" \
  -F "mask=@mask.png" \
  -F "prompt=只替换蒙版区域：把桌面上的杯子替换成一台银色笔记本电脑，其他区域保持不变" \
  -F "size=1024x1024" \
  -F "quality=medium" \
  -F "output_format=png" \
  | jq -r '.data[0].b64_json' \
  | base64 --decode > fluxcode-edit-mask.png
```

### 4.8 curl：流式图生图

```bash
curl -N -s "$FLUXCODE_BASE_URL/v1/images/edits" \
  -H "Authorization: Bearer $FLUXCODE_API_KEY" \
  -F "model=gpt-image-2" \
  -F "image[]=@source-image.png" \
  -F "prompt=将图片改成电影海报质感，增强光影，背景增加轻微景深" \
  -F "size=1024x1024" \
  -F "quality=medium" \
  -F "stream=true" \
  -F "partial_images=2"
```

典型事件类型：

- `image_edit.partial_image`：中间图事件。
- `image_edit.completed`：最终图事件，包含最终图片和 usage。

## 5. Responses API：对话式生图

FluxCode 同时支持 OpenAI 兼容的 `POST /v1/responses`。若通过 Responses API 使用 `image_generation` 工具，请调用 FluxCode 域名并使用 FluxCode API Key。

### 5.1 请求

```http
POST {FLUXCODE_BASE_URL}/v1/responses
Content-Type: application/json
Authorization: Bearer $FLUXCODE_API_KEY
```

常用 JSON 字段：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 支持 `image_generation` 工具的主线模型，例如 `gpt-5` 及更新模型。 |
| `input` | string / array | 是 | 用户输入。可以是文本，也可以是多模态消息数组。 |
| `tools` | array | 是 | 增加 `{ "type": "image_generation" }`。 |
| `tool_choice` | string / object | 否 | 可设置为 `required` 强制调用工具。 |
| `stream` | boolean | 否 | 是否启用流式返回。 |
| `previous_response_id` | string | 否 | 多轮续改时传上一轮 response id。 |

`image_generation` 工具常用字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `type` | string | 固定为 `image_generation`。 |
| `action` | string | `auto`、`generate`、`edit`。默认 `auto`；只想新建图片时用 `generate`。 |
| `size` | string | 如 `1024x1024`、`1536x1024`、`1024x1536`、`auto`。 |
| `quality` | string | `auto`、`low`、`medium`、`high`。 |
| `output_format` | string | `png`、`jpeg`、`webp`。 |
| `output_compression` | number | `0-100`。 |
| `background` | string | `auto`、`opaque`，是否支持透明取决于具体图像模型。 |
| `partial_images` | number | 流式模式下的中间图数量，`0-3`。 |

### 5.2 响应

生图结果位于 `output` 数组中，找到 `type = "image_generation_call"` 的项目后读取 `result`：

```json
{
  "id": "resp_...",
  "output": [
    {
      "type": "image_generation_call",
      "result": "base64..."
    }
  ]
}
```

### 5.3 curl：Responses API 生图并保存

```bash
curl -s "$FLUXCODE_BASE_URL/v1/responses" \
  -H "Authorization: Bearer $FLUXCODE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.5",
    "input": "生成一张灰色虎斑猫抱着戴橙色围巾的水獭的温暖插画",
    "tools": [
      {
        "type": "image_generation",
        "action": "generate",
        "size": "1024x1024",
        "quality": "medium",
        "output_format": "png"
      }
    ],
    "tool_choice": "required"
  }' \
  | jq -r '.output[] | select(.type == "image_generation_call") | .result' \
  | head -n 1 \
  | base64 --decode > response-image.png
```

如果账号暂时没有 `gpt-5.5` 权限，请替换为你项目中可用且支持 `image_generation` 工具的 `gpt-5` 或更新模型。

### 5.4 curl：Responses API 流式生图

```bash
curl -N -s "$FLUXCODE_BASE_URL/v1/responses" \
  -H "Authorization: Bearer $FLUXCODE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.5",
    "input": "绘制一条由白色羽毛组成的河流，穿过宁静的冬季森林，电影感，高细节",
    "stream": true,
    "tools": [
      {
        "type": "image_generation",
        "partial_images": 2,
        "size": "1536x1024",
        "quality": "medium"
      }
    ],
    "tool_choice": "required"
  }'
```

典型事件类型：

- `response.image_generation_call.partial_image`：中间图事件。
- `response.completed`：最终完成事件。最终图片仍在 `response.output[]` 中的 `image_generation_call.result`。

## 6. 接入建议

- 在服务端读取 `FLUXCODE_API_KEY`，不要把平台 Key 暴露给浏览器或移动端。
- 为每个终端用户传递稳定的 `user` 标识，避免使用手机号、邮箱等明文隐私数据。
- 对 prompt 做长度限制、敏感词预检查和审计日志。
- 对 `size`、`quality`、`output_format` 做白名单校验，避免不可控成本。
- 将 base64 解码后的图片上传对象存储，业务接口只返回 CDN URL，可以显著降低响应体大小。
- 对 `429`、`5xx` 做有限重试；对 `moderation_blocked`、`invalid_image_*`、`image_generation_user_error` 不要盲目重试。

## 7. 错误处理

常见错误方向：

| 类型 | 处理建议 |
| --- | --- |
| `401` / `403` | 检查 API Key、项目权限、组织验证、模型权限。 |
| `429` | 触发 rate limit 或额度限制；可指数退避重试，并提示用户稍后再试。 |
| `5xx` | OpenAI 服务端或网络临时问题；可有限重试。 |
| `moderation_blocked` | 内容安全拦截；提示用户调整 prompt，不要原样重试。 |
| `invalid_image_*` | 输入图片、base64、URL、格式或尺寸无效；修正请求后重试。 |
| `image_too_large` / `image_file_too_large` | 压缩图片或降低尺寸。 |

建议记录响应头中的 `x-request-id`，便于排查：

```bash
curl -s -D headers.txt "$FLUXCODE_BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $FLUXCODE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "一张简洁的应用图标，蓝绿色调，抽象闪电形状"
  }'

grep -i "x-request-id" headers.txt
```

## 8. 最小可用清单

上线前至少确认：

- 已配置服务端环境变量 `FLUXCODE_API_KEY`。
- FluxCode API Key 所属分组已绑定 OpenAI 兼容渠道。
- 后端接口限制 `size`、`quality`、`n`，避免成本失控。
- 前端不直接访问 FluxCode API Key 或 OpenAI 上游 Key。
- 已处理 `429`、`5xx`、内容安全拦截和输入无效错误。
- 已配置图片落库或对象存储策略，不长期把大 base64 字符串放在业务数据库热表中。
