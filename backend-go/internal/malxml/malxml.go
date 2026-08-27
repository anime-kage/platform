// Package malxml parses MyAnimeList's list export.
//
// Why a file and not an API: Jikan's /users/{name}/animelist scrapes MAL and
// answers 504 more often than not, and MAL's own v2 API needs a registered
// client id. The export (MAL → Settings → Export) needs neither, and it is
// the only route that works for a *private* list — which is the case for a
// good share of accounts. AniList imports the same way, so members have met
// this flow before.
package malxml

import (
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"
)

// Entry is one <anime> or <manga> element, in MAL's own vocabulary.
type Entry struct {
	MalID       int
	Title       string
	Status      string // "Watching", "Plan to Watch", …
	Progress    int
	Score       int // 0–10, 0 = unscored
	Notes       string
	StartedAt   *time.Time
	CompletedAt *time.Time
}

type List struct {
	// Manga is set when the file is a manga export; the two share a format
	// but use different element and field names.
	Manga   bool
	Entries []Entry
}

// The export nests everything under <myanimelist>. Anime and manga files
// differ enough in field names that both shapes are declared and whichever
// one populated wins.
type export struct {
	Anime []struct {
		ID       int    `xml:"series_animedb_id"`
		Title    string `xml:"series_title"`
		Watched  int    `xml:"my_watched_episodes"`
		Score    int    `xml:"my_score"`
		Status   string `xml:"my_status"`
		Comments string `xml:"my_comments"`
		Start    string `xml:"my_start_date"`
		Finish   string `xml:"my_finish_date"`
	} `xml:"anime"`
	Manga []struct {
		ID       int    `xml:"manga_mangadb_id"`
		Title    string `xml:"manga_title"`
		Read     int    `xml:"my_read_chapters"`
		Score    int    `xml:"my_score"`
		Status   string `xml:"my_status"`
		Comments string `xml:"my_comments"`
		Start    string `xml:"my_start_date"`
		Finish   string `xml:"my_finish_date"`
	} `xml:"manga"`
}

// MaxSize bounds what we will read. A 5000-title export is around 3 MB of
// XML; 32 MB uncompressed is far past any real list and stops a gzip bomb
// from being interesting.
const MaxSize = 32 << 20

// Parse reads a MAL export, gzipped or not. Members download `.xml.gz` and
// frequently unzip it first, so sniffing beats trusting the extension.
func Parse(r io.Reader) (*List, error) {
	br := &peekReader{r: io.LimitReader(r, MaxSize+1)}
	magic, err := br.peek(2)
	if err != nil {
		return nil, fmt.Errorf("fișier gol sau ilizibil")
	}

	var src io.Reader = br
	if len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		zr, err := gzip.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("arhiva .gz nu poate fi citită: %w", err)
		}
		defer zr.Close()
		src = io.LimitReader(zr, MaxSize)
	}

	var ex export
	if err := xml.NewDecoder(src).Decode(&ex); err != nil {
		return nil, fmt.Errorf("fișierul nu pare a fi un export MyAnimeList: %w", err)
	}

	out := &List{}
	for _, a := range ex.Anime {
		if a.ID == 0 {
			continue
		}
		out.Entries = append(out.Entries, Entry{
			MalID:       a.ID,
			Title:       strings.TrimSpace(a.Title),
			Status:      strings.TrimSpace(a.Status),
			Progress:    a.Watched,
			Score:       a.Score,
			Notes:       strings.TrimSpace(a.Comments),
			StartedAt:   parseDate(a.Start),
			CompletedAt: parseDate(a.Finish),
		})
	}
	for _, m := range ex.Manga {
		if m.ID == 0 {
			continue
		}
		out.Manga = true
		out.Entries = append(out.Entries, Entry{
			MalID:       m.ID,
			Title:       strings.TrimSpace(m.Title),
			Status:      strings.TrimSpace(m.Status),
			Progress:    m.Read,
			Score:       m.Score,
			Notes:       strings.TrimSpace(m.Comments),
			StartedAt:   parseDate(m.Start),
			CompletedAt: parseDate(m.Finish),
		})
	}

	if len(out.Entries) == 0 {
		return nil, fmt.Errorf("exportul nu conține niciun titlu")
	}
	return out, nil
}

// MAL writes "2019-04-05", and "0000-00-00" for "not set" — which
// time.Parse accepts as a year-zero date, so it has to be caught explicitly
// or every unset date imports as January of year 0.
func parseDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "0000") {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil || t.Year() < 1900 {
		return nil
	}
	return &t
}

// peekReader lets Parse sniff the gzip magic bytes without consuming them.
type peekReader struct {
	r   io.Reader
	buf []byte
}

func (p *peekReader) peek(n int) ([]byte, error) {
	for len(p.buf) < n {
		tmp := make([]byte, n-len(p.buf))
		read, err := p.r.Read(tmp)
		p.buf = append(p.buf, tmp[:read]...)
		if err != nil {
			if len(p.buf) == 0 {
				return nil, err
			}
			break
		}
	}
	return p.buf, nil
}

func (p *peekReader) Read(dst []byte) (int, error) {
	if len(p.buf) > 0 {
		n := copy(dst, p.buf)
		p.buf = p.buf[n:]
		return n, nil
	}
	return p.r.Read(dst)
}
