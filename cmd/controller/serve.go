package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/shui1iao/zeno/internal/controller/api"
)

// runCheckDB implements -check-db: migrate and verify, then exit.
func runCheckDB(options *controllerOptions) error {
	if options.dbPath == "" {
		return errors.New("-check-db requires -db")
	}
	return checkSQLiteDatabase(context.Background(), options.dbPath, options.checkDBTimeout)
}

// runResetAdminPassword implements offline admin recovery, then exits.
// Every admin session is revoked so a leaked cookie cannot outlive the reset.
func runResetAdminPassword(options *controllerOptions) error {
	if options.dbPath == "" {
		return errors.New("-reset-admin-password-file requires -db")
	}
	password, err := readSecret("", options.resetAdminPasswordFile)
	if err != nil {
		return err
	}
	store, err := api.OpenSQLiteStore(options.dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	if err := store.ResetAdminAccount(context.Background(), password); err != nil {
		return err
	}
	log.Print("admin account reset; all admin sessions revoked")
	return nil
}

// startLocalProbeCollector runs the controller-local collector until the
// returned stop function is called. Stop waits for the goroutine so a shutdown
// cannot leave a probe writing into a closing store.
func startLocalProbeCollector(runtime *controllerRuntime, options *controllerOptions) (func(), error) {
	if runtime.Store == nil {
		return nil, errors.New("-collect-local requires -db")
	}
	if err := validateLocalProbeInterval(options.probeInterval); err != nil {
		return nil, err
	}
	collector := api.NewLocalProbeCollector(runtime.Store, api.LocalProbeCollectorOptions{NodeID: options.nodeID})

	var waitGroup sync.WaitGroup
	collectorCtx, stopCollector := context.WithCancel(context.Background())
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		ticker := time.NewTicker(options.probeInterval)
		defer ticker.Stop()
		for {
			if err := collector.CollectOnce(collectorCtx); err != nil {
				log.Printf("local probe collection failed: %v", err)
			}
			select {
			case <-collectorCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	log.Printf("controller-local probe collector enabled for node %s every %s",
		options.nodeID, options.probeInterval.String())

	return func() {
		stopCollector()
		done := make(chan struct{})
		go func() {
			waitGroup.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			log.Printf("controller-local probe collector cleanup timed out")
		}
	}, nil
}

func validateLocalProbeInterval(interval time.Duration) error {
	if interval <= 0 {
		return errors.New("-probe-interval must be greater than zero")
	}
	return nil
}

// logStartupSummary records the optional subsystems that are active, so a
// deployment's effective configuration is visible in the first log lines.
func logStartupSummary(options *controllerOptions) {
	log.Printf("zeno controller listening on %s", options.addr)
	if options.webDir != "" {
		log.Printf("serving dashboard from %s", options.webDir)
	}
	if options.dbPath != "" {
		log.Printf("using SQLite data store %s", options.dbPath)
	}
	if options.agentBinaryPath != "" {
		log.Printf("serving agent binary from %s", options.agentBinaryPath)
	}
	if options.seedPreview {
		log.Printf("seeded preview data for node %s", options.nodeID)
	}
}

// serveHTTP runs the server until it fails or a termination signal arrives,
// draining in-flight requests before returning.
func serveHTTP(handler http.Handler, addr string) error {
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	shutdownCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	serverErr := make(chan error, 1)
	go func() { serverErr <- server.ListenAndServe() }()

	select {
	case err := <-serverErr:
		return ignoreServerClosed(err)
	case <-shutdownCtx.Done():
		log.Printf("shutdown signal received; draining HTTP requests")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			// Shutdown only reports that draining did not finish in time;
			// Close then ends the remaining connections.
			log.Printf("graceful shutdown timed out: %v; closing server", err)
			_ = server.Close()
		}
		return ignoreServerClosed(<-serverErr)
	}
}

// ignoreServerClosed treats the expected post-shutdown error as success.
func ignoreServerClosed(err error) error {
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
