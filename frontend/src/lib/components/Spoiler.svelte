<script lang="ts">
  import type { Snippet } from 'svelte';

  /**
   * Discord-style spoiler: `||hidden||`.
   *
   * A button rather than a div with a click handler, because revealing is an
   * action — it has to be reachable by keyboard and announced as pressable. Once
   * revealed it becomes plain text (no toggle back): re-hiding something you
   * have already read is theatre, and leaving it interactive means a stray click
   * inside a paragraph you are reading flips it away mid-sentence.
   */
  /**
   * `interactive={false}` renders the mask as a plain span with no button.
   * Needed where the spoiler sits inside an <a> — a button inside a link is
   * invalid HTML, and the row is a link to the full item anyway, which is the
   * natural place to reveal it.
   */
  let { children, interactive = true }: { children: Snippet; interactive?: boolean } = $props();
  let shown = $state(false);
</script>

{#if !interactive}
  <span class="hidden-text static" aria-label="conține spoiler"><span class="mask">{@render children()}</span></span>
{:else if shown}
  <span class="revealed">{@render children()}</span>
{:else}
  <button
    class="hidden-text"
    type="button"
    aria-label="Arată textul ascuns (spoiler)"
    onclick={(e) => {
      // The spoiler often sits inside a linked card; revealing must not also
      // navigate away from the thing being revealed.
      e.preventDefault();
      e.stopPropagation();
      shown = true;
    }}
  >
    <span class="mask">{@render children()}</span>
  </button>
{/if}

<style>
  /* Hidden state: the text is really there (so layout never jumps on reveal)
     but painted out. `color: transparent` over a block keeps the exact glyph
     widths, which is why the line does not reflow when it opens. */
  .hidden-text {
    display: inline;
    font: inherit;
    line-height: inherit;
    padding: 0 3px;
    margin: 0 -1px;
    border: none;
    border-radius: var(--radius-sm);
    background: var(--text-faint);
    color: transparent;
    cursor: pointer;
    user-select: none;
    transition: background var(--motion-fast) var(--ease);
  }
  .hidden-text:hover { background: var(--text-muted); }
  .hidden-text.static { cursor: inherit; }
  .hidden-text.static:hover { background: var(--text-faint); }
  .hidden-text:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 2px; }
  .mask { color: transparent; }

  .revealed {
    padding: 0 3px;
    margin: 0 -1px;
    border-radius: var(--radius-sm);
    background: color-mix(in srgb, var(--text-faint) 22%, transparent);
  }
</style>
