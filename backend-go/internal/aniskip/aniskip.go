// Package aniskip is a minimal client for the AniSkip API (api.aniskip.com) —
// community-sourced intro/outro timestamps keyed on MAL id + episode number.
//: it backs our skip_marks table; hits are cached there so each
// episode asks AniSkip at most once.
package aniskip

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SkipTimes maps AniSkip's op/ed to our intro/outro kinds.
type SkipTimes struct {
	Kind  string // 'intro' | 'outro'
	Start float64
	End   float64
}

type Client struct {
	base string
	http *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{base: baseURL, http: &http.Client{Timeout: 5 * time.Second}}
}

type apiResponse struct {
	Found   bool `json:"found"`
	Results []struct {
		Interval struct {
			StartTime float64 `json:"startTime"`
			EndTime   float64 `json:"endTime"`
		} `json:"interval"`
		SkipType string `json:"skipType"`
	} `json:"results"`
}

// SkipTimes fetches op/ed intervals for one episode. A 404 (unknown episode)
// returns an empty slice, not an error — "AniSkip doesn't know" is a normal
// outcome, not a failure.
func (c *Client) SkipTimes(ctx context.Context, malID, episode int) ([]SkipTimes, error) {
	url := fmt.Sprintf("%s/v2/skip-times/%d/%d?types[]=op&types[]=ed&episodeLength=0",
		c.base, malID, episode)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aniskip: unexpected status %d", resp.StatusCode)
	}
	var body apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("aniskip: decode: %w", err)
	}
	if !body.Found {
		return nil, nil
	}
	out := make([]SkipTimes, 0, len(body.Results))
	for _, r := range body.Results {
		var kind string
		switch r.SkipType {
		case "op":
			kind = "intro"
		case "ed":
			kind = "outro"
		default:
			continue
		}
		if r.Interval.EndTime <= r.Interval.StartTime {
			continue
		}
		out = append(out, SkipTimes{Kind: kind, Start: r.Interval.StartTime, End: r.Interval.EndTime})
	}
	return out, nil
}
