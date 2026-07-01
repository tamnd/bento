package node

import (
	"encoding/json"
	"net/http"
	"time"
)

// http.Agent is Node's connection pool. It maps onto Go's http.Transport, which
// is where keep-alive connection reuse lives: idle connections are kept per host
// and reused, and the Agent's knobs (keepAlive, maxSockets, maxFreeSockets,
// timeout) translate directly onto Transport fields. Each Agent is one
// http.Client with a configured Transport, held by id; requests name their agent
// so a benchmark hammering one endpoint reuses connections exactly as on Node.

// agentOptions is the subset of Node's http.Agent options bento maps.
type agentOptions struct {
	KeepAlive      *bool `json:"keepAlive"`
	MaxSockets     int   `json:"maxSockets"`
	MaxFreeSockets int   `json:"maxFreeSockets"`
	MaxTotalSocket int   `json:"maxTotalSockets"`
	Timeout        int   `json:"timeout"` // socket idle timeout in ms
}

// createAgent builds a client with a Transport configured from the Agent
// options and records it under the given id.
func (h *httpBridge) createAgent(args []any) (any, error) {
	id := int64(intArg(args, 0))
	var opts agentOptions
	if raw := str(args, 1); raw != "" {
		_ = json.Unmarshal([]byte(raw), &opts)
	}

	// Start from a clone of the default transport so proxy, dialer, and TLS
	// defaults carry over, then apply the Agent knobs.
	base, _ := http.DefaultTransport.(*http.Transport)
	tr := base.Clone()
	if opts.KeepAlive != nil && !*opts.KeepAlive {
		tr.DisableKeepAlives = true
	}
	if opts.MaxSockets > 0 {
		tr.MaxConnsPerHost = opts.MaxSockets
	}
	if opts.MaxFreeSockets > 0 {
		tr.MaxIdleConnsPerHost = opts.MaxFreeSockets
	}
	if opts.MaxTotalSocket > 0 {
		tr.MaxIdleConns = opts.MaxTotalSocket
	}
	if opts.Timeout > 0 {
		tr.IdleConnTimeout = time.Duration(opts.Timeout) * time.Millisecond
	}

	h.mu.Lock()
	h.agents[id] = &http.Client{Transport: tr}
	h.mu.Unlock()
	return nil, nil
}

// closeAgent drops an agent and closes its idle connections.
func (h *httpBridge) closeAgent(args []any) (any, error) {
	id := int64(intArg(args, 0))
	h.mu.Lock()
	client := h.agents[id]
	delete(h.agents, id)
	h.mu.Unlock()
	if client != nil {
		if tr, ok := client.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}
	return nil, nil
}

// clientFor returns the client a request should use. Agent id 0 is the implicit
// default (http.DefaultClient, keep-alive on); any other id names a registered
// Agent, falling back to the default if it was closed.
func (h *httpBridge) clientFor(agentID int64) *http.Client {
	if agentID == 0 {
		return http.DefaultClient
	}
	h.mu.Lock()
	client := h.agents[agentID]
	h.mu.Unlock()
	if client == nil {
		return http.DefaultClient
	}
	return client
}
