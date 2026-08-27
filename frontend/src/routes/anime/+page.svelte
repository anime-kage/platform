<script lang="ts">
  import { mediaUrl } from '$lib/media';
  import { goto } from '$app/navigation';
  import PosterCard from '$lib/components/PosterCard.svelte';
  import PagePicker from '$lib/components/PagePicker.svelte';
  import { displayName, genreRo, seasonRo, type Anime, displaySynopsis} from '$lib/types';

  let { data } = $props();

  let genresOpen = $state(false);
  const visibleGenres = $derived(
    genresOpen
      ? data.genres
      : data.genre && !data.genresTop.includes(data.genre)
        ? [...data.genresTop, data.genre]
        : data.genresTop
  );

  // Build an /anime URL preserving the other active params.
  const hrefWith = (patch: Record<string, string | null>) => {
    const p = new URLSearchParams();
    const cur: Record<string, string> = {
      q: data.q,
      gen: data.genre,
      year: data.year,
      season: data.season,
      studio: data.studio,
      sort: data.sort,
      dir: data.dir,
      view: data.view
    };
    // page is intentionally NOT in `cur`: changing any filter resets to page 1
    for (const [k, v] of Object.entries({ ...cur, ...patch })) {
      if (!v) continue;
      if (k === 'sort' && v === 'score') continue; // defaults stay out of the URL
      // Changing the sort resets direction: picking "A–Z" while Scor was
      // reversed should give A–Z, not Z–A. And the natural direction never
      // appears in the URL.
      if (k === 'dir' && ('sort' in patch || v === naturalDir(cur.sort))) continue;
      if (k === 'view' && v === 'grid') continue;
      if (k === 'page' && v === '1') continue;
      p.set(k, v);
    }
    const s = p.toString();
    return s ? `/anime?${s}` : '/anime';
  };

  // best/newest first everywhere, except A–Z which naturally runs forwards
  const naturalDir = (s: string) => (s === 'title' ? 'asc' : 'desc');
  const flipped = $derived(data.dir !== naturalDir(data.sort));
  // what the arrow will do next, spelled out per sort
  const revLabel = $derived(
    data.sort === 'title'
      ? flipped ? 'Sortează A–Z' : 'Sortează Z–A'
      : data.sort === 'year'
        ? flipped ? 'Cele mai noi întâi' : 'Cele mai vechi întâi'
        : flipped ? 'Cele mai bune întâi' : 'Cele mai slabe întâi'
  );

  const sorts = [
    { key: 'score', label: 'Scor' },
    { key: 'year', label: 'An' },
    { key: 'title', label: 'A–Z' }
  ];

  const seasons = ['winter', 'spring', 'summer', 'fall'];

  const metaOf = (a: Anime) =>
    [a.year, (a.type ?? 'TV').toUpperCase(), a.episodes ? `${a.episodes} ep` : null]
      .filter(Boolean)
      .join(' · ');
</script>

<svelte:head><title>Anime · Anime-Kage</title></svelte:head>

