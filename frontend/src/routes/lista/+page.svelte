<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import PosterCard from '$lib/components/PosterCard.svelte';
  import api from '$lib/api';
  import { authStore } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import { displayName } from '$lib/types';
  import type { WatchlistEntry, ReadlistEntry } from '$shared/types';

  const auth = $derived($authStore);

  let media = $state<'anime' | 'manga'>('anime');

  // deep links like /lista?media=manga
  $effect(() => {
    const m = page.url.searchParams.get('media');
    if (m === 'manga' || m === 'anime') media = m;
  });
  let watchlist = $state<WatchlistEntry[]>([]);
  let readlist = $state<ReadlistEntry[]>([]);
  let loading = $state(true);

  $effect(() => {
    if (auth.isLoading) return;
    if (!auth.isAuthenticated) {
      goto('/login?redirect=/lista');
      return;
    }
    load();
  });

  async function load() {
    loading = true;
    try {
      const [wl, rl] = await Promise.all([
        api.getWatchlist().catch(() => ({ data: [] })),
        api.getReadlist().catch(() => ({ data: [] }))
      ]);
      watchlist = wl.data;
      readlist = rl.data;
    } finally {
      loading = false;
    }
  }

  /* Letterboxd semantics: the watchlist is what you WANT to watch.
     Everything you've started or finished lives in /istoric. */
  const planAnime = $derived(watchlist.filter((e) => e.status === 'plan-to-watch' && e.anime));
  const planManga = $derived(readlist.filter((e) => e.status === 'plan-to-read' && e.manga));
  const shown = $derived(media === 'anime' ? planAnime : planManga);

  async function removeAnime(e: WatchlistEntry) {
    try {
      await api.removeFromWatchlist(e.animeId);
      watchlist = watchlist.filter((x) => x.id !== e.id);
      toast.success('Scos din watchlist.');
    } catch {
      toast.error('Eroare la eliminare.');
    }
  }
  async function removeManga(e: ReadlistEntry) {
    try {
      await api.removeFromReadlist(e.mangaId);
      readlist = readlist.filter((x) => x.id !== e.id);
      toast.success('Scos din watchlist.');
    } catch {
      toast.error('Eroare la eliminare.');
    }
  }

  const addedAt = (d?: Date | string) =>
    d ? new Date(d).toLocaleDateString('ro-RO', { day: 'numeric', month: 'short' }) : '';
</script>

<svelte:head><title>Watchlist · Anime-Kage</title></svelte:head>

<div class="container lista">
  <header class="top">
    <div>
      <p class="l-kicker">Vrei să vezi</p>
      <h1>Watchlist</h1>
    </div>
    <div class="top-side">
      <span class="count">{shown.length} titluri</span>
      <a class="hist-link" href="/istoric">Istoricul tău →</a>
    </div>
  </header>

  <div class="controls">
    <div class="pills">
      <button class="pill" class:on={media === 'anime'} onclick={() => (media = 'anime')}>
        Anime <span class="pill-n">{planAnime.length}</span>
      </button>
      <button class="pill" class:on={media === 'manga'} onclick={() => (media = 'manga')}>
        Manga <span class="pill-n">{planManga.length}</span>
      </button>
    </div>
  </div>

  {#if loading}
    <p class="muted">Se încarcă…</p>
  {:else if shown.length}
    <div class="grid">
      {#if media === 'anime'}
        {#each planAnime as e (e.id)}
          <div class="slot">
            <PosterCard a={e.anime!} href={`/anime/${e.animeId}`} />
            <button class="rm" title={`Scoate ${displayName(e.anime!)} din watchlist`} onclick={() => removeAnime(e)}>✕</button>
            {#if e.updatedAt}<span class="added">adăugat {addedAt(e.updatedAt)}</span>{/if}
          </div>
        {/each}
      {:else}
        {#each planManga as e (e.id)}
          <div class="slot">
            <PosterCard a={e.manga!} href={`/manga/${e.mangaId}`} />
            <button class="rm" title={`Scoate ${displayName(e.manga!)} din watchlist`} onclick={() => removeManga(e)}>✕</button>
            {#if e.updatedAt}<span class="added">adăugat {addedAt(e.updatedAt)}</span>{/if}
          </div>
        {/each}
      {/if}
    </div>
  {:else}
    <div class="empty">
      <p class="empty-t">Watchlistul tău e gol.</p>
      <p class="empty-m">Adaugă titlurile pe care vrei să le vezi — le găsești apoi aici, ca pe Letterboxd.</p>
      <a class="btn fill" href={media === 'anime' ? '/anime' : '/manga'}>Răsfoiește {media === 'anime' ? 'catalogul' : 'biblioteca'}</a>
    </div>
  {/if}
</div>

<style>
  .lista { padding-block: var(--space-6) var(--space-8); }
  .top {
    display: flex; align-items: flex-end; justify-content: space-between;
    flex-wrap: wrap; gap: var(--space-4);
    padding-bottom: 18px; border-bottom: 2px solid var(--text-primary);
    margin-bottom: var(--space-5);
  }
  .l-kicker { font-size: var(--fs-caption); font-weight: var(--fw-bold); color: var(--accent); }
  .top h1 { font-size: clamp(1.8rem, 1.5rem + 1.4vw, 2.375rem); letter-spacing: -0.015em; line-height: 1.05; margin-top: 10px; }
  .top-side { display: flex; align-items: baseline; gap: 18px; }
  .count { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .hist-link { font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--accent); }
  .hist-link:hover { color: var(--accent-hover); }

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
  .slot { position: relative; }
  .rm {
    position: absolute; top: 8px; right: 8px; z-index: 2;
    width: 26px; height: 26px; border-radius: 50%; border: none; cursor: pointer;
    background: rgba(10, 10, 12, 0.65); color: #fff; font-size: 0.75rem; line-height: 1;
    opacity: 0; transition: opacity 0.15s, background 0.15s;
  }
  .slot:hover .rm { opacity: 1; }
  .rm:hover { background: var(--danger); }
  .added {
    display: block; margin-top: 7px;
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
  }

  .empty { padding: var(--space-8) 0; text-align: center; }
  .empty-t { font-family: var(--font-display); font-style: italic; font-size: var(--fs-h2); font-weight: var(--fw-semibold); }
  .empty-m { margin: 10px auto 22px; max-width: 44ch; font-size: var(--fs-small); color: var(--text-muted); }
  .btn {
    display: inline-block; font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 11px 20px; border-radius: var(--radius-md); cursor: pointer;
  }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); }
  .muted { color: var(--text-muted); }

  @media (max-width: 560px) {
    .grid { grid-template-columns: repeat(auto-fill, minmax(110px, 1fr)); gap: 14px 10px; }
  }
</style>
