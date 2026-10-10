@echo off
setlocal EnableExtensions EnableDelayedExpansion
title Survey Flow - Testlink runner
cd /d "%~dp0"

rem =====================================================================
rem  Survey Flow - run a survey TEST link
rem
rem  1. paste the TEST link          2. number of completes
rem  3. type "pop" or "nopop"        4. choose a folder for the results
rem
rem  A folder named with the PID taken from the link is created and holds
rem  the memory file (learns screen-outs and validation rules over runs).
rem
rem  USE ONLY ON TEST LINKS - never on live survey fielding.
rem =====================================================================

rem ---- where the tool file may live (this folder first) ---------------
set "TOOL="
if exist "%~dp0SurveyFlow.pyw" set "TOOL=%~dp0SurveyFlow.pyw"
if not defined TOOL if exist "%~dp0dist\SurveyFlow.pyw" set "TOOL=%~dp0dist\SurveyFlow.pyw"
if not defined TOOL if exist "%LOCALAPPDATA%\SurveyFlow\SurveyFlow.pyw" set "TOOL=%LOCALAPPDATA%\SurveyFlow\SurveyFlow.pyw"

rem ---- if the tool file is missing, download it -----------------------
if not defined TOOL (
    echo Downloading the Survey Flow tool, please wait...
    if not exist "%LOCALAPPDATA%\SurveyFlow" mkdir "%LOCALAPPDATA%\SurveyFlow"
    set "DEST=%LOCALAPPDATA%\SurveyFlow\SurveyFlow.pyw"
    powershell -NoProfile -ExecutionPolicy Bypass -Command "$urls = @('https://github.com/Kotaru27/Survey-flow/releases/download/v1.0.0/SurveyFlow.pyw','https://raw.githubusercontent.com/Kotaru27/Survey-flow/main/dist/SurveyFlow.pyw','https://raw.githubusercontent.com/Kotaru27/Survey-flow/arena/7c58db95-survey-flow/dist/SurveyFlow.pyw'); foreach ($u in $urls) { try { Invoke-WebRequest -Uri $u -OutFile '!DEST!' -UseBasicParsing; if ((Get-Item '!DEST!').Length -gt 100000) { exit 0 } } catch {} }; exit 1" >nul 2>nul
    if not exist "!DEST!" (
        curl -L -s -o "!DEST!" "https://raw.githubusercontent.com/Kotaru27/Survey-flow/arena/7c58db95-survey-flow/dist/SurveyFlow.pyw" >nul 2>nul
    )
    if exist "!DEST!" for %%S in ("!DEST!") do if %%~zS LSS 100000 ( del "!DEST!" >nul 2>nul )
    if not exist "!DEST!" (
        echo.
        echo ERROR: could not download the tool. Check internet access to github.com,
        echo or copy the file SurveyFlow.pyw into this folder and run again.
        echo.
        pause
        exit /b 1
    )
    set "TOOL=!DEST!"
)

rem ---- find python ----------------------------------------------------
set "PYW="
where pythonw >nul 2>nul && set "PYW=pythonw"
if not defined PYW where py >nul 2>nul && set "PYW=py -3w"
if not defined PYW where python >nul 2>nul && set "PYW=python"
if not defined PYW (
    echo.
    echo ERROR: Python 3 was not found on this computer.
    echo Install it from https://www.python.org/downloads/
    echo and tick "Add Python to PATH" during setup.
    echo.
    pause
    exit /b 1
)

echo.
echo ====================  Survey Flow - test runner  ====================
echo.
echo Paste the survey TEST link and press Enter.
echo.

set "LINK="
set /p "LINK=Testlink: "
if not defined LINK (
    echo ERROR: no link entered.
    pause
    exit /b 1
)

set "NUM="
set /p "NUM=No of completes: "
set "NUM=!NUM: =!"
if not defined NUM (
    echo ERROR: number of completes is required.
    pause
    exit /b 1
)
for /f "delims=0123456789" %%A in ("!NUM!") do set "NUM=BAD"
if "!NUM!"=="BAD" (
    echo ERROR: "no of completes" must be a number.
    pause
    exit /b 1
)

echo.
echo Type "pop"      to open a browser popup and show the test run live.
echo Type "nopop"    to run in the background without any popup.
set "MODE="
set /p "MODE=pop or nopop: "
if /i "!MODE!"=="pop" (
    set "MODE=pop"
) else (
    set "MODE=nopop"
)

set "FOLDER="
set /p "FOLDER=Folder for results [Enter = %USERPROFILE%\SurveyFlow-results]: "
if not defined FOLDER set "FOLDER=%USERPROFILE%\SurveyFlow-results"

echo.
echo Starting the test run...
echo    link      : !LINK!
echo    completes : !NUM!
echo    mode      : !MODE!
echo    folder    : !FOLDER!
echo.
echo A sub-folder named with the PID from your link will be created there.
echo It holds the memory file that avoids screen-outs and validation errors.
echo.

if /i "!MODE!"=="pop" (
    if /i "!PYW!"=="python" (
        start "Survey Flow" /min python "!TOOL!" launch "!LINK!" --completes !NUM! --mode pop --folder "!FOLDER!" --allow-live
    ) else (
        start "Survey Flow" !PYW! "!TOOL!" launch "!LINK!" --completes !NUM! --mode pop --folder "!FOLDER!" --allow-live
    )
    echo A browser popup with the live test run should open now.
) else (
    if /i "!PYW!"=="python" (
        start "Survey Flow" /min python "!TOOL!" launch "!LINK!" --completes !NUM! --mode nopop --folder "!FOLDER!" --allow-live
    ) else (
        start "Survey Flow" !PYW! "!TOOL!" launch "!LINK!" --completes !NUM! --mode nopop --folder "!FOLDER!" --allow-live
    )
    echo Running in the background - no popup will be opened.
    echo A message will appear when the run finishes.
)

echo.
echo When finished, see status.txt in the PID folder for the result.
echo =====================================================================
echo.
pause
endlocal
