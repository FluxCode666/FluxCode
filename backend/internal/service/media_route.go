package service

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
