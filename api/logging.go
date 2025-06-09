package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

// LoggingMiddleware logs all incoming HTTP requests
func LoggingHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip detailed logging for WebSocket upgrade requests
		if r.Header.Get("Upgrade") == "websocket" {
			log.Printf("[WebSocket] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()

		// Log request details
		log.Printf("=== Incoming Request ===")
		log.Printf("Time: %s", start.Format(time.RFC3339))
		log.Printf("Method: %s", r.Method)
		log.Printf("URL: %s", r.URL.String())
		log.Printf("Remote Addr: %s", r.RemoteAddr)
		log.Printf("Headers:")
		for key, values := range r.Header {
			for _, value := range values {
				log.Printf("  %s: %s", key, value)
			}
		}

		// Log request body if present
		if r.Body != nil && r.ContentLength > 0 {
			// Read the body
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				log.Printf("Error reading body: %v", err)
			} else {
				// Restore the body for the actual handler
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

				// Try to pretty-print JSON
				if r.Header.Get("Content-Type") == "application/json" {
					var jsonData interface{}
					if err := json.Unmarshal(bodyBytes, &jsonData); err == nil {
						prettyJSON, _ := json.MarshalIndent(jsonData, "", "  ")
						log.Printf("Body (JSON):\n%s", string(prettyJSON))
					} else {
						log.Printf("Body (raw): %s", string(bodyBytes))
					}
				} else {
					log.Printf("Body (raw): %s", string(bodyBytes))
				}
			}
		}

		// Create a custom response writer to capture the status code
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Call the next handler
		next.ServeHTTP(lrw, r)

		// Log response details
		duration := time.Since(start)
		log.Printf("=== Response ===")
		log.Printf("Status: %d", lrw.statusCode)
		log.Printf("Duration: %v", duration)
		log.Printf("=================\n")
	})
}

// loggingResponseWriter wraps http.ResponseWriter to capture the status code
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

// Hijack implements the http.Hijacker interface to support WebSocket connections
func (lrw *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := lrw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, fmt.Errorf("ResponseWriter does not implement http.Hijacker")
}

// WebSocketLoggingHandler logs WebSocket connections
func WebSocketLoggingHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") == "websocket" {
			log.Printf("=== WebSocket Connection ===")
			log.Printf("Time: %s", time.Now().Format(time.RFC3339))
			log.Printf("Remote Addr: %s", r.RemoteAddr)
			log.Printf("URL: %s", r.URL.String())
			log.Printf("===========================\n")
		}
		handler.ServeHTTP(w, r)
	})
}
