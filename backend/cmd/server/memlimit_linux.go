package main

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// detectTotalMemory 获取 Linux 系统可用内存。
// 优先读取 cgroup v2/v1 内存限制（容器场景），回退到物理内存。
func detectTotalMemory() uint64 {
	// cgroup v2
	if data, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
		s := strings.TrimSpace(string(data))
		if s != "max" {
			if v, err := strconv.ParseUint(s, 10, 64); err == nil && v > 0 {
				return v
			}
		}
	}

	// cgroup v1
	if data, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); err == nil {
		s := strings.TrimSpace(string(data))
		if v, err := strconv.ParseUint(s, 10, 64); err == nil && v > 0 && v < 1<<62 {
			return v
		}
	}

	// 物理内存
	var info syscall.Sysinfo_t
	if err := syscall.Sysinfo(&info); err == nil {
		return info.Totalram * uint64(info.Unit)
	}
	return 0
}
