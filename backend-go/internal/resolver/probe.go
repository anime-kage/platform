package resolver

import (
	"context"
	"fmt"
	"net/http"
)

// Probe fetches the first bytes of a resolved manifest with the provider's
// headers. 2xx = alive; anything else (or a transport error) = dead. Resolving
// alone proves only that the ref is well-formed — the direct provider never
// fetches — so this is the liveness half used by both the health checker
// and the admin test-resolve button.
func Probe(ctx context.Context, res *Result) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, res.ManifestURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", "bytes=0-1023")
	for k, v := range res.Headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("manifest fetch: status %d", resp.StatusCode)
	}
	return nil
}
