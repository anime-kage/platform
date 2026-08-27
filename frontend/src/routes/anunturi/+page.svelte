<script lang="ts">
  import { mediaUrl } from '$lib/media';
  import { reltime } from '$lib/reltime';
  import { markdownExcerpt } from '$lib/markdown';

  let { data } = $props();

  // The tag earns its keep here rather than on the home strip: as a filter it
  // is navigation, where on the card it was only decoration.
  let tag = $state<string | null>(null);
  const shown = $derived(tag ? data.posts.filter((p) => p.tag === tag) : data.posts);

  const href = (p: (typeof data.posts)[number]) => `/anunturi/${p.slug ?? p.id}`;
</script>

<svelte:head><title>Știri & anunțuri · Anime-Kage</title></svelte:head>

<div class="container wrap">
  <header class="top">
    <div>
      <p class="kick">Anime-Kage</p>
      <h1>Știri & anunțuri</h1>
      <p class="sub">Tot ce anunță echipa — funcții noi, reguli, mentenanță.</p>
    </div>
    <span class="kicker">{data.posts.length} {data.posts.length === 1 ? 'anunț' : 'anunțuri'}</span>
  </header>

  {#if data.tags.length > 1}
    <nav class="tags" aria-label="Filtrează după etichetă">
      <button class="chip" class:on={tag === null} onclick={() => (tag = null)}>Toate</button>
      {#each data.tags as t}
        <button class="chip" class:on={tag === t} onclick={() => (tag = t)}>{t}</button>
      {/each}
    </nav>
  {/if}

  {#if shown.length === 0}
    <p class="empty">
      {data.posts.length === 0
        ? 'Niciun anunț încă. Când echipa are ceva de spus, apare aici.'
        : 'Niciun anunț cu eticheta asta.'}
    </p>
  {:else}
    <div class="list">
      {#each shown as p (p.id)}
        <a class="post" href={href(p)}>
          {#if p.coverUrl}
            <span class="cover media-tone" style={`background-image:url(${mediaUrl(p.coverUrl)})`}></span>
          {/if}
          <span class="body">
            <span class="meta">
              <span class="tag">{p.tag}</span>
              <span class="when">{reltime(p.createdAt)}</span>
              {#if p.commentCount}
                <span class="cc">{p.commentCount} {p.commentCount === 1 ? 'comentariu' : 'comentarii'}</span>
              {/if}
            </span>
            <span class="t">{p.title}</span>
            {#if p.body}<span class="ex">{markdownExcerpt(p.body)}</span>{/if}
          </span>
        </a>
      {/each}
    </div>
  {/if}
</div>

<style>
  .wrap { padding-block: var(--space-6) var(--space-8); max-width: 900px; }
  .top {
    display: flex; align-items: flex-end; justify-content: space-between;
    flex-wrap: wrap; gap: var(--space-4);
    padding-bottom: 18px; border-bottom: 2px solid var(--text-primary); margin-bottom: var(--space-5);
  }
  .kick { font-size: var(--fs-caption); font-weight: var(--fw-bold); color: var(--accent); }
  h1 { font-size: clamp(1.8rem, 1.5rem + 1.4vw, 2.375rem); letter-spacing: -0.015em; line-height: 1.05; margin-top: 10px; }
  .sub { color: var(--text-muted); margin-top: 10px; }

  .tags { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: var(--space-5); }
  .chip {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.08em; text-transform: uppercase; cursor: pointer;
    padding: 6px 13px; border-radius: var(--radius-pill);
    border: 1px solid var(--border-default); background: transparent; color: var(--text-muted);
  }
  .chip:hover { color: var(--text-primary); border-color: var(--border-strong); }
  .chip.on { background: var(--accent); border-color: var(--accent); color: var(--on-accent); }

  .list { display: flex; flex-direction: column; gap: var(--space-4); }
  .post {
    display: flex; gap: var(--space-4); align-items: stretch;
    border: 1px solid var(--border-subtle); border-radius: var(--radius-lg);
    background: var(--surface-raised); overflow: hidden;
    transition: border-color var(--motion-fast) var(--ease);
  }
  .post:hover { border-color: var(--border-default); }
  .post:hover .t { color: var(--accent); }
  .cover { flex: 0 0 168px; background-size: cover; background-position: center; }
  .body { display: flex; flex-direction: column; gap: 7px; padding: var(--space-4) var(--space-5); min-width: 0; }
  .meta { display: flex; align-items: baseline; gap: 10px; flex-wrap: wrap; font-family: var(--font-mono); font-size: var(--fs-micro); }
  .tag { letter-spacing: 0.08em; text-transform: uppercase; color: var(--accent); font-weight: var(--fw-semibold); }
  .when, .cc { color: var(--text-muted); }
  .t {
    font-family: var(--font-display); font-size: var(--fs-h3); font-weight: var(--fw-semibold);
    line-height: 1.25; color: var(--text-primary); text-wrap: pretty;
    transition: color var(--motion-fast) var(--ease);
  }
  .ex {
    font-size: var(--fs-small); line-height: 1.6; color: var(--text-muted);
    display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2;
    -webkit-box-orient: vertical; overflow: hidden;
  }
  .empty {
    border: 1px dashed var(--border-default); border-radius: var(--radius-md);
    padding: var(--space-6); text-align: center; color: var(--text-muted); font-size: var(--fs-small);
  }
  @media (max-width: 640px) {
    .post { flex-direction: column; }
    .cover { flex: 0 0 150px; }
  }
</style>
