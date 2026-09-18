@echo off
chcp 65001 >nul
title MyAI Mobile UI Prototype Preview Launcher

echo ========================================================
echo          MyAI Mobile Android UI 原型系统启动器
echo ========================================================
echo.
echo 正在为您启动原型本地预览环境...
echo.

cd /d "%~dp0"

:: 检查是否存在 python
python --version >nul 2>&1
if %errorlevel% equ 0 (
    echo [OK] 检测到 Python，正在启动本地静态服务器 (端口 8099)...
    start "" http://localhost:8099/index.html
    python -m http.server 8099
    goto end
)

:: 检查是否存在 npx / node
npx --version >nul 2>&1
if %errorlevel% equ 0 (
    echo [OK] 检测到 Node/NPX，正在启动 serve 本地服务器...
    start "" http://localhost:3000/index.html
    npx -y serve . -p 3000
    goto end
)

:: 如果均未安装，则直接使用默认浏览器打开 index.html
echo [提示] 未检测到 Python 或 Node 环境，正在直接在默认浏览器中打开...
start "" "%~dp0index.html"

:end
pause
