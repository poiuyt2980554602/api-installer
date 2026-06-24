package main

import (
	"bytes"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

var (
	Version         = "dev"
	UpstreamVersion = "unknown"
	BuiltAt         = "unknown"
)

//go:embed web
var embeddedWeb embed.FS

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.BoolVar(showVersion, "v", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Pixel Image Playground %s (upstream: %s, built: %s)\n", Version, UpstreamVersion, BuiltAt)
		return
	}

	webFS, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		log.Fatalf("load embedded web files: %v", err)
	}

	host := firstNonEmpty(os.Getenv("IMAGE_PLAYGROUND_HOST"), os.Getenv("SERVER_HOST"), "0.0.0.0")
	port := firstNonEmpty(os.Getenv("IMAGE_PLAYGROUND_PORT"), os.Getenv("SERVER_PORT"), "8090")
	addr := net.JoinHostPort(host, port)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/", serveSPA(webFS))

	server := &http.Server{
		Addr:              addr,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("Pixel Image Playground %s listening on http://%s", Version, addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server failed: %v", err)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func serveSPA(webFS fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		filePath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if filePath == "." || filePath == "" {
			filePath = "index.html"
		}

		if info, err := fs.Stat(webFS, filePath); err == nil && info.IsDir() {
			filePath = path.Join(filePath, "index.html")
		}

		data, err := fs.ReadFile(webFS, filePath)
		if err != nil {
			filePath = "index.html"
			data, err = fs.ReadFile(webFS, filePath)
			if err != nil {
				http.Error(w, "web bundle is missing index.html", http.StatusInternalServerError)
				return
			}
		}

		setStaticHeaders(w, filePath)
		http.ServeContent(w, r, filePath, time.Time{}, bytes.NewReader(data))
	}
}

func setStaticHeaders(w http.ResponseWriter, filePath string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")

	if filePath == "index.html" || strings.HasSuffix(filePath, ".webmanifest") || strings.HasSuffix(filePath, "sw.js") {
		w.Header().Set("Cache-Control", "no-cache")
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.RequestURI(), recorder.status, time.Since(start).Round(time.Millisecond))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
