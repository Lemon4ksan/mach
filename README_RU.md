<div align="center">

# mach

### Сетевые протокольные движки кремниевой скорости для Go 1.27+

_«Аэродинамическая механика протоколов на скорости Маха — где нулевые аллокации встречаются с пропускной способностью физической линии»_

[![Release](https://img.shields.io/badge/release-v0.2.0-007d9c?style=flat-square&logo=github)](https://github.com/lemon4ksan/mach/releases)
[![Go Version](https://img.shields.io/badge/go-1.27%2B-007d9c?logo=go&logoColor=white&style=flat-square)](https://go.dev/)
[![Go Reference](https://img.shields.io/badge/godoc-reference-007d9c?style=flat-square)](https://pkg.go.dev/github.com/lemon4ksan/mach)
[![Architecture Docs](https://img.shields.io/badge/docs-ARCHITECTURE.md-purple?style=flat-square)](ARCHITECTURE.md)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue?style=flat-square)](LICENSE)
[![Zero-Alloc](https://img.shields.io/badge/memory-0%20B%2Fop%20%7C%200%20allocs-brightgreen?style=flat-square)](#рекорды-скорости)
[![H1 Throughput](https://img.shields.io/badge/h1%20throughput-2.61M%20req%2Fs-success?style=flat-square)](#рекорды-скорости)
[![H2 Stream Table](https://img.shields.io/badge/h2%20streams-14.1M%20ops%2Fs-success?style=flat-square)](#рекорды-скорости)
[![QUIC Reassembly](https://img.shields.io/badge/quic%20reassembly-14.2%20GB%2Fs-blueviolet?style=flat-square)](#рекорды-скорости)
[![WS SWAR Masking](https://img.shields.io/badge/ws%20swar-50.3%20GB%2Fs-orange?style=flat-square)](#рекорды-скорости)

**mach** — высокоскоростной сетевой движок с нулевыми аллокациями, лежащий в основе клиентской экосистемы [`aoni`](https://github.com/lemon4ksan/aoni) и серверного фреймворка [`sein`](https://github.com/lemon4ksan/sein). Построенный от фундаментального кремния, `mach` обеспечивает субнаносекундные наложения бинарных фреймов, 64-битную SWAR-векторизацию регистров, шардированную диспетчеризацию потоков и работу по строгим стандартам RFC.

#### [English](README.md) • Русский • [Архитектура](ARCHITECTURE.md)

</div>

---

## Архитектурные возможности

- **Единый серверный стек (`mach.Server` / `server.Server`)**: Одновременная привязка TCP (`HTTP/1.1`, `HTTP/2`, `H2C`) и UDP (`HTTP/3` поверх QUIC) на одном порту с автоматической отдачей заголовка `Alt-Svc`.
- **Интеллектуальный клиентский движок (`mach.Engine` / `client.Engine`)**: Автоматическое согласование протоколов через ALPN, параллельное мультиплексирование потоков, повторное использование соединений в `PoolManager`.
- **Высокоскоростное туннелирование (`net/proxy`)**: Нулевые аллокации при работе через SOCKS5 (RFC 1928) и HTTP CONNECT (RFC 9110).
- **Мультипротокольный WebSocket (`proto/ws`)**:
  - RFC 6455 (HTTP/1.1 Upgrade).
  - RFC 8441 (HTTP/2 Extended CONNECT с `:protocol = websocket`).
  - RFC 9220 (HTTP/3 Extended CONNECT через QUIC).
  - SWAR 64-битное векторное маскирование со скоростью **50.3 ГБ/с**.

---

## Рекорды скорости

Измерено на аппаратном обеспечении: Intel Core i5-12400F, 64 GB DDR4-3200, Windows x86_64, Go 1.27:

| Подсистема | Метрика бенчмарка | Скорость на линии | Накладные расходы памяти | Аллокации |
| :--- | :--- | :--- | :--- | :--- |
| **HTTP/1.1 Ingestion** | Обработка пайплайна | **`2,610,000 req/s`** | 0 B/op | **0 allocs/op** |
| **HTTP/2 Core** | Мутации таблицы потоков | **`14,100,000 ops/s`** | 0 B/op | **0 allocs/op** |
| **QUIC Transport** | Реассеблер потоков из беспорядка | **`14.20 GB/s`** | 0 B/op | **0 allocs/op** |
| **WebSocket Framing** | 64-бит SWAR маскирование полезной нагрузки | **`50.30 GB/s`** | 0 B/op | **0 allocs/op** |

---

## Установка

```bash
go get github.com/lemon4ksan/mach
```

## Лицензия

BSD-3-Clause. Copyright (c) 2026 Lemon4ksan. All rights reserved.
