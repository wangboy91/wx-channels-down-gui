# 微信视频号下载器 GUI

基于 [ltaoo/wx_channels_download](https://github.com/ltaoo/wx_channels_download) 封装的跨平台桌面应用。

## 功能

- 🎯 一键启动/停止代理服务
- ⚙️ 可视化配置（代理端口、下载路径、画质等）
- 📥 下载任务管理与进度显示
- 📋 实时日志查看与搜索
- 🔗 内容抓取页面快捷入口
- 🌐 支持上游代理（Clash 等）

## 技术栈

- **后端**: Go + [Wails v2](https://wails.io)
- **前端**: Vue 3 + TypeScript + Vite
- **核心**: ltaoo/wx_channels_download（子进程模式）

## 开发环境

### 依赖

- Go ≥ 1.21
- Node.js ≥ 18
- Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- 系统检查: `wails doctor`

### 开发模式

```bash
wails dev
```

热重载，前端修改即时生效。

### 构建

```bash
# Windows
wails build

# Windows 安装包（需要安装 NSIS）
wails build -nsis

# macOS（需在 Mac 上构建）
wails build -platform darwin/universal
```

构建产物在 `build/bin/` 目录。

## 目录结构

```
wx-channels-down/
├── main.go                 # Wails 入口
├── app.go                  # Go 后端（子进程管理、就绪探测、配置读写、API 调用）
├── config_yaml.go          # 保留格式的 config.yaml 读写（支持缩进嵌套键）
├── config_yaml_test.go     # 配置读写单元测试
├── app_windows.go          # Windows 平台特定代码（系统代理）
├── app_other.go            # macOS/Linux 平台代码
├── build.bat / dev.bat     # 一键构建 / 开发模式
├── wails.json              # Wails 项目配置
├── go.mod / go.sum
├── frontend/               # Vue 3 前端
│   └── src/
│       ├── App.vue         # 全部界面（工具栏 + 设置面板 + 内嵌 Web UI）
│       └── style.css
├── opensource/             # 依赖的开源项目（不提交，见下方说明）
│   └── wx-channels-source/ # ltaoo/wx_channels_download 源码
└── build/
    └── bin/
        ├── wx-channels-download.exe   # 主程序
        └── bin/
            ├── wx_video_download.exe  # 原工具（子进程）
            └── config.yaml            # 原工具配置
```

### 关于 opensource/

`opensource/` 放的是依赖的开源项目，**不进本仓库的版本控制**（已在 `.gitignore` 中忽略）。
它保留自己的 `.git`，可以独立更新：

```bash
cd opensource/wx-channels-source
git pull                      # 拉取上游更新
git remote -v                 # 上游为 https://github.com/ltaoo/wx_channels_download.git
```

`build.bat` 会从该目录寻找已编译的原工具；找不到时会提示先编译，并保留 `build/bin/bin/`
中已有的版本，不会中断构建。

## 使用说明

### 首次使用

1. 运行 `wx-channels-download.exe`
2. 进入「设置」页面，配置：
   - **代理端口**（默认 2023）
   - **下载目录**
   - **上游代理**（如 `http://127.0.0.1:7890`，配合 Clash 使用）
3. 点击「▶ 启动服务」
4. 首次运行会自动安装 CA 证书（需管理员权限）
5. 打开微信 PC 端 → 视频号 → 出现下载按钮

> **启动需要等待几秒。** 原工具要初始化数据库、证书和代理，实测约 4-5 秒后 Web UI 才可用。
> 界面会显示「启动中」，等就绪后才加载内嵌页面；若长时间停在启动中，可点「手动重试」，
> 加载完成后也可用右下角「重新加载」刷新内嵌页面。

### 开发自检

```bash
go test ./...    # config_yaml.go 的读写单元测试
```

改动前端后建议手工确认：启动服务时页面应停留在「服务启动中…」，等原工具就绪后才出现内嵌页面，
而不是直接显示「127.0.0.1 拒绝连接」。

### 配置说明

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| 代理端口 | 工具代理监听端口 | 2023 |
| API 端口 | 管理接口端口 | 2022 |
| 下载目录 | 视频保存位置 | 用户下载目录 |
| 上游代理 | Clash 等代理地址 | 空（直连） |
| 最大同时下载 | 并发下载数 | 3 |
| 默认最高画质 | 是否下载最高画质 | 否 |
| 下载封面 | 同时保存视频封面 | 否 |
| 提示音 | 下载完成播放提示音 | 是 |

## 分发打包

### Windows 安装包

1. 安装 [NSIS](https://nsis.sourceforge.io/Download)
2. 运行 `wails build -nsis`
3. 生成的安装包在 `build/bin/`

### macOS 应用

在 Mac 上：
```bash
wails build -platform darwin/universal
```

生成 `.app` 应用包。

## 更新原工具

1. 下载最新版 [wx_channels_download](https://github.com/ltaoo/wx_channels_download/releases)
2. 替换 `build/bin/bin/wx_video_download.exe`
3. 重新打包分发

## 许可

原工具遵循 [MIT License](https://github.com/ltaoo/wx_channels_download/blob/main/LICENSE)。
本封装仅用于学习交流。
