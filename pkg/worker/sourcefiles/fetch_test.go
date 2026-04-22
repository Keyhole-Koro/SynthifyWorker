package sourcefiles

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

func TestFetchCachesContent(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	file := pipeline.SourceFile{Filename: "sample.txt", URI: server.URL}
	if err := Fetch(context.Background(), &file); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if err := Fetch(context.Background(), &file); err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("expected exactly one network fetch, got %d", got)
	}
	if string(file.Content) != "hello" {
		t.Fatalf("unexpected content: %q", string(file.Content))
	}
	if file.MimeType != "text/plain" {
		t.Fatalf("unexpected mime type: %q", file.MimeType)
	}
}
