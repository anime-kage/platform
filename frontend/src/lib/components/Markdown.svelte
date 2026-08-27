<script lang="ts">
  import { parseMarkdown, type Block, type Inline } from '$lib/markdown';
  import { mediaUrl } from '$lib/media';
  import Spoiler from '$lib/components/Spoiler.svelte';

  // Renders a post body as elements, never as an HTML string — see
  // lib/markdown.ts for why. Nothing here touches {@html}.
  let { source = '' }: { source?: string } = $props();
  const blocks = $derived(parseMarkdown(source));
</script>

{#snippet inlines(nodes: Inline[])}
  {#each nodes as n}
    {#if n.t === 'text'}{n.v}
    {:else if n.t === 'bold'}<strong>{@render inlines(n.v)}</strong>
    {:else if n.t === 'italic'}<em>{@render inlines(n.v)}</em>
    {:else if n.t === 'code'}<code>{n.v}</code>
    {:else if n.t === 'spoiler'}<Spoiler>{@render inlines(n.v)}</Spoiler>
    {:else if n.t === 'link'}<a
        href={n.href}
        rel={n.href.startsWith('/') ? undefined : 'noopener noreferrer nofollow'}
        target={n.href.startsWith('/') ? undefined : '_blank'}>{@render inlines(n.v)}</a
      >{/if}
  {/each}
{/snippet}

<div class="md">
  {#each blocks as b}
    {#if b.t === 'p'}
      <p>{@render inlines(b.v)}</p>
    {:else if b.t === 'h'}
      {#if b.level === 2}<h2>{@render inlines(b.v)}</h2>
      {:else if b.level === 3}<h3>{@render inlines(b.v)}</h3>
      {:else}<h4>{@render inlines(b.v)}</h4>{/if}
    {:else if b.t === 'ul'}
      <ul>
        {#each b.items as it}<li>{@render inlines(it)}</li>{/each}
      </ul>
    {:else if b.t === 'quote'}
      <blockquote>{@render inlines(b.v)}</blockquote>
    {:else if b.t === 'img'}
      <figure><img src={mediaUrl(b.src)} alt={b.alt} loading="lazy" /></figure>
    {:else if b.t === 'hr'}
      <hr />
    {/if}
  {/each}
</div>

<style>
  /* Reading column, platform type scale. Deliberately narrow — a news post is
     prose, and prose past ~70 characters a line gets hard to track. */
  .md { font-size: var(--fs-body); line-height: 1.7; color: var(--text-muted); max-width: 68ch; }
  .md :global(p) { margin: 0 0 var(--space-4); text-wrap: pretty; }
  .md :global(p:last-child) { margin-bottom: 0; }
  .md :global(h2),
  .md :global(h3),
  .md :global(h4) {
    font-family: var(--font-display); color: var(--text-primary);
    line-height: 1.25; margin: var(--space-5) 0 var(--space-3);
  }
  .md :global(h2) { font-size: var(--fs-h2); }
  .md :global(h3) { font-size: var(--fs-h3); }
  .md :global(h4) { font-size: var(--fs-body); font-weight: var(--fw-semibold); }
  .md :global(strong) { color: var(--text-primary); font-weight: var(--fw-semibold); }
  .md :global(em) { font-style: italic; }
  .md :global(a) { color: var(--accent); text-decoration: underline; text-underline-offset: 3px; }
  .md :global(a:hover) { color: var(--accent-hover); }
  .md :global(code) {
    font-family: var(--font-mono); font-size: 0.9em;
    background: var(--surface-inset); padding: 1px 6px; border-radius: var(--radius-sm);
    color: var(--text-primary);
  }
  .md :global(ul) { margin: 0 0 var(--space-4); padding-left: 1.25rem; display: flex; flex-direction: column; gap: 6px; }
  .md :global(li) { list-style: disc; }
  .md :global(blockquote) {
    border-left: 2px solid var(--accent); padding-left: var(--space-4);
    margin: 0 0 var(--space-4); color: var(--text-muted); font-style: italic;
  }
  .md :global(figure) { margin: var(--space-5) 0; }
  .md :global(figure img) {
    display: block; width: 100%; height: auto;
    border-radius: var(--radius-md); border: 1px solid var(--border-subtle);
  }
  .md :global(hr) { border: none; border-top: 1px solid var(--border-subtle); margin: var(--space-5) 0; }
</style>
