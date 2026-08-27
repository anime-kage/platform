package handler

// The live-chat fan-out. One in-process hub broadcasting to
// Server-Sent Event subscribers.
//
// SSE rather than WebSockets on purpose: chat is one-way from the server's
// point of view (sends arrive as ordinary POSTs), EventSource reconnects by
// itself, and it survives any proxy that speaks HTTP/1.1 without an upgrade
// dance. The cost is one goroutine and one buffered channel per viewer, which
// for an invite-only community is nothing.
//
// It lives in-process, so a second API instance would need Postgres LISTEN or
// Redis to share the fan-out. That's a scaling decision, not a rewrite: only
// publish and the subscriber map would change.

import (
	"sync"
	"sync/atomic"
)

// chatBuffer is how far a slow reader may fall behind before we give up on it.
// Dropping the connection is deliberate: a stalled client must never be able to
// block the sender, and EventSource reconnects on its own.
const chatBuffer = 32

type chatHub struct {
	mu      sync.RWMutex
	subs    map[int64]chan []byte
	nextID  atomic.Int64
	viewers atomic.Int32
}

func newChatHub() *chatHub {
	return &chatHub{subs: make(map[int64]chan []byte)}
}

// subscribe registers a listener and returns it with its own unsubscribe.
func (h *chatHub) subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, chatBuffer)
	id := h.nextID.Add(1)

	h.mu.Lock()
	h.subs[id] = ch
	h.mu.Unlock()
	h.viewers.Add(1)

	return ch, func() {
		h.mu.Lock()
		if c, ok := h.subs[id]; ok {
			delete(h.subs, id)
			close(c)
			h.viewers.Add(-1)
		}
		h.mu.Unlock()
	}
}

// publish fans a pre-encoded frame out to every subscriber. It never blocks:
// a subscriber whose buffer is full is skipped, and its connection will be torn
// down by the writer when it notices.
func (h *chatHub) publish(frame []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.subs {
		select {
		case ch <- frame:
		default: // slow reader — drop the frame rather than stall everyone
		}
	}
}

// viewerCount is the "online" number in the panel header. It counts open
// streams, not distinct people: two tabs are two viewers. That is the honest
// reading of "how many are watching this right now".
func (h *chatHub) viewerCount() int {
	return int(h.viewers.Load())
}
