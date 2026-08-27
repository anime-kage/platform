<script lang="ts">
  import { mediaUrl } from '$lib/media';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import api from '$lib/api';
  import { authStore } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import { displayName } from '$lib/types';
  import type { WatchlistEntry, ReadlistEntry } from '$shared/types';

  const auth = $derived($authStore);

  let media = $state<'anime' | 'manga'>('anime');
  let status = $state<string>('');

  // deep links like /istoric?media=manga&status=completed (profile stat cells)
  $effect(() => {
    const m = page.url.searchParams.get('media');
    const s = page.url.searchParams.get('status');
    if (m === 'manga' || m === 'anime') media = m;
    if (s !== null) status = s;
  });
  let watchlist = $state<WatchlistEntry[]>([]);
  let readlist = $state<ReadlistEntry[]>([]);
  let loading = $state(true);

  const STATUSES: [string, string, string][] = [
    // [key-anime, key-manga, label]
    ['watching', 'reading', 'În desfășurare'],
    ['completed', 'completed', 'Finalizate'],
    ['on-hold', 'on-hold', 'În așteptare'],
    ['dropped', 'dropped', 'Abandonate']
  ];

  $effect(() => {
    if (auth.isLoading) return;
    if (!auth.isAuthenticated) {
      goto('/login?redirect=/istoric');
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

  /* history = everything you've started or finished (plans live in /lista) */
  const histAnime = $derived(watchlist.filter((e) => e.status !== 'plan-to-watch' && e.anime));
  const histManga = $derived(readlist.filter((e) => e.status !== 'plan-to-read' && e.manga));

  const statusKey = $derived.by(() => {
    if (!status) return '';
    const def = STATUSES.find(([a, m]) => a === status || m === status);
    if (!def) return '';
    return media === 'anime' ? def[0] : def[1];
  });

  const animeShown = $derived(statusKey ? histAnime.filter((e) => e.status === statusKey) : histAnime);
  const mangaShown = $derived(statusKey ? histManga.filter((e) => e.status === statusKey) : histManga);
  const count = $derived(media === 'anime' ? animeShown.length : mangaShown.length);

  const statusRo = (s: string) =>
    ({
      watching: 'în vizionare',
      reading: 'în citire',
      completed: 'finalizat',
      'on-hold': 'în așteptare',
      dropped: 'abandonat'
    })[s] ?? s;

  /* The server resolves the next anime episode against what we've actually
     published, so this link can't point at an episode that doesn't exist.
     (nextCh has no equivalent yet — the readlist still clamps to MAL's
     chapter count, which can overshoot the same way.) */
  const nextCh = (e: ReadlistEntry) =>
    Math.min(e.chaptersRead + 1, e.manga?.chapters ?? e.chaptersRead + 1);
  const inProgress = (s: string) => s === 'watching' || s === 'reading' || s === 'on-hold';

  const pct = (done: number, total?: number | null) =>
    total ? Math.min(100, Math.round((done / total) * 100)) : 0;

  const lastTouched = (d?: Date | string) =>
    d ? new Date(d).toLocaleDateString('ro-RO', { day: 'numeric', month: 'short', year: 'numeric' }) : '';

  async function removeAnime(e: WatchlistEntry) {
    try {
      await api.removeFromWatchlist(e.animeId);
      watchlist = watchlist.filter((x) => x.id !== e.id);
      toast.success('Eliminat din istoric.');
    } catch {
      toast.error('Eroare la eliminare.');
    }
  }
  async function removeManga(e: ReadlistEntry) {
    try {
      await api.removeFromReadlist(e.mangaId);
      readlist = readlist.filter((x) => x.id !== e.id);
      toast.success('Eliminat din istoric.');
    } catch {
      toast.error('Eroare la eliminare.');
    }
  }
</script>

<svelte:head><title>Istoric · Anime-Kage</title></svelte:head>

<div class="container hist">
  <header class="top">
    <div>
      <p class="l-kicker">Unde ai rămas</p>
      <h1>Istoric</h1>
    </div>
    <div class="top-side">
      <span class="count">{count} titluri</span>
      <a class="wl-link" href="/lista">Watchlistul tău →</a>
    </div>
  </header>

  <div class="controls">
    <div class="pills">
      <button class="pill" class:on={media === 'anime'} onclick={() => (media = 'anime')}>
        Anime <span class="pill-n">{histAnime.length}</span>
      </button>
      <button class="pill" class:on={media === 'manga'} onclick={() => (media = 'manga')}>
        Manga <span class="pill-n">{histManga.length}</span>
      </button>
    </div>
    <div class="chips">
      <button class="chip" class:on={status === ''} onclick={() => (status = '')}>Toate</button>
      {#each STATUSES as [ka, km, label] (ka)}
        <button
          class="chip"
          class:on={statusKey === (media === 'anime' ? ka : km)}
          onclick={() => (status = media === 'anime' ? ka : km)}
        >{label}</button>
      {/each}
    </div>
  </div>

  {#if loading}
    <p class="muted">Se încarcă…</p>
  {:else if media === 'anime' ? animeShown.length : mangaShown.length}
    <div class="rows">
      {#if media === 'anime'}
        {#each animeShown as e (e.id)}
          <div class="row">
            <a class="thumb media-tone" href={`/anime/${e.animeId}`} style={e.anime?.imageUrl ? `background-image:url(${mediaUrl(e.anime.imageUrl)})` : ''} aria-label={displayName(e.anime!)}></a>
            <div class="main">
              <a class="t" href={`/anime/${e.animeId}`}>{displayName(e.anime!)}</a>
              <p class="m">
                <span class="st" class:done={e.status === 'completed'}>{statusRo(e.status)}</span>
                {#if e.status === 'completed'}
                  · {e.availableEpisodes || e.episodesWatched} episoade
                {:else}
                  · ai rămas la episodul {e.episodesWatched}{#if e.availableEpisodes}&nbsp;din {e.availableEpisodes}{/if}
                {/if}
                {#if e.score} · nota {e.score}{/if}
                {#if e.updatedAt} · {lastTouched(e.updatedAt)}{/if}
              </p>
              {#if e.status !== 'completed' && e.availableEpisodes}
                <span class="track"><span class="fill" style={`width:${pct(e.episodesWatched, e.availableEpisodes)}%`}></span></span>
              {/if}
            </div>
            <div class="side">
              {#if inProgress(e.status) && e.nextEpisode}
                <a class="btn fill sm" href={`/anime/${e.animeId}/episode/${e.nextEpisode}`}>▶ Ep. {e.nextEpisode}</a>
              {:else if inProgress(e.status)}
                <a class="btn ghost sm" href={`/anime/${e.animeId}`}>La zi →</a>
              {:else if e.status === 'completed'}
                <a class="btn ghost sm" href={`/anime/${e.animeId}`}>Revezi →</a>
              {:else}
                <a class="btn ghost sm" href={`/anime/${e.animeId}`}>Detalii →</a>
              {/if}
              <button class="rm" onclick={() => removeAnime(e)} title="Elimină din istoric">✕</button>
            </div>
          </div>
        {/each}
      {:else}
        {#each mangaShown as e (e.id)}
          <div class="row">
            <a class="thumb media-tone" href={`/manga/${e.mangaId}`} style={e.manga?.imageUrl ? `background-image:url(${mediaUrl(e.manga.imageUrl)})` : ''} aria-label={displayName(e.manga!)}></a>
            <div class="main">
              <a class="t" href={`/manga/${e.mangaId}`}>{displayName(e.manga!)}</a>
              <p class="m">
                <span class="st" class:done={e.status === 'completed'}>{statusRo(e.status)}</span>
                {#if e.status === 'completed'}
                  · {e.manga?.chapters ?? e.chaptersRead} capitole
                {:else}
                  · ai rămas la capitolul {e.chaptersRead}{#if e.manga?.chapters}&nbsp;din {e.manga.chapters}{/if}
                {/if}
                {#if e.score} · nota {e.score}{/if}
                {#if e.updatedAt} · {lastTouched(e.updatedAt)}{/if}
              </p>
              {#if e.status !== 'completed' && e.manga?.chapters}
                <span class="track"><span class="fill" style={`width:${pct(e.chaptersRead, e.manga.chapters)}%`}></span></span>
              {/if}
            </div>
            <div class="side">
              {#if inProgress(e.status)}
                <a class="btn fill sm" href={`/manga/${e.mangaId}/chapter/${nextCh(e)}`}>📖 Cap. {nextCh(e)}</a>
              {:else if e.status === 'completed'}
                <a class="btn ghost sm" href={`/manga/${e.mangaId}`}>Recitește →</a>
              {:else}
                <a class="btn ghost sm" href={`/manga/${e.mangaId}`}>Detalii →</a>
              {/if}
              <button class="rm" onclick={() => removeManga(e)} title="Elimină din istoric">✕</button>
            </div>
          </div>
        {/each}
      {/if}
    </div>
  {:else}
    <div class="empty">
      <p class="empty-t">Nimic în istoric{status ? ' cu acest status' : ''}.</p>
      <p class="empty-m">Începe ceva din watchlist — progresul tău apare aici, cu tot cu locul unde ai rămas.</p>
      <a class="btn fill" href="/lista">Vezi watchlistul →</a>
    </div>
  {/if}
</div>

<style>
  .hist { padding-block: var(--space-6) var(--space-8); }
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
  .wl-link { font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--accent); }
  .wl-link:hover { color: var(--accent-hover); }

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

  .chips { display: flex; gap: 8px; flex-wrap: wrap; }
  .chip {
    font-size: var(--fs-caption); font-weight: var(--fw-medium); color: var(--text-muted);
    padding: 5px 12px; border: 1px solid var(--border-subtle); border-radius: var(--radius-pill);
    background: none; cursor: pointer;
  }
  .chip:hover { color: var(--text-primary); }
  .chip.on {
    color: var(--accent); border-color: color-mix(in srgb, var(--accent) 55%, transparent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }

  /* flat hairline rows */
  .rows { display: flex; flex-direction: column; }
  .row {
    display: flex; align-items: center; gap: var(--space-4);
    padding: 15px 0; border-bottom: 1px solid var(--border-subtle);
  }
  .row:first-child { border-top: 1px solid var(--border-subtle); }
  .thumb {
    width: 52px; height: 76px; border-radius: 5px; flex: 0 0 auto;
    background-color: var(--surface-overlay); background-size: cover; background-position: center;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
  }
  .main { flex: 1; min-width: 0; }
  .t {
    font-family: var(--font-display); font-size: var(--fs-h3); font-weight: var(--fw-semibold);
    color: var(--text-primary); display: block;
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .t:hover { color: var(--accent); }
  .m { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); margin-top: 5px; }
  .st { color: var(--accent); }
  .st.done { color: var(--success); }
  .track {
    display: block; height: 3px; max-width: 300px; margin-top: 9px;
    border-radius: var(--radius-pill); background: var(--surface-overlay);
  }
  .fill { display: block; height: 100%; border-radius: var(--radius-pill); background: var(--accent); }

  .side { display: flex; align-items: center; gap: 10px; flex: 0 0 auto; }
  .btn {
    display: inline-block; font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 11px 18px; border-radius: var(--radius-md); white-space: nowrap; cursor: pointer;
  }
  .btn.sm { padding: 8px 14px; font-size: var(--fs-caption); }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); color: var(--on-accent); }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-primary); }
  .btn.ghost:hover { background: var(--surface-overlay); }
  .rm {
    width: 34px; height: 34px; border-radius: var(--radius-sm);
    border: 1px solid var(--border-subtle); background: none; color: var(--text-muted);
    cursor: pointer; font-size: 0.8125rem;
  }
  .rm:hover { color: var(--danger); border-color: color-mix(in srgb, var(--danger) 55%, transparent); }

  .empty { padding: var(--space-8) 0; text-align: center; }
  .empty-t { font-family: var(--font-display); font-style: italic; font-size: var(--fs-h2); font-weight: var(--fw-semibold); }
  .empty-m { margin: 10px auto 22px; max-width: 46ch; font-size: var(--fs-small); color: var(--text-muted); }
  .muted { color: var(--text-muted); }

  @media (max-width: 620px) {
    .side { flex-direction: column; align-items: flex-end; gap: 6px; }
    .track { max-width: none; }
  }
</style>
