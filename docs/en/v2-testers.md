# KVN 2.0 — tester guide

This is a closed-group release candidate. v1 and v2 are not compatible; use only 2.0.0 APK/binaries.

## Requirements

- Android 8.0+ (minSdk 26).
- APK from the release candidate.
- Server address and token from the administrator.
- The server uses a self-signed certificate by default; enable `Allow insecure TLS` in Settings. There is no shared default key: the installer generates a separate key per server.

## Android setup

1. Install the APK.
2. Settings → enable Allow insecure TLS (test server only).
3. Add a server: `https://HOST:443` + token, or scan the kvn-web QR.
4. Connect.
5. Test YouTube/sites, `ping`, Wi-Fi ↔ LTE switching, reconnect.
6. Configure included/excluded apps and Direct domains (`.ru`) if needed.

## Checklist

- [ ] YouTube and regular sites work.
- [ ] `ping 1.1.1.1` works; `traceroute` may not.
- [ ] `.ru` goes direct, the rest goes through the tunnel.
- [ ] Disconnect/reconnect leaves no stuck VPN.
- [ ] Connection recovers after phone reboot.
- [ ] kvn-web QR imports into Android.
- [ ] Multiple servers switch without reinstall.

## Reporting issues

Include:

- phone model and Android version;
- server name (not token);
- Settings → Diagnostics → Log level `debug`;
- kvn-web logs (if available) or Android logcat;
- timestamp and reproduction steps.

Never post tokens, QR codes, or full logs in public chats.
