package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"
)

// DemoApp represents the web application server
type DemoApp struct {
	startTime time.Time
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	app := &DemoApp{
		startTime: time.Now(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.handleHome)
	mux.HandleFunc("/flash/", app.handleFlashSale)
	mux.HandleFunc("/concert/", app.handleConcertTickets)
	mux.HandleFunc("/api/order", app.handleOrderAPI)
	mux.HandleFunc("/health", app.handleHealth)

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Demo target application listening on port %s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

func (a *DemoApp) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"app":    "open-wr-demo-target",
		"uptime": time.Since(a.startTime).String(),
	})
}

func (a *DemoApp) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	renderPage(w, pageData{
		Title:       "Claude Store — Open WR Protected Target",
		PageType:    "home",
		Badge:       "Open WR Target Server",
		Heading:     "Welcome to Claude Edition Store",
		Description: "This application is currently protected by Open WR edge proxy. Navigate through the high-traffic portals below to simulate queueing behavior.",
	})
}

func (a *DemoApp) handleFlashSale(w http.ResponseWriter, r *http.Request) {
	renderPage(w, pageData{
		Title:       "Flash Sale — Claude Book M4 Ultra",
		PageType:    "flash",
		Badge:       "Protected by Open WR Flash Sale Room",
		Heading:     "Claude Book M4 Ultra (Limited Drop)",
		Description: "Congratulations! You have passed through the Open WR waiting room and secured an active session. Stock is currently available.",
	})
}

func (a *DemoApp) handleConcertTickets(w http.ResponseWriter, r *http.Request) {
	renderPage(w, pageData{
		Title:       "VIP Ticket War — World Tour 2026",
		PageType:    "concert",
		Badge:       "Protected by Open WR Lottery Queue Room",
		Heading:     "Exclusive VIP Concert Ticket Portal",
		Description: "You were selected from the queueing pool! Complete your ticket allocation before your Open WR session cookie expires.",
	})
}

func (a *DemoApp) handleOrderAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":   true,
		"order_id":  fmt.Sprintf("ORD-%d", time.Now().UnixNano()%1000000),
		"message":   "Transaction completed successfully through Open WR proxy!",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

type pageData struct {
	Title       string
	PageType    string
	Badge       string
	Heading     string
	Description string
}

