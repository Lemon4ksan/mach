<img src="assets/logo.svg" align="right" width="128" height="128" alt="mach logo" />

# mach

_«Аэродинамическая механика протоколов на скорости Маха — где нулевые аллокации встречаются с пропускной способностью физической линии»_

[![Go Version](https://img.shields.io/badge/go-1.27%2B-007d9c?logo=go&logoColor=white&style=flat-square)](https://go.dev/)
[![Go Reference](https://img.shields.io/badge/godoc-reference-007d9c?style=flat-square)](https://pkg.go.dev/github.com/lemon4ksan/mach)
[![Architecture](https://img.shields.io/badge/arch-Zero--Alloc%20%7C%20Sans--I/O-6e56cf?style=flat-square)](#правила)
[![Protocols](https://img.shields.io/badge/L7-H1%20%7C%20H2%20%7C%20H3%20%7C%20QUIC%20%7C%20WS%20%7C%20SSE-0284c7?style=flat-square)](#структура)
[![Ecosystem](https://img.shields.io/badge/ecosystem-foundation-0284c7?style=flat-square)](https://github.com/lemon4ksan/foundation)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue?style=flat-square)](LICENSE)

**[English](README.md)** | **[Русский](README_RU.md)**

Zero-allocation движок протоколов L7 и транспортный стек для Go 1.27+. Лежит в основе клиентского стека [`aoni`](https://github.com/lemon4ksan/aoni) и серверного фреймворка [`sein`](https://github.com/lemon4ksan/sein), построен на базе [`foundation`](https://github.com/lemon4ksan/foundation).

## Инженерные компромиссы

| Стандартная библиотека / `net/http` | Что делает `mach` | Цена решения |
| :--- | :--- | :--- |
| Выделяет в куче новые `http.Request`, `map[string][]string` для заголовков и срезы на каждый запрос | Переиспользует пулы конвертов `Request` / `Response` (`proto/http`) и плоские непрерывные массивы `headkit.HeaderMap` с 64-битным интернированием токенов (`0 B/op`) | Объекты запроса/ответа и срезы заголовков валидны только до вызова `ReleaseRequest` / `ReleaseResponse` или завершения обработчика |
| Копирует заголовки и полезную нагрузку фреймов HTTP/2 в структуры на куче | Накладывает `proto/h2/overlay.RawFrame` напрямую поверх буфера чтения сокета (`0.49 ns/op`) и проверяет переходы потока через табличный конечный автомат | Заимствованные буферы фреймов не могут пережить текущую итерацию чтения без явного копирования |
| Связывает парсинг протоколов с блокирующими вызовами `net.Conn` и горутинами на каждый поток | Отделяет парсеры и конечные автоматы протоколов (`proto/h1`, `proto/h2`, `proto/h3`, `net/quic/core`) от сетевого ввода-вывода | Код, работающий напрямую с низкоуровневыми пакетами `proto/*`, сам управляет буферами чтения и записи |
| Раздельные серверы и сторонние библиотеки для HTTP/1.1, HTTP/2, H2C, HTTP/3 и WebSockets | Единые `server.Server` и `client.Engine`, мультиплексирующие TCP (`H1`, `H2`, `H2C`) и UDP (`H3`/`QUIC`), с промоушеном через RFC 7838 `Alt-Svc` и WebSockets по RFC 6455/8441/9220 | Конфигурация требует явного понимания размеров буферов, окон управления потоком и лимитов пула |

## Правила

1. **Ноль аллокаций на горячем пути.** Разбор фреймов, индексация заголовков (`HPACK` / `QPACK`), маскирование WebSocket, сборка внеочередных сегментов QUIC и семантический парсинг заголовков в установившемся режиме выполняются с `0 allocs/op`.
2. **Дисциплина кэш-линий.** Горячие структуры протоколов (`h2/fsm.Stream`, `quic/core/stream.Stream`, `headkit.HeaderMap`) выровнены по границе 64 байт для предотвращения false sharing между ядрами CPU.
3. **Конечные автоматы в стиле Sans-I/O.** Фрейминг и протокольная логика (`proto/*`, `net/quic/core/*`) реализованы как чистые синхронные конечные автоматы без скрытых горутин и таймеров; жизненным циклом сокетов управляют планировщики в `client/` и `server/`.
4. **Строгое соблюдение RFC.** Нарушения спецификаций (невалидные переходы состояний потока H2, повреждённые QUIC varint, неканоничный паддинг Хаффмана в HPACK, запрещённые псевдозаголовки) немедленно отклоняются с типизированными ошибками протокола. Полная матрица стандартов приведена в [ARCHITECTURE.md](ARCHITECTURE.md).

## Структура

### Уровень 0 — Проводные и семантические примитивы (`proto/http`, `proto/headkit`, `proto/semantic`, `proto/codec`)
- [`proto/http`](proto/http) — Пулируемые конверты `Request` и `Response`, токены методов/статусов и zero-copy чтение тела.
- [`proto/headkit`](proto/headkit) — Плоское непрерывное хранилище заголовков, 64-битное интернирование ASCII-токенов, статическая/динамическая таблицы и кодек Хаффмана **HPACK** ([RFC 7541](https://datatracker.ietf.org/doc/html/rfc7541)), а также потоки кодирования/декодирования **QPACK** ([RFC 9204](https://datatracker.ietf.org/doc/html/rfc9204)).
- [`proto/semantic`](proto/semantic) — Zero-alloc парсеры заголовков [RFC 9110](https://datatracker.ietf.org/doc/html/rfc9110) / [RFC 6265](https://datatracker.ietf.org/doc/html/rfc6265) / [RFC 7838](https://datatracker.ietf.org/doc/html/rfc7838) / [RFC 9218](https://datatracker.ietf.org/doc/html/rfc9218): `altsvc`, `auth`, `contentdisposition`, `cookie`, `digest`, `etag`, `hpkp`, `priority`, `status` и `uri`.
- [`proto/codec`](proto/codec) — Пулируемое потоковое сжатие (`gzip`, `brotli`, `deflate`, `zstd`) с согласованием `Accept-Encoding`.

### Уровень 1 — Фрейминг и конечные автоматы протоколов (`proto/h1`, `proto/h2`, `proto/h3`, `proto/ws`, `proto/sse`, `proto/grpcweb`)
- [`proto/h1`](proto/h1) — Парсер и сериализатор HTTP/1.1 ([RFC 9112](https://datatracker.ietf.org/doc/html/rfc9112)), chunked-кодирование, управление keep-alive и 16-байтный SIMD-поиск разделителей.
- [`proto/h2`](proto/h2) — Фрейминг HTTP/2 ([RFC 9113](https://datatracker.ietf.org/doc/html/rfc9113)), окна управления потоком, деревья приоритетов, табличный FSM потока ([`proto/h2/fsm`](proto/h2/fsm)) и in-situ наложение бинарного заголовка фрейма ([`proto/h2/overlay`](proto/h2/overlay)).
- [`proto/h3`](proto/h3) — Фрейминг HTTP/3 ([RFC 9114](https://datatracker.ietf.org/doc/html/rfc9114): `DATA`, `HEADERS`, `SETTINGS`, `GOAWAY`), однонаправленные управляющие потоки, капсулы и HTTP-датаграммы ([RFC 9297](https://datatracker.ietf.org/doc/html/rfc9297)).
- [`proto/ws`](proto/ws) — Единый фрейминг WebSocket поверх HTTP/1.1 (`Upgrade`, [RFC 6455](https://datatracker.ietf.org/doc/html/rfc6455)), HTTP/2 Extended `CONNECT` (`:protocol = websocket`, [RFC 8441](https://datatracker.ietf.org/doc/html/rfc8441)) и HTTP/3 Extended `CONNECT` ([RFC 9220](https://datatracker.ietf.org/doc/html/rfc9220)) с 64-битным SWAR XOR-маскированием.
- [`proto/sse`](proto/sse) и [`proto/grpcweb`](proto/grpcweb) — Парсер и генератор W3C Server-Sent Events, а также фрейминг с префиксом длины и кодирование трейлеров gRPC-Web.

### Уровень 2 — Транспорт и туннелирование (`net/quic`, `net/proxy`)
- [`net/quic`](net/quic) — Пользовательский транспорт QUIC v1/v2 ([RFC 9000](https://datatracker.ietf.org/doc/html/rfc9000), [RFC 9001](https://datatracker.ietf.org/doc/html/rfc9001), [RFC 9002](https://datatracker.ietf.org/doc/html/rfc9002), [RFC 9369](https://datatracker.ietf.org/doc/html/rfc9369)): целые числа переменной длины с 2-битным префиксом ([`varint`](net/quic/varint)), кодеки пакетов Long/Short Header ([`core/wire`](net/quic/core/wire)), конечный автомат рукопожатия TLS 1.3 ([`core/handshake`](net/quic/core/handshake)) и кольцевой буфер сборки внеочередных сегментов потока ([`core/stream`](net/quic/core/stream)).
- [`net/proxy`](net/proxy) — Zero-alloc туннелирование через **SOCKS5** ([RFC 1928](https://datatracker.ietf.org/doc/html/rfc1928) / [RFC 1929](https://datatracker.ietf.org/doc/html/rfc1929)) и **HTTP `CONNECT`** ([RFC 9110 §9.3.6](https://datatracker.ietf.org/doc/html/rfc9110#section-9.3.6)) с поддержкой правил `NO_PROXY` по CIDR и доменам.

### Уровень 3 — Клиентский и серверный движки (`client`, `server`)
- [`client`](client) — Мультипротокольный `Engine` (`H1`, `H2`, `H3`) с пулом соединений `PoolManager` на каждый origin, автоматическим обнаружением и переключением на HTTP/3 через `Alt-Svc` и единым WebSocket-диалером (`DialWebSocket`).
- [`server`](server) — Единый `Server`, объединяющий TCP (`HTTP/1.1`, `HTTP/2`, `H2C`) и UDP (`HTTP/3`), демультиплексирование по ALPN и 24-байтной преамбуле соединения, пулируемый `Context`, автоматическую отправку `Alt-Svc` и прозрачный `UpgradeWebSocket(ctx)`.

## Установка

```bash
go get github.com/lemon4ksan/mach
```

## Использование

### Единый сервер HTTP/1.1, HTTP/2, H2C и HTTP/3

```go
package main

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/lemon4ksan/mach/proto/http"
	"github.com/lemon4ksan/mach/server"
)

func main() {
	cert, _ := tls.LoadX509KeyPair("server.crt", "server.key")

	srv := server.NewServer(
		server.WithAddr(":8443"),
		server.WithTLSConfig(&tls.Config{Certificates: []tls.Certificate{cert}}),
		server.WithEnableH2(true),
		server.WithEnableH2C(true),
		server.WithEnableH3(true), // Открывает UDP :8443 и добавляет Alt-Svc: h3=":8443"
		server.WithReadTimeout(10*time.Second),
		server.WithHandler(server.HandlerFunc(func(ctx *server.Context) error {
			if ctx.IsWebSocketUpgrade() {
				wsConn, err := server.UpgradeWebSocket(ctx)
				if err != nil {
					return err
				}
				defer wsConn.Close()
				return wsConn.WriteText([]byte("connected over " + ctx.Protocol()))
			}

			ctx.SetHeader("X-Proto", ctx.Protocol())
			return ctx.JSON(http.StatusOK, []byte(`{"status":"ok"}`))
		})),
	)

	_ = srv.ListenAndServe(context.Background())
}
```

### Мультипротокольный клиент с поддержкой `Alt-Svc`

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/lemon4ksan/mach/client"
	"github.com/lemon4ksan/mach/proto/http"
)

func main() {
	eng := client.NewEngine(&client.EngineConfig{
		MaxConnsPerHost:     64,
		IdleConnTimeout:     90 * time.Second,
		EnableHTTP2:         true,
		EnableHTTP3:         true,
		EnableAltSvc:        true,
		HealthCheckInterval: 15 * time.Second,
	})
	defer eng.Close()

	req := http.AcquireRequest()
	defer http.ReleaseRequest(req)

	req.SetMethod(http.MethodGet)
	req.SetURI("https://example.com/api/v1/telemetry")
	req.Headers().Set("Accept", "application/json")

	resp, err := eng.Do(context.Background(), req)
	if err != nil {
		panic(err)
	}
	defer http.ReleaseResponse(resp)

	fmt.Printf("status=%d proto=%s\n", resp.StatusCode(), resp.Protocol())
}
```

### In-Situ наложение заголовка фрейма HTTP/2 (`proto/h2/overlay`)

```go
package main

import (
	"fmt"

	"github.com/lemon4ksan/mach/proto/h2/overlay"
)

func inspectFrame(wireBuf []byte) {
	frame, ok := overlay.AsRawFrame(wireBuf)
	if !ok {
		return
	}
	fmt.Printf("type=%d flags=0x%02x stream=%d len=%d\n",
		frame.Type(), frame.Flags(), frame.StreamID(), frame.Length())
}
```

## Бенчмарки

Замеры выполнены на AMD Ryzen 7 5800X (8C/16T, `windows/amd64`, Go 1.27):

| Подсистема | Операция | Задержка (`ns/op`) | Пропускная способность | Аллокации |
| :--- | :--- | ---: | ---: | ---: |
| `proto/h2/overlay` | In-situ декодирование 9-байтного `RawFrame` | `0.49 ns` | `18,452 MB/s` | `0 B/op (0 allocs)` |
| `proto/h2/fsm` | Переход FSM потока (`Open` $\to$ `Closed`) | `3.59 ns` | — | `0 B/op (0 allocs)` |
| `proto/ws` | 64-битное SWAR XOR-маскирование 4 КБ фрейма | `81.45 ns` | `50,287 MB/s` | `0 B/op (0 allocs)` |
| `net/quic/varint` | Кодирование и декодирование 2-битного Varint | `3.56 ns` | — | `0 B/op (0 allocs)` |
| `net/quic/core/stream` | Сборка 4 КБ сегмента в кольцевом буфере | `288.2 ns` | `14,211 MB/s` | `0 B/op (0 allocs)` |
| `net/quic/core/wire` | Кодек пакета Short Header 1-RTT | `19.65 ns` | — | `0 B/op (0 allocs)` |
| `proto/headkit` | Чтение / запись / интернирование в `HeaderMap` | `21.84 ns` | — | `0 B/op (0 allocs)` |
| `proto/headkit/hpack` | Декодирование блока заголовков HPACK | `142.3 ns` | — | `0 B/op (0 allocs)` |
| `proto/semantic/etag` | Условное сопоставление слабых и строгих `ETag` | `5.86 ns` | — | `0 B/op (0 allocs)` |
| `proto/semantic/uri` | Zero-alloc разбор URI по RFC 3986 | `18.30 ns` | — | `0 B/op (0 allocs)` |
| `proto/h1` | Полный цикл парсинга запроса и ответа H1 | `312.0 ns` | — | `0 B/op (0 allocs)` |
| `net/proxy` | Рукопожатие туннеля SOCKS5 и HTTP `CONNECT` | `18.42 ns` | — | `0 B/op (0 allocs)` |

## Тестирование и фаззинг

```bash
# Запуск модульных и интеграционных тестов с детектором гонок
go test -race ./...

# Проверка отсутствия аллокаций (0 allocs/op) на бенчмарках
go test -bench=. -benchmem ./...

# Запуск встроенного фаззинга (H1, H2, H3, HPACK, QPACK, QUIC wire, WS, Semantic)
go run ./scripts/fuzz_all.go -fuzztime=30s
```

## Лицензия

BSD 3-Clause License. Полный текст приведён в файле [LICENSE](LICENSE).
