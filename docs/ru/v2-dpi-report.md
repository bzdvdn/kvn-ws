# v2 DPI/active-probe отчёт

- Дата: 2026-10-28
- Target: `88.210.63.157:443`, SNI `131242.koara.live`
- Network: `lab`, region: `local`
- Скрипт: `scripts/dpi-check.sh`

## Результат

```text
$ DPI_NETWORK=lab DPI_REGION=local DPI_INSECURE=1 ./scripts/dpi-check.sh 88.210.63.157 131242.koara.live
target: 88.210.63.157:443 sni=131242.koara.live
network: lab region: local
[tls]
Protocol version: TLSv1.3
Ciphersuite: TLS_AES_128_GCM_SHA256
Verification error: self-signed certificate
warn: certificate verification failed (DPI_INSECURE=1)
[paths]
ok /: status=200 content-type=text/html;
ok /robots.txt: status=200 content-type=text/html;
ok /favicon.ico: status=200 content-type=text/html;
ok /api/health: status=200 content-type=text/html;
ok /api/v2/stream: status=200 content-type=?
[active-probe]
ok active probe: status=200 content-type=text/html;
RESULT: PASS
```

## Ограничение

Это лабораторный прогон против текущего развёрнутого сервера. Полевой прогон в целевом регионе (RU) и на новом v2-only сервере ещё не выполнен; без него нельзя заявлять «обходит DPI», только «нет явных отпечатков».

## Полевой прогон

- Дата: 2026-10-29
- Сеть: пользовательская field-сеть (точный ISP не зафиксирован)
- Регион: RU (целевой регион)
- Результат: PASS — пользователь подтвердил, что v2-подключение работает в его сети.
- Ограничение: это подтверждение доступности/подключения; отдельный pcap/active-probe с этой сети не снимался. Active-probe/fallback выше проверены в лабораторном прогоне.
