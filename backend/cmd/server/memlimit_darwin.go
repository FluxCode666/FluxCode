package main

import (
	"syscall"
	"unsafe"
)

// detectTotalMemory 获取 macOS 系统物理内存总量。
func detectTotalMemory() uint64 {
	val, err := syscall.Sysctl("hw.memsize")
	if err != nil || len(val) < 8 {
		return 0
	}
	return *(*uint64)(unsafe.Pointer(&[]byte(val)[0]))
}
