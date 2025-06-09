package api

import (
	"bufio"
	"bytes"
	"encoding/hex"
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
					// Use custom logging that handles digest arrays
					logPrettyJSON(bodyBytes)
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

// logPrettyJSON logs JSON with special handling for digest arrays
func logPrettyJSON(data []byte) {
	var jsonData interface{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		log.Printf("Body (raw): %s", string(data))
		return
	}

	// Convert the data for pretty printing
	prettyData := convertDigestsForLogging(jsonData)
	prettyJSON, err := json.MarshalIndent(prettyData, "", "  ")
	if err != nil {
		log.Printf("Body (raw): %s", string(data))
		return
	}

	log.Printf("Body (JSON):\n%s", string(prettyJSON))
}

// convertDigestsForLogging recursively converts 32-byte arrays to hex strings for logging only
func convertDigestsForLogging(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, v := range val {
			// Special handling for fields that might contain digests
			if isDigestField(k) {
				if arr, ok := v.([]interface{}); ok && len(arr) == 32 {
					if hexStr := tryConvertToHex(arr); hexStr != "" {
						result[k] = hexStr
					} else {
						result[k] = convertDigestsForLogging(v)
					}
				} else {
					result[k] = convertDigestsForLogging(v)
				}
			} else {
				result[k] = convertDigestsForLogging(v)
			}
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, elem := range val {
			result[i] = convertDigestsForLogging(elem)
		}
		return result
	default:
		return v
	}
}

// isDigestField checks if a field name likely contains a digest
func isDigestField(fieldName string) bool {
	// Add field names that typically contain digests
	digestFields := []string{"thread_id", "space_id", "model_id", "parent_id", "message_id", "user_id", "id"}
	for _, df := range digestFields {
		if fieldName == df {
			return true
		}
	}
	return false
}

// tryConvertToHex attempts to convert an array to hex if it looks like bytes
func tryConvertToHex(arr []interface{}) string {
	bytes := make([]byte, 32)
	for i, elem := range arr {
		if num, ok := elem.(float64); ok && num >= 0 && num <= 255 {
			bytes[i] = byte(num)
		} else {
			return "" // Not a byte array
		}
	}
	return hex.EncodeToString(bytes)
}