<div class="container browse">
  <header class="top">
    <div>
      <p class="cat-kicker">Catalog</p>
      <h1>Anime</h1>
    </div>

    <div class="controls">
      <div class="control">
        <span class="kicker">An</span>
        <select
          class="fsel"
          value={data.year}
          onchange={(e) => goto(hrefWith({ year: (e.currentTarget as HTMLSelectElement).value || null }))}
        >
          <option value="">Toți anii</option>
          {#each data.years as y (y)}
            <option value={String(y)}>{y}</option>
          {/each}
        </select>
      </div>
      <div class="control">
        <span class="kicker">Sezon</span>
        <select
          class="fsel"
          value={data.season}
          onchange={(e) => goto(hrefWith({ season: (e.currentTarget as HTMLSelectElement).value || null }))}
        >
          <option value="">Toate</option>
          {#each seasons as s (s)}
            <option value={s}>{seasonRo(s)}</option>
          {/each}
        </select>
      </div>
      <div class="control">
        <span class="kicker">Studio</span>
        <select
          class="fsel"
          value={data.studio}
          onchange={(e) => goto(hrefWith({ studio: (e.currentTarget as HTMLSelectElement).value || null }))}
        >
          <option value="">Toate</option>
          {#each data.studios as st (st)}
            <option value={st}>{st}</option>
          {/each}
        </select>
      </div>
      <div class="control">
        <span class="kicker">Sortare</span>
        <div class="pills">
          {#each sorts as s (s.key)}
            <a class="pill" class:on={data.sort === s.key} href={hrefWith({ sort: s.key })}>
              {s.label}
            </a>
          {/each}
          <a
            class="pill dir"
            class:flip={flipped}
            href={hrefWith({ dir: flipped ? naturalDir(data.sort) : naturalDir(data.sort) === 'asc' ? 'desc' : 'asc' })}
            title={revLabel}
            aria-label={revLabel}
          >↓</a>
        </div>
      </div>
      <div class="pills">
        <a class="pill" class:on={data.view === 'grid'} href={hrefWith({ view: 'grid' })} title="Grilă">⊞</a>
        <a class="pill" class:on={data.view === 'list'} href={hrefWith({ view: 'list' })} title="Listă">☰</a>
      </div>
    </div>
  </header>

  <!-- featured banner (same as the manga library) -->
  {#if data.featured}
    {@const f = data.featured}
    <a class="feat" href={`/anime/${f.id}`}>
      {#if f.imageUrl}
        <span class="feat-blur" style={`background-image:url(${mediaUrl(f.imageUrl)})`}></span>
      {/if}
      <span class="feat-fade"></span>
      <span class="feat-body">
        <span class="feat-kicker">Recomandarea catalogului</span>
        <span class="feat-title">{displayName(f)}</span>
        <span class="feat-meta">
          {f.year ?? '—'} · {(f.type ?? 'TV').toUpperCase()}{f.episodes ? ` · ${f.episodes} ep` : ''}{f.score ? ` · ★ ${f.score.toFixed(2)}` : ''}
        </span>
        {#if displaySynopsis(f)}<span class="feat-syn">{displaySynopsis(f)}</span>{/if}
        <span class="btn fill">▶ Vizionează</span>
      </span>
      {#if f.imageUrl}
        <span class="feat-poster">
          <img class="media-tone" src={mediaUrl(f.imageUrl)} alt="" width="150" height="225" loading="lazy" />
        </span>
      {/if}
    </a>
  {/if}

  <!-- genres: most common as chips, the rest behind the expander -->
  <div class="genres">
    <a class="chip" class:on={!data.genre} href={hrefWith({ gen: null })}>Toate</a>
    {#each visibleGenres as g (g)}
      <a class="chip" class:on={data.genre === g} href={hrefWith({ gen: g })}>{genreRo(g)}</a>
    {/each}
    {#if data.genres.length > data.genresTop.length}
      <button class="chip more" onclick={() => (genresOpen = !genresOpen)}>
        {genresOpen ? 'mai puține ▴' : `+${data.genres.length - data.genresTop.length} genuri ▾`}
      </button>
    {/if}
  </div>

  <p class="count">
    {data.count} rezultate{#if data.q}&nbsp;pentru „{data.q}"{/if}
  </p>

  {#if !data.list.length}
    <p class="empty">Niciun rezultat. Încearcă alt filtru.</p>
  {:else if data.view === 'list'}
    <div class="listing">
      {#each data.list as a (a.id)}
        <a class="lrow" href={`/anime/${a.id}`}>
          <span class="lthumb" style={a.imageUrl ? `background-image:url(${mediaUrl(a.imageUrl)})` : ''}></span>
          <span class="lmain">
            <span class="lt">{displayName(a)}</span>
            <span class="lm">{metaOf(a)}</span>
          </span>
          <span class="ltags">
            {#each (a.genres ?? []).slice(0, 3) as g}
              <span class="ltag">{g}</span>
            {/each}
          </span>
          {#if a.score}<span class="lscore">★ {a.score.toFixed(2)}</span>{/if}
        </a>
      {/each}
    </div>
  {:else}
    <div class="grid">
      {#each data.list as a (a.id)}
        <PosterCard {a} />
      {/each}
    </div>
  {/if}

  {#if data.pages > 1}
    <nav class="pager">
      {#if data.page > 1}
        <a class="pill" href={hrefWith({ page: String(data.page - 1) })}>← Anterior</a>
      {/if}
      <PagePicker page={data.page} pages={data.pages} hrefFor={(n) => hrefWith({ page: String(n) })} />
      {#if data.page < data.pages}
        <a class="pill" href={hrefWith({ page: String(data.page + 1) })}>Următor →</a>
      {/if}
    </nav>
  {/if}
</div>

<style>
  .browse { padding-block: var(--space-6) var(--space-8); }

  /* featured banner — blurred backdrop + full contained poster */
  .feat {
    position: relative; overflow: hidden;
    display: flex; align-items: center; gap: var(--space-5);
    margin: 0 0 28px;
    background: var(--surface-raised);
    border: 1px solid var(--border-subtle); border-radius: var(--radius-xl);
    transition: border-color var(--motion-base) var(--ease);
  }
  .feat:hover { border-color: var(--border-default); }
  .feat-blur {
    position: absolute; inset: 0;
    background-size: cover; background-position: center 30%;
    filter: blur(40px) saturate(0.9); opacity: 0.3; transform: scale(1.3);
  }
  .feat-fade {
    position: absolute; inset: 0;
    background: linear-gradient(
      90deg,
      color-mix(in srgb, var(--surface-base) 82%, transparent),
      color-mix(in srgb, var(--surface-base) 35%, transparent) 55%,
      transparent
    );
  }
  .feat-body {
    position: relative; flex: 1; min-width: 0;
    display: flex; flex-direction: column; align-items: flex-start;
    padding: clamp(var(--space-5), 3vw, 34px); max-width: 560px;
  }
  .feat-kicker {
    font-family: var(--font-mono); font-size: var(--fs-micro); letter-spacing: 0.14em;
    text-transform: uppercase; color: var(--accent);
  }
  .feat-title {
    font-family: var(--font-display); font-weight: var(--fw-semibold);
    font-size: clamp(1.5rem, 1.2rem + 1.2vw, 2rem); color: var(--text-primary); line-height: 1.05;
    margin: 12px 0 6px;
  }
  .feat-meta { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .feat-syn {
    font-size: var(--fs-small); line-height: 1.6; color: var(--text-muted);
    margin: 14px 0 20px;
    display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden;
  }
  .feat-poster {
    position: relative; flex: 0 0 auto;
    padding: var(--space-5) clamp(var(--space-5), 4vw, 44px) var(--space-5) 0;
  }
  .feat-poster img {
    width: clamp(120px, 14vw, 158px); aspect-ratio: 2 / 3; object-fit: cover;
    border-radius: var(--radius-md); border: 1px solid var(--border-default);
    box-shadow: 0 18px 40px rgba(0, 0, 0, 0.4);
  }
  .btn.fill {
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 12px 22px; border-radius: var(--radius-md);
    background: var(--accent); color: var(--on-accent);
  }
  .feat:hover .btn.fill { background: var(--accent-hover); }
  @media (max-width: 560px) {
    .feat-poster { display: none; }
  }

  .top {
    display: flex; align-items: flex-end; justify-content: space-between;
    flex-wrap: wrap; gap: var(--space-4); margin-bottom: var(--space-5);
    padding-bottom: 18px; border-bottom: 2px solid var(--text-primary);
  }
  .cat-kicker { font-size: var(--fs-caption); font-weight: var(--fw-bold); color: var(--accent); }
  .top h1 { font-size: clamp(2rem, 1.6rem + 1.8vw, 2.625rem); letter-spacing: -0.02em; line-height: 1; margin-top: 10px; }

  .controls { display: flex; align-items: center; gap: var(--space-4); flex-wrap: wrap; }
  .control { display: flex; align-items: center; gap: 8px; }

  .fsel {
    padding: 8px 10px; min-width: 108px; max-width: 190px;
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md); color: var(--text-primary); cursor: pointer;
    font-size: var(--fs-caption); font-family: var(--font-body); outline: none;
  }
  .fsel:hover, .fsel:focus { border-color: var(--accent); }

  .pills {
    display: flex; gap: 2px; padding: 3px;
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
  }
  .pill {
    font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--text-muted);
    padding: 6px 12px; border-radius: 7px; line-height: 1.2;
  }
  .pill:hover { color: var(--text-primary); }
  /* direction toggle: one arrow for whichever sort is active, rotating to
     show which way the list runs */
  .pill.dir {
    font-family: var(--font-mono); padding-inline: 10px;
    border-left: 1px solid var(--border-subtle); border-radius: 0 7px 7px 0;
    transition: transform var(--motion-fast) var(--ease), color var(--motion-fast) var(--ease);
  }
  .pill.dir:hover { color: var(--text-primary); }
  .pill.dir.flip { color: var(--accent); transform: rotate(180deg); }

  .pill.on { background: var(--surface-overlay); color: var(--text-primary); }

  .pager {
    display: flex; align-items: center; justify-content: center; gap: 14px;
    margin-top: var(--space-6);
  }
  .pager .pill { background: var(--surface-raised); border: 1px solid var(--border-subtle); }
  .pager .pill:hover { border-color: var(--accent); }

  .genres { display: flex; gap: 8px; flex-wrap: wrap; }
  .chip {
    font-size: var(--fs-caption); font-weight: var(--fw-medium); color: var(--text-muted);
    padding: 5px 12px; border: 1px solid var(--border-subtle); border-radius: var(--radius-pill);
    transition: color var(--motion-fast) var(--ease), border-color var(--motion-fast) var(--ease);
  }
  .chip:hover { color: var(--text-primary); border-color: var(--border-default); }
  .chip.on {
    color: var(--accent); border-color: color-mix(in srgb, var(--accent) 55%, transparent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }
  .chip.more {
    font-family: var(--font-mono); background: none; cursor: pointer;
    border-style: dashed; color: var(--text-muted);
  }
  .chip.more:hover { color: var(--text-primary); }

  .count {
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
    margin: var(--space-4) 0 var(--space-5);
  }
  .empty { color: var(--text-muted); padding: var(--space-7) 0; text-align: center; }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: var(--space-4);
  }

  /* list view */
  .listing {
    display: flex; flex-direction: column; gap: 1px;
    background: var(--border-subtle);
    border: 1px solid var(--border-subtle); border-radius: var(--radius-lg); overflow: hidden;
  }
  .lrow {
    display: flex; align-items: center; gap: var(--space-4);
    padding: 13px 18px; background: var(--surface-raised);
  }
  .lrow:hover { background: var(--surface-overlay); }
  .lthumb {
    width: 42px; height: 62px; border-radius: var(--radius-sm); flex: 0 0 auto;
    background-color: var(--surface-overlay); background-size: cover; background-position: center;
  }
  .lmain { flex: 1; min-width: 0; display: flex; flex-direction: column; }
  .lt {
    font-family: var(--font-display); font-size: var(--fs-h3); font-weight: var(--fw-semibold);
    color: var(--text-primary); line-height: 1.25;
  }
  .lrow:hover .lt { color: var(--accent); }
  .lm { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); margin-top: 4px; }
  .ltags { display: flex; gap: 6px; flex-wrap: wrap; max-width: 240px; justify-content: flex-end; }
  .ltag {
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
    border: 1px solid var(--border-default); padding: 2px 8px; border-radius: 5px;
  }
  .lscore {
    font-family: var(--font-mono); font-size: var(--fs-small); font-weight: var(--fw-medium);
    color: var(--accent); width: 64px; text-align: right; white-space: nowrap;
  }

  @media (max-width: 720px) {
    .grid { grid-template-columns: repeat(auto-fill, minmax(130px, 1fr)); gap: var(--space-3); }
    .ltags { display: none; }
  }
</style>
