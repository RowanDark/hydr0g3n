package bench

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"time"
)

// Server provides a lightweight HTTP server with predefined benchmarking
// endpoints. The server exposes fast and slow handlers as well as a custom 404
// template response for unknown routes.
type Server struct {
	srv      *httptest.Server
	notFound *template.Template
}

// NewServer initialises a new benchmarking server instance.
func NewServer() *Server {
	s := &Server{
		notFound: template.Must(template.New("404").Parse(`<!DOCTYPE html>
<html lang="en">
<head><title>Not Found</title></head>
<body>
        <h1>404 Not Found</h1>
        <p>The requested path {{.Path}} could not be located.</p>
</body>
</html>`)),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/fast", s.fastHandler)
	mux.HandleFunc("/slow", s.slowHandler)
	mux.HandleFunc("/", s.notFoundHandler)

	s.srv = httptest.NewServer(mux)
	return s
}

// URL returns the base URL of the benchmarking server.
func (s *Server) URL() string {
	if s == nil || s.srv == nil {
		return ""
	}
	return s.srv.URL
}

// Close terminates the underlying HTTP server.
func (s *Server) Close() {
	if s == nil || s.srv == nil {
		return
	}
	s.srv.Close()
}

func (s *Server) fastHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) slowHandler(w http.ResponseWriter, r *http.Request) {
	// Introduce a modest delay to simulate a heavier handler without making
	// the benchmarks excessively long.
	time.Sleep(10 * time.Millisecond)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"slow"}`))
}

func (s *Server) notFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_ = s.notFound.Execute(w, map[string]string{"Path": r.URL.Path})
}
