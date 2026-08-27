<script lang="ts">
  import { mediaUrl } from '$lib/media';
  import { goto } from '$app/navigation';
  import api from '$lib/api';
  import { toast } from '$lib/stores/toast';
  import { displayName } from '$lib/types';
  import type { Anime, Manga, MalSearchHit } from '$shared/types';

  // Catalog: find or import a title,
  // then work on it in its own page (/admin/[media]/[id]).
  let media = $state<'anime' | 'manga'>('anime');
  let q = $state('');
  let results = $state<(Anime | Manga)[]>([]);
  let searching = $state(false);
  let searchedOnce = $state(false);

  async function search(e?: SubmitEvent) {
    e?.preventDefault();
    searching = true;
    try {
      if (media === 'anime') {
        const r = q.trim()
          ? await api.searchAnime(q, { limit: 18 })
          : await api.getAnime({ limit: 18 } as never);
        results = r.data;
      } else {
        const r = q.trim()
          ? await api.searchManga(q, { limit: 18 })
          : await api.getManga({ limit: 18 } as never);
        results = r.data;
      }
      searchedOnce = true;
    } catch {
      toast.error('Căutarea a eșuat.');
    } finally {
      searching = false;
    }
  }

  function switchMedia(m: typeof media) {
    if (media === m) return;
    media = m;
    results = [];
    // MAL hits are per-medium — an anime hit cannot be imported as manga — so
    // stale results would offer an import that fails. The query text stays.
    malResults = [];
    search();
  }

  // the catalog shouldn't greet you with a blank page — recent titles on load
  $effect(() => {
    if (!searchedOnce && !searching) search();
  });

  // ── import from MyAnimeList ───────────────────────────────────────────────
  // Search by title, not by MAL id. Asking for the id meant leaving the admin
  // panel, finding the series on MAL and copying a number out of its URL — the
  // translator and publish pages have searched by name all along, so this is
  // the same flow rather than a new one.
  let malQ = $state('');
  let malResults = $state<MalSearchHit[]>([]);
  let malSearching = $state(false);
  // Which hit is mid-import, so only that row shows a spinner. null = idle.
  let importing = $state<number | null>(null);

  // api.request throws a bare `{ error }` object, not an Error, so
  // `err instanceof Error` never matched and the server's reason was replaced
  // by the generic fallback. "Titlul este deja în catalog" is worth reading.
  const errMsg = (err: unknown, fallback: string) =>
    (err as { error?: string }).error ?? (err as { message?: string }).message ?? fallback;

  // MAL first, falling back to AniList server-side — same as /translate.
  async function searchMal() {
    if (malQ.trim().length < 2) return;
    malSearching = true;
    try {
      malResults =
        media === 'manga'
          ? (await api.malSearchManga(malQ.trim())).data
          : (await api.malSearchAnime(malQ.trim())).data;
      if (malResults.length === 0) toast.info('Niciun rezultat pe MAL / AniList.');
    } catch (err) {
      toast.error(errMsg(err, 'Căutarea a eșuat.'));
    } finally {
      malSearching = false;
    }
  }

  async function importFromMal(hit: MalSearchHit) {
    importing = hit.malId;
    try {
      const r = media === 'anime' ? await api.importAnime(hit.malId) : await api.importManga(hit.malId);
      // The API is idempotent here: importing something already in the catalog
      // returns the existing row with created:false rather than failing, so say
      // which of the two happened instead of a flat "Importat".
      toast.success(
        r.created === false
          ? `„${r.data.title}” era deja în catalog — o deschid.`
          : `Adăugat în catalog: ${r.data.title}`
      );
      goto(`/admin/${media}/${r.data.id}`);
    } catch (err) {
      toast.error(errMsg(err, 'Importul a eșuat.'));
    } finally {
      importing = null;
    }
  }
</script>

