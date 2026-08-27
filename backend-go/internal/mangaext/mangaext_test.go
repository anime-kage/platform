package mangaext

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const chapterUUID = "a54c491c-8e4c-4e97-8873-5b79e59da210"

func TestMangaDexExtract(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/at-home/server/"+chapterUUID {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("expected a User-Agent header")
		}
		w.Write([]byte(`{"result":"ok","baseUrl":"https://cdn.example",
			"chapter":{"hash":"abc123","data":["1.png","2.png","3.png"]}}`))
	}))
	defer srv.Close()

	md := NewMangaDex(srv.URL)
	res, err := md.Extract(context.Background(), chapterUUID)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(res.Pages) != 3 {
		t.Fatalf("want 3 pages, got %d", len(res.Pages))
	}
	if res.Pages[0] != "https://cdn.example/data/abc123/1.png" {
		t.Fatalf("bad page URL: %s", res.Pages[0])
	}
}

func TestMangaDexExtractRejectsBadRef(t *testing.T) {
	md := NewMangaDex("http://never-called.invalid")
	if _, err := md.Extract(context.Background(), "not-a-uuid"); !errors.Is(err, ErrUnextractable) {
		t.Fatalf("want ErrUnextractable, got %v", err)
	}
}

func TestMangaDexExtractDeadChapter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	md := NewMangaDex(srv.URL)
	if _, err := md.Extract(context.Background(), chapterUUID); !errors.Is(err, ErrUnextractable) {
		t.Fatalf("want ErrUnextractable, got %v", err)
	}
}
