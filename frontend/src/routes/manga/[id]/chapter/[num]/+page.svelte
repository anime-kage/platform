<script lang="ts">
  import { mediaUrl } from '$lib/media';
  import { goto } from '$app/navigation';
  import CommentSection from '$lib/components/CommentSection.svelte';
  import { api } from '$lib/api';
  import { displayName, titleRef } from '$lib/types';
  import { sourceName } from '$lib/providers';
  import { nameHue } from '$lib/avatar';
  import type { ChapterPages, ReleaseCredits } from '$shared/types';

  let { data } = $props();
  const m = $derived(data.manga);
  const ch = $derived(data.chapter);

  const sources = $derived((ch.links ?? []).filter((l) => l.isActive !== false));
  // Was hard-coded to sources[0], so a chapter with a Google Drive and a Scribd
  // copy could only ever show the Drive one. Same picker as the episode player.
  let sourceIdx = $state(0);
  const current = $derived(sources[sourceIdx] ?? sources[0] ?? null);
  const chapNum = (n: string) => String(parseFloat(n));

  // translator / verifier behind the published RO pages
  let credits = $state<ReleaseCredits | null>(null);
  $effect(() => {
    const chId = ch.id;
    sourceIdx = 0;
    credits = null;
    api
      .getChapterCredits(m.id, chapNum(ch.chapterNumber))
      .then((r) => {
        if (ch.id === chId) credits = r.data;
      })
      .catch(() => {});
  });

  // ── native reader — our pages when we have them, iframe fallback
  let pageSet = $state<ChapterPages | null>(null);
  let pageIdx = $state(0);
  let mode = $state<'paged' | 'vertical'>('paged');
  let lang = $state<string | null>(null); // null = server default (RO first)

  if (typeof localStorage !== 'undefined') {
    const saved = localStorage.getItem('ak-reader-mode');
    if (saved === 'vertical' || saved === 'paged') mode = saved;
  }
  function setMode(next: 'paged' | 'vertical') {
    mode = next;
    try {
      localStorage.setItem('ak-reader-mode', next);
    } catch {
      /* private mode */
    }
  }

  $effect(() => {
    const chId = ch.id;
    const wanted = lang;
    pageSet = null;
    pageIdx = 0;
    api
      .getChapterPages(chId, wanted ?? undefined)
      .then((r) => {
        if (ch.id === chId) pageSet = r.data;
      })
      .catch(() => {
        if (ch.id === chId) pageSet = { language: '', languages: [], pages: [] };
      });
  });

  const pages = $derived(pageSet?.pages ?? []);
  const hasPages = $derived(pages.length > 0);

  // preload the next two pages so paging feels instant
  $effect(() => {
    if (mode !== 'paged') return;
    for (const url of pages.slice(pageIdx + 1, pageIdx + 3)) {
      const img = new Image();
      img.src = url;
    }
  });

  function nextPage() {
    if (pageIdx < pages.length - 1) pageIdx += 1;
    else if (data.next !== null) goto(`/manga/${m.id}/chapter/${data.next}`);
  }
  function prevPage() {
    if (pageIdx > 0) pageIdx -= 1;
    else if (data.prev !== null) goto(`/manga/${m.id}/chapter/${data.prev}`);
  }
  function onKey(e: KeyboardEvent) {
    if (!hasPages || mode !== 'paged') return;
    const tag = (e.target as HTMLElement)?.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
    if (e.key === 'ArrowRight') nextPage();
    if (e.key === 'ArrowLeft') prevPage();
  }

  function onSelect(e: Event) {
    goto(`/manga/${m.id}/chapter/${(e.currentTarget as HTMLSelectElement).value}`);
  }
  function onPageSelect(e: Event) {
    pageIdx = Number((e.currentTarget as HTMLSelectElement).value);
  }
</script>

<svelte:window onkeydown={onKey} />

<svelte:head><title>Cap. {chapNum(ch.chapterNumber)} · {displayName(m)} · Anime-Kage</title></svelte:head>

