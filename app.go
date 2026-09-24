package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Settings 用户可配置项
type Settings struct {
	ProxyPort      int    `json:"proxyPort"`
	APIPort        int    `json:"apiPort"`
	DownloadDir    string `json:"downloadDir"`
	UpstreamProxy  string `json:"upstreamProxy"`
	MaxRunning     int    `json:"maxRunning"`
	DefaultHighest bool   `json:"defaultHighest"`
	DownloadCover  bool   `json:"downloadCover"`
	PlayDoneAudio  bool   `json:"playDoneAudio"`
}

// Status 服务状态
type Status struct {
	Running     bool   `json:"running"`
	ProxyPort   int    `json:"proxyPort"`
	APIPort     int    `json:"apiPort"`
	Uptime      string `json:"uptime"`
	DownloadDir string `json:"downloadDir"`
}

// LogEntry 日志条目
type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// App 应用主结构
type App struct {
	ctx        context.Context
	cmd        *exec.Cmd
	running    bool
	startTime  time.Time
	logs       []LogEntry
	logMu      sync.Mutex
	configPath string
	exePath    string
	workDir    string
	cancelFunc context.CancelFunc
	// 代理状态保存（启动前保存，停止后恢复）
	savedProxyEnabled bool
	savedProxyServer  string
	savedProxyDirty   bool
}

// NewApp 创建应用实例
func NewApp() *App {
	a := &App{logs: make([]LogEntry, 0, 1000)}

	if exe, err := os.Executable(); err == nil {
		a.workDir = filepath.Dir(exe)
	} else {
		a.workDir = "."
	}

	// 查找原工具二进制
	if runtime.GOOS == "windows" {
		a.exePath = filepath.Join(a.workDir, "bin", "wx_video_download.exe")
	} else {
		a.exePath = filepath.Join(a.workDir, "bin", "wx_video_download")
	}
	a.configPath = filepath.Join(a.workDir, "bin", "config.yaml")

	// 开发模式回退
	if _, err := os.Stat(a.exePath); os.IsNotExist(err) {
		if runtime.GOOS == "windows" {
			devExe := `D:\tools\wx_channels_download\wx_video_download.exe`
			if _, e := os.Stat(devExe); e == nil {
				a.exePath = devExe
				a.configPath = `D:\tools\wx_channels_download\config.yaml`
				a.workDir = `D:\tools\wx_channels_download`
			}
		}
	}

	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.addLog("INFO", "应用已启动")
	a.addLog("INFO", fmt.Sprintf("工具: %s", a.exePath))
}

func (a *App) shutdown(ctx context.Context) {
	a.StopService()
}

func (a *App) beforeClose(ctx context.Context) bool {
	if a.running {
		a.StopService()
	}
	return false
}

// GetSettings 读取配置
func (a *App) GetSettings() Settings {
	s := Settings{ProxyPort: 2023, APIPort: 2022, MaxRunning: 3, PlayDoneAudio: true}
	data, err := os.ReadFile(a.configPath)
	if err != nil {
		return s
	}
	c := string(data)
	s.ProxyPort = yamlInt(c, "proxy.port", 2023)
	s.APIPort = yamlInt(c, "api.port", 2022)
	s.MaxRunning = yamlInt(c, "download.maxRunning", 3)
	s.DownloadDir = yamlStr(c, "download.dir")
	s.UpstreamProxy = yamlStr(c, "proxy.upstreamProxy")
	s.DefaultHighest = yamlBool(c, "channels.download.defaultHighest")
	s.DownloadCover = yamlBool(c, "channels.download.cover")
	s.PlayDoneAudio = yamlBool(c, "download.playDoneAudio")
	return s
}

