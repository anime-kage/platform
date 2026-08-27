<script lang="ts">
  import { mediaUrl } from '$lib/media';
  import { goto } from '$app/navigation';
  import api from '$lib/api';
  import { authStore as auth } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import { displayName } from '$lib/types';
  import { sourceName } from '$lib/providers';
  import { reltime } from '$lib/reltime';
  import type {
    Anime,
    Manga,
    Episode,
    Chapter,
    ContentLink,
    TestSourceResult,
    Release,
    Subtitle
  } from '$shared/types';

  // One title's workspace: metadata, episodes/chapters, sources.
  let { data }: { data: { media: 'anime' | 'manga'; id: number } } = $props();
  const media = data.media;

  const isAdmin = $derived($auth.user?.role === 'admin');
  const canReview = $derived(!!$auth.user && ['admin', 'moderator', 'verifier'].includes($auth.user.role));

  let title = $state<Anime | Manga | null>(null);
  let notFound = $state(false);
  let episodes = $state<Episode[]>([]);
  let chapters = $state<Chapter[]>([]);
  let busy = $state(false);

  $effect(() => {
    load();
  });

  async function load() {
    try {
      const r = media === 'anime' ? await api.getAnimeById(data.id) : await api.getMangaById(data.id);
      title = r.data;
      await refreshEntries();
      if (media === 'anime') loadReleases();
    } catch {
      notFound = true;
    }
  }

  async function refreshEntries() {
    if (media === 'anime') {
      episodes = (await api.getEpisodes(data.id).catch(() => ({ data: [] }))).data
        .slice()
        .sort((a, b) => a.episodeNumber - b.episodeNumber);
    } else {
      chapters = (await api.getChapters(data.id).catch(() => ({ data: [] }))).data
        .slice()
        .sort((a, b) => parseFloat(a.chapterNumber) - parseFloat(b.chapterNumber));
    }
  }

  // ── metadata ──────────────────────────────────────────────────────────────
  let editOpen = $state(false);
  let ef = $state<Record<string, string>>({});

  function openEdit() {
    editOpen = !editOpen;
    if (!editOpen || !title) return;
    const s = title;
    const base = {
      title: s.title ?? '',
      titleEnglish: s.titleEnglish ?? '',
      year: s.year ? String(s.year) : '',
      status: s.status ?? '',
      type: s.type ?? '',
      genres: (s.genres ?? []).join(', '),
      imageUrl: s.imageUrl ?? '',
      synopsis: s.synopsis ?? ''
    };
    if (media === 'anime') {
      const a = s as Anime;
      ef = { ...base, episodes: a.episodes ? String(a.episodes) : '', studios: (a.studios ?? []).join(', ') };
    } else {
      const m = s as Manga;
      ef = {
        ...base,
        chapters: m.chapters ? String(m.chapters) : '',
        volumes: m.volumes ? String(m.volumes) : '',
        authors: (m.authors ?? []).join(', ')
      };
    }
  }

  const csv = (v: string) => v.split(',').map((x) => x.trim()).filter(Boolean);

  async function saveEdit(e: SubmitEvent) {
    e.preventDefault();
    if (!title) return;
    // empty inputs are omitted — the backend patches only what's sent
    const patch: Record<string, unknown> = {};
    if (ef.title.trim()) patch.title = ef.title.trim();
    if (ef.titleEnglish.trim()) patch.titleEnglish = ef.titleEnglish.trim();
    if (ef.year.trim()) patch.year = Number(ef.year);
    if (ef.status.trim()) patch.status = ef.status.trim();
    if (ef.type.trim()) patch.type = ef.type.trim();
    if (ef.genres.trim()) patch.genres = csv(ef.genres);
    if (ef.imageUrl.trim()) patch.imageUrl = ef.imageUrl.trim();
    if (ef.synopsis.trim()) patch.synopsis = ef.synopsis.trim();
    if (media === 'anime') {
      if (ef.episodes?.trim()) patch.episodes = Number(ef.episodes);
      if (ef.studios?.trim()) patch.studios = csv(ef.studios);
    } else {
      if (ef.chapters?.trim()) patch.chapters = Number(ef.chapters);
      if (ef.volumes?.trim()) patch.volumes = Number(ef.volumes);
      if (ef.authors?.trim()) patch.authors = csv(ef.authors);
    }
    try {
      const r = media === 'anime'
        ? await api.patchAnime(data.id, patch)
        : await api.patchManga(data.id, patch);
      title = r.data;
      editOpen = false;
      toast.success('Titlu actualizat.');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Salvarea a eșuat.');
    }
  }

  async function syncFromJikan() {
    busy = true;
    try {
      const r = media === 'anime'
        ? await api.syncAnimeFromJikan(data.id)
        : await api.syncMangaFromJikan(data.id);
      title = r.data;
      toast.success('Sincronizat din Jikan.');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Sincronizarea a eșuat.');
    } finally {
      busy = false;
    }
  }

  async function deleteTitle() {
    if (!title) return;
    if (!confirm(`Ștergi definitiv „${displayName(title)}" cu toate episoadele, sursele și listele?`)) return;
    try {
      if (media === 'anime') await api.deleteAnime(data.id);
      else await api.deleteManga(data.id);
      toast.success('Titlu șters.');
      goto('/admin');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Ștergerea a eșuat.');
    }
  }

  // ── episodes / chapters ───────────────────────────────────────────────────
  let newNumber = $state('');
  let newTitle = $state('');
  let creating = $state(false);
  let syncing = $state(false);

  /**
   * Pull episode titles / air dates / filler marks for this series from MAL.
   *
   * Reports what actually changed rather than "done": the common outcome on a
   * series that is already complete is 0/0, and an editor should be able to tell
   * that apart from a silent failure. MAL being unreachable is its own message —
   * it happens for days at a time and is worth retrying, not worth re-typing 12
   * titles by hand over.
   */
  async function syncEpisodes() {
    syncing = true;
    try {
      const r = await api.syncEpisodesFromMAL(data.id);
      if (r.added === 0 && r.updated === 0) {
        toast.info(`Nimic de schimbat — MAL listează ${r.found} episoade.`);
      } else {
        toast.success(`${r.added} adăugate, ${r.updated} completate (din ${r.found} pe MAL).`);
      }
      await refreshEntries();
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Sincronizarea a eșuat.');
    } finally {
      syncing = false;
    }
  }

  async function create(e: SubmitEvent) {
    e.preventDefault();
    creating = true;
    try {
      if (media === 'anime') {
        await api.createEpisode(data.id, {
          episodeNumber: Number(newNumber),
          title: newTitle || undefined
        });
      } else {
        await api.createChapter(data.id, {
          chapterNumber: newNumber,
          title: newTitle || undefined
        } as Partial<Chapter>);
      }
      toast.success(media === 'anime' ? 'Episod creat.' : 'Capitol creat.');
      newNumber = '';
      newTitle = '';
      await refreshEntries();
    } catch {
      toast.error('Crearea a eșuat.');
    } finally {
      creating = false;
    }
  }

  // release pipeline chips
  let releasesByEp = $state<Map<number, Release>>(new Map());

  async function loadReleases() {
    try {
      const all = (await api.getReleases(canReview ? { all: true } : undefined)).data;
      const map = new Map<number, Release>();
      for (const rel of all) {
        if (rel.animeId !== data.id || rel.episodeNumber == null) continue;
        const prev = map.get(rel.episodeNumber);
        if (!prev || new Date(rel.updatedAt) > new Date(prev.updatedAt)) map.set(rel.episodeNumber, rel);
      }
      releasesByEp = map;
    } catch {
      /* pipeline chips are decoration — the page works without them */
    }
  }

  const stateLabel: Record<Release['state'], string> = {
    draft: 'ciornă',
    in_review: 'în verificare',
    changes_requested: 'modificări cerute',
    approved: 'aprobat',
    published: 'publicat'
  };

  // ── sources ────────────────────────────────────────────────────
  let sourcesFor = $state<number | null>(null); // episode/chapter id
  let links = $state<ContentLink[]>([]);
  let linksLoading = $state(false);
  let linkUrl = $state('');
  let linkQuality = $state('1080p');
  // extract is the default: it is the only kind that plays in our own player,
  // and therefore the only one that can carry our RO subtitle and skip marks.
  // An embed is a fallback for hosts that can't be extracted.
  let linkKind = $state<'embed' | 'extract'>('extract');
  let linkProvider = $state('doodstream');
  let linkRef = $state('');
  let linkLang = $state('ro');
  let linkPriority = $state(0);
  let addingLink = $state(false);
  let testing = $state(false);
  let testResult = $state<TestSourceResult | null>(null);

  // ── the episode's own subtitle tracks ─────────────────────────────────────
  // Here rather than only on /admin/subtitles because this is where an episode
  // that never went through the translation pipeline gets assembled: you add the
  // source and the track in one place, in one sitting.
  let epSubs = $state<Subtitle[]>([]);
  let subsLoading = $state(false);
  let subFile = $state<File | null>(null);
  let subLang = $state('ro');
  let subUploading = $state(false);
  let subBusyId = $state<number | null>(null);

  const SUB_LANG: Record<string, string> = { ro: 'Română', en: 'Engleză', ja: 'Japoneză' };

  async function refreshSubs(episodeId: number) {
    if (media !== 'anime') return;
    subsLoading = true;
    try {
      epSubs = (await api.getEpisodeSubtitles(episodeId)).data;
    } catch {
      epSubs = [];
    } finally {
      subsLoading = false;
    }
  }

  async function uploadSub(episodeId: number) {
    if (!subFile) return;
    subUploading = true;
    try {
      const r = await api.uploadEpisodeSubtitle(episodeId, subFile, { language: subLang });
      toast.success(`Subtitrare atașată — ${r.cues} replici.`);
      subFile = null;
      const el = document.querySelector<HTMLInputElement>('.subform input[type=file]');
      if (el) el.value = '';
      await refreshSubs(episodeId);
    } catch (err) {
      toast.error(
        (err as { error?: string }).error ?? (err as { message?: string }).message ?? 'Încărcarea a eșuat.'
      );
    } finally {
      subUploading = false;
    }
  }

  async function removeSub(s: Subtitle, episodeId: number) {
    if (!confirm(`Ștergi pista ${SUB_LANG[s.language] ?? s.language}?`)) return;
    subBusyId = s.id;
    try {
      await api.deleteSubtitle(s.id);
      toast.success('Pistă ștearsă.');
      await refreshSubs(episodeId);
    } catch {
      toast.error('Ștergerea a eșuat.');
    } finally {
      subBusyId = null;
    }
  }

  // per media: which extract providers make sense. `doodstream` first — it is
  // the one verified to send CORS, which is what our own player needs; a host
  // without it can only ever be an `embed` (no RO subtitle, no skip intro).
  const providers = media === 'anime' ? ['doodstream', 'direct', 'filemoon'] : ['mangadex'];

  // For every provider except MangaDex the reference *is* a URL, so asking for
  // both a "source URL" and a "provider reference" was two fields for one
  // value. One field, reused as both.
  const refIsUrl = $derived(linkKind === 'extract' && linkProvider !== 'mangadex');
  const urlHint = $derived(
    linkKind === 'embed'
      ? 'https://host.exemplu/e/cod-fișier (pagina de player)'
      : linkProvider === 'doodstream'
        ? 'https://playmogo.com/e/cod-fișier'
        : linkProvider === 'filemoon'
          ? 'https://filemoon.org/en/cod-fișier/embed'
          : linkProvider === 'direct'
            ? 'https://cdn.exemplu.ro/video.mp4 (fișierul în sine)'
            : 'https://mangadex.org/chapter/… (pagina capitolului)'
  );

  async function openSources(id: number) {
    if (sourcesFor === id) {
      sourcesFor = null;
      return;
    }
    sourcesFor = id;
    pagesFor = null;
    testResult = null;
    linkProvider = providers[0];
    // Reset per-episode subtitle state so an open panel never shows the previous
    // episode's tracks while the new ones load.
    epSubs = [];
    subFile = null;
    await refreshLinks(id);
    await refreshSubs(id);
  }

  // ── own scanlation pages: upload to R2/local or paste URLs ─────
  let pagesFor = $state<number | null>(null);
  let pagesLang = $state('ro');
  let pagesInfo = $state<{ language: string; languages: string[]; pages: string[] } | null>(null);
  let pagesLoading = $state(false);
  let uploadingPages = $state(false);
  let pasteUrls = $state('');

  async function openPages(id: number) {
    if (pagesFor === id) {
      pagesFor = null;
      return;
    }
    pagesFor = id;
    sourcesFor = null;
    pagesLang = 'ro';
    pasteUrls = '';
    await refreshPages(id);
  }

  async function refreshPages(id: number) {
    pagesLoading = true;
    try {
      const r = await api.getChapterPages(id, pagesLang);
      pagesInfo = r.data;
    } catch {
      pagesInfo = null;
    } finally {
      pagesLoading = false;
    }
  }

  async function uploadPages(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const files = Array.from(input.files ?? []);
    if (!files.length || pagesFor == null) return;
    uploadingPages = true;
    try {
      const r = await api.uploadChapterPages(pagesFor, pagesLang, files);
      toast.success(`${r.count} pagini încărcate (${r.storage === 'r2' ? 'R2' : 'stocare locală'}).`);
      await refreshPages(pagesFor);
      await refreshEntries();
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Încărcarea a eșuat.');
    } finally {
      uploadingPages = false;
      input.value = '';
    }
  }

  async function savePastedPages() {
    if (pagesFor == null) return;
    const urls = pasteUrls.split('\n').map((s) => s.trim()).filter(Boolean);
    if (!urls.length) return;
    try {
      await api.setChapterPages(pagesFor, { language: pagesLang, urls });
      toast.success(`${urls.length} pagini salvate.`);
      pasteUrls = '';
      await refreshPages(pagesFor);
      await refreshEntries();
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Salvarea a eșuat.');
    }
  }

  async function removePagesEdition() {
    if (pagesFor == null) return;
    if (!confirm(`Ștergi ediția „${pagesLang}" a acestui capitol?`)) return;
    try {
      await api.deleteChapterPages(pagesFor, pagesLang);
      toast.success('Ediție ștearsă.');
      await refreshPages(pagesFor);
      await refreshEntries();
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Ștergerea a eșuat.');
    }
  }

  async function refreshLinks(id: number) {
    linksLoading = true;
    try {
      if (media === 'anime') {
        links = (await api.getAdminEpisodeLinks(id)).data;
      } else {
        // chapters have no admin listing yet — show the active ones
        const ch = chapters.find((c) => c.id === id);
        links = ch?.links ?? [];
      }
    } catch {
      links = [];
      toast.error('Sursele nu au putut fi încărcate.');
    } finally {
      linksLoading = false;
    }
  }

  async function testCurrent() {
    testing = true;
    testResult = null;
    try {
      testResult = (
        await api.testSource(
          linkKind === 'extract'
            ? { kind: 'extract', provider: linkProvider, providerRef: refIsUrl ? linkUrl : linkRef }
            : { kind: 'embed', hostingUrl: linkUrl }
        )
      ).data;
    } catch (err) {
      testResult = { ok: false, message: err instanceof Error ? err.message : 'Testul a eșuat.' };
    } finally {
      testing = false;
    }
  }

  async function addLink(e: SubmitEvent) {
    e.preventDefault();
    if (sourcesFor == null) return;
    addingLink = true;
    try {
      const payload = {
        hostingUrl: linkUrl,
        quality: linkQuality,
        language: linkLang,
        kind: linkKind,
        priority: linkPriority,
        ...(linkKind === 'extract'
          ? { provider: linkProvider, providerRef: refIsUrl ? linkUrl : linkRef }
          : {})
      };
      if (media === 'anime') {
        await api.addEpisodeLink(sourcesFor, payload);
      } else {
        await api.addChapterLink(sourcesFor, payload);
      }
      toast.success('Sursă adăugată.');
      linkUrl = '';
      linkRef = '';
      testResult = null;
      await refreshEntries();
      await refreshLinks(sourcesFor);
    } catch {
      toast.error('Adăugarea sursei a eșuat.');
    } finally {
      addingLink = false;
    }
  }

  async function toggleLink(l: ContentLink) {
    try {
      await api.updateContentLink(l.id, { isActive: !l.isActive });
      if (sourcesFor != null) await refreshLinks(sourcesFor);
    } catch {
      toast.error('Actualizarea a eșuat.');
    }
  }

  async function bumpPriority(l: ContentLink, delta: number) {
    try {
      await api.updateContentLink(l.id, { priority: l.priority + delta });
      if (sourcesFor != null) await refreshLinks(sourcesFor);
    } catch {
      toast.error('Actualizarea a eșuat.');
    }
  }

  async function removeLink(l: ContentLink) {
    if (!confirm('Ștergi definitiv această sursă?')) return;
    try {
      await api.deleteContentLink(l.id);
      if (sourcesFor != null) await refreshLinks(sourcesFor);
      await refreshEntries();
    } catch {
      toast.error('Ștergerea a eșuat.');
    }
  }

  async function testExisting(l: ContentLink) {
    try {
      const res = (
        await api.testSource(
          l.kind === 'extract'
            ? { kind: 'extract', provider: l.provider, providerRef: l.providerRef }
            : { kind: 'embed', hostingUrl: l.hostingUrl }
        )
      ).data;
      if (res.ok) toast.success('Sursa răspunde.');
      else toast.error(res.message ?? 'Sursa nu răspunde.');
    } catch {
      toast.error('Testul a eșuat.');
    }
  }

  /* Plain words, not jargon. "moartă" was the worst of it: it is set by the
     nightly health checker, so a source it has never reached — or one it could
     not resolve because its provider image was stale — read as broken when it
     played fine. */
  function healthBadge(l: ContentLink): { label: string; cls: string; hint: string } {
    if (!l.isActive)
      return { label: 'Oprită', cls: 'off', hint: 'Ascunsă de la membri. Nu e ștearsă.' };
    if (l.lastOk === false)
      return { label: 'Nu răspunde', cls: 'dead', hint: 'Ultima verificare a eșuat. Se sare peste ea la redare.' };
    if (l.lastOk === true)
      return { label: 'Funcțională', cls: 'ok', hint: 'Ultima verificare a mers.' };
    return { label: 'Neverificată', cls: 'unknown', hint: 'Încă nu a trecut verificarea automată.' };
  }

  /* extract/embed is the single most important fact about a source here, and
     the two words meant nothing to anyone who had not read the architecture:
     one plays in our player (so RO subtitles and skip-intro work), the other is
     someone else's iframe (so neither does). */
  const kindBadge = (l: ContentLink) =>
    l.kind === 'extract'
      ? { label: 'Player propriu', cls: 'own', hint: 'Se redă în playerul Anime-Kage — subtitrare RO și skip intro/outro funcționează.' }
      : { label: 'Player extern', cls: 'ext', hint: 'Se redă în player-ul gazdei (iframe) — fără subtitrarea noastră și fără skip intro.' };

  /** The small print under the name: the ref or path, quality, language. */
  const srcDetail = (l: ContentLink) =>
    [l.providerRef ?? (() => { try { return new URL(l.hostingUrl).pathname; } catch { return l.hostingUrl; } })(),
     l.quality, l.language?.toUpperCase()].filter(Boolean).join(' · ');

  const chapNum = (n: string) => String(parseFloat(n));
