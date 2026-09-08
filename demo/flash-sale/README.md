# Open WR — Demo Simulation

Simulasi antrean virtual Open WR dengan aplikasi web target Go.

---

## 🏗️ Komponen Demo

Simulasi ini terdiri dari 3 layanan container Docker:
1. **`open-wr`**: Waiting Room Edge Reverse Proxy (menggunakan image Docker Hub [`sammidev/open-wr:latest`](https://hub.docker.com/r/sammidev/open-wr) & `config-demo.yaml`). Port: `8080`.
2. **`demo-app`**: Aplikasi web target yang ditulis dalam Go (`main.go`). Port: `3000`.
3. **`redis`**: Store antrean & status sesi terdistribusi (`redis:7-alpine`). Port: `6379`.

---

## 🚀 Cara Menjalankan Simulasi

1. Masuk ke direktori `demo/flash-sale`:
   ```bash
   cd demo/flash-sale
   ```

2. Jalankan seluruh layanan via Podman / Docker Compose:
   ```bash
   podman compose up -d --build
   # atau jika menggunakan docker-compose:
   # docker compose up -d --build
   ```

3. Akses melalui browser:
   - **Melalui Proxy Open WR**: [http://localhost:8080](http://localhost:8080)
   - **Langsung ke Target App (Bypass)**: [http://localhost:3000](http://localhost:3000)

---

## 🧪 Simulasi Antrean Realtime (Terminal & Browser)

Open WR telah dikonfigurasi (`config-demo.yaml`) dengan pengaturan durasi sesi cepat (1 menit) untuk keperluan simulasi:

- **VIP Concert Ticket War (`/concert/*`)**:
  - `total_active_users: 2` (Kapasitas maksimal 2 user bersamaan)
  - `session_duration_minutes: 1` (Durasi sesi 1 menit)
  - `queue_all: true` (Semua pengunjung baru wajib antre)

### Langkah Simulasi:

1. **Buka 1 Tab Browser (Incognito)** dan kunjungi:
   [http://localhost:8080/concert/tickets](http://localhost:8080/concert/tickets)
   *Browser Anda akan masuk ke halaman Waiting Room Open WR.*

2. **Jalankan Script Simulasi 7 Client di Terminal**:
   ```bash
   for i in {1..7}; do
     curl -s -o /dev/null -w "User $i -> Status Code: %{http_code}\n" http://localhost:8080/concert/tickets &
   done
   ```

3. **Perhatikan Perilaku Antrean**:
   - Terminal akan memicu 7 request pengunjuk rasa secara bersamaan yang mengisi slot antrean.
   - Browser Anda yang sedang membuka halaman Waiting Room akan otomatis memperbarui posisi antrean dan me-redirect Anda begitu slot aktif kosong dalam 1 menit!

---

## 📊 Monitoring & Admin API

Cek status room secara langsung via curl:
```bash
# Cek status room ticket_war (antrean & user aktif)
curl -s -H "X-API-Key: demo-admin-key" http://localhost:8080/api/rooms/ticket_war
```
