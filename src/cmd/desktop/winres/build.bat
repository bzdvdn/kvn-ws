@echo off
REM Generate .syso for UAC manifest embedding
REM Requires: go install github.com/akavel/rsrc@latest
rsrc -manifest kvn-desktop.exe.manifest -ico kvn.ico -o kvn-desktop.syso
