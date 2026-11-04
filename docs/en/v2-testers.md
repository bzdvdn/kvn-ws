# KVN 2.0 — tester guide

This is a closed-group release candidate. v1 and v2 are not compatible; use only 2.0.x APK/binaries.

## Requirements

- Android 8.0+ (minSdk 26).
- APK from the release candidate.
- Server address and token from the administrator.
- For a production domain with a Let's Encrypt certificate, the certificate is verified; enable `Allow insecure TLS` only for a self-signed lab server.

## Android setup

1. Install the APK.
2. Settings → enable Allow insecure TLS (test server only).
3. Add a server: `https://HOST:443` + token, or scan the kvn-web QR.
4. Connect.
5. Test YouTube/sites, `ping`, Wi-Fi ↔ LTE switching, reconnect.
6. Configure included/excluded apps and Direct domains (`.ru`) if needed.
7. Open the server website in a browser: `https://HOST/` should show the streaming site, `/login.html` a sign-in form, and a random path a 404.

## Checklist

- [ ] YouTube and regular sites work.
- [ ] `ping 1.1.1.1` works; `traceroute` may not.
- [ ] `.ru` goes direct, the rest goes through the tunnel.
- [ ] Disconnect/reconnect leaves no stuck VPN.
- [ ] Connection recovers after phone reboot.
- [ ] kvn-web QR imports into Android.
- [ ] Multiple servers switch without reinstall.
- [ ] `https://HOST/` serves a site, `/login.html` a form, a random path a 404.
- [ ] `curl --http1.1 https://HOST/` works (not h2-only).

## Reporting issues

Include:

- phone model and Android version;
- server name (not token);
- Settings → Diagnostics → Log level `debug`;
- kvn-web logs (if available) or Android logcat;
- timestamp and reproduction steps.

Never post tokens, QR codes, or full logs in public chats.
