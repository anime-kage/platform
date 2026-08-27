<script lang="ts">
  import PosterCard from '$lib/components/PosterCard.svelte';
  import { authStore } from '$lib/stores/auth';

  let { data } = $props();
  const isSelf = $derived($authStore.user?.username === data.handle);

  let media = $state<'anime' | 'manga'>('anime');
  const shown = $derived(media === 'anime' ? data.planAnime : data.planManga);
  const total = $derived(data.planAnime.length + data.planManga.length);

  const addedAt = (d?: Date | string) =>
    d ? new Date(d).toLocaleDateString('ro-RO', { day: 'numeric', month: 'short' }) : '';
</script>

<svelte:head><title>Watchlist · {data.name} · Anime-Kage</title></svelte:head>

<div class="container wl">
  <header class="top">
    <div>
      <p class="crumb"><a href={`/user/${data.handle}`}>← {data.name}</a></p>
      <p class="l-kicker">Vrea să vadă</p>
      <h1>Watchlistul lui {data.name}</h1>
    </div>
    <span class="count">{total} titluri</span>
  </header>

  <div class="controls">
    <div class="pills">
      <button class="pill" class:on={media === 'anime'} onclick={() => (media = 'anime')}>
        Anime <span class="pill-n">{data.planAnime.length}</span>
      </button>
      <button class="pill" class:on={media === 'manga'} onclick={() => (media = 'manga')}>
        Manga <span class="pill-n">{data.planManga.length}</span>
      </button>
    </div>
  </div>

  {#if shown.length}
    <div class="grid">
      {#if media === 'anime'}
        {#each data.planAnime as e (e.id)}
          <div class="slot">
            <PosterCard a={e.anime!} href={`/anime/${e.animeId}`} />
            {#if e.updatedAt}<span class="added">adăugat {addedAt(e.updatedAt)}</span>{/if}
          </div>
        {/each}
      {:else}
        {#each data.planManga as e (e.id)}
          <div class="slot">
            <PosterCard a={e.manga!} href={`/manga/${e.mangaId}`} />
            {#if e.updatedAt}<span class="added">adăugat {addedAt(e.updatedAt)}</span>{/if}
          </div>
        {/each}
      {/if}
    </div>
  {:else}
    <div class="empty">
      <p class="empty-t">
        {isSelf ? 'Watchlistul tău e gol.' : `${data.name} nu are încă ${media === 'anime' ? 'anime' : 'manga'} în watchlist.`}
      </p>
      {#if isSelf}
        <a class="go" href="/lista">Mergi la watchlistul tău →</a>
      {/if}
    </div>
  {/if}
</div>

<style>
  .wl { padding-block: var(--space-6) var(--space-8); }
  .top {
    display: flex; align-items: flex-end; justify-content: space-between;
    flex-wrap: wrap; gap: var(--space-4);
    padding-bottom: 18px; border-bottom: 2px solid var(--text-primary);
    margin-bottom: var(--space-5);
  }
  .crumb { margin-bottom: 12px; font-family: var(--font-mono); font-size: var(--fs-caption); }
  .crumb a { color: var(--text-muted); }
  .crumb a:hover { color: var(--accent); }
  .l-kicker { font-size: var(--fs-caption); font-weight: var(--fw-bold); color: var(--accent); }
  .top h1 { font-size: clamp(1.8rem, 1.5rem + 1.4vw, 2.375rem); letter-spacing: -0.015em; line-height: 1.05; margin-top: 10px; }
  .count { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }

  .controls { display: flex; align-items: center; gap: var(--space-4); flex-wrap: wrap; margin-bottom: var(--space-5); }
  .pills {
    display: flex; gap: 2px; padding: 3px;
    background: var(--surface-raised); border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
  }
  .pill {
    display: flex; align-items: baseline; gap: 7px;
    font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--text-muted);
    padding: 6px 14px; border-radius: 7px; border: none; background: none; cursor: pointer;
  }
  .pill.on { background: var(--surface-overlay); color: var(--text-primary); }
  .pill-n { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .pill.on .pill-n { color: var(--accent); }

  .grid {
    display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    gap: 18px 16px;
  }
  .added {
    display: block; margin-top: 7px;
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
  }

  .empty { padding: var(--space-8) 0; text-align: center; }
  .empty-t { font-family: var(--font-display); font-style: italic; font-size: var(--fs-h2); font-weight: var(--fw-semibold); }
  .go { display: inline-block; margin-top: 14px; font-size: var(--fs-small); font-weight: var(--fw-semibold); color: var(--accent); }
  .go:hover { color: var(--accent-hover); }

  @media (max-width: 560px) {
    .grid { grid-template-columns: repeat(auto-fill, minmax(110px, 1fr)); gap: 14px 10px; }
  }
</style>
