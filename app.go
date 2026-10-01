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
	"regexp"
	"runtime"
	"strconv"
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
	Ready       bool   `json:"ready"`
	WebURL      string `json:"webUrl"`
	ProxyPort   int    `json:"proxyPort"`
	APIPort     int    `json:"apiPort"`
	Uptime      string `json:"uptime"`
	DownloadDir string `json:"downloadDir"`
	LastError   string `json:"lastError"`
}

// LogEntry 日志条目
type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// App 应用主结构
//
// mu 保护下列可变字段：cmd / running / ready / webURL / lastError /
// startTime / exited / cancelFunc / savedProxy*。
// 注意：持有 mu 时不要调用 addLog 或 EventsEmit，避免与主线程互等。
type App struct {
	ctx        context.Context
	mu         sync.Mutex
	cmd        *exec.Cmd
	running    bool
	ready      bool
	webURL     string
	lastError  string
	startTime  time.Time
	exited     chan struct{}
	cancelFunc context.CancelFunc

	logs  []LogEntry
	logMu sync.Mutex

	configPath string
	exePath    string
	workDir    string

	// 代理状态保存（启动前保存，停止后恢复）
	savedProxyEnabled bool
	savedProxyServer  string
	savedProxyDirty   bool
}

// 原工具就绪时会往 stdout 打印这一行，可直接拿到真实监听地址
var readyLineRe = regexp.MustCompile(`API service started successfully, address:\s*(\S+)`)

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

	// 开发模式回退：wails dev 时 bin/ 下可能没有原工具，
	// 从 exe 目录逐级向上找项目里的 opensource/wx-channels-source。
	if _, err := os.Stat(a.exePath); os.IsNotExist(err) {
		name := "wx_video_download"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		dir := a.workDir
		for depth := 0; depth < 4; depth++ {
			cand := filepath.Join(dir, "opensource", "wx-channels-source", name)
			if _, e := os.Stat(cand); e == nil {
				a.exePath = cand
				a.workDir = filepath.Dir(cand)
				a.configPath = filepath.Join(a.workDir, "config.yaml")
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
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
	_ = a.StopService()
}

func (a *App) beforeClose(ctx context.Context) bool {
	a.mu.Lock()
	running := a.running
	a.mu.Unlock()
	if running {
		_ = a.StopService()
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
	doc := parseYAML(string(data))
	s.ProxyPort = doc.getInt("proxy.port", s.ProxyPort)
	s.APIPort = doc.getInt("api.port", s.APIPort)
	s.MaxRunning = doc.getInt("download.maxRunning", s.MaxRunning)
	s.DownloadDir = doc.getString("download.dir", s.DownloadDir)
	s.UpstreamProxy = doc.getString("proxy.upstreamProxy", s.UpstreamProxy)
	s.DefaultHighest = doc.getBool("channels.download.defaultHighest", s.DefaultHighest)
	s.DownloadCover = doc.getBool("channels.download.cover", s.DownloadCover)
	s.PlayDoneAudio = doc.getBool("download.playDoneAudio", s.PlayDoneAudio)
	return s
}

// SaveSettings 保存配置
func (a *App) SaveSettings(s Settings) error {
	a.mu.Lock()
	running := a.running
	a.mu.Unlock()
	if running {
		return fmt.Errorf("服务运行中，请先停止")
	}
	if err := validatePort("代理端口", s.ProxyPort); err != nil {
		return err
	}
	if err := validatePort("API 端口", s.APIPort); err != nil {
		return err
	}
	if s.MaxRunning < 1 || s.MaxRunning > 10 {
		return fmt.Errorf("最大同时下载需在 1-10 之间")
	}

	data, err := os.ReadFile(a.configPath)
	if err != nil {
		return fmt.Errorf("读取配置失败: %w", err)
	}
	doc := parseYAML(string(data))
	doc.set("proxy.port", strconv.Itoa(s.ProxyPort))
	doc.set("api.port", strconv.Itoa(s.APIPort))
	doc.set("download.maxRunning", strconv.Itoa(s.MaxRunning))
	doc.set("download.dir", quoteYAML(s.DownloadDir))
	doc.set("proxy.upstreamProxy", quoteYAML(s.UpstreamProxy))
	doc.set("channels.download.defaultHighest", strconv.FormatBool(s.DefaultHighest))
	doc.set("channels.download.cover", strconv.FormatBool(s.DownloadCover))
	doc.set("download.playDoneAudio", strconv.FormatBool(s.PlayDoneAudio))

	if err := writeFileAtomic(a.configPath, []byte(doc.String())); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}

	// 回读校验，避免再次出现「提示已保存但其实没写进去」
	check := a.GetSettings()
	if check.ProxyPort != s.ProxyPort || check.APIPort != s.APIPort ||
		check.MaxRunning != s.MaxRunning || check.DownloadDir != s.DownloadDir ||
		check.UpstreamProxy != s.UpstreamProxy || check.DefaultHighest != s.DefaultHighest ||
		check.DownloadCover != s.DownloadCover || check.PlayDoneAudio != s.PlayDoneAudio {
		return fmt.Errorf("配置写入后校验失败，请检查 %s 的格式", a.configPath)
	}

	a.addLog("INFO", "配置已保存")
	return nil
}

func validatePort(name string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s 需在 1-65535 之间", name)
	}
	return nil
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SelectDirectory 目录选择对话框
func (a *App) SelectDirectory(title string) (string, error) {
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: title})
}

