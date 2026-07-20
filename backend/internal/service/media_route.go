package service

// MediaRequestMapping is a route-level request mapping snapshot. Mapping rules
// are added by the declarative request-mapping stage.
type MediaRequestMapping struct{}

type MediaRouteRequest struct {
	GroupID        int64
	RequestedModel string
	Operation      MediaOperation
	Capability     MediaType
	SessionHash    string
	ClientAsync    bool
}

type MediaRouteTarget struct {
	AccountID       int64
	PublicModelID   string
	UpstreamModelID string
	Vendor          string
	Adapter         string
	NativeAsyncMode NativeAsyncMode
	RequestMapping  MediaRequestMapping
}
