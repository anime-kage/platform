<script lang="ts">
  import PosterCard from '$lib/components/PosterCard.svelte';
  import { animeHref, mangaHref } from '$lib/types';

  let { data } = $props();

  // The catalog's own filters (an, sezon, studio) are anime-only concepts, so
  // the one control here is the one a mixed result set actually needs.
  let kind = $state<'all' | 'anime' | 'manga'>('all');
  const showAnime = $derived(kind !== 'manga');
  const showManga = $derived(kind !== 'anime');
  const shown = $derived(
    (showAnime ? data.anime.length : 0) + (showManga ? data.manga.length : 0)
  );
</script>

<svelte:head><title>Căutare: {data.q} · Anime-Kage</title></svelte:head>

<div class="container browse">
  <header class="top">
    <div>
      <p class="cat-kicker">Căutare</p>
      <h1>{data.q}</h1>
    </div>

    <div class="controls">
      <div class="control">
        <span class="kicker">Tip</span>
        <select class="fsel" bind:value={kind}>
          <option value="all">Toate ({data.anime.length + data.manga.length})</option>
          <option value="anime">Anime ({data.anime.length})</option>
          <option value="manga">Manga ({data.manga.length})</option>
        </select>
      </div>
    </div>
  </header>

  <p class="count">{shown} rezultate&nbsp;pentru „{data.q}"</p>

  {#if shown === 0}
    <p class="empty">Niciun rezultat. Încearcă alt titlu.</p>
  {/if}

  {#if showAnime && data.anime.length}
    <h2 class="sect">Anime <span class="sect-n">{data.anime.length}</span></h2>
    <div class="grid">
      {#each data.anime as a (a.id)}
        <PosterCard {a} href={animeHref(a)} />
      {/each}
    </div>
  {/if}

  {#if showManga && data.manga.length}
    <h2 class="sect" class:spaced={showAnime && data.anime.length > 0}>
      Manga <span class="sect-n">{data.manga.length}</span>
    </h2>
    <div class="grid">
      {#each data.manga as m (m.id)}
        <PosterCard a={m} href={mangaHref(m)} />
      {/each}
    </div>
  {/if}
</div>

<style>
  /* Deliberately the catalog's own shell — same .top rule, same kicker, same
     count line, same grid. Search results are part of the catalog, not a
     separate kind of page, and the previous bespoke styling made them look
     like one. */
  .browse { padding-block: var(--space-6) var(--space-8); }

  .top {
    display: flex; align-items: flex-end; justify-content: space-between;
    flex-wrap: wrap; gap: var(--space-4); margin-bottom: var(--space-5);
    padding-bottom: 18px; border-bottom: 2px solid var(--text-primary);
  }
  .cat-kicker { font-size: var(--fs-caption); font-weight: var(--fw-bold); color: var(--accent); }
  .top h1 {
    font-size: clamp(2rem, 1.6rem + 1.8vw, 2.625rem);
    letter-spacing: -0.02em; line-height: 1; margin-top: 10px;
    overflow-wrap: anywhere;
  }

  .controls { display: flex; align-items: center; gap: var(--space-4); flex-wrap: wrap; }
  .control { display: flex; align-items: center; gap: 8px; }
  .fsel {
    padding: 8px 10px; min-width: 108px; max-width: 190px;
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md); color: var(--text-primary); cursor: pointer;
    font-size: var(--fs-caption); font-family: var(--font-body); outline: none;
  }

  .count {
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
    margin: var(--space-4) 0 var(--space-5);
  }
  .empty { color: var(--text-muted); }

  .sect {
    display: flex; align-items: center; gap: 10px;
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.14em; text-transform: uppercase;
    color: var(--text-faint); font-weight: var(--fw-regular);
    margin: 0 0 var(--space-4);
  }
  .sect.spaced { margin-top: var(--space-7); }
  .sect-n {
    font-size: var(--fs-micro); color: var(--text-muted);
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm); padding: 1px 7px; letter-spacing: 0;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: var(--space-4);
  }
</style>
