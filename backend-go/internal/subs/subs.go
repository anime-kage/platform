// Package subs parses subtitle files (SRT, ASS/SSA, VTT) into timed events
// and serializes events back to WebVTT. It exists for the release pipeline
//: uploaded subs become subtitle_events rows, the editor's live
// preview and the publish job serialize the RO column back to one .vtt.
package subs

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Event is one subtitle line. Idx is the 0-based position after parsing —
// stable per release, used to re-attach machine translations.
type Event struct {
	Idx     int
	StartMs int
	EndMs   int
	Text    string
}

// Parse picks the parser from the file extension.
func Parse(filename string, data []byte) ([]Event, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".srt":
		return ParseSRT(data)
	case ".vtt":
		return ParseVTT(data)
	case ".ass", ".ssa":
		return ParseASS(data)
	default:
		return nil, fmt.Errorf("unsupported subtitle format %q (use .srt, .ass or .vtt)", filepath.Ext(filename))
	}
}

// hh:mm:ss,mmm --> hh:mm:ss,mmm  (SRT uses ',', VTT '.'; VTT may drop hours)
var timingRe = regexp.MustCompile(
	`(?:(\d+):)?(\d{1,2}):(\d{2})[,.](\d{1,3})\s*-->\s*(?:(\d+):)?(\d{1,2}):(\d{2})[,.](\d{1,3})`)

// assTagRe strips {\...} override blocks that leak into SRT rips too.
var assTagRe = regexp.MustCompile(`\{[^}]*\}`)

// htmlTagRe strips inline markup — <font ...>, <b>, <i>, <u> — that SRT rips
// (and ffmpeg's ASS→SRT conversion) carry. The editor and our .vtt are plain
// text; a literal "<" in dialogue is escaped as &lt;, so this is safe.
var htmlTagRe = regexp.MustCompile(`(?s)<[^>]+>`)

// htmlEntities decodes the handful of entities that survive tag stripping.
var htmlEntities = strings.NewReplacer(
	"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`,
	"&#39;", "'", "&apos;", "'", "&nbsp;", " ",
)

// ParseSRT parses SubRip. Counters are ignored — events are renumbered.
func ParseSRT(data []byte) ([]Event, error) { return parseCues(data) }

// ParseVTT parses WebVTT (header, NOTE/STYLE blocks and cue ids skipped).
func ParseVTT(data []byte) ([]Event, error) { return parseCues(data) }

// parseCues handles both SRT and VTT: blank-line-separated blocks where the
// timing line is the first or second line of the block.
func parseCues(data []byte) ([]Event, error) {
	text := normalize(data)
	var events []Event
	for _, block := range strings.Split(text, "\n\n") {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		ti := -1
		for i := 0; i < len(lines) && i < 2; i++ {
			if strings.Contains(lines[i], "-->") {
				ti = i
				break
			}
		}
		if ti < 0 || ti+1 > len(lines) {
			continue // header, NOTE, STYLE, bare counter — not a cue
		}
		m := timingRe.FindStringSubmatch(lines[ti])
		if m == nil {
			continue
		}
		txt := cleanText(strings.Join(lines[ti+1:], "\n"))
		if txt == "" {
			continue
		}
		events = append(events, Event{
			Idx:     len(events),
			StartMs: clockMs(m[1], m[2], m[3], m[4]),
			EndMs:   clockMs(m[5], m[6], m[7], m[8]),
			Text:    txt,
		})
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("no subtitle cues found")
	}
	return events, nil
}

// Default [Events] field order when the file has no Format: line.
var assDefaultFormat = []string{"Layer", "Start", "End", "Style", "Name", "MarginL", "MarginR", "MarginV", "Effect", "Text"}