<div class="catalog">
  <div class="main">
    <div class="find">
      <div class="seg" role="tablist" aria-label="Tip de titlu">
        <button class="seg-b" class:on={media === 'anime'} onclick={() => switchMedia('anime')}>Anime</button>
        <button class="seg-b" class:on={media === 'manga'} onclick={() => switchMedia('manga')}>Manga</button>
      </div>
      <form class="search" onsubmit={search}>
        <input type="search" bind:value={q} placeholder={media === 'anime' ? 'Caută un anime din catalog…' : 'Caută o manga din catalog…'} />
        <button class="btn fill" type="submit" disabled={searching}>{searching ? '…' : 'Caută'}</button>
      </form>
    </div>

    <span class="kicker">{q.trim() ? 'Rezultate' : 'Adăugate recent'}</span>
    {#if searching && results.length === 0}
      <p class="muted">Se caută…</p>
    {:else if results.length === 0}
      <div class="empty">
        <p>Niciun titlu găsit{q.trim() ? ` pentru „${q}"` : ''}.</p>
        <p class="muted">Importă-l din MyAnimeList cu formularul alăturat.</p>
      </div>
    {:else}
      <ul class="results">
        {#each results as r (r.id)}
          <li>
            <a class="result" href={`/admin/${media}/${r.id}`}>
              <span class="r-thumb" style={r.imageUrl ? `background-image:url(${mediaUrl(r.imageUrl)})` : ''}></span>
              <span class="r-main">
                <span class="r-t">{displayName(r)}</span>
                <span class="r-m">{r.year ?? '—'} · {r.type ?? '—'} · {r.status ?? '—'}</span>
              </span>
              <span class="r-go">Deschide →</span>
            </a>
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  <aside class="rail">
    <form class="card" onsubmit={(e) => { e.preventDefault(); searchMal(); }}>
      <span class="kicker">Import din MyAnimeList</span>
      <p class="muted">
        Caută seria după nume și adaugă-o în catalog — metadatele vin din Jikan,
        cu AniList ca rezervă.
      </p>
      <div class="row">
        <input
          type="search"
          bind:value={malQ}
          placeholder={media === 'anime' ? 'ex. Clannad' : 'ex. Berserk'}
          aria-label="Titlul seriei pe MAL / AniList"
        />
        <button class="btn fill" type="submit" disabled={malSearching || malQ.trim().length < 2}>
          {malSearching ? '…' : 'Caută'}
        </button>
      </div>

      {#if malResults.length}
        <ul class="hits">
          {#each malResults as hit (hit.malId)}
            <li class="hit">
              {#if hit.imageUrl}
                <img class="h-thumb" src={mediaUrl(hit.imageUrl)} alt="" loading="lazy" />
              {:else}
                <span class="h-thumb"></span>
              {/if}
              <span class="h-main">
                <span class="h-t">{hit.title}</span>
                <span class="h-m">
                  {hit.type}{hit.year ? ` · ${hit.year}` : ''}{hit.episodes
                    ? ` · ${hit.episodes} ep`
                    : hit.chapters
                      ? ` · ${hit.chapters} cap`
                      : ''} · MAL #{hit.malId}
                </span>
              </span>
              <button
                type="button"
                class="btn fill sm"
                disabled={importing !== null}
                onclick={() => importFromMal(hit)}
              >
                {importing === hit.malId ? '…' : 'Adaugă'}
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </form>
  </aside>
</div>

<style>
  .catalog {
    display: grid; grid-template-columns: minmax(0, 1fr) 320px;
    gap: var(--space-6); align-items: start;
  }
  @media (max-width: 900px) {
    .catalog { grid-template-columns: minmax(0, 1fr); }
    .rail { order: -1; }
  }

  .find { display: flex; gap: var(--space-3); flex-wrap: wrap; margin-bottom: var(--space-5); }
  .seg {
    display: flex; gap: 2px; padding: 3px;
    background: var(--surface-raised); border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
  }
  .seg-b {
    font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--text-muted);
    padding: 8px 16px; border-radius: 7px; border: none; background: none; cursor: pointer;
  }
  .seg-b.on { background: var(--surface-overlay); color: var(--text-primary); }

  .search { display: flex; gap: 8px; flex: 1; min-width: 240px; }
  .search input {
    flex: 1; min-height: 44px; padding: 0 14px;
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary); outline: none;
  }
  .search input:focus { border-color: var(--accent); }

  .main > .kicker { display: block; margin-bottom: var(--space-3); }

  .results { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
  .result {
    display: flex; align-items: center; gap: var(--space-4);
    padding: 10px 14px; border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
    background: var(--surface-raised); color: var(--text-primary); min-width: 0;
    transition: border-color var(--motion-fast) var(--ease), background var(--motion-fast) var(--ease);
  }
  .result:hover { border-color: var(--border-strong); background: var(--surface-overlay); }
  .r-thumb {
    width: 38px; height: 56px; border-radius: var(--radius-sm); flex: 0 0 auto;
    background-color: var(--surface-overlay); background-size: cover; background-position: center;
  }
  .r-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
  .r-t { font-weight: var(--fw-semibold); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .r-m { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .r-go { font-size: var(--fs-small); font-weight: var(--fw-semibold); color: var(--accent); white-space: nowrap; }

  .empty {
    border: 1px dashed var(--border-default); border-radius: var(--radius-md);
    padding: var(--space-6); text-align: center; color: var(--text-primary);
  }
  .empty .muted { margin-top: 6px; }

  .rail { display: flex; flex-direction: column; gap: var(--space-4); }
  .card {
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg); padding: var(--space-4) var(--space-5) var(--space-5);
  }
  .card .kicker { display: block; margin-bottom: var(--space-2); }
  .card .muted { font-size: var(--fs-small); margin-bottom: var(--space-3); }
  .row { display: flex; gap: 8px; flex-wrap: wrap; }
  .card input, .card select {
    min-height: 42px; padding: 0 12px; flex: 1; min-width: 90px;
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary); outline: none;
  }
  .card input:focus { border-color: var(--accent); }
  .card select { cursor: pointer; }

  .muted { color: var(--text-muted); }

  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 10px 18px; border-radius: var(--radius-md); white-space: nowrap; cursor: pointer;
  }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-primary); }
  .btn.ghost:hover { background: var(--surface-overlay); }
  .btn:disabled { opacity: 0.6; cursor: wait; }
  /* The rail is 320px, so a hit row cannot afford a full-size button. */
  .btn.sm { padding: 6px 10px; font-size: var(--fs-micro); }

  /* ---- MAL search hits ---- */
  .hits {
    list-style: none; margin: var(--space-4) 0 0; padding: 0;
    display: flex; flex-direction: column; gap: 8px;
    /* A MAL title search returns up to 25 rows; without this the rail grows
       taller than the results list beside it and the page scrolls twice. */
    max-height: 420px; overflow-y: auto;
  }
  .hit {
    display: grid; grid-template-columns: 34px minmax(0, 1fr) auto;
    align-items: center; gap: 10px; padding: 6px;
    background: var(--surface-raised);
    border: 1px solid var(--border-subtle); border-radius: var(--radius-sm);
  }
  .h-thumb {
    display: block; width: 34px; height: 48px;
    object-fit: cover; border-radius: 3px; background: var(--surface-overlay);
  }
  /* minmax(0, 1fr) above plus min-width:0 here is what lets the ellipsis work —
     a grid track otherwise refuses to shrink below its content. */
  .h-main { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
  .h-t, .h-m { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .h-t { font-size: var(--fs-small); color: var(--text-primary); }
  .h-m {
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
  }
</style>
