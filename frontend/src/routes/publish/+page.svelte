<script lang="ts">
  import { onDestroy } from 'svelte';
  import { mediaUrl } from '$lib/media';
  import { goto } from '$app/navigation';
  import { api } from '$lib/api';
  import { authStore as auth } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import { reltime } from '$lib/reltime';
  import Person from '$lib/components/Person.svelte';
  import type { Anime, Manga, MalSearchHit, Release } from '$shared/types';

  // The publish gate: everything verifiers approved, waiting for
  // a coordinator to confirm the series/episode mapping, attach the source
  // and push it live. This is also where proposed-title releases get their
  // real catalog entry (MAL import) — translators can't import titles.
  // Manga releases (4.6) publish to a chapter instead: same card, no sources
  // — the pages themselves go live.

  let releases = $state<Release[]>([]);
  let loading = $state(true);

  const allowed = $derived(
    $auth.isAuthenticated && ['coordinator', 'admin'].includes($auth.user?.role ?? '')
  );
  const roleLabel = $derived($auth.user?.role === 'admin' ? 'ești administrator' : 'ești coordonator');

  $effect(() => {
    if ($auth.isLoading) return;
    if (!$auth.isAuthenticated) goto('/login?redirect=/publish');
  });

  let loaded = false;
  $effect(() => {
    if (!allowed || loaded) return;
    loaded = true;
    load();
  });

  async function load() {
    loading = true;
    try {
      const data = (await api.getReleases({ all: true })).data;
      // seed the per-card working state here — the template only reads it
      // (mutating $state from a template expression is forbidden in Svelte 5)
      for (const r of data) if (r.state === 'approved') card(r);
      releases = data;
      // Seed hardsub state so a burn started before this page was opened (or in
      // another tab) shows as in progress rather than as an untouched button.
      await refreshHardsub(
        data.filter((r) => r.state === 'approved' && r.medium !== 'manga').map((r) => r.id)
      );
      startPolling();
    } catch {
      releases = [];
    } finally {
      loading = false;
    }
  }

  const queue = $derived(releases.filter((r) => r.state === 'approved').slice().reverse());
  const orphans = $derived(queue.filter((r) => !r.animeId && !r.mangaId));
  const published = $derived(releases.filter((r) => r.state === 'published'));
  const week = 7 * 24 * 3_600_000;
  const publishedWeek = $derived(
    published.filter((r) => Date.now() - new Date(r.updatedAt).getTime() < week).length
  );

  // api.request throws a plain ApiError object, not an Error instance
  const errMsg = (err: unknown, fallback: string) =>
    (err as { error?: string }).error ?? (err as { message?: string }).message ?? fallback;

  // ── optional hardsub ────────────────────────────────────────────
  // Burning the RO track into the picture, for hosts we can only embed. Opt-in:
  // everything below the download row works identically whether or not a burn
  // has run, and publishing never waits on one.
  type HardsubView = {
    state: 'idle' | 'queued' | 'running' | 'done' | 'failed';
    progress: number;
    position: number;
    error?: string;
    ready: boolean;
  };
  let hardsub = $state<Record<number, HardsubView>>({});
  let hardsubBusy = $state<number | null>(null);

  // One card open at a time, none by default: a queue of two used to be a
  // full page of form before you had decided which one you were publishing.
  let openCard = $state<number | null>(null);
  // Paginate rather than grow without bound — the queue is the one list here
  // that can get long.
  const perPage = 8;
  let page = $state(1);
  const pageCount = $derived(Math.max(1, Math.ceil(queue.length / perPage)));
  const pageItems = $derived(queue.slice((page - 1) * perPage, page * perPage));

  // Discard a release outright.
  //
  // The publish queue is the only place this can happen to an *approved* release:
  // a translator may only delete their own unfinished work, so a test upload or a
  // duplicate would otherwise sit in this queue for ever. The server deletes the
  // row, the staging directory and the video object in R2 together.
  let deleting = $state<number | null>(null);
  async function remove(rel: Release) {
    const confirmed = confirm(
      `Ștergi definitiv „${seriesName(rel)}” — ${numLabel(rel)}?\n\n` +
        'Traducerea, videoul și fișierele din staging dispar. Nu se poate anula.'
    );
    if (!confirmed) return;
    deleting = rel.id;
    try {
      await api.deleteRelease(rel.id);
      releases = releases.filter((r) => r.id !== rel.id);
      if (openCard === rel.id) openCard = null;
      // deleting the last card on the last page would strand us past the end
      if (page > pageCount) page = pageCount;
      toast.success('Release șters.');
    } catch (err) {
      toast.error(errMsg(err, 'Ștergerea a eșuat.'));
    } finally {
      deleting = null;
    }
  }

  async function stopHardsub(rel: Release) {
    hardsubBusy = rel.id;
    try {
      await api.stopHardsub(rel.id);
      toast.info('Embed oprit.');
      await refreshHardsub([rel.id]);
    } catch (err) {
      toast.error(errMsg(err, 'Nu am putut opri embedul.'));
    } finally {
      hardsubBusy = null;
    }
  }

  // One shared poller rather than one per card: a burn takes ~13 minutes, and a
  // timer per visible release would be a dozen requests a second doing nothing.
  let poller: ReturnType<typeof setInterval> | null = null;

  async function refreshHardsub(ids: number[]) {
    for (const id of ids) {
      try {
        hardsub[id] = (await api.getHardsubStatus(id)).data;
      } catch {
        /* a card that can't report is left as it was */
      }
    }
  }

  function activeIds() {
    return queue.filter((r) => r.medium !== 'manga').map((r) => r.id);
  }

  function startPolling() {
    if (poller) return;
    poller = setInterval(() => {
      // Only poll while something is actually moving; otherwise the interval
      // just sits there and the next click re-arms it.
      const live = Object.entries(hardsub)
        .filter(([, v]) => v.state === 'queued' || v.state === 'running')
        .map(([k]) => Number(k));
      if (live.length === 0) {
        clearInterval(poller!);
        poller = null;
        return;
      }
      refreshHardsub(live);
    }, 4000);
  }

  onDestroy(() => {
    if (poller) clearInterval(poller);
  });

  async function startHardsub(rel: Release) {
    hardsubBusy = rel.id;
    try {
      await api.queueHardsub(rel.id);
      toast.success('Embed pus în coadă.');
      await refreshHardsub([rel.id]);
      startPolling();
    } catch (err) {
      toast.error(errMsg(err, 'Nu am putut porni embedul.'));
    } finally {
      hardsubBusy = null;
    }
  }

  const poster = (r: Release) => {
    const img = r.animeImage ?? r.mangaImage;
    if (img)
      return `background-image: url('${img}'); background-size: cover; background-position: center`;
    const hue = ((r.animeId ?? r.mangaId ?? r.id * 13) * 47) % 360;
    return `background: linear-gradient(158deg, oklch(0.5 0.08 ${hue}) 0%, oklch(0.3 0.06 ${hue}) 46%, oklch(0.16 0.03 ${hue}) 100%)`;
  };
  const seriesName = (r: Release) => r.animeTitle ?? r.mangaTitle ?? r.proposedTitle ?? '—';
  const numLabel = (r: Release) =>
    r.medium === 'manga' ? `capitolul ${r.chapterNumber}` : `episodul ${r.episodeNumber}`;

  // ── per-card working state, keyed by release id ────────────────────────────
  type SourceRow = { url: string; kind: 'embed' | 'extract' };
  type CardState = {
    // the card follows its release's medium: anime→episode+sources, manga→chapter
    medium: 'anime' | 'manga';
    // confirmed mapping (prefilled from the release, editable)
    seriesId?: number;
    seriesTitle?: string;
    num: number;
    // series picker
    q: string;
    results: (Anime | Manga)[];
    // MAL search for series missing from the catalog — by title, not by ID
    malQ: string;
    malResults: MalSearchHit[];
    malSearching: boolean;
    importing: number | null;
    // manual creation — the fallback when neither Jikan nor AniList answers
    manualOpen: boolean;
    mTitle: string;
    mYear: string;
    mEps: string;
    mSyn: string;
    creating: boolean;
    // sources (anime only, optional, any number) — quality is the host's business
    sources: SourceRow[];
    busy: boolean;
  };
  let cards = $state<Record<number, CardState>>({});

  function card(rel: Release): CardState {
    if (!cards[rel.id]) {
      const manga = rel.medium === 'manga';
      cards[rel.id] = {
        medium: rel.medium,
        seriesId: manga ? rel.mangaId : rel.animeId,
        seriesTitle: manga ? rel.mangaTitle : rel.animeTitle,
        num: (manga ? rel.chapterNumber : rel.episodeNumber) ?? 1,
        q: '',
        results: [],
        malQ: rel.proposedTitle ?? '',
        malResults: [],
        malSearching: false,
        importing: null,
        manualOpen: false,
        mTitle: rel.proposedTitle ?? '',
        mYear: '',
        mEps: '',
        mSyn: '',
        creating: false,
        sources: [{ url: '', kind: 'embed' }],
        busy: false
      };
    }
    return cards[rel.id];
  }

  let searchTimer: ReturnType<typeof setTimeout>;
  function onSeriesQuery(relId: number) {
    const c = cards[relId];
    clearTimeout(searchTimer);
    if (c.q.trim().length < 2) {
      c.results = [];
      return;
    }
    searchTimer = setTimeout(async () => {
      try {
        c.results =
          c.medium === 'manga'
            ? (await api.searchManga(c.q.trim())).data.slice(0, 5)
            : (await api.searchAnime(c.q.trim())).data.slice(0, 5);
      } catch {
        c.results = [];
      }
    }, 250);
  }

  function pickSeries(relId: number, a: Anime | Manga) {
    const c = cards[relId];
    c.seriesId = a.id;
    c.seriesTitle = a.title;
    c.q = '';
    c.results = [];
  }

  async function searchMal(relId: number) {
    const c = cards[relId];
    if (c.malQ.trim().length < 2) return;
    c.malSearching = true;
    try {
      c.malResults =
        c.medium === 'manga'
          ? (await api.malSearchManga(c.malQ.trim())).data
          : (await api.malSearchAnime(c.malQ.trim())).data;
      if (c.malResults.length === 0) toast.info('Niciun rezultat pe MAL.');
    } catch (err) {
      toast.error(errMsg(err, 'Căutarea pe MAL a eșuat.'));
    } finally {
      c.malSearching = false;
    }
  }

  async function importFromMal(relId: number, hit: MalSearchHit) {
    const c = cards[relId];
    c.importing = hit.malId;
    try {
      const r = c.medium === 'manga' ? await api.importManga(hit.malId) : await api.importAnime(hit.malId);
      c.seriesId = r.data.id;
      c.seriesTitle = r.data.title;
      c.malResults = [];
      toast.success(
        r.created === false ? `„${r.data.title}” era deja în catalog.` : `Importat: ${r.data.title}`
      );
    } catch (err) {
      toast.error(errMsg(err, 'Importul a eșuat.'));
    } finally {
      c.importing = null;
    }
  }

  async function createManual(relId: number) {
    const c = cards[relId];
    const title = c.mTitle.trim();
    if (!title) return;
    c.creating = true;
    try {
      const r =
        c.medium === 'manga'
          ? await api.createMangaManual({
              title,
              ...(c.mYear ? { year: Number(c.mYear) } : {}),
              ...(c.mEps ? { chapters: Number(c.mEps) } : {}),
              ...(c.mSyn.trim() ? { synopsisRomanian: c.mSyn.trim() } : {})
            })
          : await api.createAnimeManual({
              title,
              ...(c.mYear ? { year: Number(c.mYear) } : {}),
              ...(c.mEps ? { episodes: Number(c.mEps) } : {}),
              ...(c.mSyn.trim() ? { synopsisRomanian: c.mSyn.trim() } : {})
            });
      c.seriesId = r.data.id;
      c.seriesTitle = r.data.title;
      c.manualOpen = false;
      c.malResults = [];
      toast.success(`Serie creată: ${r.data.title}. Poster + descriere completă — din pagina seriei.`);
    } catch (err) {
      toast.error(errMsg(err, 'Crearea seriei a eșuat.'));
    } finally {
      c.creating = false;
    }
  }

  async function publish(rel: Release) {
    const c = card(rel);
    if (!c.seriesId) {
      toast.error('Alege sau importă seria înainte de publicare.');
      return;
    }
    c.busy = true;
    try {
      const rows = c.sources.filter((s) => s.url.trim());
      const res = await api.publishRelease(
        rel.id,
        c.medium === 'manga'
          ? { mangaId: c.seriesId, chapterNumber: c.num }
          : {
              animeId: c.seriesId,
              episodeNumber: c.num,
              sources: rows.length
                ? rows.map((s) => ({
                    hostingUrl: s.url.trim(),
                    kind: s.kind,
                    // 'direct' extract sources point straight at the file — the URL is the ref
                    ...(s.kind === 'extract' ? { provider: 'direct', providerRef: s.url.trim() } : {})
                  }))
                : undefined
            }
      );
      toast.success(res.message);
      await load();
    } catch (err) {
      toast.error(errMsg(err, 'Publicarea a eșuat.'));
    } finally {
      c.busy = false;
    }
  }

  const steps = [
    'Verificatorul aprobă release-ul — apare aici.',
    'Confirmi seria și episodul; dacă seria lipsește, o cauți pe MAL după titlu și o imporți pe loc.',
    'Atașezi sursele video (oricâte; poți și mai târziu, din Admin → Catalog).',
    'Publici: episodul se creează și subtitrarea RO intră live.'
  ];
