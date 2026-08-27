<script lang="ts">
  import { goto } from '$app/navigation';
  import { mediaUrl } from '$lib/media';
  import { api } from '$lib/api';
  import { authStore as auth } from '$lib/stores/auth';
  import Person from '$lib/components/Person.svelte';
  import { toast } from '$lib/stores/toast';
  import { reltime } from '$lib/reltime';
  import type { Anime, Manga, Release, VerifierOption, MalSearchHit } from '$shared/types';

  // The translator's workshop (redesigned in Phase 8 round 3, from the
  // Traduceri sketch): everything you have in flight, ordered by what needs
  // you first. Strictly personal — admins oversee the whole pipeline in
  // /admin, not here.

  let releases = $state<Release[]>([]);
  let loading = $state(true);

  // upload form
  let showForm = $state(false);
  // anime = episod + video + subtitrare; manga = capitol + pagini gata făcute
  let medium = $state<'anime' | 'manga'>('anime');
  let query = $state('');
  let results = $state<(Anime | Manga)[]>([]);
  let picked = $state<Anime | Manga | null>(null);
  // series not in the catalog yet — two ways out: import it now from MAL/AniList
  // (adds the real entry so the release links cleanly), or, if both are down,
  // fall back to a free-text proposed title the coordinator resolves at publish.
  let noCatalog = $state(false);
  let proposedTitle = $state('');
  // MAL/AniList import panel (shown from the Series field when nothing matches)
  let importOpen = $state(false);
  let malQ = $state('');
  let malResults = $state<MalSearchHit[]>([]);
  let malSearching = $state(false);
  let importing = $state<number | null>(null);
  let episodeNumber = $state(1);
  let chapterNumber = $state(1);
  let videoFile = $state<File | null>(null);
  let subFile = $state<File | null>(null);
  let roSubFile = $state<File | null>(null);
  let pageFiles = $state<File[]>([]);
  let enPageFiles = $state<File[]>([]);
  let uploading = $state(false);
  let progress = $state(0);
  // 'video' is the chunked send (where the minutes go); 'form' is the small
  // metadata POST after it. Worth distinguishing: a bar that sits at 100% while
  // the server assembles and probes the file reads as a hang.
  let uploadStage = $state<'idle' | 'video' | 'form'>('idle');
  // Cancellation. Both are needed: the controller stops bytes leaving the
  // browser, and the session id is how the server is told to drop the partial
  // file — without it those gigabytes sit in staging until the 24h sweep.
  let uploadAbort: AbortController | null = null;
  let videoObjectKey: string | undefined;
  let videoSize: number | undefined;

  /** Stop an upload in flight and hand the space back. */
  function cancelUpload() {
    // Aborting the XHR is all we can do from here: the object lives in R2, not on
    // our server, so there is nothing of ours to delete. A partial object is
    // never usable — the release form checks the length against what was
    // declared — and a bucket lifecycle rule on video-uploads/ reclaims it.
    uploadAbort?.abort();
    videoObjectKey = undefined;
    videoSize = undefined;
  }

  // who verifies: the list of verifiers/coordinators/admins, defaulting to
  // the translator's last pick so the usual pair costs zero clicks
  let verifiers = $state<VerifierOption[]>([]);
  let verifierId = $state<number | ''>('');
  const roleRo = (r: string) =>
    r === 'coordinator' ? 'coordonator' : r === 'admin' ? 'admin' : r;

  // translators work here; verifiers/moderators have /verify (admins both)
  const allowed = $derived(
    $auth.isAuthenticated && ['translator', 'admin'].includes($auth.user?.role ?? '')
  );
  const roleLabel = $derived($auth.user?.role === 'admin' ? 'ești administrator' : 'ești traducător');

  $effect(() => {
    if ($auth.isLoading) return;
    const role = $auth.user?.role ?? '';
    if (!$auth.isAuthenticated) goto('/login?redirect=/translate');
    else if (['verifier', 'moderator'].includes(role)) goto('/verify');
  });

  let loaded = false;
  $effect(() => {
    if (!allowed || loaded) return;
    loaded = true;
    load();
  });

  // how many unfinished releases you may hold at once — staging is finite, so
  // the cap is what keeps translating from outrunning verifying
  let quota = $state<{ used: number; limit: number; canUpload: boolean } | null>(null);

  async function loadQuota() {
    try {
      quota = (await api.getReleaseQuota()).data;
    } catch {
      quota = null; // never block the form on a failed count
    }
  }

  async function load() {
    loading = true;
    loadQuota();
    try {
      releases = (await api.getReleases()).data;
    } catch {
      releases = [];
    } finally {
      loading = false;
    }
    try {
      const res = await api.getVerifiers();
      verifiers = res.data.filter((v) => v.id !== $auth.user?.id);
      if (res.lastVerifierId && verifiers.some((v) => v.id === res.lastVerifierId)) {
        verifierId = res.lastVerifierId;
      }
    } catch {
      verifiers = [];
    }
  }

  async function reassign(rel: Release, e: Event) {
    const v = (e.currentTarget as HTMLSelectElement).value;
    const id = v ? Number(v) : null;
    try {
      await api.assignVerifier(rel.id, id);
      rel.assignedVerifierId = id ?? undefined;
      rel.assignedVerifierName = id ? verifiers.find((x) => x.id === id)?.username : undefined;
      toast.success('Verificator actualizat.');
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Eroare la reatribuire.');
    }
  }

  // workflow groups — the page reads top-to-bottom as "what needs me first"
  const drafts = $derived(releases.filter((r) => r.state === 'draft'));
  const hero = $derived(drafts[0] ?? null); // newest-first from the API
  const otherDrafts = $derived(drafts.slice(1));
  const returned = $derived(releases.filter((r) => r.state === 'changes_requested'));
  const waiting = $derived(releases.filter((r) => r.state === 'in_review'));
  const done = $derived(releases.filter((r) => r.state === 'approved' || r.state === 'published'));

  // the series' real cover; gradient stand-in when the anime has none or the
  // release only carries a proposed title
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
  const pct = (r: Release) => (r.totalEvents ? Math.round((r.doneEvents / r.totalEvents) * 100) : 0);

  let searchTimer: ReturnType<typeof setTimeout>;
  function onQuery() {
    picked = null;
    clearTimeout(searchTimer);
    if (query.trim().length < 2) {
      results = [];
      return;
    }
    searchTimer = setTimeout(async () => {
      try {
        results =
          medium === 'manga'
            ? (await api.searchManga(query.trim())).data.slice(0, 6)
            : (await api.searchAnime(query.trim())).data.slice(0, 6);
      } catch {
        results = [];
      }
    }, 250);
  }

  function pick(a: Anime | Manga) {
    picked = a;
    query = a.title;
    results = [];
    importOpen = false;
  }

  const errMsg = (err: unknown, fallback: string) =>
    (err as { error?: string }).error ?? (err as { message?: string }).message ?? fallback;

  // search MAL, falling back to AniList server-side (same as the Cereri page)
  async function searchMal() {
    if (malQ.trim().length < 2) return;
    malSearching = true;
    try {
      malResults =
        medium === 'manga'
          ? (await api.malSearchManga(malQ.trim())).data
          : (await api.malSearchAnime(malQ.trim())).data;
      if (malResults.length === 0) toast.info('Niciun rezultat pe MAL / AniList.');
    } catch (err) {
      toast.error(errMsg(err, 'Căutarea a eșuat.'));
    } finally {
      malSearching = false;
    }
  }

  // import the picked hit into our catalog and select it for the release
  async function importFromMal(hit: MalSearchHit) {
    importing = hit.malId;
    try {
      const r = medium === 'manga' ? await api.importManga(hit.malId) : await api.importAnime(hit.malId);
      picked = r.data;
      query = r.data.title;
      noCatalog = false;
      importOpen = false;
      malResults = [];
      toast.success(
        r.created === false
          ? `„${r.data.title}” era deja în catalog — am selectat-o.`
          : `Adăugat în catalog: ${r.data.title}`
      );
    } catch (err) {
      toast.error(errMsg(err, 'Importul a eșuat.'));
    } finally {
      importing = null;
    }
  }

  function fileFrom(e: Event): File | null {
    return (e.currentTarget as HTMLInputElement).files?.[0] ?? null;
  }
  function filesFrom(e: Event): File[] {
    return Array.from((e.currentTarget as HTMLInputElement).files ?? []);
  }

  function switchMedium(m: 'anime' | 'manga') {
    if (medium === m) return;
    medium = m;
    // the search results (catalog + MAL) belong to the other medium
    picked = null;
    query = '';
    results = [];
    importOpen = false;
    malResults = [];
    malQ = '';
  }

  async function upload(e: SubmitEvent) {
    e.preventDefault();
    const series = noCatalog ? proposedTitle.trim() : picked?.id;
    if (!series) return;
    if (medium === 'manga') {
      if (pageFiles.length === 0) {
        toast.error('Adaugă paginile traduse (imagini, în ordinea capitolului).');
        return;
      }
    } else {
      if (!videoFile) return;
      // a sub file is optional: if none is attached, the server tries to lift an
      // embedded soft-sub track out of the video (it 400s if there's none)
    }
    uploading = true;
    progress = 0;
    try {
      if (medium !== 'manga' && videoFile) {
        // The video goes first, on its own, in chunks. A single multipart POST
        // cannot carry it: Cloudflare rejects bodies over 100 MiB at the edge, so
        // anything but a trailer would 413 before reaching the server. The form
        // below then carries only fields and the small subtitle files.
        //
        // This is also where essentially all the time goes, so it owns the
        // progress bar; the form POST after it is near-instant.
        uploadStage = 'video';
        uploadAbort = new AbortController();
        // Straight to R2 with a presigned URL: the bytes bypass our server and
        // Cloudflare's 100 MiB body limit entirely. The form below then just
        // names the object and the server pulls it into staging.
        const up = await api.uploadVideoToR2(videoFile, (f) => (progress = f), {
          signal: uploadAbort.signal
        });
        videoObjectKey = up.key;
        videoSize = up.size;
        uploadStage = 'form';
      }
      const res = await api.createRelease(
        medium === 'manga'
          ? {
              medium: 'manga',
              mangaId: noCatalog ? undefined : picked?.id,
              proposedTitle: noCatalog ? proposedTitle.trim() : undefined,
              chapterNumber,
              pages: pageFiles,
              enPages: enPageFiles.length ? enPageFiles : undefined,
              verifierId: verifierId || undefined
            }
          : {
              animeId: noCatalog ? undefined : picked?.id,
              proposedTitle: noCatalog ? proposedTitle.trim() : undefined,
              episodeNumber,
              videoObjectKey,
              videoSize,
              sub: subFile ?? undefined,
              roSub: roSubFile ?? undefined,
              verifierId: verifierId || undefined
            },
        (f) => (progress = f)
      );
      // Claimed by the release now (pulled into staging, object deleted).
      videoObjectKey = undefined;
      videoSize = undefined;
      const direct = medium === 'manga' || !!roSubFile;
      toast.success(
        direct
          ? 'Trimis direct la verificare.'
          : res.autoSub
            ? 'Subtitrare extrasă din video — spor la tradus!'
            : 'Ciornă creată — spor la tradus!'
      );
      if (direct) {
        showForm = false;
        await load();
      } else {
        goto(`/translate/${res.data.id}`);
      }
    } catch (err) {
      const aborted = (err as { aborted?: boolean }).aborted === true;
      if (aborted) {
        // cancelUpload already dropped the session; this is not a failure.
        toast.info('Încărcare anulată.');
      } else {
        toast.error(
          (err as { error?: string }).error ?? (err as { message?: string }).message ?? 'Încărcarea a eșuat.'
        );
        loadQuota(); // a 409 means the count moved — show the real number
      }
      videoObjectKey = undefined;
      videoSize = undefined;
    } finally {
      uploading = false;
      uploadStage = 'idle';
      progress = 0;
      uploadAbort = null;
    }
  }

  async function remove(rel: Release) {
    if (!confirm(`Ștergi release-ul pentru „${seriesName(rel)}” — ${numLabel(rel)}?`)) return;
    try {
      await api.deleteRelease(rel.id);
      releases = releases.filter((r) => r.id !== rel.id);
      toast.success('Release șters.');
    } catch (err) {
      toast.error(
        (err as { error?: string }).error ?? (err as { message?: string }).message ?? 'Ștergerea a eșuat.'
      );
    }
  }

  const steps = [
    'Încarci episodul cu subtitrarea EN și creezi release-ul.',
    'Traduci replicile în editor — totul se salvează singur.',
    'Trimiți la verificare când ai tradus tot.',
    'Verificatorul aprobă sau îți returnează release-ul cu note.'
  ];
  const rules = [
    'Diacritice obligatorii — ă î ș ț â',
    'Maxim 42 de caractere pe rând',
    'Onorificele se păstrează (-san, -sama)',
    'Numele proprii rămân netraduse'
  ];
