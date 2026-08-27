<script lang="ts">
  import { mediaUrl } from '$lib/media';
  import { authStore } from '$lib/stores/auth';

  let { data } = $props();
  const auth = $derived($authStore);
  const isSelf = $derived(auth.user?.username === data.handle);

  // Pick a rating to see only those titles; pick nothing and sort instead.
  let star = $state<number | null>(null);
  let sort = $state<'high' | 'low' | 'recent'>('high');

  // buckets round to the nearest whole star, so a chip's count and the list it
  // opens can never disagree (older rows can hold a half-star)
  const bucket = (s: number) => Math.round(s);
  const countFor = (n: number) => data.rated.filter((r) => bucket(r.stars) === n).length;

  const stars = (s: number) => '★'.repeat(Math.floor(s)) + (s % 1 >= 0.5 ? '½' : '');
  const empty = (s: number) => '★'.repeat(5 - Math.ceil(s));

  const shown = $derived.by(() => {
    const list = star === null ? [...data.rated] : data.rated.filter((r) => bucket(r.stars) === star);
    return list.sort((a, b) => {
      if (sort === 'recent') return Date.parse(b.ratedAt) - Date.parse(a.ratedAt);
      return sort === 'high' ? b.stars - a.stars : a.stars - b.stars;
    });
  });

  const avg = $derived(
    data.rated.length ? data.rated.reduce((t, r) => t + r.stars, 0) / data.rated.length : 0
  );

  const fmtDate = (d: string) =>
    new Date(d).toLocaleDateString('ro-RO', { day: 'numeric', month: 'long', year: 'numeric' });
</script>

<svelte:head><title>Note · {data.name} · Anime-Kage</title></svelte:head>

<div class="container notepage">
  <header class="top">
    <div>
      <p class="crumb"><a href={`/user/${data.handle}`}>← {data.name}</a></p>
      <h1>{isSelf ? 'Notele mele' : `Notele lui ${data.name}`}</h1>
    </div>
    {#if data.rated.length}
      <span class="count">{data.rated.length} note · media {avg.toFixed(1)}/5</span>
    {/if}
  </header>

  {#if data.rated.length}
    <div class="bar">
      <div class="chips">
        <button class="chip" class:on={star === null} onclick={() => (star = null)}>Toate</button>
        {#each [5, 4, 3, 2, 1] as n (n)}
          {@const c = countFor(n)}
          <button
            class="chip st"
            class:on={star === n}
            disabled={c === 0}
            onclick={() => (star = star === n ? null : n)}
          >
            <span class="sg">{'★'.repeat(n)}</span><span class="cn">{c}</span>
          </button>
        {/each}
      </div>

      <div class="pills" role="group" aria-label="Sortare">
        <button class="pill" class:on={sort === 'high'} onclick={() => (sort = 'high')}>Mari</button>
        <button class="pill" class:on={sort === 'low'} onclick={() => (sort = 'low')}>Mici</button>
        <button class="pill" class:on={sort === 'recent'} onclick={() => (sort = 'recent')}>Recente</button>
      </div>
    </div>

    <div class="rows">
      {#each shown as r (r.key)}
        <article class="row">
          <a
            class="thumb media-tone"
            href={`/${r.kind}/${r.id}`}
            style={r.imageUrl ? `background-image:url(${mediaUrl(r.imageUrl)})` : ''}
            aria-label={r.title}
          ></a>
          <div class="main">
            <a class="t" href={`/${r.kind}/${r.id}`}>
              <em>{r.title}</em>
              {#if r.year}<span class="y">{r.year}</span>{/if}
            </a>
            <p class="m">{r.kind === 'anime' ? 'anime' : 'manga'} · {fmtDate(r.ratedAt)}</p>
          </div>
          <span class="stars" title={`${r.stars} din 5`}>
            {stars(r.stars)}<span class="off">{empty(r.stars)}</span>
          </span>
        </article>
      {/each}
    </div>
  {:else if isSelf}
    <p class="muted">
      Nicio notă încă — dă stele unui titlu din
      <a class="inline-link" href="/istoric">istoricul tău</a> și apare aici.
    </p>
  {:else}
    <p class="muted">{data.name} nu a notat încă niciun titlu.</p>
  {/if}
</div>

<style>
  .notepage { max-width: 760px; padding-block: var(--space-6) var(--space-8); }

  .top {
    display: flex; align-items: flex-end; justify-content: space-between;
    flex-wrap: wrap; gap: var(--space-4);
    padding-bottom: 18px; border-bottom: 2px solid var(--text-primary);
  }
  .crumb { font-family: var(--font-mono); font-size: var(--fs-caption); }
  .crumb a { color: var(--text-muted); }
  .crumb a:hover { color: var(--text-primary); }
  .top h1 { font-size: clamp(1.5rem, 1.2rem + 1.2vw, 1.9rem); letter-spacing: -0.015em; margin-top: 8px; }
  .count { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }

  .muted { padding-top: var(--space-5); color: var(--text-muted); font-size: var(--fs-small); }
  .inline-link { color: var(--accent); font-weight: var(--fw-semibold); }

  .bar {
    display: flex; align-items: center; justify-content: space-between;
    gap: var(--space-4); flex-wrap: wrap; padding: var(--space-4) 0;
  }
  .chips { display: flex; gap: 8px; flex-wrap: wrap; }
  .chip {
    font-size: var(--fs-caption); font-weight: var(--fw-medium); color: var(--text-muted);
    padding: 5px 12px; border: 1px solid var(--border-subtle); border-radius: var(--radius-pill);
    background: none; cursor: pointer;
  }
  .chip:hover:not(:disabled) { color: var(--text-primary); }
  .chip.on {
    color: var(--accent); border-color: color-mix(in srgb, var(--accent) 55%, transparent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }
  .chip:disabled { opacity: 0.35; cursor: default; }
  .chip.st { display: inline-flex; align-items: baseline; gap: 7px; }
  .sg { letter-spacing: 1px; }
  .cn { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-faint); }
  .chip.on .cn { color: var(--accent); }

  .pills {
    display: flex; gap: 2px; padding: 3px;
    background: var(--surface-raised); border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
  }
  .pill {
    font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--text-muted);
    padding: 6px 14px; border-radius: 7px; border: none; background: none; cursor: pointer;
  }
  .pill.on { background: var(--surface-overlay); color: var(--text-primary); }

  /* flat hairline rows, same as the reviews page */
  .rows { display: flex; flex-direction: column; }
  .row {
    display: flex; align-items: center; gap: 18px;
    padding: 16px 0; border-bottom: 1px solid var(--border-subtle);
  }
  .thumb {
    width: 46px; height: 68px; border-radius: 6px; flex: 0 0 auto;
    background-color: var(--surface-overlay); background-size: cover; background-position: center;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
  }
  .main { flex: 1; min-width: 0; }
  .t em {
    font-family: var(--font-display); font-size: var(--fs-h3);
    font-weight: var(--fw-semibold); font-style: italic; color: var(--text-primary);
  }
  .t:hover em { color: var(--accent); }
  .y { margin-left: 8px; font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .m { margin-top: 6px; font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .stars { flex: 0 0 auto; color: var(--accent); font-size: 0.9375rem; letter-spacing: 2px; white-space: nowrap; }
  .off { color: var(--surface-overlay); }

  @media (max-width: 560px) {
    .bar { align-items: flex-start; flex-direction: column; }
    .stars { font-size: 0.8125rem; letter-spacing: 1px; }
  }
</style>
