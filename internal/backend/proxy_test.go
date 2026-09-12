package backend

import (
	"net/http"
	"testing"
)

// Regression test for issue #2: hop-by-hop headers must never reach
// downstream clients on either forward path.
func TestCopyHeadersStripsHopByHop(t *testing.T) {
	src := http.Header{
		"Content-Type":      {"application/json"},
		"X-Custom":          {"a", "b"},
		"Transfer-Encoding": {"chunked"},
		"Connection":        {"keep-alive"},
		"Keep-Alive":        {"timeout=5"},
		"Upgrade":           {"h2c"},
	}
	dst := http.Header{}
	CopyHeaders(dst, src)

	if got := dst.Get("Content-Type"); got != "application/json" {
		t.Fatalf("end-to-end header lost: %q", got)
	}
	if got := dst.Values("X-Custom"); len(got) != 2 {
		t.Fatalf("multi-value header mangled: %v", got)
	}
	for _, h := range []string{"Transfer-Encoding", "Connection",
		"Keep-Alive", "Upgrade"} {
		if _, ok := dst[http.CanonicalHeaderKey(h)]; ok {
			t.Fatalf("hop-by-hop header forwarded: %s", h)
		}
	}
}
