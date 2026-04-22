package main

import (
	"log"
	"os"
	"runtime/debug"
	"strconv"
)

// initMemoryLimit 根据系统可用内存自动设置 GOMEMLIMIT。
//
// 策略：
//   - 若环境变量 GOMEMLIMIT 已设置，不做覆盖。
//   - 否则取系统总物理内存的 80% 作为 soft limit，让 GC 更早回收，
//     避免 Go 进程在小内存机器（如 2GB）上因 GC 滞后导致 OOM。
//
// 注意：此函数应在 main() 启动尽早调用。
func initMemoryLimit() {
	if os.Getenv("GOMEMLIMIT") != "" {
		return // 用户显式设置，不覆盖
	}

	totalMem := detectTotalMemory()
	if totalMem == 0 {
		return // 无法探测
	}

	// 取总内存的 80%
	limit := int64(float64(totalMem) * 0.8)
	if limit <= 0 {
		return
	}

	debug.SetMemoryLimit(limit)
	log.Printf("GOMEMLIMIT auto-set to %s (system total: %s)",
		formatBytes(limit), formatBytes(int64(totalMem)))
}

func formatBytes(b int64) string {
	const (
		MB = 1024 * 1024
		GB = 1024 * MB
	)
	switch {
	case b >= GB:
		return strconv.FormatFloat(float64(b)/float64(GB), 'f', 1, 64) + "GB"
	default:
		return strconv.FormatFloat(float64(b)/float64(MB), 'f', 0, 64) + "MB"
	}
}
