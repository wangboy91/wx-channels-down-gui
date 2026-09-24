package main

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

func hideConsole() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

// ProxyState 保存的代理状态
type ProxyState struct {
	Enabled bool
	Server  string
}

// GetSystemProxy 获取当前系统代理设置
func GetSystemProxy() ProxyState {
	key, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.READ)
	if err != nil {
		return ProxyState{}
	}
	defer key.Close()

	enabled, _, _ := key.GetIntegerValue("ProxyEnable")
	server, _, _ := key.GetStringValue("ProxyServer")

	return ProxyState{
		Enabled: enabled != 0,
		Server:  server,
	}
}

// SetSystemProxy 设置系统代理
func SetSystemProxy(state ProxyState) error {
	key, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.WRITE)
	if err != nil {
		return fmt.Errorf("打开注册表失败: %w", err)
	}
	defer key.Close()

	enabled := uint32(0)
	if state.Enabled {
		enabled = 1
	}
	if err := key.SetDWordValue("ProxyEnable", enabled); err != nil {
		return fmt.Errorf("设置 ProxyEnable 失败: %w", err)
	}
	if state.Server != "" {
		if err := key.SetStringValue("ProxyServer", state.Server); err != nil {
			return fmt.Errorf("设置 ProxyServer 失败: %w", err)
		}
	}
	NotifyProxyChange()
	return nil
}

// NotifyProxyChange 通知系统代理设置变更
func NotifyProxyChange() {
	modwininet := syscall.NewLazyDLL("wininet.dll")
	proc := modwininet.NewProc("InternetSetOptionW")
	proc.Call(0, 0x2F, 0, 0) // INTERNET_OPTION_SETTINGS_CHANGED
	proc.Call(0, 0x39, 0, 0) // INTERNET_OPTION_REFRESH
}

// IsAdmin 检查是否以管理员身份运行
func IsAdmin() bool {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SAM\SAM`, registry.READ)
	if err != nil {
		return false
	}
	key.Close()
	return true
}

func ensureAdmin() {}
