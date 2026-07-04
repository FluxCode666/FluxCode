//go:build !unit

package service

func float64PtrForTest(v float64) *float64 {
	return &v
}
