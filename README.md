<div align="center">

# Open Waiting Room

**Edge-level virtual queue engine — open source & self-hosted**

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)
[![Build](https://img.shields.io/badge/build-passing-brightgreen?style=flat-square)](#)

</div>

---

## Table of Contents

- [Open Waiting Room](#open-waiting-room)
  - [Table of Contents](#table-of-contents)
  - [1. Overview](#1-overview)
  - [2. Cara Kerja — Edge vs Origin Level](#2-cara-kerja--edge-vs-origin-level)
  - [3. Arsitektur](#3-arsitektur)
    - [Request Flow Diagram](#request-flow-diagram)
    - [Komponen](#komponen)
    - [Package Structure](#package-structure)
  - [4. Teknik & Arsitektur Inti](#4-teknik--arsitektur-inti)
    - [4.1 HMAC-Signed Cookie](#41-hmac-signed-cookie)
    - [4.2 Dual-State: Queued vs Active](#42-dual-state-queued-vs-active)
    - [4.3 FIFO vs Lottery Queueing](#43-fifo-vs-lottery-queueing)
    - [4.4 Admission Worker (Token Bucket)](#44-admission-worker-token-bucket)
    - [4.5 ETA Estimation](#45-eta-estimation)
    - [4.6 Session Renewal](#46-session-renewal)
    - [4.7 Queue-All (Pre-queue Mode)](#47-queue-all-pre-queue-mode)
    - [4.8 Status API + JS Polling dengan Jitter](#48-status-api--js-polling-dengan-jitter)
  - [5. Store: In-Memory vs Redis](#5-store-in-memory-vs-redis)
  - [6. Konfigurasi](#6-konfigurasi)
    - [Server Config](#server-config)
    - [Room Config](#room-config)
    - [Struktur Konfigurasi YAML](#struktur-konfigurasi-yaml)
  - [7. Quick Start](#7-quick-start)
    - [Jalankan (tanpa Redis)](#jalankan-tanpa-redis)
    - [Jalankan dengan Redis](#jalankan-dengan-redis)
    - [Concurrent Load Test](#concurrent-load-test)
    - [Cek Statistik](#cek-statistik)
  - [8. Makefile — Semua Perintah](#8-makefile--semua-perintah)
  - [9. API Reference](#9-api-reference)
    - [`GET /health`](#get-health)
    - [`GET /api/rooms`](#get-apirooms)
    - [`GET /api/rooms/{id}/stats`](#get-apiroomsidstats)
    - [`GET /__owr/waiting-room/status?room_id={id}`](#get-__owrwaiting-roomstatusroom_idid)
  - [10. Deploy ke Production](#10-deploy-ke-production)
    - [Arsitektur yang Direkomendasikan](#arsitektur-yang-direkomendasikan)
    - [Fly.io](#flyio)
    - [Docker Compose](#docker-compose)
  - [11. Security Considerations](#11-security-considerations)
  - [12. Pengembangan \& Kontribusi](#12-pengembangan--kontribusi)
    - [Menjalankan Tests](#menjalankan-tests)
    - [Menambah Room Store Baru](#menambah-room-store-baru)
    - [Menambah Custom Waiting Page](#menambah-custom-waiting-page)
  - [License](#license)

---

## 1. Overview

**Open Waiting Room (Open WR)** adalah reverse-proxy edge untuk virtual queueing — open source, self-hosted, dan production-ready.

Ketika traffic spike (flash sale, tiket konser, checkout peak), user yang tidak mendapatkan slot aktif langsung menerima halaman antrian dari edge — **tanpa menyentuh origin server sama sekali**. Ketika giliran tiba, cookie di-upgrade secara otomatis dan browser me-redirect ke origin.

**Fitur utama:**

| Fitur | Open WR |
|-------|:-------:|
| HMAC-signed cookie (`__owr_{room_id}`) | ✅ |
| Dua state: queued / active | ✅ |
| `new_users_per_minute` (admission rate) | ✅ |
| `total_active_users` (kapasitas) | ✅ |
| `session_duration_minutes` | ✅ |
| `queueing_method: fifo\|random` | ✅ |
| `queue_all` (pre-queue mode) | ✅ |
| Custom waiting page template | ✅ |
| `disable_session_renewal` | ✅ |
| Status polling endpoint (`/__owr/...`) | ✅ |
| JS polling dengan jitter | ✅ |
| Multi-room per path/host | ✅ |
| Admin API + API key auth | ✅ |
| In-memory + Redis store | ✅ |
| Self-hosted, zero vendor lock-in | ✅ MIT |

---

## 2. Cara Kerja — Edge vs Origin Level

Mayoritas implementasi waiting room bekerja di **origin level**: request masuk ke server aplikasi baru dicek antrean. Ini tetap membebani origin saat traffic tinggi.

Open WR bekerja di **edge level**:

```
┌─────────────────────────────────────────────────────────────────┐
│                      EDGE LEVEL (port :8080)                    │
│                                                                 │
│  User ──► Room Matcher ──► Cookie Validator                     │
│                               │                                 │
│              ┌────────────────┼──────────────────┐              │
│              ▼                ▼                  ▼              │
│           BYPASS           ALLOW              QUEUE             │
│              │                │                  │              │
│              │          Extend session    Serve waiting page    │
│              │          Set cookie        (dari edge, NO origin)│
│              │                │                                 │
│              └────────────────┘                                 │
│                       │                                         │
│                  Proxy ke Origin                                │
└─────────────────────────────────────────────────────────────────┘
                         │
                         ▼
               ┌─────────────────┐
               │  ORIGIN SERVER  │  ← Hanya terima user yg lolos
               │  (port :3000)   │
               └─────────────────┘
```

**User yang diqueue tidak pernah menyentuh origin.** Halaman antrian di-render langsung dari edge menggunakan embedded Go template.

---

## 3. Arsitektur

### Request Flow Diagram

```
HTTP Request
     │
     ▼
┌──────────────────────────────────────────────────────────────┐
│                  chi Router (port :8080)                     │
│                                                              │
│  GET /health              ──► healthHandler                  │
│  /api/rooms/*             ──► AdminAuthMiddleware            │
│                                  └──► AdminHandler           │
│  /__owr/waiting-room/status    ──► StatusHandler             │
│  /*                       ──► WaitingRoomHandler             │
│                                                              │
└───────────────────────┬──────────────────────────────────────┘
                        │  WaitingRoomHandler.ServeHTTP()
                        ▼
              roomMatcher.Match(path, host)
                        │
              ┌─────────┴──────────┐
              │ No match           │ Match → roomID
              ▼                    ▼
          serveOrigin()    Service.CheckRequest()
                                   │
                    ┌──────────────┼──────────────┐
                    ▼              ▼              ▼
                 BYPASS          ALLOW          QUEUE
                    │              │              │
              serveOrigin()  Set Cookie     Set Cookie
                             serveOrigin()  Render HTML
                                            (edge only)
```

### Komponen

| Komponen | Package | Tanggung Jawab |
|----------|---------|----------------|
| **Entry point** | `cmd/server` | Wiring semua komponen, graceful shutdown |
| **Config** | `internal/config` | Load YAML config & validasi |
| **Cookie Signer** | `internal/cookie` | HMAC-SHA256 sign & verify, typed errors |
| **Store** | `internal/room` | Interface + MemStore + RedisStore |
| **Service** | `internal/waitingroom` | Core decision logic (admit/queue/bypass) |
| **Admission Worker** | `internal/waitingroom` | Token bucket per-room, background goroutine |
| **Page Renderer** | `internal/waitingroom` | Embedded template + custom template cache |
| **API Handlers** | `internal/api` | Admin, Status, WaitingRoom handler, middleware |
| **Reverse Proxy** | `internal/proxy` | Proxy ke origin, header forwarding |

### Package Structure

```
open-wr/
├── cmd/
│   └── server/
│       └── main.go              # Entry point, DI wiring, graceful shutdown
├── internal/
│   ├── api/
│   │   ├── admin.go             # GET /api/rooms, /api/rooms/{id}/stats
│   │   ├── middleware.go        # WaitingRoomHandler, AdminAuthMiddleware,
│   │   │                        #   roomMatcher (pre-compiled path matcher)
│   │   └── status.go            # GET /__owr/waiting-room/status
│   ├── config/
│   │   └── config.go            # YAML load, env override, Validate()
│   ├── cookie/
│   │   ├── signer.go            # Sign, Verify (ErrInvalidToken/ErrExpiredToken)
│   │   └── signer_test.go
│   ├── proxy/
│   │   └── proxy.go             # httputil.ReverseProxy wrapper + DemoOriginHandler
│   ├── room/
│   │   ├── store.go             # Store interface (godoc: semua expiry = Unix millis)
│   │   ├── model.go             # QueueItem, ActiveSession, Stats
│   │   ├── mem_store.go         # In-memory impl (lazy eviction, qSet O(1) lookup)
│   │   ├── mem_store_test.go
│   │   └── redis_store.go       # Redis impl (Lua atomic random pop, pool config)
│   └── waitingroom/
│       ├── service.go           # CheckRequest, GetStatus, Stats
│       ├── service_test.go
│       ├── worker.go            # AdmissionWorker (token bucket)
│       ├── page.go              # RenderDefault (template cache per-room)
│       └── templates/
│           └── waiting.html     # Embedded dark-mode waiting page
├── config.example.yaml
├── Dockerfile
├── Makefile
└── go.mod
```

---

## 4. Teknik & Arsitektur Inti

### 4.1 HMAC-Signed Cookie

Cookie tidak menyimpan state di server untuk validasi — isinya signed sehingga bisa diverifikasi tanpa Redis roundtrip.

**Format cookie** (inner, sebelum base64url encoding):

```
{uuid}|{room_id}|{status}|{iat_unix}|{exp_unix}|{hmac_sha256_base64url}
```

**Contoh:**
```
3f7a1b2c-...|flash_sale|active|1725782400|1725783000|AbCdEfGh...
```

| Field | Tipe | Keterangan |
|-------|------|------------|
| `uuid` | string | Random visitor ID (google/uuid) |
| `room_id` | string | ID room yang diissue |
| `status` | string | `queued` atau `active` |
| `iat` | int64 | Unix seconds — issued-at |
| `exp` | int64 | Unix seconds — expiry |
| `hmac` | string | `HMAC-SHA256(fields 1–5, secret)` base64url |

Seluruh string kemudian di-**base64url** encode lagi untuk menghindari karakter spesial di Set-Cookie header.

**Cookie attributes:**
```
Set-Cookie: __owr_flash_sale=<token>; Path=/; HttpOnly; SameSite=Lax; MaxAge=600
```
`Secure` flag di-set via config `secure_cookies: true` (wajib di production HTTPS).

**Kenapa tidak JWT?** JWT lebih verbose dan butuh library. Format pipe-delimited ini lebih ringkas, mudah di-parse, dan HMAC-SHA256-nya setara keamanannya dengan HS256 JWT.

---

### 4.2 Dual-State: Queued vs Active

Setiap visitor hanya bisa berada di satu dari dua state:

```
                    NEW VISITOR
                        │
          ┌─────────────┴──────────────┐
          │ active < total_active_users │ active >= total_active_users
          │          AND NOT queue_all  │ OR queue_all = true
          ▼                            ▼
    ┌──────────┐                 ┌──────────┐
    │  ACTIVE  │◄────────────────│  QUEUED  │
    │  set     │  promoted by    │  (ZSET)  │
    │(expiry)  │  worker         │  score=  │
    └──────────┘                 │  millis  │
          │                      └──────────┘
    session expires                    │
          │                     cookie expires
          ▼                     (or evicted)
    slot freed                         │
          │                            ▼
          └──────────────────────► slot open
```

**Active set** (Redis ZSET `owr:active:{room}`):
- Member: visitor UUID
- Score: expiry timestamp Unix **millis**
- Cleanup via `ZREMRANGEBYSCORE` score `0` to `now_millis`

**Queue** (Redis ZSET `owr:queue:{room}`):
- Member: visitor UUID
- Score: enqueue timestamp Unix **millis** (untuk FIFO ordering)

---

### 4.3 FIFO vs Lottery Queueing

**FIFO (`queueing_method: fifo`):**
- Pop dari front queue via `ZPOPMIN` (Redis) atau slice head (MemStore)
- Deterministik: yang datang lebih dulu dijamin lebih dulu keluar
- Mudah di-abuse: bot yang request 1 ms lebih dulu selalu menang

**Lottery (`queueing_method: random`):**
- Di Redis: **atomic Lua script** yang fetch sample `n×5` candidates, Fisher-Yates shuffle in Lua, remove selected
- Di MemStore: `rand.Intn` per pick, remove dari slice
- Tidak deterministik: bot tidak dapat keuntungan dari request awal
- Tidak deterministik: bot tidak dapat keuntungan dari timestamp request awal

**Kenapa Lua untuk Redis?** `ZRANGE` → shuffle in Go → `ZREM` adalah tiga operasi terpisah. Di deployment multi-instance, dua worker bisa pick member yang sama. Lua script di-execute atomik di Redis, eliminasi race condition ini sepenuhnya.

---

### 4.4 Admission Worker (Token Bucket)

Background goroutine per room yang melepas visitor dari queue sesuai `new_users_per_minute`.

**Token Bucket (integer arithmetic):**

```go
ticksPerMinute := int64(time.Minute / tickInterval) // e.g. 12 ticks per minute (5s interval)
bucket         := int64(0)                           // accumulator

// setiap tick:
bucket   += newUsersPerMinute          // tambah token
toAdmit   = bucket / ticksPerMinute   // berapa yang bisa dilepas sekarang
bucket   -= toAdmit * ticksPerMinute  // sisa dibawa ke tick berikutnya
```

**Kenapa bukan floating point?**

```
float approach (sebelumnya):
  perTick = 60.0 * 5.0 / 60.0 = 5.0
  setelah 12 ticks → admit 60 ✓

  tapi dengan new_users_per_minute = 100:
  perTick = 100.0 * 5.0 / 60.0 = 8.333...
  tick 1: floor(8.333) = 8
  tick 2: floor(8.333) = 8  ...  → 96/min, bukan 100
```

Token bucket accumulator memastikan rate tepat `new_users_per_minute` bahkan untuk angka non-divisible.

**Alur setiap tick (5 detik):**
1. `CleanupExpiredActive()` — bebaskan slot dari session yang habis
2. Hitung `available = total_active_users - activeCount`
3. Hitung `toAdmit` via token bucket
4. `PopForAdmission(toAdmit, method)` — ambil dari queue
5. `AddActive(id, expiry)` — promosikan ke active set

---

### 4.5 ETA Estimation

```
estimatedWait (seconds) = ⌈ position / (new_users_per_minute / 60) ⌉
```

- **`position`**: zero-based rank di queue
- **`new_users_per_minute / 60`**: admission rate per detik
- Di-update setiap kali visitor poll status endpoint

Contoh: posisi ke-120, rate 60/min → ETA = ⌈120 / 1⌉ = 120 detik (~2 menit).

---

### 4.6 Session Renewal

Selama visitor aktif browsing, setiap request ke path yang di-protect akan **extend session expiry** secara otomatis (seperti rolling session):

```go
// Setiap request dengan valid active cookie:
if !rcfg.DisableSessionRenewal {
    newExp := time.Now().Add(sessionDuration).UnixMilli()
    store.ExtendActive(ctx, roomID, payload.ID, newExp)
}
```

Jika visitor idle lebih dari `session_duration_minutes`, slot dilepas otomatis oleh admission worker (cleanup expired).

Set `disable_session_renewal: true` untuk perilaku fixed-window (slot dilepas tepat setelah durasi).

---

### 4.7 Queue-All (Pre-queue Mode)

```yaml
queue_all: true
```

Semua visitor dipaksa masuk antrian — tidak ada yang langsung admit, meskipun kapasitas masih tersedia. Berguna untuk:
- **Pre-queue sebelum event mulai**: visitor sudah antri, ketika event start tinggal set `queue_all: false`
- **Meratakan load burst**: pastikan tidak ada thundering herd di momen pertama

---

### 4.8 Status API + JS Polling dengan Jitter

Waiting page mem-poll endpoint status setiap beberapa detik untuk mendapat update posisi real-time tanpa full page reload.

**Endpoint:** `GET /__owr/waiting-room/status?room_id={id}`

**Response:**
```json
{
  "room_id":        "flash_sale",
  "is_active":      false,
  "position":       14,
  "total_queued":   87,
  "active_count":   200,
  "estimated_wait": 14,
  "status":         "queued"
}
```

**Jitter** mencegah semua browser poll secara bersamaan (thundering herd ke status endpoint):

```js
function jitter() {
  return baseInterval + Math.random() * 2000; // 5s + 0–2s random
}
setTimeout(poll, jitter());
```

**Meta-refresh disabled oleh JS** — `<meta http-equiv="refresh">` di dalam `<noscript>` sehingga tidak double-polling. Browser tanpa JS fallback ke meta-refresh setiap 5 detik.

**Cookie validation di status endpoint:**
- `401` → cookie expired → browser reload (re-evaluate)
- `403` → cookie invalid / tampered → browser reload
- `200 + is_active: true` → `location.reload()` → browser ke origin

---

## 5. Store: In-Memory vs Redis

| | **MemStore** | **RedisStore** |
|--|:---:|:---:|
| Deployment | Single instance | Multi-instance |
| Persistence | ❌ (restart = reset) | ✅ |
| Performance | ~O(log n) queue | ~O(log n) queue |
| Horizontal scale | ❌ | ✅ |
| Random pop race-free | ✅ (mutex) | ✅ (Lua script) |
| Lazy eviction | ✅ | ✅ (ZREMRANGEBYSCORE) |
| Setup | Zero (default) | Redis required |

**MemStore details:**
- Queue: `[]QueueItem` sorted by `Score` asc, + `qSet map[string]bool` untuk O(1) existence check
- Active: `map[string]int64` (id → expiry millis) dengan lazy eviction di `ActiveCount()`
- Thread-safe via per-room `sync.RWMutex` + double-checked locking di `getOrCreate()`

**RedisStore details:**
- Queue: `ZSET owr:queue:{room}` — score = Unix millis, member = UUID
- Active: `ZSET owr:active:{room}` — score = expiry millis, member = UUID
- FIFO pop: `ZPOPMIN` (atomic)
- Random pop: Lua script (atomic Fisher-Yates sample)
- Config: pool size, dial/read/write timeout via `RedisStoreOptions`

---

## 6. Konfigurasi

### Server Config

```yaml
server:
  listen: ":8080"               # Bind address
  origin: "http://localhost:3000" # URL origin BE; kosong = demo handler
  cookie_secret: "..."          # WAJIB >= 32 karakter
  secure_cookies: false         # Set true di production (HTTPS)
  redis_addr: "localhost:6379"
  redis_enabled: false
  log_level: "info"             # debug | info | warn | error
  admin_api_key: ""             # Kosong = no auth (dev only!)
  cors_origin: "*"              # Untuk status endpoint CORS
```

### Room Config

```yaml
rooms:
  - id: "flash_sale"                    # Unik, dipakai di cookie name & status API
    name: "Flash Sale"                  # Display name di waiting page
    description: "..."                  # Subtitle di waiting page
    path: "/flash/*"                    # Glob: /* = prefix match, exact = exact match
    host: ""                            # Kosong = semua host; isi untuk vhost routing
    enabled: true
    queue_all: false                    # true = semua user wajib antre
    new_users_per_minute: 1200          # Rate admission per menit
    total_active_users: 200             # Max active sessions
    session_duration_minutes: 10        # TTL session aktif
    queueing_method: "fifo"             # fifo | random
    cookie_name: "__owr"                # Prefix; cookie final: __owr_{id}
    custom_page_template: ""            # Path ke .html; kosong = default template
    disable_session_renewal: false      # true = fixed-window session
    json_response_enabled: false        # true = 202 JSON untuk API client
```

### Konfigurasi Berbasis YAML

Seluruh pengaturan aplikasi disimpan dan dibaca secara eksklusif dari berkas `config.yaml` (contoh: `config.example.yaml`), tanpa bergantung pada berkas `.env`.

---

## 7. Quick Start

**Prasyarat:** Go 1.22+, atau Docker

### Jalankan (tanpa Redis)

```bash
git clone https://github.com/semmidev/open-wr
cd open-wr

# Set cookie secret minimum 32 karakter
export OPENWR_COOKIE_SECRET="dev-secret-change-this-in-prod!!"

make run
# Server listening di :8080
```

Buka http://localhost:8080/flash/test — kamu akan langsung masuk atau lihat waiting page.

### Jalankan dengan Redis

```bash
# Terminal 1 — Redis
docker run --rm -p 6379:6379 redis:alpine

# Terminal 2 — Server
make run-redis
```

### Concurrent Load Test

```bash
# Di terminal lain (server harus sudah running)
make load-test
# Fires 200 concurrent requests — beberapa masuk, sisanya masuk antrian
```

### Cek Statistik

```bash
curl http://localhost:8080/api/rooms | python3 -m json.tool
curl http://localhost:8080/api/rooms/flash_sale/stats | python3 -m json.tool
```

---

## 8. Makefile — Semua Perintah

```bash
make help          # Lihat semua perintah
```

| Kategori | Perintah | Fungsi |
|----------|----------|--------|
| **Dev** | `make run` | Jalankan server dengan config default |
| | `make run-debug` | Run dengan log level debug |
| | `make run-redis` | Run dengan Redis store |
| **Build** | `make build` | Binary ke `./bin/owr` (dengan version info) |
| | `make build-linux` | Cross-compile Linux/amd64 |
| | `make build-arm` | Cross-compile Linux/arm64 |
| **Test** | `make test` | Semua unit test |
| | `make test-v` | Verbose output |
| | `make test-race` | Dengan Go race detector |
| | `make test-cover` | Coverage report (text) |
| | `make test-cover-html` | Coverage report (browser) |
| | `make test-pkg PKG=./internal/room` | Test package spesifik |
| **Quality** | `make fmt` | Format semua file Go |
| | `make vet` | Static analysis |
| | `make lint` | golangci-lint |
| | `make check` | fmt + vet + race test (pre-commit) |
| **Load** | `make load-test` | 200 concurrent requests |
| **Docker** | `make docker-build` | Build image |
| | `make docker-run` | Run container |
| **Module** | `make tidy` | `go mod tidy && go mod verify` |
| | `make clean` | Hapus artifacts |
| | `make version` | Print version/commit/build-time |

---

## 9. API Reference

### `GET /health`

```json
{ "status": "ok", "service": "open-wr edge" }
```

---

### `GET /api/rooms`

> Memerlukan `Authorization: Bearer <admin_api_key>` atau `X-Admin-Key: <key>` jika `admin_api_key` dikonfigurasi.

List semua room yang terkonfigurasi:

```json
[
  {
    "id": "flash_sale",
    "name": "Flash Sale iPhone 15 Pro",
    "enabled": true,
    "new_users_per_minute": 1200,
    "total_active_users": 200,
    ...
  }
]
```

---

### `GET /api/rooms/{id}/stats`

Statistik live sebuah room:

```json
{
  "room_id":            "flash_sale",
  "active_count":       187,
  "queued_count":       4231,
  "new_users_per_minute": 1200,
  "total_active_limit": 200,
  "queueing_method":    "fifo",
  "is_queueing":        true
}
```

---

### `GET /__owr/waiting-room/status?room_id={id}`

Digunakan oleh JavaScript di waiting page untuk polling posisi. Memerlukan session cookie valid.

**Response (queued):**
```json
{
  "room_id":        "flash_sale",
  "is_active":      false,
  "position":       42,
  "total_queued":   1200,
  "active_count":   200,
  "estimated_wait": 42,
  "status":         "queued"
}
```

**Response (active):**
```json
{
  "room_id":   "flash_sale",
  "is_active": true,
  "position":  0,
  "status":    "active"
}
```

**Error codes:**
- `400` — `room_id` tidak ada di query
- `401` — Cookie tidak ada atau expired
- `403` — Cookie signature invalid (tampered)
- `404` — Room ID tidak ditemukan

---

## 10. Deploy ke Production

### Arsitektur yang Direkomendasikan

```
Internet ──► [Nginx/Caddy TLS termination]
                  │
                  ▼
         [Open Waiting Room :8080]  ← horizontal scale dengan Redis
                  │
                  ▼
          [Origin Server :3000]     ← hanya terima traffic lolos antrian
```

### Fly.io

```toml
# fly.toml
app = "my-waiting-room"

[build]
  dockerfile = "Dockerfile"

[env]
  OPENWR_REDIS_ENABLED = "true"
  OPENWR_SECURE_COOKIES = "true"
  OPENWR_LOG_LEVEL = "info"

[[services]]
  internal_port = 8080
  protocol = "tcp"
```

```bash
fly secrets set OPENWR_COOKIE_SECRET="your-strong-32-char-secret!!"
fly secrets set OPENWR_REDIS_ADDR="redis.fly.io:6379"
fly secrets set OPENWR_ADMIN_API_KEY="your-admin-api-key"
fly deploy
```

### Docker Compose

Open WR menyertakan `docker-compose.yml` dengan beberapa **profile** yang bisa dipilih sesuai kebutuhan:

| Profile | Perintah | Isi |
|---------|----------|-----|
| *(default)* | `docker compose up` | open-wr + Redis |
| `dev` | `docker compose --profile dev up` | open-wr saja, in-memory, log debug |
| `redis-ui` | `docker compose --profile redis-ui up` | + Redis Commander UI di `:8081` |

**Setup pertama kali:**

```bash
# 1. Salin template konfigurasi
cp config.example.yaml config.yaml

# 2. Isi cookie_secret pada config.yaml (wajib, min 32 char)
#    Generate: openssl rand -base64 32
vi config.yaml

# 3. Jalankan (open-wr + Redis)
docker compose up -d
```

**Shortcut via Makefile:**

```bash
make docker-run-redis   # open-wr + Redis (default)
make docker-dev         # dev mode: in-memory, no Redis, log debug
make docker-redis-ui    # + Redis Commander UI di http://localhost:8081
make docker-down        # stop semua container
make docker-down-v      # stop + hapus Redis volume (DATA HILANG)
```

---

## 11. Security Considerations

| Aspek | Implementasi |
|-------|-------------|
| **Cookie integrity** | HMAC-SHA256 dengan secret ≥ 32 byte. Tampered cookie → 403 |
| **Cookie confidentiality** | Payload tidak dienkripsi (hanya signed). Jangan simpan data sensitif di cookie |
| **Cookie transport** | `HttpOnly` (JS tidak bisa baca), `SameSite=Lax`. Set `Secure=true` di HTTPS |
| **Secret rotation** | Ganti `cookie_secret` → semua existing cookie invalid (logout semua user) |
| **Admin API** | Wajib set `admin_api_key` di production. Endpoint tanpa key expose queue stats |
| **CORS** | Set `cors_origin` ke domain spesifik di production; hindari `*` |
| **Rate limiting** | Tidak built-in. Kombinasikan dengan Nginx `limit_req` atau Edge WAF di depan |
| **Bot prevention** | `queueing_method: random` mengurangi keuntungan bot yang request lebih awal |

---

## 12. Pengembangan & Kontribusi

### Menjalankan Tests

```bash
make test           # Unit tests
make test-race      # Dengan race detector (wajib sebelum PR)
make test-cover     # Coverage
make check          # fmt + vet + race (pre-commit gate)
```

**Coverage saat ini:**
- `internal/cookie` — Sign/Verify, expired, tampered, weak secret
- `internal/room` — MemStore full (active CRUD, queue CRUD, FIFO, random, position)
- `internal/waitingroom` — Service (admit, queue, upgrade, bypass, status)

### Menambah Room Store Baru

Implementasi `room.Store` interface (11 methods). Lihat `mem_store.go` sebagai referensi.

```go
type Store interface {
    // Active session management
    ActiveCount(ctx, roomID) (int64, error)
    IsActive(ctx, roomID, id) (bool, error)
    AddActive(ctx, roomID, id string, expiresAtMillis int64) error
    ExtendActive(ctx, roomID, id string, newExpiryMillis int64) error
    RemoveActive(ctx, roomID, id) error
    CleanupExpiredActive(ctx, roomID string, nowMillis int64) (int64, error)

    // Queue operations
    Enqueue(ctx, item QueueItem) (position, total int64, err error)
    GetPosition(ctx, roomID, id) (pos, total int64, found bool, err error)
    PopForAdmission(ctx, roomID string, n int, method string) ([]QueueItem, error)
    RemoveFromQueue(ctx, roomID, id) error
    QueueLen(ctx, roomID) (int64, error)
    ExistsInQueue(ctx, roomID, id) (bool, error)
}
```

> ⚠️ **Konvensi**: semua parameter `expiresAt`/`nowMillis` adalah **Unix milliseconds**.

### Menambah Custom Waiting Page

Buat file HTML dengan Go template syntax:

```html
<!-- my-room-template.html -->
<!DOCTYPE html>
<html>
<body>
  <h1>{{.RoomName}}</h1>
  <p>Posisi kamu: #{{.Position}} dari {{.TotalQueued}}</p>
  <p>Estimasi: {{.EstimatedWait}} detik</p>
</body>
</html>
```

Daftarkan di config:

```yaml
rooms:
  - id: "my_room"
    custom_page_template: "/etc/openwr/my-room-template.html"
```

Template di-parse sekali dan di-cache. Variables yang tersedia:

| Variable | Tipe | Deskripsi |
|----------|------|-----------|
| `.RoomName` | string | Nama room |
| `.RoomDescription` | string | Deskripsi room |
| `.RoomID` | string | ID room |
| `.Position` | int64 | Posisi antrian (1-indexed) |
| `.TotalQueued` | int64 | Total user di antrian |
| `.ActiveCount` | int64 | User aktif saat ini |
| `.EstimatedWait` | int64 | ETA dalam detik |
| `.EstimatedWaitMin` | int64 | ETA dalam menit (ceiling) |
| `.RefreshInterval` | int | Interval polling JS (detik) |
| `.QueueingMethod` | string | `fifo` atau `random` |

---

## License

MIT © [semmidev](https://github.com/semmidev)
