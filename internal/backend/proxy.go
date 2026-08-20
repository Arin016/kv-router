package backend

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TTFTResult captures the time-to-first-byte measurement for a streaming request.
type TTFTResult struct {
	BackendID string
	TTFT      time.Duration
	Timestamp time.Time
}

// Forward creates a faithful upstream request. The caller owns the returned
// body and backend reservation, keeping queue depth accurate for the entire
// response lifetime rather than only until response headers arrive.
func Forward(ctx context.Context, backend *Backend, body []byte, path string, query url.Values, headers http.Header) (*http.Response, error) {
	target, err := upstreamURL(backend.URL, path, query)
	if err != nil {
		return nil, fmt.Errorf("backend %s: build upstream URL: %w", backend.ID, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("backend %s: build request: %w", backend.ID, err)
	}
	req.Header = forwardHeaders(headers)
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := backend.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("backend %s: request failed: %w", backend.ID, err)
	}
	return resp, nil
}

// ForwardStream proxies an SSE response without rewriting its status or headers.
func ForwardStream(ctx context.Context, backend *Backend, body []byte, path string, query url.Values, headers http.Header, w http.ResponseWriter) (*TTFTResult, error) {
	sendTime := time.Now()
	resp, err := Forward(ctx, backend, body, path, query, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if resp.StatusCode >= http.StatusBadRequest {
		_, err := io.Copy(w, resp.Body)
		return nil, err
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("backend %s: ResponseWriter does not support streaming", backend.ID)
	}
	buf := make([]byte, 32*1024)
	var ttft *TTFTResult
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if ttft == nil {
				now := time.Now()
				ttft = &TTFTResult{BackendID: backend.ID, TTFT: now.Sub(sendTime), Timestamp: now}
			}
			if _, err := w.Write(buf[:n]); err != nil {
				return ttft, fmt.Errorf("backend %s: write to client: %w", backend.ID, err)
			}
			flusher.Flush()
		}
		if readErr == io.EOF {
			return ttft, nil
		}
		if readErr != nil {
			return ttft, fmt.Errorf("backend %s: read stream: %w", backend.ID, readErr)
		}
	}
}

func upstreamURL(base, path string, query url.Values) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(path, "/")
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func forwardHeaders(in http.Header) http.Header {
	out := make(http.Header, len(in))
	for key, values := range in {
		if isHopByHop(key) || strings.EqualFold(key, "Host") || strings.EqualFold(key, "Content-Length") {
			continue
		}
		out[key] = append([]string(nil), values...)
	}
	return out
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		if isHopByHop(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isHopByHop(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}
