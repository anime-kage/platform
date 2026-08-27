package aniskip

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSkipTimes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/skip-times/5114/1":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"found":true,"results":[
				{"interval":{"startTime":10.5,"endTime":95.5},"skipType":"op"},
				{"interval":{"startTime":1290,"endTime":1380},"skipType":"ed"},
				{"interval":{"startTime":5,"endTime":6},"skipType":"mixed-op"},
				{"interval":{"startTime":50,"endTime":50},"skipType":"op"}]}`)
		case "/v2/skip-times/5114/2":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"found":false,"results":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := NewClient(srv.URL)

	got, err := c.SkipTimes(context.Background(), 5114, 1)
	if err != nil {
		t.Fatal(err)
	}
	// unknown skipType and empty intervals are dropped
	want := []SkipTimes{{Kind: "intro", Start: 10.5, End: 95.5}, {Kind: "outro", Start: 1290, End: 1380}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("SkipTimes = %+v, want %+v", got, want)
	}

	// found=false and 404 are both "no data", not errors
	if got, err := c.SkipTimes(context.Background(), 5114, 2); err != nil || len(got) != 0 {
		t.Errorf("found=false: got %v, %v", got, err)
	}
	if got, err := c.SkipTimes(context.Background(), 999, 1); err != nil || got != nil {
		t.Errorf("404: got %v, %v", got, err)
	}
}
