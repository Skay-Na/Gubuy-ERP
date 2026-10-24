@echo off
rem Gubuy-ERP 一键构建并更新软路由（Windows 版，见 scripts\build-deploy-router.ps1）
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0build-deploy-router.ps1" %*
