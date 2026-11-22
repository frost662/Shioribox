@echo off
setlocal
rem Builds output\ShioriBox-Setup-<version>.exe and output\ShioriBox-Update-<version>.exe with Inno Setup 6 (free: https://jrsoftware.org/isdl.php)
set "ISCC="
for %%P in ("%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe" "%ProgramFiles%\Inno Setup 6\ISCC.exe" "%LocalAppData%\Programs\Inno Setup 6\ISCC.exe") do if exist %%P set "ISCC=%%~P"
if not defined ISCC (
  echo Inno Setup 6 was not found. Install it from https://jrsoftware.org/isdl.php and run this again.
  pause
  exit /b 1
)
cd /d "%~dp0"
"%ISCC%" ShioriBox.iss
if errorlevel 1 (pause & exit /b 1)
"%ISCC%" /DUPDATE ShioriBox.iss
if errorlevel 1 (pause & exit /b 1)
echo.
echo Done. ShioriBox-Setup (new install) and ShioriBox-Update (existing install) are in: %~dp0output
explorer "%~dp0output"