// StartService 启动服务
func (a *App) StartService() error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("服务已在运行")
	}
	if _, err := os.Stat(a.exePath); os.IsNotExist(err) {
		a.mu.Unlock()
		return fmt.Errorf("未找到下载器主程序: %s", a.exePath)
	}

	// 无论原本有没有代理都记录一次，停止时原样写回
	st := GetSystemProxy()
	a.savedProxyEnabled = st.Enabled
	a.savedProxyServer = st.Server
	a.savedProxyDirty = true

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, a.exePath)
	cmd.Dir = a.workDir
	cmd.SysProcAttr = hideConsole()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		a.mu.Unlock()
		cancel()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		a.mu.Unlock()
		cancel()
		return err
	}
	if err := cmd.Start(); err != nil {
		a.mu.Unlock()
		cancel()
		return fmt.Errorf("启动失败: %w", err)
	}

	a.cmd = cmd
	a.cancelFunc = cancel
	a.running = true
	a.ready = false
	a.webURL = ""
	a.lastError = ""
	a.startTime = time.Now()
	exited := make(chan struct{})
	a.exited = exited
	a.mu.Unlock()

	apiPort := a.GetSettings().APIPort
	a.addLog("INFO", fmt.Sprintf("已保存当前系统代理: %q (启用: %v)", st.Server, st.Enabled))
	a.addLog("INFO", "服务启动中...")

	go a.readPipe(stdout, "INFO")
	go a.readPipe(stderr, "WARN")

	// 唯一的 Wait 调用点：负责收尾、恢复代理并广播状态
	go func() {
		err := cmd.Wait()
		close(exited)

		a.mu.Lock()
		a.running = false
		a.ready = false
		a.webURL = ""
		if err != nil {
			a.lastError = err.Error()
		}
		a.mu.Unlock()

		if err != nil {
			a.addLog("ERROR", fmt.Sprintf("进程退出: %v", err))
		} else {
			a.addLog("INFO", "服务已停止")
		}
		a.restoreProxy()
		wailsruntime.EventsEmit(a.ctx, "status-changed", a.GetStatus())
	}()

	go a.waitReady(ctx, apiPort)
	return nil
}

