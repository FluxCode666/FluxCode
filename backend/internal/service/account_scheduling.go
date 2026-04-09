package service

type AccountSchedulingState string

const (
	AccountSchedulingStateAvailable           AccountSchedulingState = "available"
	AccountSchedulingStateManualUnschedulable AccountSchedulingState = "manual_unschedulable"
	AccountSchedulingStateTempUnschedulable   AccountSchedulingState = "temp_unschedulable"
	AccountSchedulingStateRateLimited         AccountSchedulingState = "rate_limited"
	AccountSchedulingStateOverloaded          AccountSchedulingState = "overloaded"
	AccountSchedulingStateExpired             AccountSchedulingState = "expired"
	AccountSchedulingStateInactive            AccountSchedulingState = "inactive"
	AccountSchedulingStateError               AccountSchedulingState = "error"
	AccountSchedulingStateBanned              AccountSchedulingState = "banned"
)

var accountSchedulingStateSet = map[AccountSchedulingState]struct{}{
	AccountSchedulingStateAvailable:           {},
	AccountSchedulingStateManualUnschedulable: {},
	AccountSchedulingStateTempUnschedulable:   {},
	AccountSchedulingStateRateLimited:         {},
	AccountSchedulingStateOverloaded:          {},
	AccountSchedulingStateExpired:             {},
	AccountSchedulingStateInactive:            {},
	AccountSchedulingStateError:               {},
	AccountSchedulingStateBanned:              {},
}

func IsValidAccountSchedulingState(state AccountSchedulingState) bool {
	_, ok := accountSchedulingStateSet[state]
	return ok
}

func NormalizeAccountSchedulingState(state AccountSchedulingState) AccountSchedulingState {
	if !IsValidAccountSchedulingState(state) {
		return ""
	}
	return state
}
