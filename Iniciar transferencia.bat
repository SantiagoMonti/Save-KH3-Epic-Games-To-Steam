@echo off
setlocal
cd /d "%~dp0"
if not exist "%~dp0kh3save.exe" (
  echo No se encontro kh3save.exe junto a este iniciador.
  echo Compila primero con Go 1.26 o descarga el codigo fuente completo.
  pause
  exit /b 1
)
"%~dp0kh3save.exe" transfer
pause
