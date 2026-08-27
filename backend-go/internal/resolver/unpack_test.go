package resolver

import (
	"strings"
	"testing"
)

// A real packed block, of the shape XFileSharing hosts emit. Dictionary indices
// are base 36.
const packedSample = `eval(function(p,a,c,k,e,d){e=function(c){return(c<a?'':e(parseInt(c/a)))+((c=c%a)>35?String.fromCharCode(c+29):c.toString(36))};` +
	`while(c--)if(k[c])p=p.replace(new RegExp('\\b'+e(c)+'\\b','g'),k[c]);return p}` +
	`('0.1({2:[{3:"4://5.6/7/8.9"}]})',10,10,'jwplayer|setup|sources|file|https|cdn|test|hls|master|m3u8'.split('|'),0,{}))`

func TestUnpack(t *testing.T) {
	got := Unpack(packedSample)
	want := `jwplayer.setup({sources:[{file:"https://cdn.test/hls/master.m3u8"}]})`
	if !strings.Contains(got, want) {
		t.Fatalf("Unpack() = %q\nwant it to contain %q", got, want)
	}
}

// Unpacking must not be required: plain input yields "" so callers fall back to
// searching the raw text rather than treating it as an error.
func TestUnpackNoPackedBlock(t *testing.T) {
	if got := Unpack(`<html><script>var a = "hello";</script></html>`); got != "" {
		t.Fatalf("Unpack() = %q, want empty", got)
	}
}

// Tokens that are not valid dictionary indices are source text and must survive.
func TestUnpackKeepsNonIndexTokens(t *testing.T) {
	// radix 10, two words; "99" is out of range and must be left alone.
	src := `}('0 99 1',10,2,'alpha|beta'.split('|'),0,{}))`
	got := Unpack(src)
	for _, want := range []string{"alpha", "99", "beta"} {
		if !strings.Contains(got, want) {
			t.Errorf("Unpack() = %q, want it to contain %q", got, want)
		}
	}
}

func TestUnpackHandlesEscapedSlashes(t *testing.T) {
	src := `}('0:\/\/1',10,2,'https|example.com'.split('|'),0,{}))`
	if got := Unpack(src); !strings.Contains(got, "https://example.com") {
		t.Fatalf("Unpack() = %q, want the unescaped URL", got)
	}
}

// Malformed input must not panic or hang.
func TestUnpackJunk(t *testing.T) {
	for _, src := range []string{
		"", "eval(function(p,a,c,k,e,d){", `}('0',0,0,''.split('|'),0,{}))`,
		`}('0',99,1,'x'.split('|'),0,{}))`, // radix out of range
	} {
		_ = Unpack(src)
	}
}