// ParseASS parses Advanced SubStation Alpha. Only Dialogue lines in [Events]
// matter; override tags are stripped (styling lives in ASS, our editor and
// .vtt are plain text). Events are sorted by start time — ASS files are
// frequently out of order.
func ParseASS(data []byte) ([]Event, error) {
	format := assDefaultFormat
	inEvents := false
	var events []Event

	for _, line := range strings.Split(normalize(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inEvents = strings.EqualFold(line, "[events]")
			continue
		}
		if !inEvents {
			continue
		}
		if v, ok := strings.CutPrefix(line, "Format:"); ok {
			format = nil
			for _, f := range strings.Split(v, ",") {
				format = append(format, strings.TrimSpace(f))
			}
			continue
		}
		v, ok := strings.CutPrefix(line, "Dialogue:")
		if !ok {
			continue
		}
		// the Text field is last and may contain commas — cap the split
		parts := strings.SplitN(strings.TrimSpace(v), ",", len(format))
		if len(parts) < len(format) {
			continue
		}
		var start, end, txt string
		for i, f := range format {
			switch f {
			case "Start":
				start = parts[i]
			case "End":
				end = parts[i]
			case "Text":
				txt = parts[i]
			}
		}
		startMs, err1 := assClockMs(start)
		endMs, err2 := assClockMs(end)
		txt = cleanText(strings.NewReplacer(`\N`, "\n", `\n`, "\n", `\h`, " ").Replace(txt))
		if err1 != nil || err2 != nil || txt == "" {
			continue
		}
		events = append(events, Event{StartMs: startMs, EndMs: endMs, Text: txt})
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("no dialogue lines found")
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].StartMs < events[j].StartMs })
	for i := range events {
		events[i].Idx = i
	}
	return events, nil
}

// WriteVTT serializes events to a WebVTT document. Events with empty text
// (untranslated rows) are skipped.
func WriteVTT(events []Event) string {
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for _, e := range events {
		txt := strings.TrimSpace(e.Text)
		if txt == "" {
			continue
		}
		fmt.Fprintf(&b, "%s --> %s\n%s\n\n", vttClock(e.StartMs), vttClock(e.EndMs), txt)
	}
	return b.String()
}

// WriteSRT serializes events to a SubRip document (the format hosts accept as a
// sidecar subtitle). Empty rows are skipped and cues are renumbered.
func WriteSRT(events []Event) string {
	var b strings.Builder
	n := 0
	for _, e := range events {
		txt := strings.TrimSpace(e.Text)
		if txt == "" {
			continue
		}
		n++
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", n, srtClock(e.StartMs), srtClock(e.EndMs), txt)
	}
	return b.String()
}

func srtClock(ms int) string {
	if ms < 0 {
		ms = 0
	}
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

func vttClock(ms int) string {
	if ms < 0 {
		ms = 0
	}
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

// clockMs converts h/m/s + fraction captures ("" hour allowed) to ms.
// A 1–3 digit fraction is milliseconds left-padded by position: "5" = 500ms.
func clockMs(h, m, s, frac string) int {
	hi, _ := strconv.Atoi(h) // "" → 0 (VTT short form)
	mi, _ := strconv.Atoi(m)
	si, _ := strconv.Atoi(s)
	for len(frac) < 3 {
		frac += "0"
	}
	fi, _ := strconv.Atoi(frac)
	return ((hi*60+mi)*60+si)*1000 + fi
}

// assClockMs parses ASS "H:MM:SS.cc" (centiseconds).
func assClockMs(s string) (int, error) {
	var h, m, sec, cs int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d:%d:%d.%d", &h, &m, &sec, &cs); err != nil {
		return 0, err
	}
	return ((h*60+m)*60+sec)*1000 + cs*10, nil
}

func cleanText(s string) string {
	s = assTagRe.ReplaceAllString(s, "")  // {\an8}, {\i1}…
	s = htmlTagRe.ReplaceAllString(s, "") // <font …>, <b>, <i>…
	s = htmlEntities.Replace(s)
	return strings.TrimSpace(s)
}

// normalize strips a UTF-8 BOM and CR line endings.
func normalize(data []byte) string {
	s := strings.TrimPrefix(string(data), "\ufeff")
	return strings.ReplaceAll(s, "\r\n", "\n")
}
