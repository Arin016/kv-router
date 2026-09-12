package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/arinmallanna/kv-router/internal/backend"
	"github.com/arinmallanna/kv-router/internal/scorer"
	"github.com/arinmallanna/kv-router/internal/telemetry"
	"github.com/arinmallanna/kv-router/internal/tokenizer"
)

const (
	headerBackend   = "X-KV-Router-Backend"
	maxRequestBytes = 10 << 20 // 10 MiB
)

// handleChatCompletions is the core routing handler. It:
//  1. Parses the OpenAI-compatible request
//  2. Hashes the message prefix for KV-cache lookup
//  3. Queries the radix tree for per-backend prefix matches
//  4. Builds backend state from eviction model + pool health
//  5. Scores and selects the optimal backend
//  6. Records the routing decision
//  7. Proxies the request (streaming or non-streaming)
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// --- 1. Parse request ---
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maxRequestBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "request body too large or unreadable")
		return
	}

	var req ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages array must not be empty")
		return
	}

	// --- 2. Hash message prefix ---
	tokMsgs := make([]tokenizer.Message, len(req.Messages))
	for i, m := range req.Messages {
		tokMsgs[i] = tokenizer.Message{Role: m.Role, Content: m.Content}
	}
	prefixHashes := s.tokenizer.HashPrefix(tokMsgs)
	totalBlocks := len(prefixHashes)

	// --- 3. Cache directory lookup, scoped by model and API contract ---
	// Engine adapters can extend this namespace with tokenizer/template/revision
	// identifiers; never reuse affinity across an explicitly different model.
	namespace := "chat:" + req.Model

	// --- 4. Build backend states ---
	healthyBackends := s.pool.Healthy()
	if len(healthyBackends) == 0 {
		writeError(w, http.StatusServiceUnavailable, "no healthy backends available")
		return
	}

	states := make([]scorer.BackendState, 0, len(healthyBackends))
	matchesByBackend := make(map[string]int, len(healthyBackends))
	for _, b := range healthyBackends {
		matchedBlocks := s.cache.Lookup(b.ID, namespace, prefixHashes)
		matchesByBackend[b.ID] = matchedBlocks
		usage := s.cache.Usage(b.ID)

		states = append(states, scorer.BackendState{
			ID:            b.ID,
			MatchedBlocks: matchedBlocks,
			QueueDepth:    int(b.QueueDepth()),
			MaxQueueDepth: 64, // reasonable default max concurrent requests
			UsedBlocks:    usage.Used,
			TotalCapacity: usage.Capacity,
			Healthy:       true, // already filtered
		})
	}

	// --- 5. Rank backends, reserve in order ---
	// Scoring sees a snapshot; by reserve time the winner may be full.
	// Walk the ranking and take the first backend that is still healthy
	// and reserves, so one contended winner can't fail the request while
	// capacity sits idle elsewhere. Fixes #1.
	routeReq := &scorer.Request{
		BlockHashes: prefixHashes,
		TotalBlocks: totalBlocks,
	}
	ranked := s.scorer.Rank(routeReq, states)
	var selectedBackend *backend.Backend
	var selectedState scorer.BackendState
	selectedID := ""
	for _, candidate := range ranked {
		b := s.pool.Get(candidate.ID)
		if b == nil || !b.IsHealthy() {
			continue
		}
		if !b.TryReserve() {
			continue
		}
		selectedBackend = b
		selectedID = candidate.ID
		selectedState = candidate
		break
	}
	if selectedBackend == nil {
		if len(ranked) == 0 {
			writeError(w, http.StatusServiceUnavailable, "scorer could not select a backend")
		} else {
			writeError(w, http.StatusTooManyRequests, "all backends are at capacity")
		}
		return
	}
	// The reservation is held for the full response lifetime.
	defer selectedBackend.Release()
	selectedScore := s.scorer.Score(routeReq, &selectedState)

	slog.Debug("routed request",
		"backend", selectedID,
		"prefix_match", matchesByBackend[selectedID],
		"total_blocks", totalBlocks,
		"model", req.Model,
	)

	// --- 6. Forward to backend (reservation already held) ---
	w.Header().Set(headerBackend, selectedID)

	if req.Stream {
		ttftResult, err := backend.ForwardStream(ctx, selectedBackend, body, r.URL.Path, r.URL.Query(), r.Header, w)
		if err != nil {
			slog.Error("backend stream failed", "backend", selectedID, "err", err)
			// Can't write error if headers already sent; just log.
			s.recordRoute(req, selectedID, matchesByBackend[selectedID], totalBlocks, selectedScore, selectedState.QueueDepth, http.StatusBadGateway, 0)
			return
		}
		if ttftResult != nil {
			s.commitCacheObservation(selectedID, namespace, prefixHashes)
			slog.Info("TTFT measured",
				"backend", ttftResult.BackendID,
				"ttft_ms", ttftResult.TTFT.Milliseconds(),
			)
		}
		ttftMillis := int64(0)
		if ttftResult != nil {
			ttftMillis = ttftResult.TTFT.Milliseconds()
		}
		s.recordRoute(req, selectedID, matchesByBackend[selectedID], totalBlocks, selectedScore, selectedState.QueueDepth, http.StatusOK, ttftMillis)
	} else {
		resp, err := backend.Forward(ctx, selectedBackend, body, r.URL.Path, r.URL.Query(), r.Header)
		if err != nil {
			slog.Error("backend forward failed", "backend", selectedID, "err", err)
			writeError(w, http.StatusBadGateway, "backend request failed")
			s.recordRoute(req, selectedID, matchesByBackend[selectedID], totalBlocks, selectedScore, selectedState.QueueDepth, http.StatusBadGateway, 0)
			return
		}
		defer resp.Body.Close()

		// Copy upstream headers without hop-by-hop framing headers.
		backend.CopyHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		if resp.StatusCode < http.StatusBadRequest {
			s.commitCacheObservation(selectedID, namespace, prefixHashes)
		}
		s.recordRoute(req, selectedID, matchesByBackend[selectedID], totalBlocks, selectedScore, selectedState.QueueDepth, resp.StatusCode, 0)
	}
}

func (s *Server) recordRoute(req ChatCompletionRequest, backendID string, matched, total int, score float64, queueDepth, status int, ttftMillis int64) {
	s.telemetry.Record(telemetry.RouteEvent{Model: telemetry.SanitizeModel(req.Model), BackendID: backendID, MatchedBlocks: matched, TotalBlocks: total, Score: score, QueueDepth: queueDepth, StatusCode: status, Stream: req.Stream, TTFTMillis: ttftMillis})
}

// commitCacheObservation updates the local cache model only after upstream
// evidence shows the request was accepted. Any LRU evictions are immediately
// removed from the prefix index to avoid advertising stale affinity.
func (s *Server) commitCacheObservation(backendID, namespace string, hashes []uint64) {
	s.cache.Commit(backendID, namespace, hashes)
}

// writeError sends a JSON error response matching OpenAI's error format.
func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	resp := struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}{}
	resp.Error.Message = msg
	resp.Error.Type = "invalid_request_error"
	if code >= 500 {
		resp.Error.Type = "server_error"
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// formatSSEDone emits the standard OpenAI streaming terminator.
func formatSSEDone() string {
	return fmt.Sprintf("data: [DONE]\n\n")
}