// waitReady 轮询 API 端口直到返回 200（实测首次启动约需 4-5 秒）
func (a *App) waitReady(ctx context.Context, apiPort int) {
	url := fmt.Sprintf("http://127.0.0.1:%d/", apiPort)
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(60 * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}

		a.mu.Lock()
		running := a.running
		a.mu.Unlock()
		if !running {
			return
		}

		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			a.markReady(url)
			return
		}
	}

	a.mu.Lock()
	running := a.running
	a.mu.Unlock()
	if running {
		a.addLog("WARN", fmt.Sprintf("等待 %s 就绪超时，请查看日志或调整 API 端口", url))
		wailsruntime.EventsEmit(a.ctx, "status-changed", a.GetStatus())
	}
}

func (a *App) markReady(webURL string) {
	a.mu.Lock()
	if !a.running || a.ready {
		a.mu.Unlock()
		return
	}
	a.ready = true
	a.webURL = strings.TrimRight(webURL, "/")
	url := a.webURL
	a.mu.Unlock()

	a.addLog("INFO", "✓ 服务已就绪: "+url)
	wailsruntime.EventsEmit(a.ctx, "status-changed", a.GetStatus())
}

// StopService 停止服务
func (a *App) StopService() error {
	a.mu.Lock()
	if !a.running || a.cmd == nil {
		a.mu.Unlock()
		return fmt.Errorf("服务未运行")
	}
	cmd := a.cmd
	exited := a.exited
	cancel := a.cancelFunc
	a.mu.Unlock()

	a.addLog("INFO", "正在停止服务...")
	if cancel != nil {
		cancel()
	}

	if exited != nil {
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			select {
			case <-exited:
			case <-time.After(3 * time.Second):
			}
		}
	}

	// 收尾由 Wait goroutine 负责，这里同步一次状态，保证界面立刻刷新
	a.mu.Lock()
	a.running = false
	a.ready = false
	a.webURL = ""
	a.mu.Unlock()

	a.restoreProxy()
	a.addLog("INFO", "服务已停止")
	wailsruntime.EventsEmit(a.ctx, "status-changed", a.GetStatus())
	return nil
}

// restoreProxy 把启动前记录的系统代理原样写回（只执行一次）
func (a *App) restoreProxy() {
	a.mu.Lock()
	if !a.savedProxyDirty {
		a.mu.Unlock()
		return
	}
	state := ProxyState{Enabled: a.savedProxyEnabled, Server: a.savedProxyServer}
	a.savedProxyDirty = false
	a.mu.Unlock()

	if err := SetSystemProxy(state); err != nil {
		a.addLog("WARN", fmt.Sprintf("恢复系统代理失败: %v", err))
		return
	}
	a.addLog("INFO", fmt.Sprintf("已恢复系统代理: %q (启用: %v)", state.Server, state.Enabled))
}

// GetStatus 获取状态
func (a *App) GetStatus() Status {
	a.mu.Lock()
	running, ready, webURL, lastError, startTime := a.running, a.ready, a.webURL, a.lastError, a.startTime
	a.mu.Unlock()

	s := a.GetSettings()
	uptime := ""
	if running {
		uptime = time.Since(startTime).Round(time.Second).String()
	}
	if !ready {
		webURL = ""
	}
	if !running {
		lastError = ""
	}
	return Status{
		Running:     running,
		Ready:       ready,
		WebURL:      webURL,
		ProxyPort:   s.ProxyPort,
		APIPort:     s.APIPort,
		Uptime:      uptime,
		DownloadDir: s.DownloadDir,
		LastError:   lastError,
	}
}

// CheckReady 主动探测一次，供界面「重试」使用
func (a *App) CheckReady() bool {
	a.mu.Lock()
	running, ready := a.running, a.ready
	a.mu.Unlock()
	if !running {
		return false
	}
	if ready {
		return true
	}
	go a.waitReady(context.Background(), a.GetSettings().APIPort)
	return false
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
	s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		a.addLog(level, line)
		if m := readyLineRe.FindStringSubmatch(line); m != nil {
			a.markReady(m[1])
		}
	}
}
