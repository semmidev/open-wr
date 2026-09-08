# Open WR — Demo Simulation

simulasi antrean virtual Open WR dengan aplikasi web sederhana

---

## 🏗️ Komponen Demo

Simulasi ini terdiri dari 3 layanan container Docker:
1. **`open-wr`**: Waiting Room Edge Reverse Proxy (menggunakan image Docker Hub [`sammidev/open-wr:latest`](https://hub.docker.com/r/sammidev/open-wr)). Port: `8080`.
2. **`demo-app`**: Aplikasi web target yang ditulis dalam Go (`main.go`) dengan tampilan UI Claude Design System (`Source Serif 4`, `#f8f8f6` parchment background, `#121212` text). Port: `3000`.
3. **`redis`**: Store antrean & status sesi terdistribusi (`redis:7-alpine`). Port: `6379`.

---

## 🚀 Cara Menjalankan Simulasi

1. Masuk ke direktori `demo`:
   ```bash
   cd demo
   ```

2. Jalankan seluruh layanan via Docker Compose:
   ```bash
   docker compose up --build
   ```

3. Akses melalui browser:
   - **Melalui Proxy Open WR**: [http://localhost:8080](http://localhost:8080)
   - **Langsung ke Target App (Bypass)**: [http://localhost:3000](http://localhost:3000)

---

## 🧪 Menguji Simulasi Antrean

Open WR telah dikonfigurasi (`config.yaml`) dengan dua waiting room:

### 1. Flash Sale Room (`/flash/*`)
- **URL**: [http://localhost:8080/flash/checkout](http://localhost:8080/flash/checkout)
- **Kapasitas Aktif**: Max 5 user bersamaan (`total_active_users: 5`).
- **Pelepasan**: 120 user / menit (`queueing_method: fifo`).

### 2. VIP Concert Ticket War (`/concert/*`)
- **URL**: [http://localhost:8080/concert/tickets](http://localhost:8080/concert/tickets)
- **Kapasitas Aktif**: Max 3 user bersamaan (`total_active_users: 3`).
- **Pelepasan**: 60 user / menit (`queueing_method: random` / lottery).
- **Behavior**: `queue_all: true` (semua pengunjung wajib masuk antrean sebelum masuk room).

---

## 📊 Monitoring & Admin API

Cek status room secara langsung via curl:
```bash
# Cek status room flash_sale
curl -H "X-API-Key: demo-admin-key" http://localhost:8080/api/rooms/flash_sale

# Cek status room ticket_war
curl -H "X-API-Key: demo-admin-key" http://localhost:8080/api/rooms/ticket_war
```
