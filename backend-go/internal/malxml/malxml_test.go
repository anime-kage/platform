package malxml

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

const animeExport = `<?xml version="1.0" encoding="UTF-8" ?>
<myanimelist>
  <myinfo><user_name>tester</user_name></myinfo>
  <anime>
    <series_animedb_id>5114</series_animedb_id>
    <series_title><![CDATA[Fullmetal Alchemist: Brotherhood]]></series_title>
    <my_watched_episodes>64</my_watched_episodes>
    <my_start_date>2019-04-05</my_start_date>
    <my_finish_date>0000-00-00</my_finish_date>
    <my_score>10</my_score>
    <my_status>Watching</my_status>
    <my_comments><![CDATA[cel mai bun]]></my_comments>
  </anime>
</myanimelist>`

func TestParsePlainAndGzipped(t *testing.T) {
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"plain", []byte(animeExport)},
		{"gzipped", gzipped(t, animeExport)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// members download .xml.gz and often unzip it first, so both
			// have to work off the same endpoint
			list, err := Parse(bytes.NewReader(tc.body))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if list.Manga {
				t.Error("anime export flagged as manga")
			}
			if len(list.Entries) != 1 {
				t.Fatalf("got %d entries, want 1", len(list.Entries))
			}
			e := list.Entries[0]
			if e.MalID != 5114 || e.Progress != 64 || e.Score != 10 || e.Status != "Watching" {
				t.Errorf("bad entry: %+v", e)
			}
			if e.Notes != "cel mai bun" {
				t.Errorf("CDATA comment not read: %q", e.Notes)
			}
			if e.StartedAt == nil || e.StartedAt.Year() != 2019 {
				t.Errorf("start date not parsed: %v", e.StartedAt)
			}
			// the one that actually bites: 0000-00-00 is MAL's "unset", and
			// time.Parse accepts it as a year-zero date
			if e.CompletedAt != nil {
				t.Errorf("0000-00-00 became a real date: %v", e.CompletedAt)
			}
		})
	}
}

func TestParseMangaExportUsesItsOwnFieldNames(t *testing.T) {
	const mangaExport = `<?xml version="1.0" encoding="UTF-8" ?>
<myanimelist>
  <manga>
    <manga_mangadb_id>2</manga_mangadb_id>
    <manga_title><![CDATA[Berserk]]></manga_title>
    <my_read_chapters>380</my_read_chapters>
    <my_score>10</my_score>
    <my_status>Reading</my_status>
  </manga>
</myanimelist>`

	list, err := Parse(strings.NewReader(mangaExport))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !list.Manga {
		t.Fatal("manga export not detected — it would import into the watchlist")
	}
	if len(list.Entries) != 1 || list.Entries[0].Progress != 380 {
		t.Fatalf("chapters not read from my_read_chapters: %+v", list.Entries)
	}
}

func TestParseRejectsRubbish(t *testing.T) {
	for _, body := range []string{"", "not xml at all", "<html><body>nope</body></html>"} {
		if _, err := Parse(strings.NewReader(body)); err == nil {
			t.Errorf("accepted non-export input %q", body)
		}
	}
}

func gzipped(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
	zw.Close()
	return buf.Bytes()
}
