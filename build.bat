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
copy /Y "D:\tools\wx_channels_download\wx_video_download.exe" "build\bin\bin\" >nul
copy /Y "D:\tools\wx_channels_download\config.yaml" "build\bin\bin\" >nul

echo.
echo ========================================
echo   构建完成！
echo   产物: build\bin\wx-channels-download.exe
echo ========================================
echo.
pause
