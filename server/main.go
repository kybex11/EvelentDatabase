package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"db/server/handlers"
	"db/server/internal/db"
)

func enableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

func routeCollectionSubpaths(w http.ResponseWriter, r *http.Request, path string) bool {
	path = strings.TrimSuffix(path, "/")
	if r.Method == http.MethodGet && strings.HasSuffix(path, "/stats") {
		handlers.CollectionStatsHandler(w, r)
		return true
	}
	if !strings.Contains(path, "/indexes") {
		return false
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	idxAt := -1
	for i, p := range parts {
		if p == "indexes" {
			idxAt = i
			break
		}
	}
	if idxAt < 0 {
		return false
	}
	hasField := idxAt+1 < len(parts) && parts[idxAt+1] != ""
	switch r.Method {
	case http.MethodDelete:
		if hasField {
			handlers.DropIndexHandler(w, r)
			return true
		}
	case http.MethodGet:
		if !hasField {
			handlers.ListIndexesHandler(w, r)
			return true
		}
	case http.MethodPost:
		if !hasField {
			handlers.CreateIndexHandler(w, r)
			return true
		}
	}
	return false
}

// kvRouter dispatches /api/kv/... requests to either the collection-level
// handler (keys, stats, flush, batch ops) or the per-key handler.
func kvRouter() http.HandlerFunc {
	collectionOps := map[string]bool{
		"keys": true, "stats": true, "flush": true,
		"mget": true, "mset": true, "mdel": true,
	}
	return func(w http.ResponseWriter, r *http.Request) {
		rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/kv/"), "/")
		if collectionOps[rest] {
			handlers.KVCollectionHandler(w, r)
			return
		}
		handlers.KVKeyHandler(w, r)
	}
}

func apiHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if routeCollectionSubpaths(w, r, path) {
			return
		}
		docsRoute := strings.Contains(path, "/docs/") || strings.HasSuffix(strings.TrimSuffix(path, "/"), "/docs")
		if docsRoute || strings.Contains(path, "/find") {
			if r.Method == http.MethodPost && strings.HasSuffix(path, "/docs/batch") {
				handlers.InsertManyDocumentsHandler(w, r)
				return
			}
			if r.Method == http.MethodPost && strings.HasSuffix(strings.TrimSuffix(path, "/"), "/docs") {
				handlers.InsertDocumentHandler(w, r)
				return
			}
			if r.Method == http.MethodGet && strings.Contains(path, "/docs/") {
				handlers.GetDocumentHandler(w, r)
				return
			}
			if r.Method == http.MethodPut && strings.Contains(path, "/docs/") {
				handlers.UpdateDocumentHandler(w, r)
				return
			}
			if r.Method == http.MethodDelete && strings.Contains(path, "/docs/") {
				handlers.DeleteDocumentHandler(w, r)
				return
			}
			if r.Method == http.MethodPost && strings.HasSuffix(path, "/find") {
				handlers.FindDocumentsHandler(w, r)
				return
			}
		}
		handlers.CollectionHandler(w, r)
	}
}

func main() {
	var (
		addr              string
		portFlag          string
		dataDir           string
		gomaxprocs        int
		readHeaderTimeout time.Duration
		readTimeout       time.Duration
		writeTimeout      time.Duration
		idleTimeout       time.Duration
	)

	flag.StringVar(&addr, "addr", ":8080", "TCP address to listen on (e.g. :8080 or 0.0.0.0:9090); ignored if -port is set")
	flag.StringVar(&portFlag, "port", "", "Listen port or address: 8080 → :8080; or :9090; or 127.0.0.1:3000 (overrides -addr)")
	flag.StringVar(&dataDir, "data-dir", "", "Directory for storing database files (default: executable directory + /data)")
	flag.IntVar(&gomaxprocs, "gomaxprocs", 0, "GOMAXPROCS value (0 = use all CPU cores)")
	flag.DurationVar(&readHeaderTimeout, "http-read-header-timeout", 10*time.Second, "Maximum duration for reading request headers")
	flag.DurationVar(&readTimeout, "http-read-timeout", 60*time.Second, "Maximum duration for reading the entire request")
	flag.DurationVar(&writeTimeout, "http-write-timeout", 0, "Maximum duration before timing out writes (0 = no timeout)")
	flag.DurationVar(&idleTimeout, "http-idle-timeout", 180*time.Second, "Maximum amount of time to wait for the next request when keep-alives are enabled")

	flag.Parse()

	if strings.TrimSpace(portFlag) != "" {
		pf := strings.TrimSpace(portFlag)
		if strings.Contains(pf, ":") {
			addr = pf
		} else {
			addr = ":" + pf
		}
	} else if flag.NArg() > 0 {
		portArg := flag.Arg(0)
		if !strings.Contains(portArg, ":") {
			portArg = ":" + portArg
		}
		addr = portArg
	}

	if gomaxprocs > 0 {
		runtime.GOMAXPROCS(gomaxprocs)
	}

	if dataDir == "" {
		execPath, err := os.Executable()
		if err != nil {
			log.Fatalf("Failed to get executable path: %v", err)
		}
		dataDir = filepath.Join(filepath.Dir(execPath), "data")
	}

	database, err := db.NewDatabase(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	handlers.Init(database)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handlers.HealthHandler)
	mux.HandleFunc("/api/collections", enableCORS(handlers.CollectionsHandler))
	mux.HandleFunc("/api/collections/", enableCORS(apiHandler()))
	mux.HandleFunc("/api/kv", enableCORS(handlers.KVCollectionHandler))
	mux.HandleFunc("/api/kv/", enableCORS(kvRouter()))

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", addr, err)
	}

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    1 << 20,
	}

	log.Printf("Starting server on %s", addr)
	log.Printf("Data directory: %s", dataDir)
	log.Printf("GOMAXPROCS = %d", runtime.GOMAXPROCS(0))

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("Shutting down, flushing in-memory store...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown error: %v", err)
	}
	if err := database.Close(); err != nil {
		log.Printf("database close error: %v", err)
	}
	log.Println("Goodbye.")
}
