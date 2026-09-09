<script lang="ts">
  import { onMount } from 'svelte';
  import Spoiler from '$lib/components/Spoiler.svelte';
  import { parseText, isGifUrl } from '$lib/markdown';
  import { emotes as emoteStore, loadEmotes, emoteMap } from '$lib/stores/emotes';

  /**
   * User-written plain text — comments, reviews, chat.
   *
   * Handles three things beyond literal text: `||spoilers||`, bare Giphy links
   * which render as the GIF, and the uploaded emote codes. Deliberately NOT
   * the Markdown renderer; see parseText() for why.
   */
  let { text = '', interactive = true }: { text?: string; interactive?: boolean } = $props();
  const parts = $derived(parseText(text));

  /* Emotes used to be chat-only, so a code written in a comment or a review
     rendered as the literal `:code:` text. The set is fetched once per session
     and shared through the store, so a page of comments costs one request
     rather than one per comment. */
  onMount(() => {
    loadEmotes();
  });
  const codes = $derived(emoteMap($emoteStore));

  /* Whitespace-delimited exact matches, the same rule the chat tokenizer uses.
     The separators are kept as tokens of their own so the original spacing is
     reproduced exactly. */
  const tokenize = (s: string) =>
    s
      .split(/(\s+)/)
      .filter((t) => t !== '')
      .map((t, i) => ({ key: i, raw: t, url: codes[t] ?? null }));
</script>

{#snippet body(s: string)}{#each tokenize(s) as tk (tk.key)}{#if tk.url}<span
      class="emote-wrap"
      data-name={tk.raw}><img
        class="emote-img"
        src={tk.url}
        alt={tk.raw}
        loading="lazy"
      /></span
    >{:else}{tk.raw}{/if}{/each}{/snippet}

{#each parts as p}{#if p.kind === 'spoiler'}<Spoiler {interactive}
    >{#if isGifUrl(p.text.trim())}<!-- "hide this GIF" is the obvious use of a
      spoiler round a link, so reveal the image rather than the URL -->
      <img
        class="gif"
        src={p.text.trim()}
        alt="GIF"
        loading="lazy"
        referrerpolicy="no-referrer"
      />{:else}{@render body(p.text)}{/if}</Spoiler
  >{:else if p.kind === 'gif'}<img
    class="gif"
    src={p.url}
    alt="GIF"
    loading="lazy"
    referrerpolicy="no-referrer"
  />{:else}{@render body(p.text)}{/if}{/each}

<style>
  /* Capped rather than full-width: a GIF in a comment is punctuation, not the
     content. `referrerpolicy` above keeps the page URL out of Giphy's logs. */
  .gif {
    display: block;
    max-width: min(320px, 100%);
    height: auto;
    margin: 6px 0 2px;
    border-radius: var(--radius-md);
    border: 1px solid var(--border-subtle);
    background: var(--surface-overlay);
  }

  /* Same normalisation the chat uses, so one emote looks identical wherever it
     is written: fixed height, width auto to keep the aspect, and a max-width
     so an unusually wide upload cannot stretch the line. inline-block is
     load-bearing — base.css resets `img` to display:block, which would push
     the emote onto its own line. */
  .emote-wrap {
    position: relative;
    display: inline-block;
    vertical-align: middle;
  }
  .emote-img {
    display: inline-block;
    height: 28px;
    width: auto;
    max-width: 112px;
    vertical-align: middle;
    margin: -2px 1px;
    object-fit: contain;
  }
  .emote-wrap::after {
    content: attr(data-name);
    position: absolute;
    bottom: calc(100% + 5px);
    left: 50%;
    transform: translateX(-50%);
    padding: 3px 7px;
    border-radius: var(--radius-sm);
    background: var(--surface-overlay);
    border: 1px solid var(--border-subtle);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: var(--fs-micro);
    line-height: 1.4;
    white-space: nowrap;
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.12s;
    z-index: 5;
  }
  .emote-wrap:hover::after {
    opacity: 1;
  }
</style>
