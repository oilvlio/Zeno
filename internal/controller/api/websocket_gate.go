package api

import (
	"sync"
)

type websocketGate struct {
	mu        sync.Mutex
	current   int
	max       int
	maxPerKey int
	byKey     map[string]int
}

func newWebSocketGate(max int) *websocketGate {
	return newWebSocketGateWithPerKey(max, 0)
}

func newWebSocketGateWithPerKey(max, maxPerKey int) *websocketGate {
	return &websocketGate{max: max, maxPerKey: maxPerKey, byKey: make(map[string]int)}
}

func (gate *websocketGate) acquire() (func(), bool) {
	return gate.acquireFor("")
}

func (gate *websocketGate) acquireFor(key string) (func(), bool) {
	if gate == nil || gate.max <= 0 {
		return func() {}, true
	}
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if gate.current >= gate.max || (key != "" && gate.maxPerKey > 0 && gate.byKey[key] >= gate.maxPerKey) {
		return nil, false
	}
	gate.current++
	if key != "" && gate.maxPerKey > 0 {
		gate.byKey[key]++
	}
	released := false
	return func() {
		gate.mu.Lock()
		defer gate.mu.Unlock()
		if released {
			return
		}
		released = true
		if gate.current > 0 {
			gate.current--
		}
		if key != "" && gate.maxPerKey > 0 {
			if gate.byKey[key] <= 1 {
				delete(gate.byKey, key)
			} else {
				gate.byKey[key]--
			}
		}
	}, true
}