</script>

<svelte:head>
  <title>{title ? `${displayName(title)} · Administrare` : 'Administrare'} · Anime-Kage</title>
</svelte:head>

{#if notFound}
  <div class="empty">
    <p>Titlul nu există (sau a fost șters).</p>
    <a class="crumb" href="/admin/catalog">← Înapoi la catalog</a>
  </div>
{:else if !title}
  <p class="muted">Se încarcă…</p>
{:else}
  <a class="crumb" href="/admin/catalog">← Catalog</a>

  <header class="head">
    <span class="poster" style={title.imageUrl ? `background-image:url(${mediaUrl(title.imageUrl)})` : ''}></span>
    <div class="head-main">
      <h2>{displayName(title)}</h2>
      <p class="meta">
        {title.year ?? '—'} · {title.type ?? '—'} · {title.status ?? '—'}
        {#if title.malId} · MAL {title.malId}{/if}
      </p>
      <p class="meta faint">
        {media === 'anime' ? `${episodes.length} episoade` : `${chapters.length} capitole`} în baza de date
      </p>
    </div>
    <div class="tools">
      <button class="btn ghost sm" onclick={openEdit}>{editOpen ? 'Închide editarea' : 'Editează'}</button>
      <button class="btn ghost sm" disabled={busy} onclick={syncFromJikan}>Sincronizează Jikan</button>
      {#if isAdmin}
        <button class="btn ghost sm danger" onclick={deleteTitle}>Șterge titlul</button>
      {/if}
    </div>
  </header>

  {#if editOpen}
    <form class="card editform" onsubmit={saveEdit}>
      <span class="kicker">Editare manuală</span>
      <p class="muted note">Câmpurile goale rămân neschimbate; sincronizarea Jikan le suprascrie.</p>
      <div class="frow">
        <label class="grow"><span class="lbl">Titlu</span><input bind:value={ef.title} /></label>
        <label class="grow"><span class="lbl">Titlu EN</span><input bind:value={ef.titleEnglish} /></label>
        <label><span class="lbl">An</span><input bind:value={ef.year} inputmode="numeric" class="narrow" /></label>
      </div>
      <div class="frow">
        <label><span class="lbl">Status</span><input bind:value={ef.status} placeholder={media === 'anime' ? 'airing / completed' : 'publishing / completed'} /></label>
        <label><span class="lbl">Tip</span><input bind:value={ef.type} placeholder={media === 'anime' ? 'tv / movie / ova' : 'manga / manhwa'} /></label>
        {#if media === 'anime'}
          <label><span class="lbl">Episoade</span><input bind:value={ef.episodes} inputmode="numeric" class="narrow" /></label>
        {:else}
          <label><span class="lbl">Capitole</span><input bind:value={ef.chapters} inputmode="numeric" class="narrow" /></label>
          <label><span class="lbl">Volume</span><input bind:value={ef.volumes} inputmode="numeric" class="narrow" /></label>
        {/if}
      </div>
      <div class="frow">
        <label class="grow"><span class="lbl">Genuri (virgulă)</span><input bind:value={ef.genres} /></label>
        {#if media === 'anime'}
          <label class="grow"><span class="lbl">Studiouri</span><input bind:value={ef.studios} /></label>
        {:else}
          <label class="grow"><span class="lbl">Autori</span><input bind:value={ef.authors} /></label>
        {/if}
      </div>
      <div class="frow">
        <label class="grow"><span class="lbl">Imagine (URL)</span><input bind:value={ef.imageUrl} /></label>
      </div>
      <div class="frow">
        <label class="grow"><span class="lbl">Sinopsis</span><textarea rows="4" bind:value={ef.synopsis}></textarea></label>
      </div>
      <div class="frow end">
        <button class="btn fill" type="submit">Salvează modificările</button>
      </div>
    </form>
  {/if}

  <section class="block">
    <div class="block-head">
      <span class="kicker">{media === 'anime' ? 'Episoade' : 'Capitole'}</span>
      <form class="addform" onsubmit={create}>
        <input required bind:value={newNumber} inputmode="decimal" placeholder="Nr." class="nr" aria-label="Număr" />
        <input bind:value={newTitle} placeholder="Titlu (opțional)" aria-label="Titlu" />
        <button class="btn ghost sm" type="submit" disabled={creating}>{creating ? '…' : '+ Adaugă'}</button>
        {#if media === 'anime'}
          <!-- Fills titles, air dates and filler marks from MAL. Needed because
               the nightly job only polls airing/upcoming series, so a completed
               one added by hand never gets them. -->
          <button
            class="btn ghost sm"
            type="button"
            onclick={syncEpisodes}
            disabled={syncing}
            title="Ia titlurile, datele și marcajele filler de pe MyAnimeList"
          >
            {syncing ? 'Se sincronizează…' : '↻ Sincronizează de pe MAL'}
          </button>
        {/if}
      </form>
    </div>

    {#if (media === 'anime' ? episodes : chapters).length === 0}
      <div class="empty slim"><p class="muted">Niciun {media === 'anime' ? 'episod' : 'capitol'} încă — adaugă primul mai sus.</p></div>
    {/if}

    <div class="entries">
      {#each media === 'anime' ? episodes : chapters as entry (entry.id)}
        <div class="entry" class:open={sourcesFor === entry.id}>
          <span class="e-n">{media === 'anime' ? (entry as Episode).episodeNumber : chapNum((entry as Chapter).chapterNumber)}</span>
          <span class="e-main">
            <span class="e-t" class:filler={media === 'anime' && (entry as Episode).isFiller}>
              {entry.title ?? (media === 'anime'
                ? `Episodul ${(entry as Episode).episodeNumber}`
                : `Capitolul ${chapNum((entry as Chapter).chapterNumber)}`)}
              {#if media === 'anime' && (entry as Episode).isFiller}<span class="e-tag">(filler)</span>
              {:else if media === 'anime' && (entry as Episode).isRecap}<span class="e-tag">(recap)</span>{/if}
            </span>
            <span class="e-m">
              {entry.links?.length ?? 0} surse active
              {#if media === 'anime' && releasesByEp.has((entry as Episode).episodeNumber)}
                {@const rel = releasesByEp.get((entry as Episode).episodeNumber)!}
                · <a class="relchip s-{rel.state}" href={canReview ? `/verify/${rel.id}` : `/translate/${rel.id}`} title="Deschide release-ul în pipeline">
                  sub: {stateLabel[rel.state]}
                </a>
              {/if}
            </span>
          </span>
          {#if media === 'manga'}
            <button class="btn ghost sm" onclick={() => openPages(entry.id)}>
              {pagesFor === entry.id ? 'Închide' : 'Pagini'}
            </button>
          {/if}
          <button class="btn ghost sm" onclick={() => openSources(entry.id)}>
            {sourcesFor === entry.id ? 'Închide' : 'Surse'}
          </button>
        </div>

        {#if media === 'manga' && pagesFor === entry.id}
          <div class="srcpanel">
            {#if pagesLoading}
              <p class="muted">Se încarcă…</p>
            {:else}
              <div class="pg-head">
                <select bind:value={pagesLang} onchange={() => refreshPages(entry.id)} title="Ediția">
                  <option value="ro">ro</option><option value="en">en</option>
                </select>
                <span class="s-m">
                  {#if pagesInfo && pagesInfo.pages.length}
                    ediția „{pagesInfo.language}": {pagesInfo.pages.length} pagini
                  {:else}
                    nicio pagină încă
                  {/if}
                  {#if pagesInfo?.languages.length}
                    · ediții existente: {pagesInfo.languages.join(', ')}
                  {/if}
                </span>
                {#if pagesInfo?.languages.includes(pagesLang)}
                  <button class="btn ghost sm danger" onclick={removePagesEdition}>Șterge ediția</button>
                {/if}
              </div>

              {#if pagesInfo && pagesInfo.pages.length && pagesInfo.language === pagesLang}
                <div class="pg-thumbs">
                  {#each pagesInfo.pages.slice(0, 8) as p, i (p)}
                    <img src={p} alt={`pagina ${i + 1}`} loading="lazy" />
                  {/each}
                  {#if pagesInfo.pages.length > 8}<span class="s-m">+{pagesInfo.pages.length - 8}</span>{/if}
                </div>
              {/if}

              <label class="btn ghost sm upl">
                {uploadingPages ? 'Se încarcă…' : '↑ Încarcă imaginile paginilor (ordonate după numele fișierului)'}
                <input type="file" accept="image/*" multiple onchange={uploadPages} disabled={uploadingPages} />
              </label>

              <div class="pasterow">
                <textarea
                  rows="3"
                  bind:value={pasteUrls}
                  placeholder="…sau lipește URL-uri deja găzduite (unul pe linie, https)"
                ></textarea>
                <button class="btn ghost sm" onclick={savePastedPages} disabled={!pasteUrls.trim()}>
                  Salvează URL-urile
                </button>
              </div>
              <p class="s-m">Încărcarea sau salvarea înlocuiește complet ediția „{pagesLang}".</p>
            {/if}
          </div>
        {/if}

        {#if sourcesFor === entry.id}
          <div class="srcpanel">
            {#if linksLoading}
              <p class="muted">Se încarcă…</p>
            {:else}
              <p class="src-help">
                Sursele se încearcă <strong>în ordinea de mai jos</strong> — prima care
                răspunde e cea redată. Săgețile schimbă ordinea.
              </p>
              {#each links as l, i (l.id)}
                {@const b = healthBadge(l)}
                {@const k = kindBadge(l)}
                <div class="srcrow" class:inactive={!l.isActive}>
                  <!-- Position, not the raw priority number. "200" told nobody
                       anything; "1st of 12" is the whole point of the field. -->
                  <span class="rank">
                    <button class="mini" title="Mută mai sus" disabled={i === 0}
                      onclick={() => bumpPriority(l, 1)}>▲</button>
                    <span class="rank-n">{i + 1}</span>
                    <button class="mini" title="Mută mai jos" disabled={i === links.length - 1}
                      onclick={() => bumpPriority(l, -1)}>▼</button>
                  </span>

                  <span class="s-main">
                    <span class="s-head">
                      <span class="s-name">{sourceName(l, i)}</span>
                      <span class="tag {k.cls}" title={k.hint}>{k.label}</span>
                      <span class="dot {b.cls}" title={b.hint}></span>
                      <span class="s-state {b.cls}" title={b.hint}>{b.label}</span>
                    </span>
                    <span class="s-m" title={l.hostingUrl}>
                      {srcDetail(l)}
                      <span class="s-when">
                        · {l.lastCheckedAt ? `verificat ${reltime(l.lastCheckedAt)}` : 'neverificată'}
                      </span>
                    </span>
                  </span>

                  <span class="s-actions">
                    <button class="btn ghost sm" onclick={() => testExisting(l)}>Testează</button>
                    <button class="btn ghost sm" onclick={() => toggleLink(l)}
                      title={l.isActive ? 'Ascunde sursa de la membri' : 'Fă sursa vizibilă din nou'}
                      >{l.isActive ? 'Oprește' : 'Pornește'}</button>
                    <button class="btn ghost sm danger" title="Șterge definitiv"
                      onclick={() => removeLink(l)}>✕</button>
                  </span>
                </div>
              {:else}
                <p class="muted">Nicio sursă încă.</p>
              {/each}
            {/if}

            <form class="linkform" onsubmit={addLink}>
              <span class="lbl">Sursă nouă</span>
              <div class="lf-row">
                <select bind:value={linkKind} title="Tipul sursei">
                  <option value="extract">
                    {media === 'anime' ? 'Playerul nostru — cu subtitrare RO' : 'Readerul nostru'}
                  </option>
                  <option value="embed">
                    {media === 'anime' ? 'Iframe de la host — fără subtitrarea noastră' : 'Iframe de la host'}
                  </option>
                </select>
                <input required type="url" bind:value={linkUrl} placeholder={urlHint} />
                {#if media === 'anime'}
                  <select bind:value={linkQuality} title="Calitate">
                    <option>1080p</option><option>720p</option><option>480p</option>
                  </select>
                {:else}
                  <select bind:value={linkLang} title="Limbă">
                    <option value="ro">ro</option><option value="en">en</option>
                  </select>
                {/if}
                <input class="prio-in" type="number" bind:value={linkPriority} title="Prioritate" />
              </div>
              {#if linkKind === 'extract'}
                <div class="lf-row">
                  <select bind:value={linkProvider} title="Provider">
                    {#each providers as p (p)}<option value={p}>{p}</option>{/each}
                  </select>
                  {#if !refIsUrl}
                    <input required bind:value={linkRef} placeholder="ID-ul capitolului MangaDex (uuid)" />
                  {/if}
                </div>
                {#if media === 'anime'}
                  <p class="lf-note">
                    Doar hosturile care trimit antetul CORS pot fi redate în playerul nostru —
                    acolo ajung subtitrarea RO și marcajele de intro. Apasă <strong>Testează</strong>
                    înainte de a salva.
                  </p>
                {/if}
              {/if}
              <div class="lf-row end">
                {#if testResult}
                  <span class="pill {testResult.ok ? 'ok' : 'dead'}">
                    {testResult.ok ? (testResult.manifestUrl ? `ok · ${testResult.streamKind}` : 'ok') : (testResult.message ?? 'a eșuat')}
                  </span>
                {/if}
                <button class="btn ghost sm" type="button" onclick={testCurrent} disabled={testing}>
                  {testing ? '…' : 'Testează'}
                </button>
                <button class="btn fill sm" type="submit" disabled={addingLink}>{addingLink ? '…' : 'Salvează'}</button>
              </div>
            </form>

            {#if media === 'anime'}
              <!-- The episode's own subtitle, attached here rather than only
                   through the translation pipeline: an episode translated
                   elsewhere (or before this database existed) still needs its
                   track, and this is where its source is being set up anyway. -->
              <form class="subform" onsubmit={(e) => { e.preventDefault(); uploadSub(entry.id); }}>
                <span class="lbl">Subtitrarea noastră</span>

                {#if subsLoading}
                  <p class="muted">Se încarcă…</p>
                {:else if epSubs.length}
                  <div class="subtracks">
                    {#each epSubs as s (s.id)}
                      <span class="subtrack">
                        <span class="st-lang">{SUB_LANG[s.language] ?? s.language}</span>
                        <span class="st-fmt">{s.format.toUpperCase()}</span>
                        <button
                          type="button"
                          class="st-del"
                          disabled={subBusyId === s.id}
                          onclick={() => removeSub(s, entry.id)}
                        >{subBusyId === s.id ? '…' : '×'}</button>
                      </span>
                    {/each}
                  </div>
                {/if}

                <div class="lf-row">
                  <input
                    type="file"
                    accept=".srt,.ass,.ssa,.vtt"
                    onchange={(e) => (subFile = (e.currentTarget as HTMLInputElement).files?.[0] ?? null)}
                  />
                  <select bind:value={subLang} title="Limbă">
                    <option value="ro">ro</option>
                    <option value="en">en</option>
                    <option value="ja">ja</option>
                  </select>
                  <button class="btn fill sm" type="submit" disabled={subUploading || !subFile}>
                    {subUploading ? '…' : 'Atașează'}
                  </button>
                </div>
                <p class="lf-note">
                  .srt, .ass sau .vtt — se convertește în WebVTT la încărcare, fiindcă
                  playerul nu redă altceva. Reîncărcarea aceleiași limbi înlocuiește pista.
                  Se afișează doar pentru sursele <strong>redate în playerul nostru</strong>;
                  un <code>embed</code> nu poate purta subtitrarea noastră.
                </p>
              </form>
            {/if}
          </div>
        {/if}
      {/each}
    </div>
  </section>
{/if}

<style>
  .crumb {
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
    display: inline-block; margin-bottom: var(--space-4);
  }
  .crumb:hover { color: var(--text-primary); }

  .head { display: flex; gap: var(--space-4); align-items: center; flex-wrap: wrap; margin-bottom: var(--space-5); }
  .poster {
    width: 56px; height: 80px; border-radius: var(--radius-sm); flex: 0 0 auto;
    background-color: var(--surface-overlay); background-size: cover; background-position: center;
  }
  .head-main { flex: 1; min-width: 220px; }
  h2 { font-family: var(--font-display); font-size: var(--fs-h2); line-height: var(--lh-snug); }
  .meta { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); margin-top: 5px; }
  .meta.faint { color: var(--text-muted); margin-top: 2px; }
  .tools { display: flex; gap: 8px; flex-wrap: wrap; }

  .card {
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg); padding: var(--space-4) var(--space-5) var(--space-5);
  }
  .editform { margin-bottom: var(--space-5); }
  .editform .kicker { display: block; }
  .note { font-size: var(--fs-small); margin: 6px 0 var(--space-4); }
  .frow { display: flex; gap: var(--space-3); align-items: flex-end; flex-wrap: wrap; }
  .frow + .frow { margin-top: var(--space-3); }
  .frow.end { justify-content: flex-end; }
  label { display: flex; flex-direction: column; gap: 6px; }
  label.grow { flex: 1; min-width: 200px; }
  .lbl {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-muted);
  }
  .frow input, .frow textarea {
    min-height: 42px; padding: 0 12px; min-width: 90px;
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary); outline: none;
    font-size: var(--fs-small);
  }
  .frow input.narrow { max-width: 100px; }
  .frow textarea { width: 100%; padding: 10px 12px; resize: vertical; line-height: 1.5; }
  .frow input:focus, .frow textarea:focus { border-color: var(--accent); }

  .block { margin-top: var(--space-6); }
  .block-head {
    display: flex; align-items: center; justify-content: space-between;
    gap: var(--space-4); flex-wrap: wrap; margin-bottom: var(--space-3);
  }
  .addform { display: flex; gap: 8px; flex-wrap: wrap; }
  .addform input {
    min-height: 38px; padding: 0 12px;
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary); outline: none;
    font-size: var(--fs-small);
  }
  .addform input:focus { border-color: var(--accent); }
  .addform input.nr { width: 72px; }

  .entries { display: flex; flex-direction: column; gap: 6px; }
  .entry {
    display: flex; align-items: center; gap: var(--space-4);
    padding: 10px 14px; border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
    background: var(--surface-raised);
  }
  .entry.open { border-color: var(--border-strong); border-bottom-left-radius: 0; border-bottom-right-radius: 0; }
  .e-n {
    font-family: var(--font-display); font-size: 1.1rem; font-weight: var(--fw-semibold);
    color: var(--text-muted); min-width: 2.4ch; text-align: right;
  }
  .e-main { flex: 1; min-width: 0; display: flex; flex-direction: column; }
  .e-t { font-size: var(--fs-small); font-weight: var(--fw-semibold); color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .e-t.filler { color: var(--danger); }
  .e-tag { font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-regular); margin-left: 5px; }
  .e-m { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); margin-top: 2px; }
  .relchip { font-weight: var(--fw-semibold); color: var(--text-muted); text-decoration: underline; }
  .relchip.s-in_review { color: var(--warning); }
  .relchip.s-changes_requested { color: var(--danger); }
  .relchip.s-approved, .relchip.s-published { color: var(--success); }

  .srcpanel {
    display: flex; flex-direction: column; gap: 6px;
    padding: 12px 14px; margin-top: -6px; margin-bottom: 4px;
    border: 1px solid var(--border-strong); border-top: none;
    border-radius: 0 0 var(--radius-md) var(--radius-md);
    background: var(--surface-inset);
  }
  /* One sentence of context beats four columns of jargon: the whole model is
     "tried in order, first one that answers wins". */
  .src-help {
    font-size: var(--fs-caption); color: var(--text-muted); line-height: 1.5;
    padding-bottom: 4px;
  }
  .src-help strong { color: var(--text-primary); font-weight: var(--fw-semibold); }

  .srcrow {
    display: flex; align-items: center; gap: 12px; flex-wrap: wrap;
    padding: 10px 12px; border: 1px solid var(--border-subtle); border-radius: var(--radius-sm);
    background: var(--surface-raised);
  }
  .srcrow.inactive { opacity: 0.55; }

  /* rank: the arrows read as "move", and the number is the position */
  .rank { display: flex; flex-direction: column; align-items: center; gap: 1px; flex: 0 0 auto; }
  .rank-n {
    font-family: var(--font-display); font-size: var(--fs-body); font-weight: var(--fw-semibold);
    color: var(--text-muted); line-height: 1; min-width: 1.5ch; text-align: center;
  }

  .s-main { flex: 1; min-width: 220px; display: flex; flex-direction: column; gap: 4px; }
  .s-head { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
  .s-name {
    font-family: var(--font-display); font-size: var(--fs-body);
    font-weight: var(--fw-semibold); color: var(--text-primary);
  }
  /* the status is a dot plus the word — colour alone is not a label */
  .dot { width: 7px; height: 7px; border-radius: 50%; flex: 0 0 auto; }
  .dot.ok { background: var(--success); }
  .dot.dead { background: var(--danger); }
  .dot.unknown, .dot.off { background: var(--text-faint); }
  .s-state { font-size: var(--fs-micro); font-weight: var(--fw-semibold); }
  .s-state.ok { color: var(--success); }
  .s-state.dead { color: var(--danger); }
  .s-state.unknown, .s-state.off { color: var(--text-muted); }

  /* which player this source ends up in — the fact that actually matters */
  .tag {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.06em; padding: 2px 8px; border-radius: var(--radius-pill);
    white-space: nowrap; cursor: help;
  }
  .tag.own { background: color-mix(in srgb, var(--accent) 16%, transparent); color: var(--accent); }
  .tag.ext { background: var(--surface-overlay); color: var(--text-muted); }

  .s-m {
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 60ch;
  }
  .s-when { color: var(--text-faint); }
  .s-actions { display: flex; gap: 6px; flex-wrap: wrap; margin-left: auto; }
  .mini {
    border: 1px solid var(--border-default); background: var(--surface-raised);
    color: var(--text-muted); border-radius: var(--radius-sm);
    font-size: var(--fs-micro); padding: 3px 6px; cursor: pointer;
  }
  .mini:hover:not(:disabled) { color: var(--text-primary); background: var(--surface-overlay); }
  .mini:disabled { opacity: 0.3; cursor: default; }

  .pill {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.06em; text-transform: uppercase;
    padding: 3px 10px; border-radius: var(--radius-pill); white-space: nowrap;
  }
  .pill.ok { background: color-mix(in srgb, var(--success) 14%, transparent); color: var(--success); }
  .pill.dead { background: color-mix(in srgb, var(--danger) 14%, transparent); color: var(--danger); }
  .pill.unknown { background: var(--surface-overlay); color: var(--text-muted); }
  .pill.off { background: var(--surface-overlay); color: var(--text-muted); }

  .linkform,
  .subform {
    display: flex; flex-direction: column; gap: 8px;
    padding-top: 10px; border-top: 1px dashed var(--border-default);
  }
  .subform { margin-top: 10px; }
  /* Inputs and selects inside the subtitle form get the link form's treatment;
     the two sit in the same panel and should not look like different eras. */
  .subform input[type='file'] {
    flex: 1; min-width: 220px; font-size: var(--fs-caption); color: var(--text-muted);
  }
  .subform select {
    min-height: 40px; padding: 0 10px; cursor: pointer;
    background: var(--surface-raised); border: 1px solid var(--border-default);
    border-radius: var(--radius-sm); color: var(--text-primary);
    font-family: var(--font-mono); font-size: var(--fs-caption);
  }
  .subtracks { display: flex; gap: 8px; flex-wrap: wrap; }
  .subtrack {
    display: inline-flex; align-items: center; gap: 8px; padding: 5px 8px;
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm); font-size: var(--fs-caption);
  }
  .st-lang { color: var(--text-primary); }
  .st-fmt { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .st-del {
    background: none; border: none; cursor: pointer; padding: 0 2px;
    color: var(--text-muted); font-size: 1rem; line-height: 1;
  }
  .st-del:hover:not(:disabled) { color: var(--danger, #e5484d); }
  .st-del:disabled { cursor: wait; opacity: 0.6; }
  .lf-row { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
  .lf-note {
    margin-top: 8px; max-width: 62ch;
    font-size: var(--fs-micro); color: var(--text-muted); line-height: 1.55;
  }
  .lf-note strong { color: var(--text-primary); font-weight: var(--fw-semibold); }
  .lf-row.end { justify-content: flex-end; }
  .linkform input {
    flex: 1; min-width: 220px; min-height: 40px; padding: 0 12px;
    background: var(--surface-raised); border: 1px solid var(--border-default);
    border-radius: var(--radius-sm); color: var(--text-primary); outline: none;
    font-family: var(--font-mono); font-size: var(--fs-caption);
  }
  .linkform input:focus { border-color: var(--accent); }
  .linkform input.prio-in { flex: 0 0 76px; min-width: 76px; }
  .linkform select {
    min-height: 40px; padding: 0 10px;
    background: var(--surface-raised); border: 1px solid var(--border-default);
    border-radius: var(--radius-sm); color: var(--text-primary); cursor: pointer;
  }

  /* chapter pages manager (PLAN 5.7) */
  .pg-head { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .pg-head select {
    min-height: 36px; padding: 0 10px;
    background: var(--surface-raised); border: 1px solid var(--border-default);
    border-radius: var(--radius-sm); color: var(--text-primary); cursor: pointer;
  }
  .pg-thumbs { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
  .pg-thumbs img {
    width: 46px; height: 66px; object-fit: cover;
    border: 1px solid var(--border-default); border-radius: var(--radius-sm);
    background: var(--surface-overlay);
  }
  .upl { position: relative; overflow: hidden; text-align: center; }
  .upl input { position: absolute; inset: 0; opacity: 0; cursor: pointer; }
  .pasterow { display: flex; gap: 8px; align-items: flex-end; flex-wrap: wrap; }
  .pasterow textarea {
    flex: 1; min-width: 240px; padding: 10px 12px; resize: vertical;
    background: var(--surface-raised); border: 1px solid var(--border-default);
    border-radius: var(--radius-sm); color: var(--text-primary); outline: none;
    font-family: var(--font-mono); font-size: var(--fs-caption); line-height: 1.5;
  }
  .pasterow textarea:focus { border-color: var(--accent); }

  .empty {
    border: 1px dashed var(--border-default); border-radius: var(--radius-md);
    padding: var(--space-6); text-align: center; color: var(--text-primary);
  }
  .empty.slim { padding: var(--space-4); }
  .empty .crumb { margin: var(--space-3) 0 0; }
  .muted { color: var(--text-muted); }

  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 10px 18px; border-radius: var(--radius-md); white-space: nowrap; cursor: pointer;
  }
  .btn.sm { padding: 7px 13px; font-size: var(--fs-caption); }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-primary); }
  .btn.ghost:hover { background: var(--surface-overlay); }
  .btn.ghost.danger { color: var(--danger); border-color: color-mix(in srgb, var(--danger) 40%, transparent); }
  .btn.ghost.danger:hover { background: color-mix(in srgb, var(--danger) 12%, transparent); }
  .btn:disabled { opacity: 0.6; cursor: wait; }
</style>
