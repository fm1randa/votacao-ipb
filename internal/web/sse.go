package web

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Hub é um pub/sub em memória: cada cliente SSE assina um canal; toda mutação
// chama Broadcast, que emite um "tick" para todos. O sinal é leve — o cliente
// re-busca o fragmento da sua tela (ver ADR-0007).
type Hub struct {
	mu   sync.Mutex
	subs map[chan struct{}]bool
}

func newHub() *Hub { return &Hub{subs: map[chan struct{}]bool{}} }

func (h *Hub) sub() chan struct{} {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.subs[ch] = true
	h.mu.Unlock()
	return ch
}

func (h *Hub) unsub(ch chan struct{}) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

func (h *Hub) Broadcast() {
	h.mu.Lock()
	for ch := range h.subs {
		select {
		case ch <- struct{}{}:
		default: // já há um tick pendente; o cliente vai re-sincronizar
		}
	}
	h.mu.Unlock()
}

// events é o stream SSE. Público — só sinaliza "estado mudou", sem dado sensível.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming não suportado", http.StatusInternalServerError)
		return
	}
	// SSE é longo: limpa o WriteTimeout do servidor para esta resposta.
	if rc := http.NewResponseController(w); rc != nil {
		rc.SetWriteDeadline(time.Time{})
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.hub.sub()
	defer s.hub.unsub(ch)
	fmt.Fprint(w, ": ok\n\n") // abre o stream
	fl.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			fmt.Fprint(w, "event: tick\ndata: 1\n\n")
			fl.Flush()
		case <-time.After(25 * time.Second):
			fmt.Fprint(w, ": ping\n\n") // keepalive
			fl.Flush()
		}
	}
}

// mut envolve um handler de mutação: roda o handler e dispara o Broadcast depois.
func (s *Server) mut(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(w, r)
		s.hub.Broadcast()
	}
}