// SaveSettings 保存配置
func (a *App) SaveSettings(s Settings) error {
	if a.running {
		return fmt.Errorf("服务运行中，请先停止")
	}
	data, err := os.ReadFile(a.configPath)
	if err != nil {
		return err
	}
	c := string(data)
	c = yamlSetInt(c, "proxy.port", s.ProxyPort)
	c = yamlSetInt(c, "api.port", s.APIPort)
	c = yamlSetInt(c, "download.maxRunning", s.MaxRunning)
	c = yamlSetStr(c, "download.dir", s.DownloadDir)
	c = yamlSetStr(c, "proxy.upstreamProxy", s.UpstreamProxy)
	c = yamlSetBool(c, "channels.download.defaultHighest", s.DefaultHighest)
	c = yamlSetBool(c, "channels.download.cover", s.DownloadCover)
	c = yamlSetBool(c, "download.playDoneAudio", s.PlayDoneAudio)
	if err := os.WriteFile(a.configPath, []byte(c), 0644); err != nil {
		return err
	}
	a.addLog("INFO", "配置已保存")
	return nil
}

// SelectDirectory 目录选择对话框
func (a *App) SelectDirectory(title string) (string, error) {
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: title})
}

// StartService 启动服务
func (a *App) StartService() error {
	if a.running {
		return fmt.Errorf("服务已在运行")
	}
	if _, err := os.Stat(a.exePath); os.IsNotExist(err) {
		return fmt.Errorf("工具不存在: %s", a.exePath)
	}

	// 保存当前系统代理状态（停止时恢复）
	a.savedProxyEnabled, a.savedProxyServer = false, ""
	if st := GetSystemProxy(); st.Server != "" {
		a.savedProxyEnabled = st.Enabled
		a.savedProxyServer = st.Server
		a.savedProxyDirty = true
		a.addLog("INFO", fmt.Sprintf("已保存当前代理设置: %s (启用: %v)", st.Server, st.Enabled))
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.cancelFunc = cancel
	a.cmd = exec.CommandContext(ctx, a.exePath)
	a.cmd.Dir = a.workDir
	a.cmd.SysProcAttr = hideConsole()

	stdout, err := a.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := a.cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := a.cmd.Start(); err != nil {
		return err
	}

	a.running = true
	a.startTime = time.Now()
	a.addLog("INFO", "服务启动中...")

	go a.readPipe(stdout, "INFO")
	go a.readPipe(stderr, "WARN")

	go func() {
		err := a.cmd.Wait()
		a.running = false
		if err != nil {
			a.addLog("ERROR", fmt.Sprintf("进程退出: %v", err))
		} else {
			a.addLog("INFO", "服务已停止")
		}
		// 进程退出时也恢复代理
		if a.savedProxyDirty {
			if e := SetSystemProxy(ProxyState{Enabled: a.savedProxyEnabled, Server: a.savedProxyServer}); e != nil {
				a.addLog("WARN", fmt.Sprintf("恢复代理失败: %v", e))
			} else {
				a.addLog("INFO", fmt.Sprintf("已恢复系统代理: %s", a.savedProxyServer))
			}
			a.savedProxyDirty = false
		}
		wailsruntime.EventsEmit(a.ctx, "status-changed", a.GetStatus())
	}()

	// 等待 API 就绪
	go func() {
		for i := 0; i < 15; i++ {
			time.Sleep(1 * time.Second)
			s := a.GetSettings()
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d", s.APIPort))
			if err == nil {
				resp.Body.Close()
				a.addLog("INFO", "✓ 代理服务启动成功")
				wailsruntime.EventsEmit(a.ctx, "status-changed", a.GetStatus())
				return
			}
		}
		a.addLog("WARN", "服务可能未就绪")
		wailsruntime.EventsEmit(a.ctx, "status-changed", a.GetStatus())
	}()

	return nil
}

// StopService 停止服务
func (a *App) StopService() error {
	if !a.running || a.cmd == nil {
		return fmt.Errorf("服务未运行")
	}
	a.addLog("INFO", "正在停止服务...")
	if a.cancelFunc != nil {
		a.cancelFunc()
	}
	done := make(chan error, 1)
	go func() { done <- a.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		if a.cmd.Process != nil {
			a.cmd.Process.Kill()
		}
	}
	a.running = false
	a.addLog("INFO", "服务已停止")

	// 恢复原系统代理设置
	if a.savedProxyDirty {
		if err := SetSystemProxy(ProxyState{Enabled: a.savedProxyEnabled, Server: a.savedProxyServer}); err != nil {
			a.addLog("WARN", fmt.Sprintf("恢复代理失败: %v", err))
		} else {
			a.addLog("INFO", fmt.Sprintf("已恢复系统代理: %s (启用: %v)", a.savedProxyServer, a.savedProxyEnabled))
		}
		a.savedProxyDirty = false
	}

	wailsruntime.EventsEmit(a.ctx, "status-changed", a.GetStatus())
	return nil
}

// GetStatus 获取状态
func (a *App) GetStatus() Status {
	s := a.GetSettings()
	uptime := ""
	if a.running {
		uptime = time.Since(a.startTime).Round(time.Second).String()
	}
	return Status{Running: a.running, ProxyPort: s.ProxyPort, APIPort: s.APIPort, Uptime: uptime, DownloadDir: s.DownloadDir}
}

// GetLogs 获取日志
func (a *App) GetLogs() []LogEntry {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	r := make([]LogEntry, len(a.logs))
	copy(r, a.logs)
	return r
}

// ClearLogs 清空日志
func (a *App) ClearLogs() {
	a.logMu.Lock()
	a.logs = a.logs[:0]
	a.logMu.Unlock()
	wailsruntime.EventsEmit(a.ctx, "logs-cleared")
}

func (a *App) addLog(level, msg string) {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	e := LogEntry{Time: time.Now().Format("15:04:05"), Level: level, Message: msg}
	a.logs = append(a.logs, e)
	if len(a.logs) > 2000 {
		a.logs = a.logs[1000:]
	}
	wailsruntime.EventsEmit(a.ctx, "new-log", e)
}

func (a *App) readPipe(r io.Reader, level string) {
	s := bufio.NewScanner(r)
	for s.Scan() {
		if line := strings.TrimSpace(s.Text()); line != "" {
			a.addLog(level, line)
		}
	}
}

// YAML 工具函数

func yamlInt(c, k string, d int) int {
	for _, l := range strings.Split(c, "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, k+":") {
			v := strings.TrimSpace(strings.TrimPrefix(t, k+":"))
			var n int
			if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
				return n
			}
		}
	}
	return d
}

