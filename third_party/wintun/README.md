# Wintun (prebuilt binaries)

Версия: **0.14.1**

Источник: https://www.wintun.net/builds/wintun-0.14.1.zip

Содержимое:
- `wintun.dll` — amd64 (Windows-релиз kvn-ws собирается только под `windows/amd64`).
- `arm64/wintun.dll` — arm64 (для ручной установки на ARM-машинах).
- `LICENSE.txt` — лицензия предсобранных бинарников с wintun.net (пермиссивная).

Использование:
- Копируется в каталог установки (`wintun.dll` рядом с `kvn-web.exe` / `kvn-client.exe`) инсталляторами `install-client.ps1` / `install-web.ps1`.
- Включается в Windows-релизные архивы (см. `release` job в `.github/workflows/ci.yml`).

Обновление: скачать новый `wintun-<ver>.zip`, заменить `wintun.dll` (amd64/arm64) и поднять версию в `scripts/install-client.ps1` / `scripts/install-web.ps1` (`$WintunVersion`) и здесь.