@echo off
chcp 65001 >nul 2>&1
title 微信视频号下载器 - 构建

echo ========================================
echo   构建微信视频号下载器
echo ========================================
echo.

set PATH=%PATH%;C:\Program Files\Go\bin;%USERPROFILE%\go\bin

cd /d "%~dp0"

echo [1/2] 构建应用...
wails build
if %errorlevel% neq 0 (
    echo [错误] 构建失败！
    pause
    exit /b 1
)

echo.
echo [2/2] 复制原工具...
if not exist "build\bin\bin" mkdir "build\bin\bin"

rem 原工具来自 opensource\wx-channels-source（该目录不提交，需自行 clone / 更新）
set "SRC=%~dp0opensource\wx-channels-source"
set "TOOL="
if exist "%SRC%\wx_video_download.exe" set "TOOL=%SRC%\wx_video_download.exe"
if not defined TOOL if exist "%SRC%\build\wx_channel.exe" set "TOOL=%SRC%\build\wx_channel.exe"
if not defined TOOL if exist "%SRC%\build\wx_video_download_windows_x86_64.exe" set "TOOL=%SRC%\build\wx_video_download_windows_x86_64.exe"

if defined TOOL (
    copy /Y "%TOOL%" "build\bin\bin\wx_video_download.exe" >nul
    echo   已从 "%TOOL%" 更新原工具
) else (
    echo   [提示] 未在 opensource\wx-channels-source 下找到已编译的原工具
    echo          先在 opensource\wx-channels-source 执行 build\build-go.bat 生成，
    echo          或手动把 wx_video_download.exe 放进 build\bin\bin\
    if not exist "build\bin\bin\wx_video_download.exe" (
        echo   [警告] build\bin\bin 里也没有原工具，启动服务会失败
    )
)

if not exist "build\bin\bin\config.yaml" (
    echo   [提示] build\bin\bin\config.yaml 不存在，首次运行原工具时会自动生成
)

echo.
echo ========================================
echo   构建完成！
echo   产物: build\bin\wx-channels-download.exe
echo ========================================
echo.
pause
