// Package openai_compat 提供 OpenAI 协议族在不同上游间的能力差异判定工具。
//
// 背景：sub2api 的 OpenAI APIKey 账号通过 base_url 接入多种第三方 OpenAI 兼容上游
// （DeepSeek、Kimi、GLM、Qwen 等）。这些上游普遍只支持 /v1/chat/completions，
// 不存在 /v1/responses 端点。但网关历史代码无差别走 CC→Responses 转换并打到
// /v1/responses，导致兼容上游 404。
//
// 本包提供账号探测结果与手动协议开关的判定，配合
// internal/service/openai_apikey_responses_probe.go 在创建/修改账号时一次性
// 探测并落标。
//
// 设计取舍：
//   - 不维护静态 host 白名单，避免新增厂商时必须改代码
//   - 能力探测只提供提示，协议路由完全由账号的手动开关决定
package openai_compat

// AccountResponsesSupport 描述账号上游对 OpenAI Responses API 的探测结果。
//
// 仅用于 platform=openai + type=apikey 的账号；其他账号类型不应调用本包判定。
type AccountResponsesSupport int

const (
	// ResponsesSupportUnknown 表示账号尚未完成能力探测（extra 字段缺失）。
	ResponsesSupportUnknown AccountResponsesSupport = iota

	// ResponsesSupportYes 探测确认上游支持 /v1/responses。
	ResponsesSupportYes

	// ResponsesSupportNo 探测确认上游不支持 /v1/responses；管理员可据此
	// 手动开启 Chat Completions 协议。
	ResponsesSupportNo
)

// ResponsesSupportMode 描述账号级 Responses API 路由覆盖模式。
type ResponsesSupportMode string

const (
	// ResponsesSupportModeAuto 是兼容已有数据的默认值；当前语义为使用
	// Responses API，不跟随能力探测结果自动切换。
	ResponsesSupportModeAuto ResponsesSupportMode = "auto"

	// ResponsesSupportModeForceChatCompletions 强制使用 /v1/chat/completions。
	ResponsesSupportModeForceChatCompletions ResponsesSupportMode = "force_chat_completions"
)

// ExtraKeyResponsesMode 是 accounts.extra JSON 中存储手动覆盖模式的键名。
const ExtraKeyResponsesMode = "openai_responses_mode"

// ExtraKeyResponsesSupported 是 accounts.extra JSON 中存储探测结果的键名。
// 值类型为 bool：true=支持、false=不支持、键缺失=未探测。
const ExtraKeyResponsesSupported = "openai_responses_supported"

// NormalizeResponsesSupportMode 归一化账号级 Responses API 路由覆盖模式。
// 缺失或非法值按 auto 处理，即默认使用 Responses API。
func NormalizeResponsesSupportMode(mode string) ResponsesSupportMode {
	if ResponsesSupportMode(mode) == ResponsesSupportModeForceChatCompletions {
		return ResponsesSupportModeForceChatCompletions
	}
	return ResponsesSupportModeAuto
}

// ResolveResponsesSupport 从账号的 extra map 中读取能力探测标记。
//
// 该函数只描述上游能力，不参与协议路由；手动 Chat Completions 开关由
// ShouldUseResponsesAPI 独立判断。标记缺失或类型不匹配时返回
// ResponsesSupportUnknown。
func ResolveResponsesSupport(extra map[string]any) AccountResponsesSupport {
	if extra == nil {
		return ResponsesSupportUnknown
	}
	v, ok := extra[ExtraKeyResponsesSupported]
	if !ok {
		return ResponsesSupportUnknown
	}
	supported, ok := v.(bool)
	if !ok {
		return ResponsesSupportUnknown
	}
	if supported {
		return ResponsesSupportYes
	}
	return ResponsesSupportNo
}

// ShouldUseResponsesAPI 判断 OpenAI APIKey 账号是否应请求上游 Responses API。
//
// 协议路由只受账号级 Chat Completions 开关控制：仅当手动模式为
// force_chat_completions 时返回 false；其余情况一律返回 true。
// ExtraKeyResponsesSupported 仅记录能力探测结果，不得隐式改变请求链路。
func ShouldUseResponsesAPI(extra map[string]any) bool {
	if extra == nil {
		return true
	}
	mode, _ := extra[ExtraKeyResponsesMode].(string)
	return NormalizeResponsesSupportMode(mode) != ResponsesSupportModeForceChatCompletions
}
