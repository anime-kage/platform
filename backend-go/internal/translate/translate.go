// Package translate is the Claude auto-translate step of the release
// pipeline: EN subtitle lines in, RO
// lines out, over windows of ~40 lines. Structured output pins the shape to
// {index, text} pairs so lines re-attach by index; the caller only ever
// fills rows whose ro_text is still empty — human edits are never touched.
package translate

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Line is one subtitle line keyed by its stable event index.
type Line struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
}

// WindowSize is how many lines go into one request — large enough for the
// model to keep dialogue context, small enough that one failure loses little.
const WindowSize = 40

type Translator struct {
	client anthropic.Client
	model  anthropic.Model
}

// New builds a Translator. model defaults to Claude Sonnet 5 (the plan's
// pick: translation is high-volume). baseURL overrides the API endpoint so
// tests can stub it.
func New(apiKey, baseURL, model string) *Translator {
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	if model == "" {
		model = "claude-sonnet-5"
	}
	return &Translator{client: anthropic.NewClient(opts...), model: anthropic.Model(model)}
}

// The stable instruction prefix. It must stay byte-identical across requests
// — the per-series glossary block after it carries the cache breakpoint, so
// all windows of an episode share one cached prefix.
const systemPrompt = `You are a professional Romanian fansubber translating English anime subtitles into Romanian.

Rules:
- Translate into natural, spoken Romanian with correct diacritics (ă, â, î, ș, ț).
- Match the register of the dialogue: casual speech stays casual, formal stays formal.
- Keep Japanese honorifics (-san, -kun, -chan, -sama) and proper names untranslated.
- Preserve line breaks: if the English text has a newline, the Romanian text keeps a newline at a natural point.
- Keep each line short enough to read at subtitle speed — favor concision over literalism.
- Translate every line you are given. Never merge, split, drop, or reorder lines.
- Return each line with its original index, unchanged.`

var outputSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"lines": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"index": map[string]any{"type": "integer"},
					"text":  map[string]any{"type": "string"},
				},
				"required":             []string{"index", "text"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"lines"},
	"additionalProperties": false,
}

// TranslateWindow translates one window of lines. glossary is the per-series
// context (title, term choices) — it sits in the cached system prefix, so
// keep it stable for the whole run.
func (t *Translator) TranslateWindow(ctx context.Context, glossary string, lines []Line) ([]Line, error) {
	if len(lines) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal(map[string]any{"lines": lines})
	if err != nil {
		return nil, err
	}

	system := []anthropic.TextBlockParam{{Text: systemPrompt}}
	if glossary = strings.TrimSpace(glossary); glossary != "" {
		system = append(system, anthropic.TextBlockParam{Text: "Series context and glossary:\n" + glossary})
	}
	// cache breakpoint on the last system block: instructions + glossary are
	// identical for every window of the run
	system[len(system)-1].CacheControl = anthropic.NewCacheControlEphemeralParam()

	resp, err := t.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     t.model,
		MaxTokens: 16000,
		System:    system,
		OutputConfig: anthropic.OutputConfigParam{
			Format: anthropic.JSONOutputFormatParam{Schema: outputSchema},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(
				"Translate these subtitle lines to Romanian:\n" + string(payload))),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("translate request: %w", err)
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return nil, fmt.Errorf("translate request refused")
	}

	var text string
	for _, block := range resp.Content {
		if b, ok := block.AsAny().(anthropic.TextBlock); ok {
			text = b.Text
			break
		}
	}
	if text == "" {
		return nil, fmt.Errorf("empty translation response (stop_reason %s)", resp.StopReason)
	}

	var out struct {
		Lines []Line `json:"lines"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, fmt.Errorf("parse translation response: %w", err)
	}

	// keep only lines that answer an index we actually asked about — the
	// schema pins the shape, not the indices
	asked := make(map[int]bool, len(lines))
	for _, l := range lines {
		asked[l.Index] = true
	}
	result := out.Lines[:0]
	for _, l := range out.Lines {
		if asked[l.Index] && strings.TrimSpace(l.Text) != "" {
			result = append(result, l)
		}
	}
	return result, nil
}

// TranslateProse translates one prose block — a catalog synopsis — into
// Romanian. Separate from the subtitle path: different register (editorial,
// not spoken) and no line bookkeeping.
func (t *Translator) TranslateProse(ctx context.Context, title, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}
	resp, err := t.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     t.model,
		MaxTokens: 2048,
		System: []anthropic.TextBlockParam{{Text: `You translate anime and manga catalog synopses into Romanian for a streaming site.

Rules:
- Natural, editorial Romanian with correct diacritics (ă, â, î, ș, ț).
- Keep proper names, Japanese terms and honorifics untranslated.
- Drop trailing attributions like "[Written by MAL Rewrite]" or "(Source: ...)".
- Return only the translated synopsis, nothing else.`}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(
				fmt.Sprintf("Title: %s\n\nSynopsis:\n%s", title, text))),
		},
	})
	if err != nil {
		return "", fmt.Errorf("translate synopsis: %w", err)
	}
	for _, block := range resp.Content {
		if b, ok := block.AsAny().(anthropic.TextBlock); ok {
			return strings.TrimSpace(b.Text), nil
		}
	}
	return "", fmt.Errorf("empty synopsis translation (stop_reason %s)", resp.StopReason)
}

// Windows splits lines into WindowSize chunks.
func Windows(lines []Line) [][]Line {
	var wins [][]Line
	for start := 0; start < len(lines); start += WindowSize {
		end := min(start+WindowSize, len(lines))
		wins = append(wins, lines[start:end])
	}
	return wins
}
