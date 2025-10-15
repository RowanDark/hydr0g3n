package hydroapi

import (
	"context"
	"errors"
	"sync"

	"hydr0g3n/pkg/engine"
)

// Config is an alias to engine.Config so callers can configure scans using the
// same options as the command line.
type Config = engine.Config

// Result matches engine.Result and is returned through the result channel
// supplied to StartScan.
type Result = engine.Result

// API coordinates execution of scans via the fuzzing engine. It wraps engine
// primitives with a small interface that is easy to embed inside other Go
// programs.
type API struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	running bool
}

// New returns a ready-to-use API instance.
func New() *API {
	return &API{}
}

// StartScan launches a scan with the provided configuration. Results are
// streamed to the supplied channel until the scan completes or StopScan is
// called. The channel is closed automatically when the scan stops. It is an
// error to invoke StartScan while another scan is running on the same API
// instance.
func (a *API) StartScan(ctx context.Context, cfg Config, results chan Result) error {
	if results == nil {
		return errors.New("results channel cannot be nil")
	}

	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return errors.New("a scan is already running")
	}

	scanCtx, cancel := context.WithCancel(ctx)
	stream, err := engine.Run(scanCtx, engine.Config(cfg))
	if err != nil {
		cancel()
		a.mu.Unlock()
		return err
	}

	done := make(chan struct{})
	a.cancel = cancel
	a.done = done
	a.running = true
	a.mu.Unlock()

	go func() {
		defer close(results)
		defer a.finalize(done)

		for res := range stream {
			select {
			case <-scanCtx.Done():
				return
			case results <- Result(res):
			}
		}
	}()

	return nil
}

// StopScan cancels the currently running scan (if any) and waits for it to
// finish. Calling StopScan when no scan is running is a no-op.
func (a *API) StopScan() {
	a.mu.Lock()
	cancel := a.cancel
	done := a.done
	running := a.running
	a.mu.Unlock()

	if !running {
		return
	}

	if cancel != nil {
		cancel()
	}

	if done != nil {
		<-done
	}
}

func (a *API) finalize(done chan struct{}) {
	a.mu.Lock()
	a.running = false
	a.cancel = nil
	a.done = nil
	a.mu.Unlock()

	close(done)
}