func yamlStr(c, k string) string {
	for _, l := range strings.Split(c, "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, k+":") {
			v := strings.TrimSpace(strings.TrimPrefix(t, k+":"))
			return strings.Trim(v, "\"'")
		}
	}
	return ""
}

func yamlBool(c, k string) bool {
	for _, l := range strings.Split(c, "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, k+":") {
			return strings.TrimSpace(strings.TrimPrefix(t, k+":")) == "true"
		}
	}
	return false
}

func yamlSetInt(c, k string, v int) string {
	lines := strings.Split(c, "\n")
	p := k + ":"
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, p) {
			indent := l[:len(l)-len(strings.TrimLeft(l, " "))]
			lines[i] = fmt.Sprintf("%s%s %d", indent, p, v)
			break
		}
	}
	return strings.Join(lines, "\n")
}

func yamlSetStr(c, k, v string) string {
	lines := strings.Split(c, "\n")
	p := k + ":"
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, p) {
			indent := l[:len(l)-len(strings.TrimLeft(l, " "))]
			if v == "" {
				lines[i] = fmt.Sprintf("%s%s \"\"", indent, p)
			} else {
				lines[i] = fmt.Sprintf("%s%s \"%s\"", indent, p, v)
			}
			break
		}
	}
	return strings.Join(lines, "\n")
}

func yamlSetBool(c, k string, v bool) string {
	lines := strings.Split(c, "\n")
	p := k + ":"
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, p) {
			indent := l[:len(l)-len(strings.TrimLeft(l, " "))]
			lines[i] = fmt.Sprintf("%s%s %t", indent, p, v)
			break
		}
	}
	return strings.Join(lines, "\n")
}