<!-- sticky reader bar -->
<div class="subbar">
  <div class="container subbar-in">
    <a class="back" href={`/manga/${titleRef(m)}`}>← Bibliotecă</a>
    <span class="vr"></span>
    <span class="show-title">{displayName(m)}</span>
    <span class="spacer"></span>

    {#if hasPages}
      {#if (pageSet?.languages.length ?? 0) > 1}
        <div class="seg" role="group" aria-label="Ediție">
          {#each pageSet?.languages ?? [] as l (l)}
            <button class="seg-btn" class:on={pageSet?.language === l} onclick={() => (lang = l)}>
              {l.toUpperCase()}
            </button>
          {/each}
        </div>
      {/if}
      <div class="seg" role="group" aria-label="Mod de citire">
        <button class="seg-btn" class:on={mode === 'paged'} onclick={() => setMode('paged')} title="Pagină cu pagină">▭</button>
        <button class="seg-btn" class:on={mode === 'vertical'} onclick={() => setMode('vertical')} title="Derulare verticală">≣</button>
      </div>
    {/if}

    <select class="ch-select" value={chapNum(ch.chapterNumber)} onchange={onSelect}>
      {#each data.chapters as c (c.id)}
        <option value={chapNum(c.chapterNumber)}>
          Cap. {chapNum(c.chapterNumber)}{c.title ? ` — ${c.title}` : ''}
        </option>
      {/each}
    </select>
  </div>
</div>

<div class="container reader" class:wide={hasPages && mode === 'vertical'}>
  {#if hasPages}
    {#if mode === 'paged'}
      <div class="pageview">
        <img
          class="page-img"
          src={pages[pageIdx]}
          alt={`Pagina ${pageIdx + 1}`}
          referrerpolicy="no-referrer"
        />
        <!-- click zones: left half back, right half forward -->
        <button class="zone left" aria-label="Pagina anterioară" onclick={prevPage}></button>
        <button class="zone right" aria-label="Pagina următoare" onclick={nextPage}></button>
      </div>
      <div class="pagebar">
        <button class="btn ghost sm" onclick={prevPage} disabled={pageIdx === 0 && data.prev === null}>←</button>
        <select class="ch-select" value={String(pageIdx)} onchange={onPageSelect} aria-label="Sari la pagina">
          {#each pages as _, i (i)}
            <option value={String(i)}>{i + 1} / {pages.length}</option>
          {/each}
        </select>
        <button class="btn ghost sm" onclick={nextPage} disabled={pageIdx === pages.length - 1 && data.next === null}>→</button>
      </div>
    {:else}
      <div class="strip">
        {#each pages as url, i (i)}
          <img class="strip-img" src={url} alt={`Pagina ${i + 1}`} loading="lazy" referrerpolicy="no-referrer" />
        {/each}
      </div>
    {/if}
  {:else if pageSet === null}
    <div class="page-ph"><div class="ph-in"><span class="ph-cap">Se încarcă…</span></div></div>
  {:else if current}
    <!-- no own pages: third-party iframe, as before -->
    <div class="frame">
      <iframe
        src={current.hostingUrl}
        title={`${displayName(m)} — cap. ${chapNum(ch.chapterNumber)}`}
        sandbox="allow-scripts allow-same-origin allow-presentation"
        allowfullscreen
        referrerpolicy="no-referrer"
      ></iframe>
    </div>
  {:else}
    <div class="page-ph">
      {#if m.imageUrl}
        <div class="ph-bg" style={`background-image:url(${mediaUrl(m.imageUrl)})`}></div>
      {/if}
      <div class="ph-in">
        <span class="ph-mark">影</span>
        <span class="ph-cap">Cap. {chapNum(ch.chapterNumber)}{ch.title ? ` · ${ch.title}` : ''}</span>
        <p class="ph-msg">Nicio sursă disponibilă încă pentru acest capitol.</p>
      </div>
    </div>
  {/if}

  <!-- Source picker. Only for the iframe path: when we have our own pages
       there is nothing to switch between, and only when there is more than one
       source, since a single pill is just a label. -->
  {#if sources.length > 1 && !hasPages}
    <div class="srcrow" role="group" aria-label="Surse">
      <span class="kicker srcrow-label">Surse</span>
      {#each sources as s, i (s.id)}
        <button
          class="srcpill"
          class:on={i === sourceIdx}
          aria-pressed={i === sourceIdx}
          onclick={() => (sourceIdx = i)}
        >
          {sourceName(s, i)}
        </button>
      {/each}
    </div>
  {/if}

  <div class="nav">
    {#if data.prev !== null}
      <a class="btn ghost" href={`/manga/${titleRef(m)}/chapter/${data.prev}`}>← Înapoi</a>
    {:else}
      <span></span>
    {/if}
    <span class="hint">{hasPages ? `${pages.length} pagini · ${pageSet?.language.toUpperCase()}` : ch.pages ? `${ch.pages} pagini` : ''}</span>
    {#if data.next !== null}
      <a class="btn fill" href={`/manga/${titleRef(m)}/chapter/${data.next}`}>Continuă →</a>
    {:else}
      <span></span>
    {/if}
  </div>

  {#if credits && (credits.translator || credits.verifier)}
    <div class="credits">
      {#if credits.translator}
        {@const t = credits.translator}
        <a class="credit" href={`/user/${t.username}`} title="Traducător">
          {#if t.avatarUrl}
            <span class="cr-av"><img src={api.resolveUrl(t.avatarUrl)} alt={t.username} /></span>
          {:else}
            <span class="cr-av monogram" style={`--mg-hue:${nameHue(t.username)}`}>{t.username.charAt(0).toUpperCase()}</span>
          {/if}
          <span class="cr-txt"><span class="cr-role">Tradus de</span><span class="cr-name">{t.username}</span></span>
        </a>
      {/if}
      {#if credits.verifier}
        {@const v = credits.verifier}
        <a class="credit" href={`/user/${v.username}`} title="Verificator">
          {#if v.avatarUrl}
            <span class="cr-av"><img src={api.resolveUrl(v.avatarUrl)} alt={v.username} /></span>
          {:else}
            <span class="cr-av monogram" style={`--mg-hue:${nameHue(v.username)}`}>{v.username.charAt(0).toUpperCase()}</span>
          {/if}
          <span class="cr-txt"><span class="cr-role">Verificat de</span><span class="cr-name">{v.username}</span></span>
        </a>
      {/if}
    </div>
  {/if}

  <!-- per-chapter comments -->
  <div class="ch-comments">
    {#key ch.id}
      <CommentSection mangaId={m.id} chapterId={ch.id} heading={`Comentarii · Capitolul ${chapNum(ch.chapterNumber)}`} />
    {/key}
  </div>
</div>

<style>
  /* source picker — same language as the episode player's row */
  .srcrow { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; margin-top: var(--space-4); }
  .srcrow-label { margin-right: 2px; }
  .srcpill {
    font-size: var(--fs-micro); color: var(--text-muted);
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
    border-radius: 999px; padding: 5px 12px; cursor: pointer; white-space: nowrap;
    transition: color 120ms ease, background 120ms ease, border-color 120ms ease;
  }
  .srcpill:hover { color: var(--text-primary); border-color: var(--border-default); }
  .srcpill:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 2px; }
  .srcpill.on {
    background: var(--accent); border-color: var(--accent);
    color: var(--on-accent); font-weight: var(--fw-semibold);
  }
  @media (pointer: coarse) {
    .srcpill { display: inline-flex; align-items: center; min-height: var(--tap-min); padding-block: 0; }
  }

  .subbar {
    position: sticky; top: 62px; z-index: 20;
    background: color-mix(in srgb, var(--surface-raised) 88%, transparent);
    backdrop-filter: blur(10px); -webkit-backdrop-filter: blur(10px);
    border-bottom: 1px solid var(--border-subtle);
  }
  .subbar-in { display: flex; align-items: center; gap: 14px; padding-block: 12px; }
  .back { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .back:hover { color: var(--text-primary); }
  .vr { width: 1px; height: 20px; background: var(--border-default); }
  .show-title {
    font-family: var(--font-display); font-size: var(--fs-body); font-weight: var(--fw-semibold);
    min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .spacer { flex: 1; }
  .ch-comments { margin-top: var(--space-7); max-width: 780px; margin-inline: auto; }

  /* translator / verifier credits */
  .credits { display: flex; flex-wrap: wrap; justify-content: center; gap: 10px; margin-top: 18px; }
  .credit {
    display: flex; align-items: center; gap: 9px; padding: 6px 12px 6px 6px;
    border: 1px solid var(--border-subtle); border-radius: var(--radius-pill);
    background: var(--surface-raised); color: inherit;
  }
  .credit:hover { border-color: color-mix(in srgb, var(--accent) 45%, transparent); background: var(--surface-overlay); }
  .cr-av {
    width: 28px; height: 28px; border-radius: 50%; flex: 0 0 auto; overflow: hidden;
    display: grid; place-items: center; font-size: var(--fs-micro); font-weight: var(--fw-bold); color: #fff;
  }
  .cr-av img { width: 100%; height: 100%; object-fit: cover; }
  .cr-txt { display: flex; flex-direction: column; line-height: 1.15; }
  .cr-role {
    font-family: var(--font-mono); font-size: var(--fs-micro); letter-spacing: 0.05em;
    text-transform: uppercase; color: var(--text-muted);
  }
  .cr-name { font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--text-primary); }
  .credit:hover .cr-name { color: var(--accent); }
  .ch-select {
    background: var(--surface-overlay); border: 1px solid var(--border-default);
    border-radius: var(--radius-sm); padding: 8px 12px; color: var(--text-primary);
    font-size: var(--fs-small); cursor: pointer; outline: none; max-width: 260px;
  }
  .ch-select:focus { border-color: var(--accent); }

  .seg {
    display: flex; gap: 2px; padding: 2px;
    background: var(--surface-overlay); border: 1px solid var(--border-default); border-radius: var(--radius-sm);
  }
  .seg-btn {
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
    padding: 5px 10px; border: none; background: none; border-radius: 5px; cursor: pointer;
  }
  .seg-btn.on { background: var(--surface-raised); color: var(--accent); }

  .reader { max-width: 760px; padding-block: var(--space-5) var(--space-7); }
  .reader.wide { max-width: 900px; }

  /* ── paged mode ── */
  .pageview { position: relative; border-radius: var(--radius-sm); overflow: hidden; background: #05070a;
    box-shadow: 0 20px 50px rgba(0, 0, 0, 0.6); }
  .page-img { display: block; width: 100%; height: auto; min-height: 300px; }
  .zone { position: absolute; top: 0; bottom: 0; width: 50%; border: none; background: none; cursor: pointer; }
  .zone.left { left: 0; cursor: w-resize; }
  .zone.right { right: 0; cursor: e-resize; }
  .pagebar { display: flex; align-items: center; justify-content: center; gap: 10px; margin-top: 12px; }

  /* ── vertical mode ── */
  .strip { display: flex; flex-direction: column; }
  .strip-img { display: block; width: 100%; height: auto; }

  .frame {
    aspect-ratio: 2 / 3; border-radius: var(--radius-sm); overflow: hidden;
    box-shadow: 0 20px 50px rgba(0, 0, 0, 0.6); background: #05070a;
  }
  .frame iframe { width: 100%; height: 100%; border: 0; }

  .page-ph {
    position: relative; aspect-ratio: 2 / 3; border-radius: var(--radius-sm); overflow: hidden;
    box-shadow: 0 20px 50px rgba(0, 0, 0, 0.6); background: var(--surface-raised);
  }
  .ph-bg {
    position: absolute; inset: 0; background-size: cover; background-position: center;
    filter: blur(26px) brightness(0.4); transform: scale(1.15);
  }
  .ph-in {
    position: absolute; inset: 0; display: flex; flex-direction: column;
    align-items: center; justify-content: center; gap: 14px; text-align: center; padding: var(--space-5);
  }
  .ph-mark { font-family: var(--font-display); font-size: 5rem; color: color-mix(in srgb, var(--text-primary) 16%, transparent); }
  .ph-cap { font-family: var(--font-mono); font-size: var(--fs-caption); letter-spacing: 0.1em; color: var(--text-muted); }
  .ph-msg { color: var(--text-muted); font-size: var(--fs-small); max-width: 36ch; }

  .nav { display: flex; align-items: center; justify-content: space-between; margin-top: 18px; gap: var(--space-3); }
  .hint { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 11px 20px; border-radius: var(--radius-md); white-space: nowrap; cursor: pointer;
  }
  .btn.sm { padding: 8px 14px; }
  .btn.ghost { border: 1px solid var(--border-default); color: var(--text-primary); background: transparent; }
  .btn.ghost:hover { background: var(--surface-raised); color: var(--text-primary); }
  .btn.ghost:disabled { opacity: 0.4; cursor: default; }
  .btn.fill { background: var(--accent); color: var(--on-accent); }
  .btn.fill:hover { background: var(--accent-hover); color: var(--on-accent); }

  @media (max-width: 640px) {
    .show-title { display: none; }
    .subbar .vr { display: none; }
  }
</style>
