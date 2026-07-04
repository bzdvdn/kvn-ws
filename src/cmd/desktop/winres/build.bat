@echo off
REM Generate .syso for UAC manifest embedding
REM Requires: go install github.com/akavel/rsrc@latest
rsrc -manifest kvn-desktop.exe.manifest -o kvn-desktop.syso
