package api

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDetailJSONCacheCoalescesConcurrentLoads(t *testing.T) {
	cache := newDetailJSONCache()
	started := make(chan struct{})
	release := make(chan struct{})
	var loads atomic.Int32
	load := func() ([]byte, error) {
		if loads.Add(1) == 1 {
			close(started)
		}
		<-release
		return []byte(`{"ok":true}`), nil
	}

	const callers = 8
	results := make(chan string, callers)
	var wait sync.WaitGroup
	wait.Add(callers)
	for range callers {
		go func() {
			defer wait.Done()
			payload, err := cache.get(context.Background(), "node-latency:example-node-a:1d", time.Second, load)
			if err != nil {
				results <- "error: " + err.Error()
				return
			}
			results <- string(payload)
		}()
	}
	<-started
	close(release)
	wait.Wait()
	close(results)

	if got := loads.Load(); got != 1 {
		t.Fatalf("load count = %d, want 1", got)
	}
	for result := range results {
		if result != `{"ok":true}` {
			t.Fatalf("payload = %q", result)
		}
	}
}

func TestDetailJSONCachePrunesEntriesUsingStoredFreshness(t *testing.T) {
	cache := newDetailJSONCache()
	now := time.Now().Add(-2 * time.Minute)
	cache.entries["short"] = detailJSONCacheEntry{payload: []byte(`{"short":true}`), updatedAt: now, maxAge: time.Minute}
	cache.entries["long"] = detailJSONCacheEntry{payload: []byte(`{"long":true}`), updatedAt: now, maxAge: 5 * time.Minute}

	if _, err := cache.get(context.Background(), "trigger", time.Minute, func() ([]byte, error) {
		return []byte(`{"trigger":true}`), nil
	}); err != nil {
		t.Fatalf("trigger prune: %v", err)
	}

	cache.mu.Lock()
	_, hasShort := cache.entries["short"]
	_, hasLong := cache.entries["long"]
	cache.mu.Unlock()
	if hasShort || !hasLong {
		t.Fatalf("stored freshness pruning short=%v long=%v, want false/true", hasShort, hasLong)
	}
}

func TestDetailJSONCacheRefreshWinsOverOlderInflightLoad(t *testing.T) {
	cache := newDetailJSONCache()
	oldStarted := make(chan struct{})
	releaseOld := make(chan struct{})
	oldDone := make(chan string, 1)
	go func() {
		payload, err := cache.get(context.Background(), "node-state:example-node-a:1h", time.Second, func() ([]byte, error) {
			close(oldStarted)
			<-releaseOld
			return []byte(`{"value":"old"}`), nil
		})
		if err != nil {
			oldDone <- "error: " + err.Error()
			return
		}
		oldDone <- string(payload)
	}()
	<-oldStarted

	payload, err := cache.refreshWithAge("node-state:example-node-a:1h", detailCacheFreshFor, func() ([]byte, error) {
		return []byte(`{"value":"fresh"}`), nil
	})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if string(payload) != `{"value":"fresh"}` {
		t.Fatalf("refresh payload = %s", payload)
	}
	close(releaseOld)
	if oldResult := <-oldDone; oldResult != `{"value":"fresh"}` {
		t.Fatalf("invalidated in-flight caller received %s", oldResult)
	}

	payload, err = cache.get(context.Background(), "node-state:example-node-a:1h", time.Second, func() ([]byte, error) {
		return []byte(`{"value":"unexpected"}`), nil
	})
	if err != nil {
		t.Fatalf("cached get: %v", err)
	}
	if string(payload) != `{"value":"fresh"}` {
		t.Fatalf("cached payload = %s, want fresh", payload)
	}
}
