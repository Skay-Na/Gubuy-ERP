@echo off
rem Gubuy-ERP Windows dev stack entry (see scripts\dev.ps1 for lifecycle commands)
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0dev.ps1" %*