func renderPage(w http.ResponseWriter, data pageData) {
	tmpl, err := template.New("page").Parse(htmlTemplate)
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = tmpl.Execute(w, data)
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="id">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{ .Title }}</title>

  <!-- Google Fonts: Source Serif 4 (Headlines) & Inter (Body) -->
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;550;580;600&family=Source+Serif+4:opsz,wght@8..60,400;600&display=swap" rel="stylesheet">

  <style>
    :root {
      /* Claude Design Tokens */
      --color-bone-parchment: #f8f8f6;
      --color-paper-white: #ffffff;
      --color-soft-stone: #efeeeb;
      --color-carbon-ink: #121212;
      --color-graphite: #373734;
      --color-ashen: #7b7974;
      --color-pebble: #9c9a92;
      --color-mist: #b7b7b5;
      --color-chalk: #e7e6e1;
      --color-obsidian: #000000;
      --color-clay: #d97757;

      --font-serif: 'Source Serif 4', Georgia, serif;
      --font-sans: 'Inter', system-ui, -apple-system, sans-serif;

      --shadow-card: rgba(0, 0, 0, 0.04) 0px 4px 20px 0px;
      --shadow-hover: oklab(0.431435 -0.02915 -0.125723 / 0.1) 0px 4px 24px 0px;
    }

    * {
      box-sizing: border-box;
      margin: 0;
      padding: 0;
    }

    body {
      background-color: var(--color-bone-parchment);
      color: var(--color-carbon-ink);
      font-family: var(--font-sans);
      font-size: 14px;
      line-height: 1.5;
      -webkit-font-smoothing: antialiased;
      min-height: 100vh;
      display: flex;
      flex-direction: column;
    }

    /* Top Minimal Navigation */
    header {
      background: var(--color-bone-parchment);
      border-bottom: 1px solid var(--color-chalk);
      padding: 16px 32px;
      display: flex;
      align-items: center;
      justify-content: space-between;
      max-width: 1200px;
      width: 100%;
      margin: 0 auto;
    }

    .brand {
      display: flex;
      align-items: center;
      gap: 8px;
      font-family: var(--font-serif);
      font-size: 20px;
      color: var(--color-carbon-ink);
      text-decoration: none;
    }

    .brand-mark {
      width: 10px;
      height: 10px;
      background-color: var(--color-clay);
      border-radius: 50%;
      display: inline-block;
    }

    nav {
      display: flex;
      gap: 16px;
      align-items: center;
    }

    nav a {
      color: var(--color-graphite);
      text-decoration: none;
      font-size: 14px;
      font-weight: 500;
      padding: 8px 12px;
      border-radius: 8px;
      transition: background-color 0.2s ease, color 0.2s ease;
    }

    nav a:hover, nav a.active {
      color: var(--color-carbon-ink);
      background-color: var(--color-soft-stone);
    }

    /* Main Container */
    main {
      flex: 1;
      max-width: 1200px;
      width: 100%;
      margin: 0 auto;
      padding: 48px 32px 80px;
    }

    /* Editorial Hero Section */
    .hero {
      text-align: center;
      max-width: 720px;
      margin: 0 auto 64px;
    }

    .badge {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      background-color: var(--color-soft-stone);
      color: var(--color-graphite);
      font-size: 12px;
      font-weight: 550;
      padding: 4px 12px;
      border-radius: 8px;
      margin-bottom: 24px;
      border: 1px solid var(--color-chalk);
    }

    .badge-dot {
      width: 6px;
      height: 6px;
      background-color: var(--color-clay);
      border-radius: 50%;
    }

    h1 {
      font-family: var(--font-serif);
      font-size: 36px;
      font-weight: 400;
      line-height: 1.2;
      color: var(--color-carbon-ink);
      margin-bottom: 16px;
      letter-spacing: -0.01em;
    }

    .hero-desc {
      font-size: 16px;
      color: var(--color-ashen);
      line-height: 1.6;
    }

    /* Grid Layout */
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(340px, 1fr));
      gap: 32px;
    }

    /* Claude Paper Elevated Cards */
    .card {
      background-color: var(--color-paper-white);
      border-radius: 24px;
      padding: 32px;
      box-shadow: var(--shadow-card);
      border: 1px solid var(--color-chalk);
      display: flex;
      flex-direction: column;
      justify-content: space-between;
      transition: transform 0.2s ease, box-shadow 0.2s ease;
    }

    .card:hover {
      box-shadow: var(--shadow-hover);
      transform: translateY(-2px);
    }

    .card-header {
      margin-bottom: 24px;
    }

    .card-tag {
      font-size: 11px;
      text-transform: uppercase;
      letter-spacing: 0.05em;
      color: var(--color-ashen);
      font-weight: 600;
      margin-bottom: 8px;
    }

    .card-title {
      font-family: var(--font-serif);
      font-size: 24px;
      font-weight: 400;
      color: var(--color-carbon-ink);
      margin-bottom: 8px;
    }

    .card-desc {
      color: var(--color-ashen);
      font-size: 14px;
      line-height: 1.5;
    }

    /* Nested Stone Surface inside Cards */
    .nested-info {
      background-color: var(--color-soft-stone);
      border-radius: 16px;
      padding: 20px;
      margin-bottom: 24px;
    }

    .info-row {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding: 6px 0;
      border-bottom: 1px dashed var(--color-mist);
    }

    .info-row:last-child {
      border-bottom: none;
    }

    .info-label {
      color: var(--color-ashen);
      font-size: 13px;
    }

    .info-value {
      color: var(--color-carbon-ink);
      font-weight: 550;
      font-size: 13px;
    }

    /* Buttons */
    .btn {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      background-color: var(--color-carbon-ink);
      color: var(--color-bone-parchment);
      text-decoration: none;
      padding: 12px 24px;
      border-radius: 8px;
      font-size: 14px;
      font-weight: 500;
      border: none;
      cursor: pointer;
      width: 100%;
      transition: background-color 0.2s ease, opacity 0.2s ease;
    }

    .btn:hover {
      opacity: 0.9;
    }

    .btn-secondary {
      background-color: var(--color-soft-stone);
      color: var(--color-carbon-ink);
      border: 1px solid var(--color-chalk);
    }

    .btn-secondary:hover {
      background-color: var(--color-chalk);
    }

    /* Status Notification Panel */
    .status-panel {
      background-color: var(--color-paper-white);
      border-radius: 24px;
      padding: 32px;
      margin-top: 48px;
      border: 1px solid var(--color-chalk);
      box-shadow: var(--shadow-card);
    }

    .status-title {
      font-family: var(--font-serif);
      font-size: 24px;
      margin-bottom: 12px;
    }

    .code-box {
      background-color: var(--color-soft-stone);
      border-radius: 12px;
      padding: 16px;
      font-family: monospace;
      font-size: 13px;
      color: var(--color-graphite);
      overflow-x: auto;
      margin-top: 12px;
    }

    /* Footer Band */
    footer {
      background-color: var(--color-obsidian);
      color: var(--color-pebble);
      padding: 48px 32px;
      margin-top: auto;
    }

    .footer-content {
      max-width: 1200px;
      margin: 0 auto;
      display: flex;
      justify-content: space-between;
      align-items: center;
    }

    .footer-text {
      font-size: 13px;
    }
  </style>
