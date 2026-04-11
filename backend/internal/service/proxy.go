package service

import (
	"net"
	"net/url"
	"strconv"
	"time"
)

type Proxy struct {
	ID        int64
	Name      string
	Protocol  string
	Host      string
	Port      int
	Username  string
	Password  string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (p *Proxy) IsActive() bool {
	return p.Status == StatusActive
}

func (p *Proxy) URL() string {
	u := &url.URL{
		Scheme: p.Protocol,
		Host:   net.JoinHostPort(p.Host, strconv.Itoa(p.Port)),
	}
	if p.Username != "" && p.Password != "" {
		u.User = url.UserPassword(p.Username, p.Password)
	}
	return u.String()
}

type ProxyWithAccountCount struct {
	Proxy
	AccountCount   int64
	LatencyMs      *int64
	LatencyStatus  string
	LatencyMessage string
	IPAddress      string
	Country        string
	CountryCode    string
	Region         string
	City           string
	QualityStatus  string
	QualityScore   *int
	QualityGrade   string
	QualitySummary string
	QualityChecked *int64
}

type ProxyAccountCountState string

const (
	ProxyAccountCountStateAllActive           ProxyAccountCountState = "all_active"
	ProxyAccountCountStateAvailable           ProxyAccountCountState = "available"
	ProxyAccountCountStateManualUnschedulable ProxyAccountCountState = "manual_unschedulable"
	ProxyAccountCountStateTempUnschedulable   ProxyAccountCountState = "temp_unschedulable"
	ProxyAccountCountStateRateLimited         ProxyAccountCountState = "rate_limited"
	ProxyAccountCountStateOverloaded          ProxyAccountCountState = "overloaded"
	ProxyAccountCountStateExpired             ProxyAccountCountState = "expired"
	ProxyAccountCountStateInactive            ProxyAccountCountState = "inactive"
	ProxyAccountCountStateError               ProxyAccountCountState = "error"
	ProxyAccountCountStateBanned              ProxyAccountCountState = "banned"
)

var proxyAccountCountStateSet = map[ProxyAccountCountState]struct{}{
	ProxyAccountCountStateAllActive:           {},
	ProxyAccountCountStateAvailable:           {},
	ProxyAccountCountStateManualUnschedulable: {},
	ProxyAccountCountStateTempUnschedulable:   {},
	ProxyAccountCountStateRateLimited:         {},
	ProxyAccountCountStateOverloaded:          {},
	ProxyAccountCountStateExpired:             {},
	ProxyAccountCountStateInactive:            {},
	ProxyAccountCountStateError:               {},
	ProxyAccountCountStateBanned:              {},
}

type ProxyAccountCountItem struct {
	ProxyID      int64
	AccountCount int64
}

func IsValidProxyAccountCountState(state ProxyAccountCountState) bool {
	_, ok := proxyAccountCountStateSet[state]
	return ok
}

func NormalizeProxyAccountCountStates(states []ProxyAccountCountState) []ProxyAccountCountState {
	if len(states) == 0 {
		return []ProxyAccountCountState{ProxyAccountCountStateAvailable}
	}

	seen := make(map[ProxyAccountCountState]struct{}, len(states))
	out := make([]ProxyAccountCountState, 0, len(states))
	for _, state := range states {
		if !IsValidProxyAccountCountState(state) {
			continue
		}
		if _, ok := seen[state]; ok {
			continue
		}
		seen[state] = struct{}{}
		if state == ProxyAccountCountStateAllActive {
			return []ProxyAccountCountState{ProxyAccountCountStateAllActive}
		}
		out = append(out, state)
	}
	if len(out) == 0 {
		return []ProxyAccountCountState{ProxyAccountCountStateAvailable}
	}
	return out
}

func ProxyAccountCountStateStrings(states []ProxyAccountCountState) []string {
	if len(states) == 0 {
		return nil
	}
	out := make([]string, 0, len(states))
	for _, state := range NormalizeProxyAccountCountStates(states) {
		out = append(out, string(state))
	}
	return out
}

type ProxyAccountSummary struct {
	ID       int64
	Name     string
	Platform string
	Type     string
	Notes    *string
}
