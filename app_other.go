//go:build !windows

package main

import "syscall"

func hideConsole() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

// ProxyState 保存的代理状态
type ProxyState struct {
	Enabled bool
	Server  string
}

// GetSystemProxy 获取当前系统代理（macOS/Linux 暂不支持自动读取）
func GetSystemProxy() ProxyState {
	return ProxyState{}
}

// SetSystemProxy 设置系统代理（macOS/Linux 暂不支持自动设置）
func SetSystemProxy(state ProxyState) error {
	return nil
}

// NotifyProxyChange 通知系统代理变更
func NotifyProxyChange() {}

// IsAdmin 检查是否 root
func IsAdmin() bool {
	return syscall.Getuid() == 0
}

func ensureAdmin() {}