</head>
<body>

  <header>
    <a href="/" class="brand">
      <span class="brand-mark"></span>
      Claude Edition
    </a>
    <nav>
      <a href="/" class="{{ if eq .PageType "home" }}active{{ end }}">Overview</a>
      <a href="/flash/checkout" class="{{ if eq .PageType "flash" }}active{{ end }}">Flash Sale</a>
      <a href="/concert/tickets" class="{{ if eq .PageType "concert" }}active{{ end }}">VIP Tickets</a>
    </nav>
  </header>

  <main>
    <section class="hero">
      <div class="badge">
        <span class="badge-dot"></span>
        {{ .Badge }}
      </div>
      <h1>{{ .Heading }}</h1>
      <p class="hero-desc">{{ .Description }}</p>
    </section>

    {{ if eq .PageType "home" }}
    <section class="grid">
      <!-- Card 1: Flash Sale Simulation -->
      <div class="card">
        <div>
          <div class="card-tag">Standard Queue • FIFO</div>
          <h2 class="card-title">Claude Book M4 Ultra Drop</h2>
          <p class="card-desc">Simulasikan traffic tinggi pada produk terbatas. Open WR mengizinkan batch user secara bertahap.</p>
        </div>
        <div>
          <div class="nested-info">
            <div class="info-row">
              <span class="info-label">Protected Endpoint</span>
              <span class="info-value">/flash/*</span>
            </div>
            <div class="info-row">
              <span class="info-label">Release Rate</span>
              <span class="info-value">120 users / min</span>
            </div>
            <div class="info-row">
              <span class="info-label">Active Capacity</span>
              <span class="info-value">5 concurrent users</span>
            </div>
          </div>
          <a href="/flash/checkout" class="btn">Uji Antrean Flash Sale &rarr;</a>
        </div>
      </div>

      <!-- Card 2: Ticket War Simulation -->
      <div class="card">
        <div>
          <div class="card-tag">Pre-Queue • Lottery Anti-Bot</div>
          <h2 class="card-title">VIP Concert Ticket War</h2>
          <p class="card-desc">Semua pengunjung masuk antrean awal. Sistem mengocok antrean secara fair (random lottery).</p>
        </div>
        <div>
          <div class="nested-info">
            <div class="info-row">
              <span class="info-label">Protected Endpoint</span>
              <span class="info-value">/concert/*</span>
            </div>
            <div class="info-row">
              <span class="info-label">Queue Mode</span>
              <span class="info-value">queue_all: true</span>
            </div>
            <div class="info-row">
              <span class="info-label">Method</span>
              <span class="info-value">random (lottery)</span>
            </div>
          </div>
          <a href="/concert/tickets" class="btn">Uji Ticket War Portal &rarr;</a>
        </div>
      </div>
    </section>

    <section class="status-panel">
      <h2 class="status-title">Informasi Integrasi Open WR</h2>
      <p class="card-desc">Halaman ini berjalan di server backend target (Port 3000). Semua request dari browser user telah disaring dan diautentikasi oleh proxy edge Open WR (Port 8080).</p>
      <div class="code-box">
Proxy Edge: http://localhost:8080<br>
Target Backend: http://demo-app:3000<br>
Active Open WR Session Cookie: __owr_flash_sale / __owr_ticket_war
      </div>
    </section>

    {{ else if eq .PageType "flash" }}
    <section class="grid" style="max-width: 680px; margin: 0 auto;">
      <div class="card">
        <div>
          <div class="card-tag">Akses Berhasil Diberikan</div>
          <h2 class="card-title">Checkout Claude Book M4 Ultra</h2>
          <p class="card-desc">Anda berhasil melewati antrean Open WR! Sesi checkout Anda aktif selama 5 menit.</p>
        </div>
        <div>
          <div class="nested-info">
            <div class="info-row">
              <span class="info-label">Item</span>
              <span class="info-value">Claude Book M4 Ultra (1TB, 36GB)</span>
            </div>
            <div class="info-row">
              <span class="info-label">Price</span>
              <span class="info-value">$2,499.00</span>
            </div>
            <div class="info-row">
              <span class="info-label">Status Sesi</span>
              <span class="info-value" style="color: #d97757;">Active (Cookie Verified)</span>
            </div>
          </div>

          <button id="buyBtn" class="btn" onclick="submitOrder()">Beli Sekarang (Simulasi API)</button>
          <a href="/" class="btn btn-secondary" style="margin-top: 12px; text-align: center;">Kembali ke Home</a>
          <div id="orderResult" style="margin-top: 16px;"></div>
        </div>
      </div>
    </section>

    {{ else if eq .PageType "concert" }}
    <section class="grid" style="max-width: 680px; margin: 0 auto;">
      <div class="card">
        <div>
          <div class="card-tag">Lottery Ticket Allocation</div>
          <h2 class="card-title">VIP Seat Selection</h2>
          <p class="card-desc">Selamat! Anda adalah salah satu pemenang antrean acak (Lottery Queue). Silakan pilih kategori tiket Anda.</p>
        </div>
        <div>
          <div class="nested-info">
            <div class="info-row">
              <span class="info-label">Kategori</span>
              <span class="info-value">VIP Front Stage (Row A)</span>
            </div>
            <div class="info-row">
              <span class="info-label">Harga</span>
              <span class="info-value">Rp 3.500.000</span>
            </div>
            <div class="info-row">
              <span class="info-label">Queue Engine</span>
              <span class="info-value">Open WR Lottery (Random)</span>
            </div>
          </div>

          <button id="buyBtn" class="btn" onclick="submitOrder()">Konfirmasi Pembayaran</button>
          <a href="/" class="btn btn-secondary" style="margin-top: 12px; text-align: center;">Kembali ke Home</a>
          <div id="orderResult" style="margin-top: 16px;"></div>
        </div>
      </div>
    </section>
    {{ end }}

  </main>

  <footer>
    <div class="footer-content">
      <div class="footer-text">
        Open WR Demo Simulation &bull; Powered by Anthropic Claude Editorial Design Language
      </div>
      <div class="footer-text">
        Engine Version 1.2.1
      </div>
    </div>
  </footer>

  <script>
    function submitOrder() {
      const btn = document.getElementById('buyBtn');
      const resultDiv = document.getElementById('orderResult');
      btn.innerText = 'Processing...';
      btn.disabled = true;

      fetch('/api/order', { method: 'POST' })
        .then(res => res.json())
        .then(data => {
          btn.innerText = 'Pesanan Berhasil!';
          resultDiv.innerHTML = '<div class="nested-info" style="background: #efeeeb; border: 1px solid #d97757;"><strong style="color: #121212;">' + data.message + '</strong><br><small style="color: #7b7974;">Order ID: ' + data.order_id + '</small></div>';
        })
        .catch(err => {
          btn.innerText = 'Gagal';
          btn.disabled = false;
          resultDiv.innerHTML = '<p style="color: red;">Error: ' + err.message + '</p>';
        });
    }
  </script>
</body>
</html>
`
