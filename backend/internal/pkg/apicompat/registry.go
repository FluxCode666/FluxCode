package apicompat

type Protocol string

const (
	ProtocolChatCompletions   Protocol = "chat_completions"
	ProtocolResponses         Protocol = "responses"
	ProtocolAnthropicMessages Protocol = "anthropic_messages"
	ProtocolEmbeddings        Protocol = "embeddings"
)

type CompatibilityProfile string

const (
	ProfileNonStreamText CompatibilityProfile = "non_stream_text_v1"
	ProfileStreamText    CompatibilityProfile = "stream_text_v1"
	ProfileFunctionTools CompatibilityProfile = "function_tools_v1"
)

const (
	ReasonNoAdapter          = "no_adapter"
	ReasonInvalidJSON        = "invalid_json"
	ReasonUnknownField       = "unknown_field"
	ReasonManagedState       = "managed_state_unsupported"
	ReasonFeatureUnsupported = "feature_unsupported"
	ReasonStreamUnsupported  = "stream_unsupported"
	ReasonInputUnsupported   = "input_unsupported"
)

type CompatibilityResult struct {
	Compatible bool
	Profile    CompatibilityProfile
	ReasonCode string
	Detail     string
}

type requestChecker func([]byte) CompatibilityResult

type Registry struct {
	checkers map[[2]Protocol]requestChecker
}

func NewRegistry() *Registry {
	return &Registry{checkers: map[[2]Protocol]requestChecker{
		{ProtocolResponses, ProtocolChatCompletions}: checkResponsesToChatRequest,
	}}
}

func (r *Registry) CheckRequest(from, to Protocol, raw []byte) CompatibilityResult {
	if r == nil {
		return CompatibilityResult{ReasonCode: ReasonNoAdapter}
	}
	checker, ok := r.checkers[[2]Protocol{from, to}]
	if !ok {
		return CompatibilityResult{ReasonCode: ReasonNoAdapter}
	}
	return checker(raw)
}

func (r *Registry) HasDirection(from, to Protocol) bool {
	if r == nil {
		return false
	}
	_, ok := r.checkers[[2]Protocol{from, to}]
	return ok
}
