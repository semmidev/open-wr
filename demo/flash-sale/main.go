package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"
)

//go:embed templates/index.html
var templateFS embed.FS

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
		Title:       "VIP Tech Store — Open WR Protected Target",
		PageType:    "home",
		Badge:       "Open WR Target Server",
		Heading:     "Welcome to VIP Tech Store",
		Description: "This application is currently protected by Open WR edge proxy. Navigate through the high-traffic portals below to simulate queueing behavior.",
	})
}

func (a *DemoApp) handleFlashSale(w http.ResponseWriter, r *http.Request) {
	renderPage(w, pageData{
		Title:       "Flash Sale — Pro Laptop M4 Ultra",
		PageType:    "flash",
		Badge:       "Protected by Open WR Flash Sale Room",
		Heading:     "Pro Laptop M4 Ultra (Limited Drop)",
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
	tmpl, err := template.ParseFS(templateFS, "templates/index.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = tmpl.Execute(w, data)
}
