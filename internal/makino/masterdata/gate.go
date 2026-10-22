package masterdata

import (
	"context"
	"sync"
)

// Gate rejects new work while allowing admitted requests to finish. It is not a
// RWMutex: waiting HTTP/gRPC requests must get maintenance, not wait for a writer.
type Gate struct {
	mu      sync.Mutex
	paused  bool
	active  int
	drained chan struct{}
}

func (g *Gate) Enter() (leave func(), accepted bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.paused {
		return nil, false
	}
	g.active++
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			g.active--
			if g.active == 0 && g.drained != nil {
				close(g.drained)
				g.drained = nil
			}
		})
	}, true
}

func (g *Gate) Pause(ctx context.Context) error {
	g.mu.Lock()
	g.paused = true
	if g.active == 0 {
		g.mu.Unlock()
		return nil
	}
	if g.drained == nil {
		g.drained = make(chan struct{})
	}
	done := g.drained
	g.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *Gate) Resume() { g.mu.Lock(); g.paused = false; g.mu.Unlock() }