</script>

<svelte:head>
  <title>Traduceri · Anime-Kage</title>
</svelte:head>

<div class="container page">
  {#if !$auth.isLoading && $auth.isAuthenticated && !allowed}
    <div class="denied">
      <h1>Atelierul de traduceri</h1>
      <p>Pagina e rezervată traducătorilor. Dacă vrei să traduci pentru Anime-Kage, scrie-ne!</p>
      <a class="btn ghost" href="/home">Înapoi acasă</a>
    </div>
  {:else if allowed}
    <header class="top">
      <div>
        <p class="pg-kicker">Echipă · {roleLabel}</p>
        <h1>Atelierul de traduceri</h1>
        <p class="sub">
          Creezi un release dintr-o subtitrare EN, traduci în editor, apoi îl trimiți la verificare.
          Aici e tot ce ai în lucru.
        </p>
      </div>
      <button class="btn fill" onclick={() => (showForm = !showForm)}>
        {showForm ? '× Închide' : '+ Release nou'}
      </button>
    </header>

    <div class="strip" role="list">
      <div class="cell" role="listitem">
        <span class="n accent">{drafts.length}</span>
        <span class="l">în lucru</span>
      </div>
      <div class="cell" role="listitem">
        <span class="n" class:warn={returned.length > 0}>{returned.length}</span>
        <span class="l">de corectat</span>
      </div>
      <div class="cell" role="listitem">
        <span class="n">{waiting.length}</span>
        <span class="l">la verificare</span>
      </div>
      <div class="cell" role="listitem">
        <span class="n">{done.length}</span>
        <span class="l">publicate</span>
      </div>
    </div>

    {#if showForm}
      <form class="upload" onsubmit={upload}>
        {#if medium !== 'manga' && quota && quota.limit > 0}
          <div class="slots" class:full={!quota.canUpload}>
            <span class="s-count">{quota.used} / {quota.limit}</span>
            <span class="s-text">
              {quota.canUpload ? 'episoade neterminate' : 'limită atinsă — publică unul înainte de a urca altul'}
            </span>
          </div>
        {/if}
        <div class="u-head">
          <span class="u-title">Release nou</span>
          <div class="mtoggle" role="tablist" aria-label="Tip release">
            <button
              type="button"
              class="mopt"
              class:on={medium === 'anime'}
              onclick={() => switchMedium('anime')}>Anime · episod</button
            >
            <button
              type="button"
              class="mopt"
              class:on={medium === 'manga'}
              onclick={() => switchMedium('manga')}>Manga · capitol</button
            >
          </div>
        </div>
        <div class="u-grid">
          <div class="field grow searchbox">
            <label for="anime-q">Serie</label>
            {#if picked}
              <div class="chosen">
                <span class="chosen-t">{picked.title}{picked.year ? ` · ${picked.year}` : ''}</span>
                <button type="button" class="unpick" onclick={() => { picked = null; query = ''; }}>schimbă</button>
              </div>
            {:else if noCatalog}
              <input
                id="anime-q"
                type="text"
                placeholder="Scrie titlul seriei…"
                bind:value={proposedTitle}
                autocomplete="off"
              />
              <button type="button" class="fhint-link" onclick={() => (noCatalog = false)}>← caută seria în catalog</button>
            {:else}
              <input
                id="anime-q"
                type="text"
                placeholder="Caută după titlu…"
                bind:value={query}
                oninput={onQuery}
                autocomplete="off"
              />
              {#if results.length}
                <ul class="results">
                  {#each results as a (a.id)}
                    <li>
                      <button type="button" onclick={() => pick(a)}>
                        {a.title} {a.year ? `(${a.year})` : ''}
                      </button>
                    </li>
                  {/each}
                </ul>
              {/if}
              <button type="button" class="fhint-link" onclick={() => { importOpen = !importOpen; if (importOpen && !malQ) malQ = query; }}>
                Nu e în catalog? Adaug-o din MAL / AniList
              </button>
            {/if}
          </div>
          <div class="field num">
            {#if medium === 'manga'}
              <label for="ch-num">Capitol</label>
              <input id="ch-num" type="number" min="0.1" step="0.1" bind:value={chapterNumber} required />
            {:else}
              <label for="ep-num">Episod</label>
              <input id="ep-num" type="number" min="1" bind:value={episodeNumber} required />
            {/if}
          </div>
          <div class="field vfield">
            <label for="f-verif">Verificator</label>
            <select id="f-verif" class="vselect" bind:value={verifierId}>
              <option value="">— oricine e liber —</option>
              {#each verifiers as v (v.id)}
                <option value={v.id}>{v.username}{v.role !== 'verifier' ? ` · ${roleRo(v.role)}` : ''}</option>
              {/each}
            </select>
          </div>
        </div>

        {#if importOpen && !picked && !noCatalog}
          <div class="import">
            <div class="imp-bar">
              <input
                class="imp-q"
                type="text"
                placeholder={medium === 'manga' ? 'Titlul manga-ului pe MAL / AniList…' : 'Titlul anime-ului pe MAL / AniList…'}
                bind:value={malQ}
                onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); searchMal(); } }}
              />
              <button type="button" class="btn ghost sm" disabled={malSearching || malQ.trim().length < 2} onclick={searchMal}>
                {malSearching ? 'Se caută…' : 'Caută'}
              </button>
            </div>
            {#if malResults.length}
              <div class="imp-hits">
                {#each malResults as hit (hit.malId)}
                  <div class="imp-hit">
                    {#if hit.imageUrl}
                      <img class="ih-thumb" src={mediaUrl(hit.imageUrl)} alt="" loading="lazy" />
                    {:else}
                      <span class="ih-thumb"></span>
                    {/if}
                    <span class="ih-main">
                      <span class="ih-title">{hit.title}</span>
                      <span class="ih-meta">
                        {hit.type}{hit.year ? ` · ${hit.year}` : ''}{hit.episodes
                          ? ` · ${hit.episodes} ep`
                          : hit.chapters
                            ? ` · ${hit.chapters} cap`
                            : ''} · MAL #{hit.malId}
                      </span>
                    </span>
                    <button type="button" class="btn fill sm" disabled={importing !== null} onclick={() => importFromMal(hit)}>
                      {importing === hit.malId ? 'Se adaugă…' : 'Adaugă'}
                    </button>
                  </div>
                {/each}
              </div>
            {/if}
            <button
              type="button"
              class="fhint-link quiet"
              onclick={() => { noCatalog = true; importOpen = false; if (!proposedTitle) proposedTitle = malQ || query; }}
            >
              Trimite doar titlul
            </button>
          </div>
        {/if}

        {#if medium === 'manga'}
          <div class="u-files two">
            <div class="field">
              <label for="f-pages">Pagini traduse <span class="lim">în ordinea numelor</span></label>
              <input id="f-pages" type="file" accept="image/*" multiple onchange={(e) => (pageFiles = filesFrom(e))} required />
              {#if pageFiles.length}<span class="fcount">{pageFiles.length} pagini</span>{/if}
            </div>
            <div class="field">
              <label for="f-enpages">
                Pagini originale
                <button
                  type="button"
                  class="tag"
                  data-tip="Verificatorul le vede alături de cele traduse."
                  aria-label="Opțional. Verificatorul le vede alături de cele traduse.">opțional</button>
              </label>
              <input id="f-enpages" type="file" accept="image/*" multiple onchange={(e) => (enPageFiles = filesFrom(e))} />
              {#if enPageFiles.length}<span class="fcount">{enPageFiles.length} pagini</span>{/if}
            </div>
          </div>
        {:else}
          <div class="u-files">
            <div class="field">
              <label for="f-video">Video <span class="lim">max 3 GB</span></label>
              <input id="f-video" type="file" accept=".mp4,.webm,.mkv,.m4v" onchange={(e) => (videoFile = fileFrom(e))} required />
            </div>
            <div class="field">
              <label for="f-sub">
                Subtitrare EN
                <button
                  type="button"
                  class="tag"
                  data-tip="Lasă gol și extragem pista de subtitrare din video."
                  aria-label="Opțional. Lasă gol și extragem pista de subtitrare din video.">opțional</button>
              </label>
              <input id="f-sub" type="file" accept=".srt,.ass,.ssa,.vtt" onchange={(e) => (subFile = fileFrom(e))} />
            </div>
            <div class="field">
              <label for="f-rosub">
                Subtitrare RO
                <button
                  type="button"
                  class="tag"
                  data-tip="Sare peste editor și merge direct la verificare."
                  aria-label="Opțional. Sare peste editor și merge direct la verificare.">opțional</button>
              </label>
              <input id="f-rosub" type="file" accept=".srt,.ass,.ssa,.vtt" onchange={(e) => (roSubFile = fileFrom(e))} />
            </div>
          </div>
        {/if}

        {#if uploading}
          <div class="progress" role="progressbar" aria-valuenow={Math.round(progress * 100)} aria-valuemin="0" aria-valuemax="100">
            <div class="bar" style="width: {progress * 100}%"></div>
            <span>{Math.round(progress * 100)}%</span>
          </div>
          {#if uploadStage === 'form'}
            <!-- The bar has hit 100% but the request is not done: the server is
                 assembling the file and probing it for an embedded soft-sub,
                 which on a large episode takes a moment. Say so. -->
            <p class="u-stage">Se finalizează — se caută o subtitrare încorporată…</p>
          {:else if uploadStage === 'video'}
            <p class="u-stage">Se trimite videoul în bucăți — poți relua dacă se întrerupe.</p>
          {/if}
        {/if}

        <div class="u-actions">
          <!-- While an upload is running this cancels it; otherwise it closes the
               form. It used to only ever do the latter, which hid the form and
               left the chunk loop uploading gigabytes in the background. -->
          <button
            class="btn ghost"
            type="button"
            onclick={() => (uploading ? cancelUpload() : (showForm = false))}
          >
            {uploading ? 'Oprește încărcarea' : 'Anulează'}
          </button>
          <button
            class="btn fill"
            type="submit"
            disabled={uploading ||
              (medium !== 'manga' && quota?.canUpload === false) ||
              (medium === 'manga' ? pageFiles.length === 0 : !videoFile) ||
              (noCatalog ? !proposedTitle.trim() : !picked)}
          >
            {uploading ? 'Se încarcă…' : 'Creează release-ul'}
          </button>
        </div>
      </form>
    {/if}

    <div class="cols">
      <div class="main">
        {#if loading}
          <p class="muted">Se încarcă…</p>
        {:else if releases.length === 0}
          <div class="empty-hero">
            <h2>Niciun release încă</h2>
            <p>Alege o serie, încarcă subtitrarea EN și primești un editor cu toate replicile gata de tradus.</p>
            <button class="btn fill" onclick={() => (showForm = true)}>+ Primul tău release</button>
          </div>
        {:else}
          {#if hero}
            <section class="sect">
              <h2 class="s-label">Continuă de unde ai rămas</h2>
              <a class="hero" href="/translate/{hero.id}">
                <span class="poster lg" style={poster(hero)}></span>
                <span class="hero-main">
                  <span class="h-title">{seriesName(hero)} <span class="dim">— {numLabel(hero)}</span></span>
                  <span class="h-meta">modificat {reltime(hero.updatedAt)} · salvare automată</span>
                  <span class="h-bar"><span class="h-fill" style="width:{Math.max(2, pct(hero))}%"></span></span>
                  <span class="h-prog"><strong>{hero.doneEvents} / {hero.totalEvents} replici</strong>
                    {hero.totalEvents > hero.doneEvents ? ` · mai ai ${hero.totalEvents - hero.doneEvents}` : ' · totul e tradus'}</span>
                </span>
                <span class="btn fill nolink">Deschide editorul →</span>
              </a>
            </section>
          {/if}

          {#if returned.length > 0}
            <section class="sect">
              <h2 class="s-label">De corectat <span class="s-count">· {returned.length}</span></h2>
              {#each returned as rel (rel.id)}
                <article class="returned">
                  <div class="r-top">
                    <span class="pill warn">returnat</span>
                    <span class="r-title">{seriesName(rel)} <span class="dim">— {numLabel(rel)}</span></span>
                    <span class="r-meta">
                      {#if rel.reviewerName}
                        verificat de <Person name={rel.reviewerName} /> ·
                      {/if}
                      {reltime(rel.updatedAt)}
                    </span>
                  </div>
                  {#if rel.reviewNotes}
                    <blockquote class="r-quote">„{rel.reviewNotes}”</blockquote>
                  {/if}
                  <div class="r-actions">
                    <a class="btn ghost" href="/translate/{rel.id}">Vezi notele și corectează →</a>
                  </div>
                </article>
              {/each}
            </section>
          {/if}

          {#if otherDrafts.length > 0}
            <section class="sect">
              <h2 class="s-label">Alte ciorne <span class="s-count">· {otherDrafts.length}</span></h2>
              <div class="listcard">
                {#each otherDrafts as rel (rel.id)}
                  <div class="lrow">
                    <span class="poster sm" style={poster(rel)}></span>
                    <span class="l-main">
                      <span class="l-title">{seriesName(rel)} <span class="dim">— {numLabel(rel)}</span></span>
                      <span class="l-meta">început {reltime(rel.createdAt)} · modificat {reltime(rel.updatedAt)}</span>
                    </span>
                    <span class="l-prog">
                      <span class="h-bar"><span class="h-fill" style="width:{Math.max(2, pct(rel))}%"></span></span>
                      <span class="l-nums">{rel.doneEvents} / {rel.totalEvents}</span>
                    </span>
                    <a class="btn ghost sm" href="/translate/{rel.id}">Continuă →</a>
                    <button class="del" title="Șterge release-ul" onclick={() => remove(rel)}>×</button>
                  </div>
                {/each}
              </div>
            </section>
          {/if}

          {#if waiting.length > 0}
            <section class="sect">
              <h2 class="s-label">La verificare <span class="s-count">· {waiting.length}</span></h2>
              <div class="listcard">
                {#each waiting as rel (rel.id)}
                  <div class="lrow">
                    <span class="poster sm faded" style={poster(rel)}></span>
                    <span class="l-main">
                      <span class="l-title">{seriesName(rel)} <span class="dim">— {numLabel(rel)}</span></span>
                      <span class="l-meta">trimis {reltime(rel.updatedAt)}</span>
                    </span>
                    <span class="pill warn dotted"><span class="dot"></span>în verificare</span>
                    <span class="l-meta">verifică</span>
                    <select
                      class="vselect sm"
                      value={rel.assignedVerifierId ?? ''}
                      onchange={(e) => reassign(rel, e)}
                      title="Schimbă verificatorul dacă cel ales lipsește"
                    >
                      <option value="">oricine</option>
                      {#each verifiers as v (v.id)}
                        <option value={v.id}>{v.username}</option>
                      {/each}
                    </select>
                  </div>
                {/each}
              </div>
            </section>
          {/if}

          {#if done.length > 0}
            <section class="sect">
              <h2 class="s-label">Publicate recent</h2>
              <div class="plain">
                {#each done.slice(0, 6) as rel (rel.id)}
                  <div class="prow">
                    <span class="pill ok">{rel.state === 'published' ? 'publicat' : 'aprobat'}</span>
                    <span class="p-title">{seriesName(rel)} <span class="dim">— {numLabel(rel)}</span></span>
                    <span class="p-meta">
                      {#if rel.reviewerName}
                        aprobat de <Person name={rel.reviewerName} /> ·
                      {/if}
                      {reltime(rel.updatedAt)}
                    </span>
                    {#if rel.animeId}
                      <a class="p-link" href="/anime/{rel.animeId}">vezi pe site →</a>
                    {:else if rel.mangaId}
                      <a class="p-link" href="/manga/{rel.mangaId}">vezi pe site →</a>
                    {/if}
                  </div>
                {/each}
              </div>
            </section>
          {/if}
        {/if}
      </div>

      <aside class="rail">
        <section class="r-sect">
          <h2 class="r-label">Cum funcționează</h2>
          {#each steps as step, i (i)}
            <div class="step">
              <span class="step-n">{String(i + 1).padStart(2, '0')}</span>
              <span class="step-t">{step}</span>
            </div>
          {/each}
        </section>

        <section class="r-sect">
          <h2 class="r-label">Ghidul echipei</h2>
          {#each rules as rule (rule)}
            <div class="rule">
              <span class="check">✓</span>
              <span class="step-t">{rule}</span>
            </div>
          {/each}
          <p class="r-note">Glosarul complet e pe forum, la secțiunea Meta.</p>
        </section>
      </aside>
    </div>
  {/if}
</div>

<style>
  .page { padding-block: var(--space-6) var(--space-8); }

  .top {
    display: flex; align-items: flex-end; justify-content: space-between;
    flex-wrap: wrap; gap: var(--space-4);
  }
  .pg-kicker {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-bold);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--accent);
  }
  h1 {
    font-family: var(--font-display); font-size: var(--fs-h1);
    letter-spacing: -0.015em; line-height: var(--lh-tight); margin-top: 10px;
  }
  .sub { color: var(--text-muted); margin-top: 10px; max-width: 54ch; line-height: 1.55; }

  /* ── stat strip: the page's 2px rule doubles as a scoreboard ── */
  .strip {
    display: grid; grid-template-columns: repeat(4, minmax(0, 1fr));
    border-top: 2px solid var(--text-primary);
    margin-top: var(--space-5);
  }
  .cell { padding: 14px 18px 0 0; }
  .cell + .cell { border-left: 1px solid var(--border-subtle); padding-left: 18px; }
  .n {
    display: block; font-family: var(--font-display);
    font-size: 1.7rem; font-weight: var(--fw-semibold); line-height: 1;
  }
  .n.accent { color: var(--accent); }
  .n.warn { color: var(--warning); }
  .l {
    display: block; margin-top: 6px;
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-muted);
  }

  /* ── upload panel ── */
  .upload {
    border: 1px solid var(--border-strong); border-radius: var(--radius-lg);
    background: var(--surface-raised); padding: var(--space-5);
    margin-top: var(--space-5);
    display: flex; flex-direction: column; gap: var(--space-4);
  }
  .slots {
    display: flex; align-items: baseline; gap: 10px; flex-wrap: wrap;
    margin-bottom: var(--space-4); padding: 10px 14px;
    border: 1px solid var(--border-subtle); border-left: 2px solid var(--accent);
    border-radius: var(--radius-md); background: var(--surface-inset);
  }
  .slots.full { border-left-color: var(--danger); }
  .s-count {
    font-family: var(--font-mono); font-size: var(--fs-small);
    font-weight: var(--fw-bold); color: var(--text-primary); white-space: nowrap;
  }
  .s-text { font-size: var(--fs-caption); color: var(--text-muted); line-height: 1.5; }

  .u-head { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
  .u-title { font-family: var(--font-display); font-size: var(--fs-h3); font-weight: var(--fw-semibold); }
  .mtoggle {
    display: inline-flex; border: 1px solid var(--border-default);
    border-radius: var(--radius-pill); overflow: hidden;
  }
  .mopt {
    background: none; border: 0; cursor: pointer; color: var(--text-muted);
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.06em; text-transform: uppercase; padding: 8px 14px;
  }
  .mopt.on { background: var(--accent); color: var(--on-accent); }
  .mopt:not(.on):hover { color: var(--text-primary); background: var(--surface-overlay); }
  .u-files.two { grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); }
  .fcount { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--accent); }
  .u-grid { display: flex; gap: var(--space-4); flex-wrap: wrap; }
  .u-files { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: var(--space-4); }
  .field { display: flex; flex-direction: column; gap: 7px; min-width: 0; }
  .field.grow { flex: 1; min-width: 240px; }
  .field.num { width: 7rem; }
  .field label {
    display: flex; align-items: center; gap: 8px;
    font-family: var(--font-body); font-size: var(--fs-caption);
    font-weight: var(--fw-semibold); color: var(--text-primary);
  }
  /* a hard constraint on what you may drop in — not an explanation */
  .lim {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    font-weight: var(--fw-regular); color: var(--text-faint);
  }

  /* "opțional" carries the detail in a tooltip: the field stays one line, and
     the one thing that is genuinely non-obvious is a hover away */
  .tag {
    position: relative; cursor: help;
    font-family: var(--font-mono); font-size: 0.625rem; font-weight: var(--fw-semibold);
    letter-spacing: 0.06em; text-transform: uppercase;
    color: var(--text-muted); background: var(--surface-overlay);
    border: 1px solid var(--border-subtle); border-radius: var(--radius-pill);
    padding: 2px 8px; outline: none;
  }
  .tag:hover, .tag:focus-visible { color: var(--accent); border-color: var(--accent); }
  .tag::after {
    content: attr(data-tip);
    position: absolute; left: 0; bottom: calc(100% + 8px); z-index: 20;
    width: max-content; max-width: 240px;
    padding: 7px 10px; border-radius: var(--radius-sm);
    background: var(--surface-overlay); border: 1px solid var(--border-default);
    box-shadow: 0 8px 22px rgba(0, 0, 0, 0.35);
    font-family: var(--font-body); font-size: var(--fs-micro); font-weight: var(--fw-regular);
    letter-spacing: 0; text-transform: none; color: var(--text-primary);
    line-height: 1.45; text-align: left; white-space: normal;
    opacity: 0; pointer-events: none; transition: opacity var(--motion-fast) var(--ease);
  }
  .tag:hover::after, .tag:focus-visible::after { opacity: 1; }
  .field input[type='text'],
  .field input[type='number'] {
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary);
    padding: 10px 14px; font-size: var(--fs-small); min-height: 44px; outline: none;
  }
  .field input:focus { border-color: var(--accent); }
  .field.vfield { min-width: 200px; }
  .vselect {
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary); cursor: pointer;
    padding: 10px 12px; font-size: var(--fs-small); min-height: 44px; outline: none;
    font-family: var(--font-body);
  }
  .vselect:hover, .vselect:focus { border-color: var(--accent); }
  .vselect.sm { min-height: 30px; padding: 3px 8px; font-size: var(--fs-caption); border-radius: var(--radius-sm); }
  .field input[type='file'] { font-size: var(--fs-caption); color: var(--text-muted); padding: 6px 0; }
  .field input[type='file']::file-selector-button {
    border: 1px solid var(--border-default); background: var(--surface-inset);
    color: var(--text-primary); font-weight: var(--fw-semibold); font-size: var(--fs-caption);
    padding: 8px 14px; border-radius: var(--radius-sm); margin-right: 12px; cursor: pointer;
  }
  .field input[type='file']::file-selector-button:hover { background: var(--surface-overlay); }

  .searchbox { position: relative; }

  /* subtle inline affordance under a field — not a comment, an action */
  .fhint-link {
    align-self: flex-start; background: none; border: 0; cursor: pointer;
    padding: 2px 0; margin-top: 2px;
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.04em; color: var(--accent);
  }
  .fhint-link:hover { color: var(--accent-hover); }
  .fhint-link.quiet { color: var(--text-muted); font-weight: var(--fw-regular); }
  .fhint-link.quiet:hover { color: var(--text-primary); }

  /* chosen series chip */
  .chosen {
    display: flex; align-items: center; gap: 10px; min-height: 44px;
    padding: 0 8px 0 14px; background: var(--surface-inset);
    border: 1px solid var(--border-strong); border-radius: var(--radius-md);
  }
  .chosen-t {
    flex: 1; min-width: 0; font-size: var(--fs-small); font-weight: var(--fw-semibold);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .unpick {
    flex: 0 0 auto; background: none; border: 0; cursor: pointer;
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
    min-height: 32px; padding: 0 8px; border-radius: var(--radius-sm);
  }
  .unpick:hover { color: var(--text-primary); background: var(--surface-overlay); }

  /* import-from-MAL panel */
  .import {
    display: flex; flex-direction: column; gap: var(--space-3);
    padding: var(--space-4); border: 1px dashed var(--border-default);
    border-radius: var(--radius-md); background: var(--surface-inset);
  }
  .imp-bar { display: flex; gap: 10px; align-items: center; }
  .imp-q {
    flex: 1; min-width: 0; background: var(--surface-raised); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary);
    padding: 10px 14px; font-size: var(--fs-small); min-height: 42px; outline: none;
  }
  .imp-q:focus { border-color: var(--accent); }
  .imp-hits {
    border: 1px solid var(--border-default); border-radius: var(--radius-md);
    background: var(--surface-raised); overflow: hidden;
  }
  .imp-hit { display: flex; align-items: center; gap: 12px; padding: 10px 12px; }
  .imp-hit + .imp-hit { border-top: 1px solid var(--border-subtle); }
  .ih-thumb {
    width: 32px; height: 46px; border-radius: 4px; flex: 0 0 auto;
    object-fit: cover; background: var(--surface-overlay);
  }
  .ih-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
  .ih-title {
    font-size: var(--fs-small); font-weight: var(--fw-semibold);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .ih-meta { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }

  .results {
    position: absolute; top: 100%; left: 0; right: 0; z-index: 10;
    background: var(--surface-overlay); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); box-shadow: var(--shadow-2);
    list-style: none; margin: 6px 0 0; padding: 4px; overflow: hidden;
  }
  .results button {
    display: block; width: 100%; text-align: left;
    background: none; border: 0; color: var(--text-primary);
    padding: 9px 12px; border-radius: var(--radius-sm); cursor: pointer; font-size: var(--fs-small);
  }
  .results button:hover { background: var(--surface-raised); }

  .progress {
    position: relative; height: 1.6rem;
    background: var(--surface-inset); border-radius: var(--radius-pill); overflow: hidden;
  }
  .progress .bar { height: 100%; background: var(--accent); transition: width var(--motion-fast) linear; }
  .u-stage {
    margin-top: 6px;
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
  }
  .progress span {
    position: absolute; inset: 0; display: grid; place-items: center;
    font-size: var(--fs-caption); font-family: var(--font-mono);
  }
  .u-actions {
    display: flex; align-items: center; gap: var(--space-3);
    border-top: 1px solid var(--border-subtle); padding-top: var(--space-4);
  }

  /* ── two columns ── */
  .cols {
    display: grid; grid-template-columns: minmax(0, 1fr) 280px;
    gap: var(--space-7); align-items: start; margin-top: var(--space-6);
  }
  @media (max-width: 900px) {
    .cols { grid-template-columns: minmax(0, 1fr); }
  }

  .sect { margin-bottom: var(--space-6); }
  .s-label {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--text-muted);
    padding-bottom: 12px; border-bottom: 1px solid var(--border-default);
    margin-bottom: var(--space-4);
  }
  .s-count { color: var(--text-muted); font-weight: var(--fw-regular); }
  .dim { color: var(--text-muted); font-weight: var(--fw-regular); }

  .poster { border-radius: var(--radius-sm); flex: 0 0 auto; }
  .poster.lg { width: 54px; height: 80px; border-radius: 8px; box-shadow: var(--shadow-2); }
  .poster.sm { width: 38px; height: 56px; }
  .poster.faded { opacity: 0.7; }

  /* hero: the one release you're most likely here for */
  .hero {
    display: flex; align-items: center; gap: var(--space-4);
    border: 1px solid var(--border-default); border-radius: var(--radius-lg);
    background: var(--surface-raised); padding: var(--space-4) var(--space-5);
    color: var(--text-primary);
    transition: border-color var(--motion-fast) var(--ease);
  }
  .hero:hover { border-color: var(--border-strong); }
  .hero-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 6px; }
  .h-title {
    font-family: var(--font-display); font-size: var(--fs-h3); font-weight: var(--fw-semibold);
    line-height: var(--lh-snug);
  }
  .h-meta { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .h-bar {
    display: block; height: 5px; border-radius: var(--radius-pill);
    background: var(--surface-inset); max-width: 420px; overflow: hidden; margin-top: 4px;
  }
  .h-fill { display: block; height: 100%; border-radius: var(--radius-pill); background: var(--accent); }
  .h-prog { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .h-prog strong { color: var(--accent); font-weight: var(--fw-medium); }
  .nolink { pointer-events: none; }

  /* returned card */
  .returned {
    border: 1px solid var(--border-default); border-radius: var(--radius-lg);
    background: var(--surface-raised); padding: var(--space-4) var(--space-5);
  }
  .returned + .returned { margin-top: var(--space-3); }
  .r-top { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
  .r-title { font-family: var(--font-display); font-size: var(--fs-body); font-weight: var(--fw-semibold); }
  .r-meta { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); margin-left: auto; }
  .r-quote {
    font-size: var(--fs-small); font-style: italic; color: var(--text-muted);
    line-height: 1.55; margin: 13px 0 0; padding-left: 13px;
    border-left: 2px solid var(--border-default); max-width: 60ch;
  }
  .r-actions { display: flex; align-items: center; gap: 14px; margin-top: 15px; }

  /* list cards (ciorne, la verificare) */
  .listcard {
    border: 1px solid var(--border-subtle); border-radius: var(--radius-lg);
    background: var(--surface-raised); overflow: hidden;
  }
  .lrow {
    display: flex; align-items: center; gap: var(--space-4);
    padding: 14px 18px;
  }
  .lrow + .lrow { border-top: 1px solid var(--border-subtle); }
  .lrow:hover { background: var(--surface-overlay); }
  .l-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
  .l-title {
    font-family: var(--font-display); font-size: var(--fs-body); font-weight: var(--fw-semibold);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .l-meta { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .l-prog { width: 150px; flex: 0 0 auto; }
  .l-prog .h-bar { max-width: none; }
  .l-nums { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); display: block; margin-top: 6px; }
  .del {
    background: none; border: 0; color: var(--text-muted); font-size: 1.1rem;
    cursor: pointer; padding: 4px 6px; border-radius: var(--radius-sm); line-height: 1;
  }
  .del:hover { color: var(--danger); background: color-mix(in srgb, var(--danger) 10%, transparent); }

  /* published: quiet rows, no card */
  .prow {
    display: flex; align-items: center; gap: 14px;
    padding: 13px 4px; border-bottom: 1px solid var(--border-subtle);
  }
  .p-title {
    font-weight: var(--fw-semibold); min-width: 0;
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .p-meta { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); margin-left: auto; white-space: nowrap; }
  .p-link { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--accent); white-space: nowrap; }
  .p-link:hover { color: var(--accent-hover); }

  .pill {
    display: inline-flex; align-items: center; gap: 6px;
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.08em; text-transform: uppercase; white-space: nowrap; flex: 0 0 auto;
  }
  .pill.warn { color: var(--warning); }
  .pill.ok { color: var(--success); }
  .dot {
    width: 5px; height: 5px; border-radius: 50%; background: var(--warning);
    animation: pulse 2s ease-in-out infinite;
  }
  @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.35; } }

  /* ── right rail ── */
  .rail { position: sticky; top: calc(var(--header-h) + var(--space-4)); }
  @media (max-width: 900px) { .rail { position: static; } }
  .r-sect { margin-bottom: var(--space-6); }
  .r-label {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--accent);
    padding-bottom: 12px;
  }
  .step, .rule { display: flex; gap: 13px; padding: 11px 0; border-top: 1px solid var(--border-subtle); }
  .step-n { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--accent); flex: 0 0 auto; padding-top: 1px; }
  .check { color: var(--accent); font-size: var(--fs-caption); flex: 0 0 auto; padding-top: 2px; }
  .step-t { font-size: var(--fs-small); line-height: 1.5; color: var(--text-muted); }
  .r-note { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); margin-top: 12px; line-height: 1.6; }

  /* ── shared ── */
  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 11px 18px; border-radius: var(--radius-md); white-space: nowrap; cursor: pointer;
    transition: background var(--motion-fast) var(--ease), border-color var(--motion-fast) var(--ease);
  }
  .btn.sm { padding: 8px 14px; font-size: var(--fs-caption); }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-primary); }
  .btn.ghost:hover { background: var(--surface-overlay); border-color: var(--border-strong); }
  .btn:disabled { opacity: 0.5; cursor: not-allowed; }

  .empty-hero {
    text-align: center; padding: var(--space-8) var(--space-5);
    border: 1px dashed var(--border-default); border-radius: var(--radius-xl);
  }
  .empty-hero h2 { font-family: var(--font-display); font-size: var(--fs-h2); font-weight: var(--fw-semibold); }
  .empty-hero p { color: var(--text-muted); margin: 10px auto 0; max-width: 42ch; line-height: 1.6; }
  .empty-hero .btn { margin-top: var(--space-5); }

  .muted { color: var(--text-muted); }
  .denied {
    display: flex; flex-direction: column; align-items: center; gap: var(--space-4);
    color: var(--text-muted); padding: var(--space-8) 0; text-align: center;
  }
  .denied h1 { font-family: var(--font-display); font-size: var(--fs-h1); color: var(--text-primary); }

  @media (max-width: 640px) {
    .strip { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .cell:nth-child(3) { border-left: 0; padding-left: 0; }
    .l-prog { display: none; }
    .r-meta { margin-left: 0; }
  }
</style>
