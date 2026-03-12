@echo off
powershell -NoProfile -ExecutionPolicy Bypass -Command "& '%~dp0build.ps1'; & '%~dp0debug-screenshot.ps1'"
