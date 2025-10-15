package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"time"

	"hydr0g3n/pkg/hydroapi"
)

// This example demonstrates how to embed the hydr0g3n engine inside another
// Go program. It spins up a local HTTP server, runs a short scan against it
// and stops the scan after a timeout.
func main() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "admin") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		fmt.Fprintf(w, "saw %s", r.URL.Path)
	}))
	defer server.Close()

	cfg := hydroapi.Config{
		URL:         server.URL + "/FUZZ",
		Wordlist:    filepath.Join("wordlists", "sample_small.txt"),
		Method:      http.MethodGet,
		Concurrency: 5,
		Timeout:     5 * time.Second,
	}

	api := hydroapi.New()
	ctx := context.Background()
	results := make(chan hydroapi.Result)

	stopTimer := time.AfterFunc(2*time.Second, func() {
		fmt.Println("stopping scan...")
		api.StopScan()
	})
	defer stopTimer.Stop()

	if err := api.StartScan(ctx, cfg, results); err != nil {
		log.Fatalf("start scan: %v", err)
	}

	for res := range results {
		if res.Err != nil {
			log.Printf("request error: %v", res.Err)
			continue
		}

		fmt.Printf("%s -> %d (%d bytes)\n", res.URL, res.StatusCode, len(res.Body))
	}

	fmt.Println("scan finished")
}
