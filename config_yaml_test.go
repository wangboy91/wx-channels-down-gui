package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleConfig = `debug:
  error: false

download:
  filenameTemplate: "{{filename}}_{{spec}}"
  dir: "D:/videos/wx_channels"
  playDoneAudio: true
  maxRunning: 3
  connectionConcurrency: 0 # 0 derives resourceConcurrency * segmentConcurrency

api:
  protocol: "http"
  hostname: "127.0.0.1"
  port: 2022

proxy:
  enabled: true
  system: true
  hostname: "127.0.0.1"
  port: 2023
  upstreamProxy: "http://127.0.0.1:7890"

update:
  sources:
    - type: github
      priority: 1
      enabled: true

channels:
  disableLocationToHome: false
  download:
    defaultHighest: false
    cover: false
`

func TestNestedRead(t *testing.T) {
	doc := parseYAML(sampleConfig)

	cases := []struct {
		path string
		want string
	}{
		{"download.dir", "D:/videos/wx_channels"},
		{"download.filenameTemplate", "{{filename}}_{{spec}}"},
		{"api.port", "2022"},
		{"proxy.port", "2023"},
		{"proxy.upstreamProxy", "http://127.0.0.1:7890"},
		{"channels.download.cover", "false"},
		{"channels.download.defaultHighest", "false"},
		{"debug.error", "false"},
	}
	for _, c := range cases {
		if got := doc.getString(c.path, "<missing>"); got != c.want {
			t.Errorf("getString(%q) = %q, want %q", c.path, got, c.want)
		}
	}

	if got := doc.getInt("proxy.port", 0); got != 2023 {
		t.Errorf("getInt(proxy.port) = %d, want 2023", got)
	}
	// 带行尾注释的数字也要能解析
	if got := doc.getInt("download.connectionConcurrency", -1); got != 0 {
		t.Errorf("getInt(download.connectionConcurrency) = %d, want 0", got)
	}
	if got := doc.getBool("download.playDoneAudio", false); got != true {
		t.Errorf("getBool(download.playDoneAudio) = %v, want true", got)
	}
	if got := doc.getBool("channels.download.cover", true); got != false {
		t.Errorf("getBool(channels.download.cover) = %v, want false", got)
	}
	// 列表项不能被当成映射键
	if _, ok := doc.get("sources.type"); ok {
		t.Errorf("列表项不应被解析为键路径")
	}
}

func TestDefaultsWhenMissing(t *testing.T) {
	doc := parseYAML(sampleConfig)
	if got := doc.getInt("proxy.missing", 2023); got != 2023 {
		t.Errorf("缺失键应返回默认值, got %d", got)
	}
	if got := doc.getBool("download.playDoneAudio2", true); got != true {
		t.Errorf("缺失键应返回默认值, got %v", got)
	}
	// 键存在但为空字符串时，应返回空串而不是默认值
	doc2 := parseYAML("download:\n  dir: \"\"\n")
	if got := doc2.getString("download.dir", "fallback"); got != "" {
		t.Errorf("键存在但为空时应返回空串, got %q", got)
	}
}

func TestSetRoundTrip(t *testing.T) {
	doc := parseYAML(sampleConfig)
	doc.set("download.dir", quoteYAML("D:/videos/new"))
	doc.set("proxy.port", "12023")
	doc.set("channels.download.cover", "true")
	doc.set("download.playDoneAudio", "false")

	out := doc.String()
	if !strings.Contains(out, `  dir: "D:/videos/new"`) {
		t.Errorf("替换未生效:\n%s", out)
	}
	if !strings.Contains(out, "  port: 12023") {
		t.Errorf("proxy.port 替换未生效:\n%s", out)
	}
	// 注释与其它行必须保留
	if !strings.Contains(out, "# 0 derives resourceConcurrency * segmentConcurrency") {
		t.Errorf("注释丢失:\n%s", out)
	}
	if !strings.Contains(out, "    - type: github") {
		t.Errorf("列表项丢失:\n%s", out)
	}

	again := parseYAML(out)
	if got := again.getString("download.dir", ""); got != "D:/videos/new" {
		t.Errorf("回读 download.dir = %q", got)
	}
	if got := again.getInt("proxy.port", 0); got != 12023 {
		t.Errorf("回读 proxy.port = %d", got)
	}
	if got := again.getBool("channels.download.cover", false); got != true {
		t.Errorf("回读 channels.download.cover = %v", got)
	}
	if got := again.getBool("download.playDoneAudio", true); got != false {
		t.Errorf("回读 download.playDoneAudio = %v", got)
	}
}

func TestSetCreatesMissingKeys(t *testing.T) {
	doc := parseYAML(sampleConfig)
	doc.set("proxy.newKey", `"x"`)
	doc.set("brand.new.section", "1")

	out := doc.String()
	again := parseYAML(out)
	if got := again.getString("proxy.newKey", ""); got != "x" {
		t.Errorf("新增键回读失败: %q\n%s", got, out)
	}
	if got := again.getInt("brand.new.section", 0); got != 1 {
		t.Errorf("新增嵌套键回读失败: %d\n%s", got, out)
	}
	// 新键应落在正确的父块内（缩进 2）
	if !strings.Contains(out, "  newKey: \"x\"") {
		t.Errorf("新键缩进不正确:\n%s", out)
	}
}

// TestWindowsPathRoundTrip Windows 目录选择对话框返回的是反斜杠路径，
// 写回 YAML 时必须转义，否则会被当成转义序列解析坏掉。
func TestWindowsPathRoundTrip(t *testing.T) {
	doc := parseYAML(sampleConfig)
	doc.set("download.dir", quoteYAML(`D:\videos\wx_channels`))
	out := doc.String()

	if !strings.Contains(out, `  dir: "D:\\videos\\wx_channels"`) {
		t.Errorf("反斜杠未转义:\n%s", out)
	}
	if got := parseYAML(out).getString("download.dir", ""); got != `D:\videos\wx_channels` {
		t.Errorf("回读 = %q, want %q", got, `D:\videos\wx_channels`)
	}
}

// TestRealConfigFile 在构建产物存在时直接验证真实配置文件
func TestRealConfigFile(t *testing.T) {
	path := filepath.Join("build", "bin", "bin", "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("未找到真实 config.yaml，跳过")
	}
	doc := parseYAML(string(data))
	if got := doc.getString("download.dir", ""); got == "" {
		t.Errorf("真实配置里 download.dir 不应为空")
	}
	if got := doc.getInt("api.port", 0); got != 2022 {
		t.Errorf("真实配置里 api.port = %d, want 2022", got)
	}
	if got := doc.getInt("proxy.port", 0); got != 2023 {
		t.Errorf("真实配置里 proxy.port = %d, want 2023", got)
	}
	if got := doc.getInt("download.maxRunning", 0); got != 3 {
		t.Errorf("真实配置里 download.maxRunning = %d, want 3", got)
	}
	if !strings.HasPrefix(doc.getString("proxy.upstreamProxy", ""), "http") {
		t.Errorf("真实配置里 proxy.upstreamProxy 应以 http 开头")
	}
	// 写回后必须与原文件语义一致（不丢注释、不丢行）
	out := doc.String()
	if strings.Count(out, "\n") != strings.Count(string(data), "\n") {
		t.Errorf("无修改写回后行数发生变化: %d -> %d",
			strings.Count(string(data), "\n"), strings.Count(out, "\n"))
	}
}
