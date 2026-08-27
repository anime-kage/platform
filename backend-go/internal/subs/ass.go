package subs

// ASS output, for burning the Romanian track into a copy of the video.
//
// Why ASS and not the .srt we already write: burning goes through libass, and
// libass only takes styling from ASS. Handing ffmpeg an .srt means accepting its
// defaults (or fighting them through force_style, which is the same styling
// expressed as a less readable string). One ASS style, declared here, is the
// canonical definition of what a burned subtitle looks like.
//
// That direction matters and is worth stating once: the *preview* is styled to
// match this, not the other way round. libass rendering cannot be made
// pixel-identical to a browser rendering WebVTT with CSS — they are different
// engines with different metrics, shaping and outline maths. Trying to make ASS
// match arbitrary CSS never converges; declaring ASS canonical does.

import (
	"fmt"
	"strconv"
	"strings"
)

// The canonical burned-in style. Deliberately conservative: a fansub track has
// to stay legible over bright scenes at 480p on a phone, and burned pixels can
// never be restyled after the fact.
//
// Colours are ASS &HAABBGGRR — alpha first, then *reversed* RGB. &H00FFFFFF is
// opaque white; &H00000000 opaque black. Getting the byte order wrong here
// yields a plausible-looking colour that is not the one you asked for, which is
// why these are named rather than inlined.
const (
	assFontName = "Noto Sans" // installed via font-noto; carries ș/ț with comma below
	// ≈5.7% of frame height at PlayResY 1080. Chosen by rendering candidates over
	// mid-grey and comparing: 54 with a 3px border read small and slightly muddy,
	// and a bold weight read chunky. This is the size and weight Netflix and
	// Crunchyroll both land on — larger than feels right in a still, correct at
	// viewing distance and on a phone.
	assFontSize = 62
	assPrimary  = "&H00FFFFFF"
	assOutline  = "&H00000000"
	assBack     = "&H80000000" // 50% black, for the shadow
	// Not bold, and a *thinner* border than before. The outline is there to keep
	// white legible over a white scene, not to be seen: 2.6 with a soft shadow
	// under it keeps the glyph edges crisp, where 3 started to fill in the
	// counters of a, e and o at this size.
	assOutlineW = 2.6
	assShadowW  = 1.4
	// ≈5% of frame height off the bottom. 40 sat close enough to the edge to
	// collide with a player's control bar; this clears it and matches where the
	// big streamers place the last line.
	assMarginV = 56

	// PlayRes fixes the coordinate space the sizes above are relative to.
	// libass scales the whole style to the real frame, so one style renders
	// consistently on a 1080p source and a 720p one.
	assPlayResX = 1920
	assPlayResY = 1080
)

// WriteASS renders events as an ASS v4+ subtitle file in the canonical style.
//
// Empty events are dropped, matching WriteVTT: an untranslated row should be
// silent, not an empty box on screen.
func WriteASS(events []Event) string {
	var b strings.Builder

	b.WriteString("[Script Info]\n")
	b.WriteString("ScriptType: v4.00+\n")
	b.WriteString("WrapStyle: 0\n")
	b.WriteString("ScaledBorderAndShadow: yes\n")
	fmt.Fprintf(&b, "PlayResX: %d\nPlayResY: %d\n\n", assPlayResX, assPlayResY)

	b.WriteString("[V4+ Styles]\n")
	b.WriteString("Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, " +
		"OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, " +
		"Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, " +
		"MarginV, Encoding\n")
	// Alignment 2 = bottom-centre. Encoding 1 = default/Unicode.
	fmt.Fprintf(&b,
		"Style: Kage,%s,%d,%s,%s,%s,%s,0,0,0,0,100,100,0,0,1,%s,%s,2,60,60,%d,1\n\n",
		assFontName, assFontSize, assPrimary, assPrimary, assOutline, assBack,
		strconv.FormatFloat(assOutlineW, 'f', -1, 64),
		strconv.FormatFloat(assShadowW, 'f', -1, 64),
		assMarginV)

	b.WriteString("[Events]\n")
	b.WriteString("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")
	for _, e := range events {
		txt := strings.TrimSpace(e.Text)
		if txt == "" {
			continue
		}
		fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Kage,,0,0,0,,%s\n",
			assClock(e.StartMs), assClock(e.EndMs), assEscape(txt))
	}
	return b.String()
}

// assClock formats h:mm:ss.cc — ASS uses centiseconds and a single-digit hour,
// unlike SRT's HH:MM:SS,mmm.
func assClock(ms int) string {
	if ms < 0 {
		ms = 0
	}
	cs := (ms % 1000) / 10
	s := ms / 1000
	return fmt.Sprintf("%d:%02d:%02d.%02d", s/3600, (s/60)%60, s%60, cs)
}

// assEscape makes text safe for a Dialogue line.
//
// A newline must become the literal `\N` (ASS's hard line break) — a real
// newline would end the event and the rest of the line would be dropped
// silently, losing half a subtitle. Braces open an override block, so a stray
// `{` in dialogue would swallow text until the next `}`; they are replaced
// rather than escaped because ASS has no escape for them.
func assEscape(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\n", `\N`)
	s = strings.ReplaceAll(s, "{", "(")
	s = strings.ReplaceAll(s, "}", ")")
	return s
}
