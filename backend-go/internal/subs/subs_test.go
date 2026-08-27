package subs

import (
	"strings"
	"testing"
)

func TestParseSRT(t *testing.T) {
	src := "\ufeff1\r\n00:00:01,500 --> 00:00:04,000\r\nHello there.\r\n<i>Second line.</i>\r\n\r\n2\r\n00:01:00,000 --> 00:01:02,250\r\n{\\an8}Sign text\r\n\r\n3\r\n00:02:00,000 --> 00:02:01,000\r\n{\\an8}\r\n"
	events, err := ParseSRT([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2 (empty-after-clean cue dropped)", len(events))
	}
	e := events[0]
	if e.Idx != 0 || e.StartMs != 1500 || e.EndMs != 4000 {
		t.Errorf("event 0 timing: %+v", e)
	}
	if e.Text != "Hello there.\nSecond line." {
		t.Errorf("event 0 text (inline tags should be stripped): %q", e.Text)
	}
	if events[1].Text != "Sign text" {
		t.Errorf("ASS override tag not stripped: %q", events[1].Text)
	}
}

// ffmpeg's ASS→SRT conversion and many rips wrap lines in <font>/<b> markup —
// the editor and our .vtt must show plain text.
func TestParseSRTStripsFontTags(t *testing.T) {
	src := "1\n00:00:01,000 --> 00:00:03,000\n<font face=\"Trebuchet MS\" size=\"24\">Are you sure this is far enough?</font>\n\n" +
		"2\n00:00:04,000 --> 00:00:06,000\n<font size=\"24\"><b>Beyond Journey&#39;s End</b></font>\n"
	events, err := ParseSRT([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Text != "Are you sure this is far enough?" {
		t.Errorf("font tag not stripped: %q", events[0].Text)
	}
	if events[1].Text != "Beyond Journey's End" {
		t.Errorf("nested tags/entity not cleaned: %q", events[1].Text)
	}
}

func TestParseVTT(t *testing.T) {
	src := "WEBVTT\n\nNOTE a comment\n\nintro-cue\n00:05.000 --> 00:07.500\nShort-form timing.\n\n00:00:10.000 --> 00:00:12.000\nFull timing.\n"
	events, err := ParseVTT([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].StartMs != 5000 || events[0].EndMs != 7500 {
		t.Errorf("short-form timing: %+v", events[0])
	}
	if events[1].StartMs != 10000 {
		t.Errorf("full timing: %+v", events[1])
	}
}

func TestParseASS(t *testing.T) {
	src := strings.Join([]string{
		"[Script Info]",
		"Title: test",
		"",
		"[Events]",
		"Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text",
		`Dialogue: 0,0:00:05.00,0:00:07.50,Default,,0,0,0,,{\i1}Later line, with a comma{\i0}`,
		`Dialogue: 0,0:00:01.25,0:00:03.00,Default,,0,0,0,,First\Nsecond`,
		"Comment: 0,0:00:00.00,0:00:01.00,Default,,0,0,0,,not dialogue",
	}, "\n")
	events, err := ParseASS([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	// sorted by start time despite file order
	if events[0].StartMs != 1250 || events[0].Idx != 0 {
		t.Errorf("sort/renumber: %+v", events[0])
	}
	if events[0].Text != "First\nsecond" {
		t.Errorf("\\N conversion: %q", events[0].Text)
	}
	if events[1].Text != "Later line, with a comma" {
		t.Errorf("tag strip / comma split: %q", events[1].Text)
	}
}

func TestWriteVTT(t *testing.T) {
	out := WriteVTT([]Event{
		{StartMs: 1500, EndMs: 4000, Text: "Salut."},
		{StartMs: 5000, EndMs: 6000, Text: ""}, // untranslated → skipped
		{StartMs: 3661000, EndMs: 3662500, Text: "O oră mai târziu."},
	})
	want := "WEBVTT\n\n00:00:01.500 --> 00:00:04.000\nSalut.\n\n01:01:01.000 --> 01:01:02.500\nO oră mai târziu.\n\n"
	if out != want {
		t.Errorf("got:\n%q\nwant:\n%q", out, want)
	}
}

func TestParseUnsupported(t *testing.T) {
	if _, err := Parse("subs.sub", nil); err == nil {
		t.Fatal("expected error for unsupported extension")
	}
	if _, err := Parse("empty.srt", []byte("garbage")); err == nil {
		t.Fatal("expected error for cue-less file")
	}
}
