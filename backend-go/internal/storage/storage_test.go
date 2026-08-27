package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"animekage/backend/internal/config"
)

func TestNewUnconfigured(t *testing.T) {
	c, err := New(&config.Config{})
	if err != nil || c != nil {
		t.Fatalf("unconfigured env should mean (nil, nil), got (%v, %v)", c, err)
	}
}

func TestPutDelete(t *testing.T) {
	type req struct {
		method, path, body, contentType string
	}
	var got []req
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, req{r.Method, r.URL.Path, string(b), r.Header.Get("Content-Type")})
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, err := New(&config.Config{
		R2AccessKey: "ak", R2SecretKey: "sk", R2Bucket: "media",
		R2PublicURL: "https://cdn.example.com/", R2Endpoint: srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("expected a configured client")
	}

	url, err := c.Put(context.Background(), "/manga/7/12.5/ro/003.jpg", "image/jpeg", strings.NewReader("fakejpg"))
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://cdn.example.com/manga/7/12.5/ro/003.jpg" {
		t.Fatalf("public URL = %q", url)
	}
	if err := c.Delete(context.Background(), "manga/7/12.5/ro/003.jpg"); err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(got))
	}
	if got[0].method != "PUT" || got[0].path != "/media/manga/7/12.5/ro/003.jpg" {
		t.Fatalf("put request = %+v", got[0])
	}
	if got[0].body != "fakejpg" || got[0].contentType != "image/jpeg" {
		t.Fatalf("put payload = %+v", got[0])
	}
	if got[1].method != "DELETE" || got[1].path != "/media/manga/7/12.5/ro/003.jpg" {
		t.Fatalf("delete request = %+v", got[1])
	}
}