</script>

<svelte:head>
  <title>Publicare · Anime-Kage</title>
</svelte:head>

<div class="container page">
  {#if !$auth.isLoading && $auth.isAuthenticated && !allowed}
    <div class="denied">
      <h1>Publicare</h1>
      <p>Pagina e rezervată coordonatorilor și administratorilor.</p>
      <a class="btn ghost" href="/home">Înapoi acasă</a>
    </div>
  {:else if allowed}
    <header class="top">
      <div>
        <p class="pg-kicker">Echipă · {roleLabel}</p>
        <h1>Publicare</h1>
        <p class="sub">
          Release-urile aprobate de verificatori. Confirmi seria și episodul, atașezi sursa,
          apoi le publici — abia atunci ajung pe site.
        </p>
      </div>
    </header>

    <div class="strip" role="list">
      <div class="cell" role="listitem">
        <span class="n accent">{queue.length}</span>
        <span class="l">de publicat</span>
      </div>
      <div class="cell" role="listitem">
        <span class="n" class:warn={orphans.length > 0}>{orphans.length}</span>
        <span class="l">fără serie în catalog</span>
      </div>
      <div class="cell" role="listitem">
        <span class="n">{publishedWeek}</span>
        <span class="l">publicate (7 zile)</span>
      </div>
      <div class="cell" role="listitem">
        <span class="n">{published.length}</span>
        <span class="l">publicate total</span>
      </div>
    </div>

    <div class="cols">
      <div class="main">
        {#if loading}
          <p class="muted">Se încarcă…</p>
        {:else if queue.length === 0}
          <section class="sect">
            <h2 class="s-label">De publicat</h2>
            <div class="empty-hero">
              <h2>Nimic de publicat</h2>
              <p>Release-urile apar aici după ce un verificator le aprobă.</p>
            </div>
          </section>
        {:else}
          <section class="sect">
            <h2 class="s-label">De publicat <span class="s-count">· {queue.length}</span></h2>
            {#each pageItems as rel (rel.id)}
              {@const c = cards[rel.id]}
              {@const open = openCard === rel.id}
              <article class="pub" class:collapsed={!open}>
                <div class="p-head">
                  <span class="poster" style={poster(rel)}></span>
                  <div class="p-main">
                    <span class="p-title">
                      {seriesName(rel)} <span class="dim">— {numLabel(rel)}</span>
                    </span>
                    <span class="p-meta">
                      tradus de <Person name={rel.uploaderName} /> ·
                      {rel.medium === 'manga' ? `${rel.pageCount ?? 0} pagini` : `${rel.doneEvents} replici`}
                      {#if rel.reviewerName}· aprobat de <Person name={rel.reviewerName} />{/if}
                      · {reltime(rel.updatedAt)}
                    </span>
                    {#if !rel.animeId && !rel.mangaId}
                      <span class="pill warn">serie propusă — nu există încă în catalog</span>
                    {/if}
                  </div>
                  <a class="preview" href="/verify/{rel.id}">{rel.medium === 'manga' ? 'vezi paginile →' : 'vezi replicile →'}</a>
                  <!-- Collapsed by design: one open form filled the whole page with
                       two episodes in the queue. -->
                  <button
                    class="p-toggle"
                    type="button"
                    aria-expanded={open}
                    onclick={() => (openCard = open ? null : rel.id)}
                  >{open ? 'Închide ▲' : 'Publică ▼'}</button>
                  <!-- In the head row, so a test upload can be dropped without
                       opening the publish form first. -->
                  <button
                    class="p-del"
                    type="button"
                    title="Șterge release-ul"
                    disabled={deleting === rel.id}
                    onclick={() => remove(rel)}
                  >{deleting === rel.id ? 'se șterge…' : 'șterge'}</button>
                </div>

                {#if open}
                <div class="p-form">
                  <div class="f-row">
                    <div class="field grow searchbox">
                      <span class="f-label">Serie</span>
                      {#if c.seriesId}
                        <div class="chosen">
                          <span class="chosen-t">{c.seriesTitle}</span>
                          <button class="unpick" title="Alege altă serie" onclick={() => { c.seriesId = undefined; c.seriesTitle = undefined; }}>schimbă</button>
                        </div>
                      {:else}
                        <input
                          type="text"
                          placeholder="Caută în catalog…"
                          bind:value={c.q}
                          oninput={() => onSeriesQuery(rel.id)}
                          autocomplete="off"
                        />
                        {#if c.results.length}
                          <ul class="results">
                            {#each c.results as a (a.id)}
                              <li><button type="button" onclick={() => pickSeries(rel.id, a)}>{a.title} {a.year ? `(${a.year})` : ''}</button></li>
                            {/each}
                          </ul>
                        {/if}
                      {/if}
                    </div>
                    <div class="field num">
                      <span class="f-label">{c.medium === 'manga' ? 'Capitol' : 'Episod'}</span>
                      {#if c.medium === 'manga'}
                        <input type="number" min="0.1" step="0.1" bind:value={c.num} />
                      {:else}
                        <input type="number" min="1" bind:value={c.num} />
                      {/if}
                    </div>
                  </div>

                  {#if !c.seriesId}
                    <div class="mal">
                      <div class="f-row">
                        <span class="mal-hint">Nu e în catalog? Caută titlul pe MyAnimeList și importă-l:</span>
                        <input
                          class="mal-q"
                          type="text"
                          placeholder="Titlul seriei…"
                          bind:value={c.malQ}
                          onkeydown={(e) => e.key === 'Enter' && searchMal(rel.id)}
                        />
                        <button class="btn ghost sm" disabled={c.malSearching || c.malQ.trim().length < 2} onclick={() => searchMal(rel.id)}>
                          {c.malSearching ? 'Se caută…' : 'Caută pe MAL'}
                        </button>
                      </div>
                      {#if c.malResults.length}
                        <div class="mal-hits">
                          {#each c.malResults as hit (hit.malId)}
                            <div class="mal-hit">
                              {#if hit.imageUrl}
                                <img class="mh-thumb" src={mediaUrl(hit.imageUrl)} alt="" loading="lazy" />
                              {:else}
                                <span class="mh-thumb"></span>
                              {/if}
                              <span class="mh-main">
                                <span class="mh-title">{hit.title}</span>
                                <span class="mh-meta">
                                  {hit.type}{hit.year ? ` · ${hit.year}` : ''}{hit.episodes
                                    ? ` · ${hit.episodes} ep`
                                    : hit.chapters
                                      ? ` · ${hit.chapters} cap`
                                      : ''} · MAL #{hit.malId}
                                </span>
                              </span>
                              <button class="btn ghost sm" disabled={c.importing !== null} onclick={() => importFromMal(rel.id, hit)}>
                                {c.importing === hit.malId ? 'Se importă…' : 'Importă'}
                              </button>
                            </div>
                          {/each}
                        </div>
                      {/if}

                      <button class="man-toggle" onclick={() => (c.manualOpen = !c.manualOpen)}>
                        {c.manualOpen ? '▾' : '▸'} MAL nu răspunde? Creează seria manual
                      </button>
                      {#if c.manualOpen}
                        <div class="man">
                          <div class="f-row">
                            <div class="field grow">
                              <span class="f-label">Titlu</span>
                              <input type="text" bind:value={c.mTitle} placeholder="Titlul seriei" />
                            </div>
                            <div class="field num">
                              <span class="f-label">An</span>
                              <input type="number" min="1950" max="2100" bind:value={c.mYear} placeholder="—" />
                            </div>
                            <div class="field num">
                              <span class="f-label">{c.medium === 'manga' ? 'Capitole' : 'Episoade'}</span>
                              <input type="number" min="1" bind:value={c.mEps} placeholder="—" />
                            </div>
                          </div>
                          <div class="field">
                            <span class="f-label">Descriere în română <span class="opt">opțional</span></span>
                            <textarea rows="3" bind:value={c.mSyn} placeholder="Poți completa și mai târziu, din pagina seriei…"></textarea>
                          </div>
                          <div class="f-row man-foot">
                            <span class="mal-hint">Fără identitate MAL — se poate lega mai târziu. Posterul îl încarci din pagina seriei.</span>
                            <button class="btn fill sm" disabled={c.creating || !c.mTitle.trim()} onclick={() => createManual(rel.id)}>
                              {c.creating ? 'Se creează…' : 'Creează seria'}
                            </button>
                          </div>
                        </div>
                      {/if}
                    </div>
                  {/if}

                  {#if c.medium === 'anime'}
                  {@const hs = hardsub[rel.id]}
                  <div class="dl">
                    <span class="f-label">1 · Descarcă pentru host</span>
                    <div class="dl-row">
                      <a class="dl-btn primary" href={api.fileUrl(`/api/releases/${rel.id}/download.mp4`)}>⬇ Video pentru host (.mp4)</a>
                      <a class="dl-btn" href={api.fileUrl(`/api/releases/${rel.id}/download`)}>⬇ Video + subtitrare RO (.mkv)</a>
                      <a class="dl-btn" href={api.fileUrl(`/api/releases/${rel.id}/subtitle.srt`)}>⬇ Subtitrare (.srt)</a>
                    </div>

                    <!-- Optional: burn the RO track into the picture. Needed only
                         when the host can do nothing but be embedded — an iframe
                         cannot carry our track, so there the subtitle has to be
                         in the pixels. Publishing does not wait on this.
                         (`hs` is declared on the {#if} above: {@const} has to be
                         the immediate child of a block, not of an element.) -->
                    <div class="hs">
                      {#if !hs || hs.state === 'idle' || hs.state === 'failed'}
                        <button
                          class="dl-btn"
                          type="button"
                          disabled={hardsubBusy === rel.id}
                          onclick={() => startHardsub(rel)}
                        >
                          {hardsubBusy === rel.id ? '…' : '⧉ Embed subtitrarea'}
                        </button>
                        {#if hs?.state === 'failed'}
                          <span class="hs-err">{hs.error ?? 'Embedul a eșuat.'}</span>
                        {/if}
                      {:else if hs.state === 'queued'}
                        <span class="hs-state">
                          În coadă{hs.position > 1 ? ` · locul ${hs.position}` : ''}
                        </span>
                        <button class="dl-btn" type="button" disabled={hardsubBusy === rel.id} onclick={() => stopHardsub(rel)}>
                          {hardsubBusy === rel.id ? '…' : '■ Scoate din coadă'}
                        </button>
                      {:else if hs.state === 'running'}
                        <div class="hs-bar" role="progressbar" aria-valuenow={Math.round(hs.progress * 100)} aria-valuemin="0" aria-valuemax="100">
                          <div class="hs-fill" style="width: {Math.max(2, hs.progress * 100)}%"></div>
                          <span>{Math.round(hs.progress * 100)}%</span>
                        </div>
                        <p class="dl-note">Continuă pe server — poți închide pagina.</p>
                        <button class="dl-btn" type="button" disabled={hardsubBusy === rel.id} onclick={() => stopHardsub(rel)}>
                          {hardsubBusy === rel.id ? '…' : '■ Oprește embedul'}
                        </button>
                      {:else if hs.state === 'done'}
                        <a class="dl-btn primary" href={api.fileUrl(`/api/releases/${rel.id}/download.hardsub.mp4`)}>
                          ⬇ Video cu subtitrare embedată (.mp4)
                        </a>
                        <button class="dl-btn" type="button" disabled={hardsubBusy === rel.id} onclick={() => startHardsub(rel)}>
                          {hardsubBusy === rel.id ? '…' : '↻ Refă embedul'}
                        </button>
                      {/if}
                    </div>
                  </div>
                  <div class="srcs">
                    <span class="f-label">2 · Surse video</span>
                    {#each c.sources as src, i (i)}
                      <div class="f-row src">
                        <div class="field grow">
                          <input type="url" placeholder="https://…" bind:value={src.url} />
                        </div>
                        <div class="field kind">
                          <select bind:value={src.kind}>
                            <option value="embed">embed (iframe)</option>
                            <option value="extract">direct (player propriu)</option>
                          </select>
                        </div>
                        {#if c.sources.length > 1}
                          <button class="rm" title="Scoate sursa" onclick={() => c.sources.splice(i, 1)}>×</button>
                        {/if}
                      </div>
                    {/each}
                    <button class="addsrc" onclick={() => c.sources.push({ url: '', kind: 'embed' })}>+ încă o sursă</button>
                  </div>
                  {/if}

                  <div class="f-actions">
                    <span class="f-note">
                      {c.medium === 'manga'
                        ? 'paginile din staging intră live pe capitol la publicare'
                        : c.sources.some((s) => s.kind === 'extract' && s.url.trim())
                          ? 'sursă directă — subtitrarea RO va merge în player-ul nostru'
                          : 'subtitrarea RO intră live pe episod la publicare'}
                    </span>
                    <button class="btn fill" disabled={c.busy || !c.seriesId} onclick={() => publish(rel)}>
                      {c.busy ? 'Se publică…' : c.medium === 'manga' ? 'Publică capitolul' : 'Publică episodul'}
                    </button>
                  </div>
                </div>
                {/if}
              </article>
            {/each}

            {#if pageCount > 1}
              <nav class="pager" aria-label="Paginare">
                <button class="dl-btn" type="button" disabled={page === 1} onclick={() => (page = page - 1)}>← înapoi</button>
                <span class="pager-n">{page} / {pageCount}</span>
                <button class="dl-btn" type="button" disabled={page === pageCount} onclick={() => (page = page + 1)}>înainte →</button>
              </nav>
            {/if}
          </section>
        {/if}

        {#if published.length > 0}
          <section class="sect">
            <h2 class="s-label">Publicate recent</h2>
            <div class="plain">
              {#each published.slice(0, 6) as rel (rel.id)}
                <div class="prow">
                  <span class="pill ok">publicat</span>
                  <span class="pr-title">{seriesName(rel)} <span class="dim">— {numLabel(rel)}</span></span>
                  <span class="pr-meta">de <Person name={rel.uploaderName} /> · {reltime(rel.updatedAt)}</span>
                  {#if rel.animeId}
                    <a class="pr-link" href="/anime/{rel.animeId}">vezi pe site →</a>
                  {:else if rel.mangaId}
                    <a class="pr-link" href="/manga/{rel.mangaId}">vezi pe site →</a>
                  {/if}
                </div>
                <!-- publishing must not cost you the file: re-uploading to a
                     new host is normal, and the staged video is still here -->
                {#if rel.medium === 'anime' && rel.hasVideo}
                  <!-- Downloads only. Re-embedding belongs on the approved card,
                       before publishing — once it is live the video is on its way
                       out, so offering to spend ten minutes of CPU on a file that
                       is about to be deleted would be offering a trap. -->
                  <div class="prow-dl">
                    <a class="dl-btn sm" href={api.fileUrl(`/api/releases/${rel.id}/download.mp4`)}>⬇ .mp4 pentru host</a>
                    <a class="dl-btn sm" href={api.fileUrl(`/api/releases/${rel.id}/download`)}>⬇ .mkv cu subtitrare</a>
                    <a class="dl-btn sm" href={api.fileUrl(`/api/releases/${rel.id}/subtitle.srt`)}>⬇ .srt</a>
                    <span class="pr-grace">videoul se șterge la scurt timp după publicare · subtitrarea rămâne</span>
                  </div>
                {/if}
              {/each}
            </div>
          </section>
        {/if}
      </div>

      <aside class="rail">
        <section class="r-sect">
          <h2 class="r-label">Cum publici</h2>
          {#each steps as step, i (i)}
            <div class="step">
              <span class="step-n">{String(i + 1).padStart(2, '0')}</span>
              <span class="step-t">{step}</span>
            </div>
          {/each}
          <p class="r-note">
            Traducătorii nu importă titluri — tu legi release-ul de seria corectă, ca în catalog
            să nu apară dubluri sau titluri greșite.
          </p>
        </section>
      </aside>
    </div>
  {/if}
</div>

<style>
  .page { padding-block: var(--space-6) var(--space-8); }

  .top { display: flex; align-items: flex-end; justify-content: space-between; flex-wrap: wrap; gap: var(--space-4); }
  .pg-kicker {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-bold);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--accent);
  }
  h1 {
    font-family: var(--font-display); font-size: var(--fs-h1);
    letter-spacing: -0.015em; line-height: var(--lh-tight); margin-top: 10px;
  }
  .sub { color: var(--text-muted); margin-top: 10px; max-width: 54ch; line-height: 1.55; }

  .strip {
    display: grid; grid-template-columns: repeat(4, minmax(0, 1fr));
    border-top: 2px solid var(--text-primary);
    margin-top: var(--space-5);
  }
  .cell { padding: 14px 18px 0 0; }
  .cell + .cell { border-left: 1px solid var(--border-subtle); padding-left: 18px; }
  .n { display: block; font-family: var(--font-display); font-size: 1.7rem; font-weight: var(--fw-semibold); line-height: 1; }
  .n.accent { color: var(--accent); }
  .n.warn { color: var(--warning); }
  .l {
    display: block; margin-top: 6px;
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-muted);
  }

  .cols {
    display: grid; grid-template-columns: minmax(0, 1fr) 280px;
    gap: var(--space-7); align-items: start; margin-top: var(--space-6);
  }
  @media (max-width: 900px) { .cols { grid-template-columns: minmax(0, 1fr); } }

  .sect { margin-bottom: var(--space-6); }
  .s-label {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--text-muted);
    padding-bottom: 12px; border-bottom: 1px solid var(--border-default);
    margin-bottom: var(--space-4);
  }
  .s-count { color: var(--text-muted); font-weight: var(--fw-regular); }
  .dim { color: var(--text-muted); font-weight: var(--fw-regular); }

  /* ── the publish card ── */
  .pub {
    border: 1px solid var(--border-default); border-radius: var(--radius-lg);
    background: var(--surface-raised); padding: var(--space-4) var(--space-5);
  }
  .pub + .pub { margin-top: var(--space-4); }
  .p-head { display: flex; align-items: center; gap: var(--space-4); }
  .poster { width: 46px; height: 68px; border-radius: 6px; flex: 0 0 auto; box-shadow: var(--shadow-1); }
  .p-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 5px; }
  .p-title { font-family: var(--font-display); font-size: var(--fs-h3); font-weight: var(--fw-semibold); line-height: var(--lh-snug); }
  .p-meta { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .preview { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--accent); white-space: nowrap; }
  .preview:hover { color: var(--accent-hover); }

  .p-form {
    border-top: 1px solid var(--border-subtle);
    margin-top: var(--space-4); padding-top: var(--space-4);
    display: flex; flex-direction: column; gap: var(--space-3);
  }
  .f-row { display: flex; gap: var(--space-3); flex-wrap: wrap; align-items: flex-end; }
  .field { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  .field.grow { flex: 1; min-width: 220px; }
  .field.num { width: 6.5rem; }
  .field.kind { width: 13rem; }
  .f-label {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.08em; text-transform: uppercase; color: var(--text-muted);
  }
  .opt { text-transform: none; letter-spacing: 0; }
  .field textarea {
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary);
    padding: 8px 12px; font-size: var(--fs-small); outline: none;
    font-family: var(--font-body); resize: vertical;
  }
  .field textarea:focus { border-color: var(--accent); }

  .man-toggle {
    align-self: flex-start; background: none; border: none; padding: 2px 0;
    cursor: pointer; font-family: var(--font-mono); font-size: var(--fs-caption);
    color: var(--text-muted);
  }
  .man-toggle:hover { color: var(--accent); }
  .man {
    display: flex; flex-direction: column; gap: var(--space-3);
    padding: 12px; border: 1px dashed var(--border-default); border-radius: var(--radius-md);
  }
  .man-foot { justify-content: space-between; }

  .field input, .field select {
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary);
    padding: 9px 12px; font-size: var(--fs-small); min-height: 40px; outline: none;
  }
  .field input:focus, .field select:focus { border-color: var(--accent); }
  .field select { cursor: pointer; }

  .searchbox { position: relative; }
  .results {
    position: absolute; top: 100%; left: 0; right: 0; z-index: 10;
    background: var(--surface-overlay); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); box-shadow: var(--shadow-2);
    list-style: none; margin: 6px 0 0; padding: 4px; overflow: hidden;
  }
  .results button {
    display: block; width: 100%; text-align: left;
    background: none; border: 0; color: var(--text-primary);
    padding: 8px 12px; border-radius: var(--radius-sm); cursor: pointer; font-size: var(--fs-small);
  }
  .results button:hover { background: var(--surface-raised); }

  .chosen {
    display: flex; align-items: center; gap: 10px; min-height: 40px;
    padding: 0 12px; background: var(--surface-inset);
    border: 1px solid var(--border-strong); border-radius: var(--radius-md);
  }
  .chosen-t { font-size: var(--fs-small); font-weight: var(--fw-semibold); min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .unpick {
    margin-left: auto; background: none; border: 0; cursor: pointer;
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
    min-height: 32px; padding: 0 6px;
  }
  .unpick:hover { color: var(--text-primary); }

  .mal { display: flex; flex-direction: column; gap: var(--space-3); }
  .mal .f-row { align-items: center; gap: 10px; }
  .mal-hint { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--warning); }
  .mal-q {
    flex: 1; min-width: 180px; background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary);
    padding: 7px 12px; font-size: var(--fs-small); min-height: 38px; outline: none;
  }
  .mal-q:focus { border-color: var(--accent); }
  .mal-hits {
    border: 1px solid var(--border-default); border-radius: var(--radius-md);
    background: var(--surface-inset); overflow: hidden;
  }
  .mal-hit { display: flex; align-items: center; gap: 12px; padding: 9px 12px; }
  .mal-hit + .mal-hit { border-top: 1px solid var(--border-subtle); }
  .mh-thumb {
    width: 30px; height: 42px; border-radius: 4px; flex: 0 0 auto;
    object-fit: cover; background: var(--surface-overlay);
  }
  .mh-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
  .mh-title {
    font-size: var(--fs-small); font-weight: var(--fw-semibold);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .mh-meta { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }

  .dl { display: flex; flex-direction: column; gap: 8px; }
  .dl-row { display: flex; flex-wrap: wrap; gap: 8px; }
  .prow-dl {
    display: flex; flex-wrap: wrap; gap: 6px;
    padding: 0 0 14px 2px; margin-top: -6px;
    border-bottom: 1px solid var(--border-subtle);
  }
  .dl-btn.sm { padding: 6px 11px; font-size: var(--fs-micro); }
  .dl-note {
    margin-top: 10px; max-width: 62ch;
    font-size: var(--fs-caption); color: var(--text-muted); line-height: 1.55;
  }
  .dl-btn {
    display: inline-flex; align-items: center; gap: 6px;
    font-family: var(--font-mono); font-size: var(--fs-caption); font-weight: var(--fw-semibold);
    color: var(--text-primary); background: var(--surface-inset);
    border: 1px solid var(--border-default); border-radius: var(--radius-md);
    padding: 8px 13px; white-space: nowrap;
  }
  .dl-btn:hover { border-color: var(--border-strong); background: var(--surface-overlay); }
  .dl-btn.primary {
    color: var(--accent); border-color: color-mix(in srgb, var(--accent) 45%, transparent);
    background: color-mix(in srgb, var(--accent) 8%, transparent);
  }
  .dl-btn.primary:hover { background: color-mix(in srgb, var(--accent) 14%, transparent); }
  .dl-btn:disabled { opacity: 0.6; cursor: wait; }

  .p-toggle {
    align-self: center; white-space: nowrap; cursor: pointer;
    padding: 8px 14px; border-radius: var(--radius-md);
    border: 1px solid var(--border-default); background: var(--surface-raised);
    color: var(--text-primary); font-size: var(--fs-small); font-weight: var(--fw-semibold);
  }
  .p-toggle:hover { background: var(--surface-overlay); }
  /* Destructive and permanent, so it is deliberately quiet until you reach for
     it — a red button sitting next to "Publică" would compete with the action
     this page actually exists for. */
  .p-del {
    align-self: center; white-space: nowrap; cursor: pointer;
    padding: 8px 10px; border-radius: var(--radius-md);
    border: 1px solid transparent; background: none;
    color: var(--text-faint);
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.08em; text-transform: uppercase;
    transition: color var(--motion-fast) var(--ease), border-color var(--motion-fast) var(--ease);
  }
  .p-del:hover:not(:disabled) { color: var(--danger); border-color: var(--danger); }
  .p-del:disabled { opacity: 0.5; cursor: default; }
  /* A collapsed card is a row, not a panel — the header alone should read as one
     line per episode. */
  .pub.collapsed { padding-bottom: var(--space-4); }
  .pager {
    display: flex; align-items: center; justify-content: center; gap: var(--space-4);
    margin-top: var(--space-5);
  }
  .pr-grace {
    font-size: var(--fs-micro); color: var(--text-muted);
  }
  .pager-n { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }

  /* ---- optional hardsub ---- */
  .hs {
    display: flex; flex-direction: column; align-items: flex-start; gap: 8px;
    margin-top: var(--space-4); padding-top: var(--space-4);
    border-top: 1px dashed var(--border-subtle);
  }
  .hs-state {
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--accent);
  }
  .hs-err { font-size: var(--fs-caption); color: var(--danger, #e5484d); }
  /* Full width so the percentage has somewhere to sit; a 13-minute job deserves
     a bar rather than a spinner. */
  .hs-bar {
    position: relative; width: 100%; height: 1.5rem; overflow: hidden;
    background: var(--surface-inset); border-radius: var(--radius-pill);
  }
  .hs-fill {
    height: 100%; background: var(--accent);
    transition: width var(--motion-fast) linear;
  }
  .hs-bar span {
    position: absolute; inset: 0; display: grid; place-items: center;
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-primary);
  }

  .srcs { display: flex; flex-direction: column; gap: var(--space-2); }
  .srcs > .f-label { margin-bottom: 2px; }
  .f-row.src { flex-wrap: nowrap; align-items: center; }
  .rm {
    flex: 0 0 auto; width: 34px; min-height: 40px;
    background: none; border: 1px solid var(--border-default); border-radius: var(--radius-md);
    color: var(--text-muted); font-size: var(--fs-body); cursor: pointer;
  }
  .rm:hover { color: var(--danger); border-color: var(--danger); }
  .addsrc {
    align-self: flex-start; background: none; border: 0; cursor: pointer; padding: 4px 0;
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--accent);
  }
  .addsrc:hover { color: var(--accent-hover); }

  .f-actions {
    display: flex; align-items: center; gap: var(--space-3);
    border-top: 1px solid var(--border-subtle); padding-top: var(--space-3);
  }
  .f-note { font-size: var(--fs-caption); color: var(--text-muted); line-height: 1.5; flex: 1; min-width: 0; }

  .pill {
    display: inline-flex; align-items: center; gap: 6px;
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.08em; text-transform: uppercase; white-space: nowrap;
  }
  .pill.warn { color: var(--warning); }
  .pill.ok { color: var(--success); flex: 0 0 auto; }

  .prow {
    display: flex; align-items: center; gap: 14px;
    padding: 13px 4px; border-bottom: 1px solid var(--border-subtle);
  }
  .pr-title { font-weight: var(--fw-semibold); min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .pr-meta { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); margin-left: auto; white-space: nowrap; }
  .pr-link { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--accent); white-space: nowrap; }
  .pr-link:hover { color: var(--accent-hover); }

  .rail { position: sticky; top: calc(var(--header-h) + var(--space-4)); }
  @media (max-width: 900px) { .rail { position: static; } }
  .r-sect { margin-bottom: var(--space-6); }
  .r-label {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--accent);
    padding-bottom: 12px;
  }
  .step { display: flex; gap: 13px; padding: 11px 0; border-top: 1px solid var(--border-subtle); }
  .step-n { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--accent); flex: 0 0 auto; padding-top: 1px; }
  .step-t { font-size: var(--fs-small); line-height: 1.5; color: var(--text-muted); }
  .r-note { font-size: var(--fs-small); color: var(--text-muted); margin-top: 12px; line-height: 1.6; }

  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 10px 18px; border-radius: var(--radius-md); white-space: nowrap; cursor: pointer;
    transition: background var(--motion-fast) var(--ease), border-color var(--motion-fast) var(--ease);
  }
  .btn.sm { padding: 7px 13px; font-size: var(--fs-caption); }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-primary); }
  .btn.ghost:hover { background: var(--surface-overlay); border-color: var(--border-strong); }
  .btn:disabled { opacity: 0.5; cursor: not-allowed; }

  .empty-hero {
    text-align: center; padding: var(--space-7) var(--space-5);
    border: 1px dashed var(--border-default); border-radius: var(--radius-xl);
  }
  .empty-hero h2 { font-family: var(--font-display); font-size: var(--fs-h2); font-weight: var(--fw-semibold); }
  .empty-hero p { color: var(--text-muted); margin: 10px auto 0; max-width: 42ch; line-height: 1.6; }

  .muted { color: var(--text-muted); }
  .denied {
    display: flex; flex-direction: column; align-items: center; gap: var(--space-4);
    color: var(--text-muted); padding: var(--space-8) 0; text-align: center;
  }
  .denied h1 { font-family: var(--font-display); font-size: var(--fs-h1); color: var(--text-primary); }

  @media (max-width: 640px) {
    .strip { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .cell:nth-child(3) { border-left: 0; padding-left: 0; }
    .pr-meta { display: none; }
  }
</style>
