<script lang="ts">
  import { mediaUrl } from '$lib/media';
  import { displayName } from '$lib/types';

  let { data } = $props();
  const def = $derived(data.def);
  const base = $derived(def.kind === 'anime' ? '/anime' : '/manga');

  const pageHref = (p: number) => `?page=${p}`;

  const metaOf = (a: (typeof data.items)[number]['a']) => {
    const count =
      'episodes' in a && a.episodes ? `${a.episodes} ep` : 'chapters' in a && a.chapters ? `${a.chapters} cap` : null;
    return [a.year, (('type' in a && a.type) || (def.kind === 'anime' ? 'TV' : 'Manga')).toUpperCase(), count]
      .filter(Boolean)
      .join(' · ');
  };
</script>

<svelte:head><title>{def.title} · Anime-Kage</title></svelte:head>

<div class="container detail">
  <a class="back" href="/liste">← Toate listele</a>

  <header class="hero">
    <div class="covers" aria-hidden="true">
      {#each data.covers as cv, i (i)}
        <span class="cover media-tone" style={`background-image:url(${mediaUrl(cv)})`}></span>
      {/each}
      <span class="cfade"></span>
    </div>
    <div class="hbody">
      <span class="flag"><span class="dot"></span> De la Anime-Kage</span>
      <h1>{def.title}</h1>
      <p class="desc">{def.desc}</p>
      <p class="stat">{data.total} titluri</p>
    </div>
  </header>

  <ol class="rows" start={(data.page - 1) * 20 + 1}>
    {#each data.items as { a, rank } (a.id)}
      <li>
        <a class="row" href={`${base}/${a.id}`}>
          <span class="rank" class:top={rank <= 3}>{rank}</span>
          <span class="thumb media-tone" style={`background-image:url(${mediaUrl(a.imageUrl)})`}></span>
          <span class="main">
            <span class="t">{displayName(a)}</span>
            <span class="m">{metaOf(a)}</span>
          </span>
          {#if a.score}
            <span class="score"><span class="star">★</span>{a.score.toFixed(2)}</span>
          {/if}
        </a>
      </li>
    {/each}
  </ol>

  {#if data.pages > 1}
    <nav class="pager">
      {#if data.page > 1}
        <a class="pg-btn" href={pageHref(data.page - 1)}>← Anterior</a>
      {:else}
        <span class="pg-btn off">← Anterior</span>
      {/if}
      <span class="pg-info">Pagina {data.page} din {data.pages}</span>
      {#if data.page < data.pages}
        <a class="pg-btn" href={pageHref(data.page + 1)}>Următor →</a>
      {:else}
        <span class="pg-btn off">Următor →</span>
      {/if}
    </nav>
  {/if}
</div>

<style>
  .detail { padding-block: var(--space-6) var(--space-8); max-width: 900px; }
  .back { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .back:hover { color: var(--accent); }

  .hero {
    position: relative; overflow: hidden; margin: 18px 0 30px;
    border: 1px solid var(--border-subtle); border-radius: 16px; background: var(--surface-raised);
  }
  .covers { position: relative; display: flex; height: 150px; }
  .cover { flex: 1; background-color: var(--surface-overlay); background-size: cover; background-position: center 22%; }
  .cfade { position: absolute; inset: 0; background: linear-gradient(180deg, transparent 20%, var(--surface-raised)); }
  .hbody { position: relative; padding: 6px 26px 24px; margin-top: -34px; }
  .flag {
    display: inline-flex; align-items: center; gap: 7px;
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--accent); margin-bottom: 10px;
  }
  .dot { width: 5px; height: 5px; border-radius: 50%; background: var(--accent); }
  .hero h1 { font-size: clamp(1.6rem, 1.3rem + 1.4vw, 2.125rem); letter-spacing: -0.01em; line-height: 1.1; }
  .desc { font-size: 0.9375rem; color: var(--text-muted); margin: 10px 0 12px; max-width: 560px; }
  .stat { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }

  .rows { list-style: none; }
  .rows li { border-bottom: 1px solid var(--border-subtle); }
  .row { display: flex; align-items: center; gap: 16px; padding: 11px 8px; }
  .row:hover { background: var(--surface-raised); }
  .rank {
    flex: 0 0 34px; text-align: center;
    font-family: var(--font-display); font-size: 1.125rem; font-weight: var(--fw-semibold);
    color: var(--text-muted);
  }
  .rank.top { color: var(--accent); }
  .thumb {
    flex: 0 0 auto; width: 40px; height: 58px; border-radius: 6px;
    background-color: var(--surface-overlay); background-size: cover; background-position: center;
  }
  .main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
  .t {
    font-size: var(--fs-body); font-weight: var(--fw-semibold); color: var(--text-primary);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .row:hover .t { color: var(--accent); }
  .m { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .score {
    flex: 0 0 auto; display: flex; align-items: center; gap: 4px;
    font-family: var(--font-mono); font-size: var(--fs-caption); font-weight: var(--fw-medium); color: var(--accent);
  }
  .star { font-size: 0.625rem; }

  .pager { display: flex; align-items: center; justify-content: center; gap: 16px; margin-top: 30px; }
  .pg-btn {
    font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--text-primary);
    padding: 8px 16px; border-radius: 9px;
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
  }
  .pg-btn:hover { border-color: var(--accent); }
  .pg-btn.off { opacity: 0.4; }
  .pg-info { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }

  @media (max-width: 620px) {
    .covers { height: 120px; }
  }
</style>
