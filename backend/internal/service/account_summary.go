package service

type AccountSummaryCounts struct {
	All                 int64 `json:"all"`
	Active              int64 `json:"active"`
	Inactive            int64 `json:"inactive"`
	Expired             int64 `json:"expired"`
	Error               int64 `json:"error"`
	Banned              int64 `json:"banned"`
	Available           int64 `json:"available"`
	ManualUnschedulable int64 `json:"manual_unschedulable"`
	TempUnschedulable   int64 `json:"temp_unschedulable"`
	RateLimited         int64 `json:"rate_limited"`
	Overloaded          int64 `json:"overloaded"`
}

func (c *AccountSummaryCounts) Add(other AccountSummaryCounts) {
	if c == nil {
		return
	}
	c.All += other.All
	c.Active += other.Active
	c.Inactive += other.Inactive
	c.Expired += other.Expired
	c.Error += other.Error
	c.Banned += other.Banned
	c.Available += other.Available
	c.ManualUnschedulable += other.ManualUnschedulable
	c.TempUnschedulable += other.TempUnschedulable
	c.RateLimited += other.RateLimited
	c.Overloaded += other.Overloaded
}

type AccountPlatformSummaryItem struct {
	Platform string               `json:"platform"`
	Counts   AccountSummaryCounts `json:"counts"`
}

type AccountSummaryResponse struct {
	Overall   AccountSummaryCounts         `json:"overall"`
	Platforms []AccountPlatformSummaryItem `json:"platforms"`
}
