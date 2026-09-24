@echo off
chcp 65001 >nul 2>&1
title 微信视频号下载器 - 开发模式

echo ========================================
echo   微信视频号下载器 - 开发模式
echo ========================================
echo.

set PATH=%PATH%;C:\Program Files\Go\bin;%USERPROFILE%\go\bin

cd /d "%~dp0"
wails dev

pause
