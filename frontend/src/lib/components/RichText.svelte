<script lang="ts">
  import Spoiler from '$lib/components/Spoiler.svelte';
  import { parseText, isGifUrl } from '$lib/markdown';

  /**
   * User-written plain text — comments, reviews, chat.
   *
   * Handles exactly two things beyond literal text: `||spoilers||` and bare
   * Giphy links, which render as the GIF. Deliberately NOT the Markdown
   * renderer; see parseText() for why.
   */
  let { text = '', interactive = true }: { text?: string; interactive?: boolean } = $props();
  const parts = $derived(parseText(text));
</script>

{#each parts as p}{#if p.kind === 'spoiler'}<Spoiler {interactive}
    >{#if isGifUrl(p.text.trim())}<!-- "hide this GIF" is the obvious use of a
      spoiler round a link, so reveal the image rather than the URL -->
      <img
        class="gif"
        src={p.text.trim()}
        alt="GIF"
        loading="lazy"
        referrerpolicy="no-referrer"
      />{:else}{p.text}{/if}</Spoiler
  >{:else if p.kind === 'gif'}<img
    class="gif"
    src={p.url}
    alt="GIF"
    loading="lazy"
    referrerpolicy="no-referrer"
  />{:else}{p.text}{/if}{/each}

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
</style>
