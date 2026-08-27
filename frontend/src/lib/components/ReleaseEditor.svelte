<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte';
  import { goto } from '$app/navigation';
  import { api } from '$lib/api';
  import { authStore as auth } from '$lib/stores/auth';
  import Person from '$lib/components/Person.svelte';
  import { toast } from '$lib/stores/toast';
  import type { Release, SubtitleEvent } from '$shared/types';

  // One editor, two jobs (reworked again in Phase 8 round 3
  // from the Editor Traducere sketch):
  //  - mode 'translate' — the translator's workbench (/translate/[id])
  //  - mode 'verify'    — the reviewer's gate (/verify/[id]): fix in place,
  //    then approve or send back with notes
  // Layout: a sticky workbar (title, autosave state, progress, submit) over a
  // two-pane workspace — video rail on the left, the subtitle grid on the
  // right, filterable by tabs (Toate / Netraduse / Atenție).
  // Manga releases swap the grid for a page flipper: the pages
  // arrive finished, so there is nothing to type — verify reads, the
  // translator re-uploads the edition when changes are requested.
  let { releaseId, mode }: { releaseId: number; mode: 'translate' | 'verify' } = $props();

  const backHref = mode === 'verify' ? '/verify' : '/translate';
  const backLabel = mode === 'verify' ? 'Verificare' : 'Atelier';

  let release = $state<Release | null>(null);
  let events = $state<SubtitleEvent[]>([]);
  let error = $state('');
  let videoEl = $state<HTMLVideoElement | null>(null);
  let trackUrl = $state('');
  let saving = $state<Record<number, 'saving' | 'saved' | 'error'>>({});
  let saveState = $state<'idle' | 'saving' | 'saved'>('idle');
  let submitting = $state(false);
  let translating = $state(false);
  let reviewNotes = $state('');
  // rows the reviewer changed in this session — the "Corectate" filter
  let touched = $state<Set<number>>(new Set());

  // grid view options
  type Filter = 'toate' | 'netraduse' | 'atentie' | 'corectate';
  let filter = $state<Filter>('toate');
  let follow = $state(false);
  let currentMs = $state(-1);

  // Long scripts (1000+ rows) are windowed, Aegisub-style: the grid is its own
  // scroll pane and rows materialize as you approach the bottom — no pagination
  // to lose your place in.
  const CHUNK = 200;
  let limit = $state(CHUNK);
  let gridEl = $state<HTMLDivElement | null>(null);

  function onGridScroll() {
    if (!gridEl) return;
    if (gridEl.scrollTop + gridEl.clientHeight > gridEl.scrollHeight - 600) {
      if (limit < events.length) limit += CHUNK;
    }
  }

  // resume marker: the last row you touched, per release, survives reloads
  const posKey = `ak-editor-pos-${releaseId}`;
  let resumeAt = $state<number | null>(null);

  function rememberPos(idx: number) {
    resumeAt = idx;
    try {
      localStorage.setItem(posKey, String(idx));
    } catch {
      /* private mode */
    }
  }

  async function jumpTo(idx: number) {
    filter = 'toate';
    if (limit < idx + CHUNK) limit = idx + CHUNK;
    await tick();
    const row = document.querySelector(`[data-row="${idx}"]`);
    row?.scrollIntoView({ block: 'center' });
    row?.querySelector('textarea')?.focus();
  }

  function jumpToFirstEmpty() {
    const first = events.find((e) => e.roText.trim() === '');
    if (first) jumpTo(first.idx);
  }

  let editable = $derived(
    release !== null &&
      (mode === 'verify'
        ? release.state === 'in_review'
        : release.state === 'draft' || release.state === 'changes_requested')
  );
  // the verdict is the verifier's; coordinators only preview here (they
  // decide on /publish), so hide the decision panel from them
  const canDecide = $derived(['verifier', 'moderator', 'admin'].includes($auth.user?.role ?? ''));
  const seriesName = $derived(release?.animeTitle ?? release?.mangaTitle ?? release?.proposedTitle ?? '—');

  // manga releases: the flipper's state
  const isManga = $derived(release?.medium === 'manga');
  const pageCount = $derived(release?.pageCount ?? 0);
  const enCount = $derived(release?.enPageCount ?? 0);
  let pageIdx = $state(1);
  let showEn = $state(false);
  let reupFiles = $state<File[]>([]);
  let reuploading = $state(false);

  function flip(delta: number) {
    pageIdx = Math.min(Math.max(1, pageIdx + delta), Math.max(1, pageCount));
  }

  async function reupload() {
    if (reupFiles.length === 0) return;
    reuploading = true;
    try {
      const res = await api.reuploadReleasePages(releaseId, 'ro', reupFiles);
      toast.success(`Ediția RO a fost înlocuită — ${res.count} pagini.`);
      release = (await api.getRelease(releaseId)).data;
      pageIdx = 1;
      reupFiles = [];
    } catch (err) {
      toast.error(
        (err as { error?: string }).error ?? (err as { message?: string }).message ?? 'Încărcarea a eșuat.'
      );
    } finally {
      reuploading = false;
    }
  }
  let translated = $derived(events.filter((e) => e.roText.trim() !== '').length);
  let aiRows = $derived(events.filter((e) => e.roText !== '' && !e.edited).length);

  // reading speed: subtitles over ~20 chars/second are hard to read in time
  const CPS_LIMIT = 20;
  function cps(ev: SubtitleEvent): number {
    const secs = (ev.endMs - ev.startMs) / 1000;
    if (secs <= 0 || ev.roText.trim() === '') return 0;
    return ev.roText.length / secs;
  }
  let warnRows = $derived(events.filter((e) => cps(e) > CPS_LIMIT));

  let filtered = $derived(
    filter === 'netraduse'
      ? events.filter((e) => e.roText.trim() === '')
      : filter === 'atentie'
        ? warnRows
        : filter === 'corectate'
          ? events.filter((e) => touched.has(e.idx))
          : events
  );
  let visible = $derived(filtered.slice(0, limit));
  let hiddenCount = $derived(filtered.length - visible.length);
  let activeIdx = $derived(
    currentMs < 0 ? -1 : (events.find((e) => currentMs >= e.startMs && currentMs < e.endMs)?.idx ?? -1)
  );

  const filterTabs = $derived([
    { k: 'toate' as Filter, label: 'Toate', count: events.length },
    { k: 'netraduse' as Filter, label: 'Netraduse', count: events.length - translated },
    ...(mode === 'verify'
      ? [{ k: 'corectate' as Filter, label: 'Corectate', count: touched.size }]
      : []),
    { k: 'atentie' as Filter, label: 'Atenție', count: warnRows.length }
  ]);

  // keep the playing line in view — but never yank the page while typing
  $effect(() => {
    if (!follow || activeIdx < 0) return;
    if (document.activeElement instanceof HTMLTextAreaElement) return;
    if (limit < activeIdx + 50) limit = activeIdx + CHUNK;
    tick().then(() => {
      document
        .querySelector(`[data-row="${activeIdx}"]`)
        ?.scrollIntoView({ block: 'center', behavior: 'smooth' });
    });
  });

  onMount(async () => {
    try {
      const stored = localStorage.getItem(posKey);
      if (stored !== null) resumeAt = Number(stored);
    } catch {
      /* private mode */
    }
    try {
      release = (await api.getRelease(releaseId)).data;
      if (release.medium !== 'manga') {
        events = (await api.getReleaseEvents(releaseId)).data;
        if (release.translating) {
          translating = true;
          startPolling();
        }
        await refreshTrack();
        if (release.hasVideo) {
          await pollRemux();
          if (converting) watchRemux();
        }
      }
    } catch (err) {
      error =
        (err as { error?: string; message?: string }).error ??
        (err as { message?: string }).message ??
        'Release-ul nu a putut fi încărcat.';
    }
  });

  onDestroy(() => {
    if (trackUrl) URL.revokeObjectURL(trackUrl);
    clearInterval(pollTimer);
    clearInterval(remuxTimer);
  });

  // MKV→MP4 rewrap. An .mkv upload plays in no browser, so until the
  // server has rewrapped it the <video> below would be a black box with no error.
  // Show the conversion instead, and swap the player in when it finishes.
  let remuxTimer: ReturnType<typeof setInterval>;
  let remuxState = $state<'idle' | 'queued' | 'running' | 'done' | 'failed'>('idle');
  let remuxProgress = $state(0);
  let remuxPosition = $state(0);
  let remuxError = $state('');
  // Bumped when the conversion finishes, to force the <video> to re-request a
  // URL that now points at a playable file rather than reuse the failed one.
  let videoNonce = $state(0);
  const converting = $derived(remuxState === 'queued' || remuxState === 'running');

  async function pollRemux() {
    try {
      const { data } = await api.getRemuxStatus(releaseId);
      const was = remuxState;
      remuxState = data.state;
      remuxProgress = data.progress;
      remuxPosition = data.position;
      remuxError = data.error ?? '';
      if (data.state === 'queued' || data.state === 'running') return;
      clearInterval(remuxTimer);
      // Only reload the player if we actually watched it convert — on a fresh
      // load of an already-converted release there is nothing to swap.
      if (data.state === 'done' && (was === 'queued' || was === 'running')) {
        videoNonce++;
        toast.success('Conversia video s-a terminat — previzualizarea e gata.');
      }
    } catch {
      // Status is a nicety; a failed poll must not break the editor.
      clearInterval(remuxTimer);
    }
  }

  function watchRemux() {
    clearInterval(remuxTimer);
    remuxTimer = setInterval(pollRemux, 4000);
  }

  // Auto-translate: async server-side; poll and merge machine text
  // into rows that are STILL empty locally, never over a translator's typing.
  let pollTimer: ReturnType<typeof setInterval>;
  // progress for the AI bar; transTotal 0 = unknown yet
  let transFilled = $state(0);
  let transTotal = $state(0);
  async function autoTranslate() {
    translating = true;
    transFilled = 0;
    transTotal = 0;
    try {
      const res = await api.autoTranslateRelease(releaseId);
      transTotal = res.pending;
      toast.info('Traducerea automată a pornit — vezi progresul mai jos.');
      startPolling();
    } catch (err) {
      translating = false;
      toast.error(
        (err as { error?: string }).error ?? (err as { message?: string }).message ?? 'Traducerea automată a eșuat.'
      );
    }
  }

  // Poll the status endpoint (progress + terminal result) and merge machine
  // text into rows still empty locally — never over a translator's typing. The
  // status is what lets us surface a background failure (e.g. no Anthropic
  // credit) instead of spinning silently until a timeout.
  function startPolling() {
    const deadline = Date.now() + 20 * 60_000;
    clearInterval(pollTimer);
    pollTimer = setInterval(async () => {
      try {
        const [status, fresh] = await Promise.all([
          api.getTranslationStatus(releaseId),
          api.getReleaseEvents(releaseId)
        ]);
        for (const f of fresh.data) {
          const local = events.find((e) => e.idx === f.idx);
          if (local && local.roText === '' && f.roText !== '') {
            local.roText = f.roText;
            local.edited = f.edited;
          }
        }
        if (status.total) transTotal = status.total;
        transFilled = status.filled;
        if (!status.running) {
          clearInterval(pollTimer);
          translating = false;
          scheduleTrackRefresh();
          if (status.error) toast.error(status.error);
          else if (status.done)
            toast.success(`Traducere automată gata — ${status.filled} replici. Verifică rândurile marcate AI.`);
        } else if (Date.now() > deadline) {
          clearInterval(pollTimer);
          translating = false;
        }
      } catch {
        /* keep polling */
      }
    }, 2000);
  }

  // Live preview track: re-serialized server-side from the RO rows, swapped
  // as a blob URL (<track> can't send auth headers).
  let trackTimer: ReturnType<typeof setTimeout>;
  async function refreshTrack() {
    try {
      const vtt = await api.getReleaseDraftVtt(releaseId);
      const old = trackUrl;
      trackUrl = URL.createObjectURL(new Blob([vtt], { type: 'text/vtt' }));
      if (old) URL.revokeObjectURL(old);
      requestAnimationFrame(() => {
        if (videoEl && videoEl.textTracks[0]) videoEl.textTracks[0].mode = 'showing';
      });
    } catch {
      /* preview only — never block editing */
    }
  }
  function scheduleTrackRefresh() {
    clearTimeout(trackTimer);
    trackTimer = setTimeout(refreshTrack, 800);
  }

  async function saveRow(ev: SubtitleEvent, value: string) {
    if (value === ev.roText && ev.edited) return;
    saving = { ...saving, [ev.idx]: 'saving' };
    saveState = 'saving';
    try {
      await api.saveReleaseEvent(releaseId, ev.idx, value);
      ev.roText = value;
      ev.edited = true;
      if (mode === 'verify') touched = new Set(touched).add(ev.idx);
      saving = { ...saving, [ev.idx]: 'saved' };
      saveState = 'saved';
      scheduleTrackRefresh();
    } catch {
      saving = { ...saving, [ev.idx]: 'error' };
      saveState = 'idle';
      toast.error('Salvarea a eșuat — verifică conexiunea.');
    }
  }

  // Enter saves and jumps to the next visible row (Shift+Enter = newline)
  function onKeydown(e: KeyboardEvent, ev: SubtitleEvent) {
    if (e.key !== 'Enter' || e.shiftKey) return;
    e.preventDefault();
    const ta = e.currentTarget as HTMLTextAreaElement;
    saveRow(ev, ta.value);
    const all = Array.from(document.querySelectorAll<HTMLTextAreaElement>('textarea[data-idx]'));
    all[all.indexOf(ta) + 1]?.focus();
  }

  function seek(ms: number) {
    if (!videoEl) return;
    videoEl.currentTime = Math.max(0, ms / 1000 - 0.5);
    videoEl.play().catch(() => {});
  }

  function clock(ms: number): string {
    const s = Math.floor(ms / 1000);
    const mm = String(Math.floor(s / 60) % 60).padStart(2, '0');
    const ss = String(s % 60).padStart(2, '0');
    const h = Math.floor(s / 3600);
    return h > 0 ? `${h}:${mm}:${ss}` : `${mm}:${ss}`;
  }

  function grow(e: Event) {
    autosize(e.currentTarget as HTMLTextAreaElement);
  }

  // textareas hug their content from the first paint (rows=1 would clip
  // pre-filled multi-line rows), and keep hugging while windowing swaps rows
  function autosize(ta: HTMLTextAreaElement) {
    ta.style.height = 'auto';
    ta.style.height = `${ta.scrollHeight}px`;
  }

  async function submit() {
    if (!release) return;
    submitting = true;
    try {
      await api.submitRelease(releaseId);
      release.state = 'in_review';
      toast.success('Trimis la verificare!');
    } catch (err) {
      toast.error((err as { error?: string }).error ?? (err as { message?: string }).message ?? 'Trimiterea a eșuat.');
    } finally {
      submitting = false;
    }
  }

  async function approve() {
    submitting = true;
    try {
      await api.approveRelease(releaseId);
      if (release) release.state = 'approved';
      toast.success('Aprobat — trimis la publicare.');
      goto('/verify');
    } catch (err) {
      toast.error((err as { error?: string }).error ?? (err as { message?: string }).message ?? 'Aprobarea a eșuat.');
    } finally {
      submitting = false;
    }
  }

  async function requestChanges() {
    if (!reviewNotes.trim()) {
      toast.error('Scrie ce trebuie corectat.');
      return;
    }
    submitting = true;
    try {
      await api.requestReleaseChanges(releaseId, reviewNotes.trim());
      if (release) {
        release.state = 'changes_requested';
        release.reviewNotes = reviewNotes.trim();
      }
      toast.success('Trimis înapoi cu observații.');
      goto('/verify');
    } catch (err) {
      toast.error((err as { error?: string }).error ?? (err as { message?: string }).message ?? 'Trimiterea a eșuat.');
    } finally {
      submitting = false;
    }
  }

  const stateLabels: Record<Release['state'], string> = {
    draft: 'ciornă',
    in_review: 'în verificare',
    changes_requested: 'modificări cerute',
    approved: 'aprobat',
    published: 'publicat'
  };
</script>

{#if error}
  <div class="container"><div class="empty"><p>{error}</p><a href={backHref}>← {backLabel}</a></div></div>
{:else if !release}
  <div class="container"><p class="muted center">Se încarcă…</p></div>
{:else}
  <div class="workbar">
    <div class="wb-inner container">
      <a class="back" href={backHref}>← {backLabel}</a>
      <span class="vr"></span>
      <div class="wb-title">
        <span class="wb-name">{seriesName}
          <span class="dim">— {isManga ? `capitolul ${release.chapterNumber}` : `episodul ${release.episodeNumber}`}</span></span>
        {#if !release.animeId && !release.mangaId}
          <span class="byline">serie propusă — se importă la publicare</span>
        {/if}
        <span class="state s-{release.state}">{stateLabels[release.state]}</span>
        {#if release.uploaderName}
          <span class="byline">tradus de <Person name={release.uploaderName} /></span>
        {/if}
      </div>
      {#if !isManga}
        <span class="savestate" class:saving={saveState === 'saving'} class:saved={saveState === 'saved'}>
          {saveState === 'saving' ? '⟳ se salvează…' : saveState === 'saved' ? '✓ salvat' : 'salvare automată activă'}
        </span>
        <span class="vr"></span>
        <span class="wb-prog">
          <span class="wb-count"><strong>{translated}</strong> / {events.length}</span>
          <span class="wb-bar"><span class="wb-fill" style="width:{events.length ? (translated / events.length) * 100 : 0}%"></span></span>
        </span>
      {:else}
        <span class="wb-count"><strong>{pageCount}</strong> pagini</span>
      {/if}
      {#if mode === 'translate' && editable && !isManga}
        <button
          class="btn fill"
          onclick={submit}
          disabled={submitting || translated === 0}
          title={translated === events.length
            ? 'Trimite release-ul către echipa de verificare'
            : `${events.length - translated} replici încă netraduse — poți trimite și parțial`}
        >
          Trimite la verificare
        </button>
      {/if}
    </div>
  </div>

  <div class="editor container">
    <div class="workspace">
      <aside class="rail">
        {#if release.hasVideo}
          <div class="vcard">
            {#if converting}
              <div class="converting">
                <span class="conv-title">Se convertește în MP4…</span>
                <div class="conv-bar"><div class="conv-fill" style="width:{Math.round(remuxProgress * 100)}%"></div></div>
                <span class="conv-note">
                  {#if remuxState === 'queued'}
                    {remuxPosition > 1 ? `${remuxPosition}‑lea la rând` : 'urmează imediat'}
                  {:else}
                    {Math.round(remuxProgress * 100)}%
                  {/if}
                </span>
                <p class="conv-hint">
                  Browserele nu redau .mkv, așa că serverul rescrie fișierul ca .mp4.
                  Poți traduce între timp — subtitrarea engleză e deja extrasă.
                </p>
              </div>
            {:else}
              {#if remuxState === 'failed'}
                <p class="conv-fail">{remuxError || 'Conversia în MP4 a eșuat — previzualizarea poate să nu funcționeze.'}</p>
              {/if}
              {#key videoNonce}
                <!--
                  Deliberately NO crossorigin here, unlike the public VideoPlayer.

                  This src 302s to a presigned R2 URL, and R2 does not send
                  Access-Control-Allow-Origin on object GETs. With
                  crossorigin="anonymous" the element fetches in CORS mode, follows
                  the redirect, finds no ACAO and drops the response — a black box
                  with no error anywhere. Without it the fetch is no-cors, which is
                  how a <video> has always been allowed to play a cross-origin file.

                  The cost of no-cors is a tainted resource, which would only matter
                  if we drew this into a canvas. We don't. The <track> below is a
                  blob: URL built from a fetch we already made, so it is same-origin
                  and needs no CORS either.
                -->
                <video
                  bind:this={videoEl}
                  class="preview"
                  controls
                  preload="metadata"
                  src={api.releaseVideoUrl(releaseId)}
                  ontimeupdate={() => (currentMs = (videoEl?.currentTime ?? 0) * 1000)}
                >
                  {#if trackUrl}
                    <track kind="subtitles" srclang="ro" label="Română (ciornă)" src={trackUrl} default />
                  {/if}
                </video>
              {/key}
            {/if}
          </div>
        {/if}

        {#if isManga}
          <div class="card">
            <div class="p-top">
              <span class="p-num">{pageCount}</span>
              <span class="p-suffix">pagini RO</span>
            </div>
            <span class="p-label">{enCount > 0 ? `${enCount} pagini originale alături` : 'fără paginile originale'}</span>
            <p class="p-note">textul e deja în imagini — aici doar se citește și se decide</p>
            {#if enCount > 0}
              <button class="btn ghost wide" onclick={() => (showEn = !showEn)}>
                {showEn ? 'Ascunde originalul' : 'Arată originalul alături'}
              </button>
            {/if}
          </div>
        {:else}
          <div class="card">
            <div class="p-top">
              <span class="p-num">{translated}</span>
              <span class="p-suffix">/ {events.length}</span>
            </div>
            <span class="p-label">replici traduse{aiRows > 0 ? ` · ${aiRows} de la AI` : ''}</span>
            <span class="p-track"><span class="p-fill" style="width:{events.length ? (translated / events.length) * 100 : 0}%"></span></span>
            <p class="p-note">
              {translated === events.length
                ? 'totul e tradus — poți trimite la verificare'
                : `mai ai ${events.length - translated} replici`}
            </p>
            {#if warnRows.length > 0}
              <p class="p-warn">
                ⚠ {warnRows.length === 1 ? 'o replică e' : `${warnRows.length} replici sunt`} peste {CPS_LIMIT} car/s — vezi filtrul Atenție
              </p>
            {/if}
            {#if editable && translated < events.length}
              <button class="btn ghost wide" onclick={autoTranslate} disabled={translating}>
                {translating ? 'Se traduce…' : '✦ Tradu automat golurile'}
              </button>
            {/if}
            {#if translating}
              <div class="ai-prog" role="progressbar" aria-valuenow={transFilled} aria-valuemin="0" aria-valuemax={transTotal || undefined}>
                <div class="ai-track"><div class="ai-fill" class:indet={!transTotal} style={transTotal ? `width:${(transFilled / transTotal) * 100}%` : ''}></div></div>
                <span class="ai-lbl">
                  {#if transTotal}{transFilled} / {transTotal} replici traduse de AI{:else}se pregătește…{/if}
                </span>
              </div>
            {/if}
            {#if editable && resumeAt !== null && resumeAt > 0}
              <button class="btn ghost wide" onclick={() => jumpTo(resumeAt!)}>
                Continuă de la replica {resumeAt + 1}
              </button>
            {/if}
          </div>
        {/if}

        {#if isManga && mode === 'translate' && editable}
          <div class="card">
            <span class="v-kicker">Înlocuiește ediția RO</span>
            <p class="p-note">încarci toate paginile din nou, în ordinea numelor de fișier</p>
            <input
              class="reup"
              type="file"
              accept="image/*"
              multiple
              onchange={(e) => (reupFiles = Array.from((e.currentTarget as HTMLInputElement).files ?? []))}
            />
            <button class="btn ghost wide" onclick={reupload} disabled={reuploading || reupFiles.length === 0}>
              {reuploading ? 'Se încarcă…' : `Înlocuiește paginile${reupFiles.length ? ` (${reupFiles.length})` : ''}`}
            </button>
            <button class="btn fill wide" onclick={submit} disabled={submitting}>Retrimite la verificare</button>
          </div>
        {/if}

        {#if mode === 'verify' && release.state === 'in_review' && canDecide}
          <div class="card verdict">
            <span class="v-kicker">Verdictul tău</span>
            <p class="v-summary">
              {isManga
                ? `${pageCount} pagini de citit — decizi când ai terminat`
                : touched.size === 0
                  ? 'nicio replică corectată încă'
                  : `${touched.size} replici corectate de tine`}
            </p>
            <textarea
              class="notes"
              rows="3"
              bind:value={reviewNotes}
              placeholder="Mesaj pentru {release.uploaderName ?? 'traducător'} — obligatoriu dacă returnezi…"
            ></textarea>
            <div class="v-actions">
              <button class="btn ghost warn" onclick={requestChanges} disabled={submitting}>Returnează</button>
              <button class="btn fill grow" onclick={approve} disabled={submitting}>Aprobă release-ul</button>
            </div>
            <p class="v-note">
              {isManga
                ? 'Nu poți edita paginile — dacă ceva e greșit, returnează cu observații.'
                : 'Corecturile tale mici se păstrează la aprobare — nu mai e nevoie de retrimitere.'}
            </p>
          </div>
        {/if}

        {#if mode === 'translate' && release.state === 'changes_requested' && release.reviewNotes}
          <div class="banner warn">
            <span class="b-pill warn">returnat</span>
            <p>„{release.reviewNotes}”{#if release.reviewerName}&nbsp;— <Person name={release.reviewerName} />{/if}</p>
          </div>
        {:else if mode === 'translate' && release.state === 'in_review'}
          <div class="banner">
            <span class="b-pill">în verificare</span>
            <p>Trimis către echipa de verificare. Replicile sunt doar în citire până primești răspunsul.</p>
          </div>
        {:else if release.state === 'approved'}
          <div class="banner ok">
            <span class="b-pill ok">aprobat</span>
            <p>
              {isManga
                ? 'Așteaptă publicarea — un coordonator confirmă seria și capitolul pe pagina de publicare.'
                : 'Așteaptă publicarea — un coordonator confirmă seria, episodul și sursa pe pagina de publicare.'}
            </p>
          </div>
        {:else if mode === 'verify' && release.state !== 'in_review'}
          <div class="banner">
            <p>Release-ul nu mai e în coadă — starea curentă: {stateLabels[release.state]}.</p>
          </div>
        {/if}

        {#if editable && !isManga}
          <div class="card shortcuts">
            <span class="v-kicker faint">Scurtături</span>
            <div class="sc-row"><kbd>Enter</kbd><span>salvează și trece la următoarea</span></div>
            <div class="sc-row"><kbd>Shift+Enter</kbd><span>rând nou în aceeași replică</span></div>
            <div class="sc-row"><kbd>click pe timp</kbd><span>sare în video la replica aia</span></div>
          </div>
        {:else if isManga}
          <div class="card shortcuts">
            <span class="v-kicker faint">Scurtături</span>
            <div class="sc-row"><kbd>←</kbd> <kbd>→</kbd><span>pagina anterioară / următoare</span></div>
          </div>
        {/if}
      </aside>

      {#if isManga}
        <section class="flipper">
          <div class="fbar">
            <button class="fnav" onclick={() => flip(-1)} disabled={pageIdx <= 1}>← anterioară</button>
            <span class="fpos">pagina <strong>{pageIdx}</strong> / {pageCount}</span>
            <button class="fnav" onclick={() => flip(1)} disabled={pageIdx >= pageCount}>următoarea →</button>
          </div>
          {#if pageCount === 0}
            <div class="row-empty">Paginile nu mai sunt în staging — release-ul a fost deja publicat.</div>
          {:else}
            <div class="fpages" class:double={showEn && pageIdx <= enCount}>
              <img
                class="fpage"
                src={api.releasePageUrl(releaseId, 'ro', pageIdx)}
                alt="Pagina {pageIdx} — traducerea RO"
              />
              {#if showEn && pageIdx <= enCount}
                <img
                  class="fpage"
                  src={api.releasePageUrl(releaseId, 'en', pageIdx)}
                  alt="Pagina {pageIdx} — originalul"
                />
              {/if}
            </div>
          {/if}
        </section>
      {:else}
      <section class="gridpane">
        <div class="gridbar">
          <div class="tabs" role="tablist" aria-label="Filtre replici">
            {#each filterTabs as t (t.k)}
              <button
                class="tab"
                class:on={filter === t.k}
                role="tab"
                aria-selected={filter === t.k}
                onclick={() => (filter = t.k)}
              >
                {t.label} <span class="t-count">{t.count}</span>
              </button>
            {/each}
          </div>
          <span class="spacer"></span>
          {#if release.hasVideo}
            <button
              class="switch"
              class:on={follow}
              role="switch"
              aria-checked={follow}
              title="Replica activă urmărește redarea video"
              onclick={() => (follow = !follow)}
            >
              <span class="knob"></span>
              <span class="sw-label">urmărește redarea</span>
            </button>
          {/if}
          {#if editable && translated < events.length}
            <button class="jump" onclick={jumpToFirstEmpty}>↓ prima netradusă</button>
          {/if}
        </div>

        <div class="gridcard">
          <div class="row header" role="row">
            <span>#</span><span>timp</span><span>engleză</span><span>română</span><span></span>
          </div>
          <div class="grid" role="table" aria-label="Replici" bind:this={gridEl} onscroll={onGridScroll}>
            {#each visible as ev (ev.id)}
              <div
                class="row"
                role="row"
                data-row={ev.idx}
                class:playing={ev.idx === activeIdx}
              >
                <span class="idx">{ev.idx + 1}</span>
                <button class="time" class:active={ev.idx === activeIdx} onclick={() => seek(ev.startMs)} title="Sari în video la {clock(ev.startMs)}">
                  {clock(ev.startMs)}
                </button>
                <div class="en">{ev.enText || '—'}</div>
                <div class="ro">
                  <textarea
                    use:autosize
                    data-idx={ev.idx}
                    rows="1"
                    value={ev.roText}
                    disabled={!editable}
                    placeholder="scrie traducerea…"
                    oninput={grow}
                    onfocus={(e) => {
                      grow(e);
                      rememberPos(ev.idx);
                    }}
                    onkeydown={(e) => onKeydown(e, ev)}
                    onblur={(e) => saveRow(ev, (e.currentTarget as HTMLTextAreaElement).value)}
                  ></textarea>
                  {#if cps(ev) > CPS_LIMIT}
                    <span class="cps">⚠ {Math.round(cps(ev))} car/s — prea rapid de citit, scurtează</span>
                  {/if}
                  {#if mode === 'verify' && touched.has(ev.idx)}
                    <span class="fixed-tag">corectat de tine</span>
                  {/if}
                </div>
                <span class="mark" class:err={saving[ev.idx] === 'error'} class:ai={ev.roText !== '' && !ev.edited}>
                  {saving[ev.idx] === 'saving'
                    ? '·'
                    : saving[ev.idx] === 'saved'
                      ? '✓'
                      : saving[ev.idx] === 'error'
                        ? '!'
                        : ev.roText !== '' && !ev.edited
                          ? 'AI'
                          : ''}
                </span>
              </div>
            {:else}
              <div class="row-empty">Nicio replică în filtrul ăsta.</div>
            {/each}
            {#if hiddenCount > 0}
              <div class="row-more">încă {hiddenCount} replici — derulează pentru a le încărca</div>
            {/if}
          </div>
        </div>
        <p class="gridnote">
          Fiecare replică se salvează singură când apeși Enter sau când ieși din câmp.
          Timpii vin din fișierul EN și nu se modifică aici.
        </p>
      </section>
      {/if}
    </div>
  </div>
{/if}

<svelte:window
  onkeydown={(e) => {
    if (!isManga || pageCount === 0) return;
    if (document.activeElement instanceof HTMLTextAreaElement || document.activeElement instanceof HTMLInputElement) return;
    if (e.key === 'ArrowRight') flip(1);
    else if (e.key === 'ArrowLeft') flip(-1);
  }}
/>

<style>
  /* ── workbar: sticky context strip under the site header ── */
  .workbar {
    position: sticky; top: var(--header-h); z-index: 40;
    background: color-mix(in srgb, var(--surface-raised) 88%, transparent);
    backdrop-filter: blur(10px); -webkit-backdrop-filter: blur(10px);
    border-bottom: 1px solid var(--border-subtle);
  }
  .wb-inner {
    display: flex; align-items: center; gap: 14px;
    padding-block: 10px;
  }
  .back {
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
    white-space: nowrap;
  }
  .back:hover { color: var(--text-primary); }
  .vr { width: 1px; height: 20px; background: var(--border-default); flex: 0 0 auto; }
  /* the title is the only thing allowed to shrink — everything else keeps
     its place on one line */
  .wb-title { display: flex; align-items: center; gap: 11px; min-width: 0; flex: 1; }
  .wb-name {
    font-family: var(--font-display); font-size: var(--fs-body); font-weight: var(--fw-semibold);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0;
  }
  .dim { color: var(--text-muted); font-weight: var(--fw-regular); }
  .byline { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); white-space: nowrap; display: inline-flex; align-items: center; gap: 5px; }

  .savestate { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); white-space: nowrap; }
  .savestate.saving { color: var(--accent); animation: pulse 1s ease-in-out infinite; }
  .savestate.saved { color: var(--success); }
  @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.35; } }

  .wb-prog { display: flex; align-items: center; gap: 9px; flex: 0 0 auto; }
  .wb-count { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .wb-count strong { color: var(--accent); font-weight: var(--fw-medium); }
  .wb-bar {
    width: 90px; height: 5px; border-radius: var(--radius-pill);
    background: var(--surface-inset); overflow: hidden;
  }
  .wb-fill { display: block; height: 100%; background: var(--accent); border-radius: var(--radius-pill); }

  .state {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.08em; text-transform: uppercase;
    padding: 3px 10px; border-radius: var(--radius-pill); white-space: nowrap;
    background: var(--surface-overlay); color: var(--text-muted);
  }
  .state.s-in_review { background: color-mix(in srgb, var(--warning) 14%, transparent); color: var(--warning); }
  .state.s-changes_requested { background: color-mix(in srgb, var(--danger) 14%, transparent); color: var(--danger); }
  .state.s-approved, .state.s-published { background: color-mix(in srgb, var(--success) 14%, transparent); color: var(--success); }

  /* ── workspace ── */
  .editor { padding-block: var(--space-5) var(--space-8); }
  .workspace {
    display: grid; grid-template-columns: minmax(300px, 370px) minmax(0, 1fr);
    gap: var(--space-5); align-items: start;
  }
  .rail {
    position: sticky; top: calc(var(--header-h) + 3.6rem);
    display: flex; flex-direction: column; gap: var(--space-3);
    max-height: calc(100vh - var(--header-h) - 4.5rem);
    overflow-y: auto; scrollbar-width: thin;
  }
  @media (max-width: 1000px) {
    .workspace { grid-template-columns: minmax(0, 1fr); }
    .rail { position: static; max-height: none; overflow: visible; }
  }

  .vcard {
    border: 1px solid var(--border-subtle); border-radius: var(--radius-lg);
    overflow: hidden; background: #000; flex: 0 0 auto;
  }
  .preview { display: block; width: 100%; aspect-ratio: 16 / 9; background: #000; }

  /* stands in for the player while the server rewraps an .mkv as .mp4 */
  .converting {
    aspect-ratio: 16 / 9; display: flex; flex-direction: column;
    align-items: center; justify-content: center; gap: 0.6rem;
    padding: 1rem 1.25rem; text-align: center;
  }
  .conv-title { font-weight: 600; color: var(--text-primary); }
  .conv-bar {
    width: 100%; max-width: 260px; height: 6px; border-radius: 999px;
    background: rgba(255, 255, 255, 0.12); overflow: hidden;
  }
  .conv-fill { height: 100%; background: var(--accent); transition: width 0.4s ease; }
  .conv-note { font-size: 0.8rem; color: var(--text-muted); font-variant-numeric: tabular-nums; }
  .conv-hint { margin: 0; font-size: 0.78rem; line-height: 1.5; color: var(--text-muted); max-width: 34ch; }
  .conv-fail {
    margin: 0; padding: 0.6rem 0.75rem; font-size: 0.8rem;
    color: var(--danger); background: rgba(239, 68, 68, 0.08);
  }
  /* subtitle cues rendered Netflix/Crunchyroll-style: white text with a soft
     dark outline, no black box behind it */
  .preview::cue {
    background: rgba(0, 0, 0, 0);
    color: #fff;
    font-family: var(--font-body);
    text-shadow:
      0 0 3px rgba(0, 0, 0, 0.9),
      0 2px 4px rgba(0, 0, 0, 0.8),
      0 0 8px rgba(0, 0, 0, 0.6);
  }

  .card {
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg); padding: var(--space-4) var(--space-4);
    display: flex; flex-direction: column; gap: var(--space-2); flex: 0 0 auto;
  }
  .p-top { display: flex; align-items: baseline; gap: 7px; }
  .p-num {
    font-family: var(--font-display); font-size: 2rem; font-weight: var(--fw-semibold);
    letter-spacing: -0.015em; line-height: 1;
  }
  .p-suffix { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .p-label {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-muted);
  }
  .p-track {
    height: 5px; border-radius: var(--radius-pill); background: var(--surface-inset);
    overflow: hidden; margin-top: 4px;
  }
  .p-fill { display: block; height: 100%; background: var(--accent); transition: width var(--motion-base) var(--ease); }
  .p-note { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); line-height: 1.6; }

  /* auto-translate progress */
  .ai-prog { display: flex; flex-direction: column; gap: 6px; margin-top: 4px; }
  .ai-track {
    height: 6px; border-radius: var(--radius-pill); background: var(--surface-inset); overflow: hidden;
  }
  .ai-fill {
    display: block; height: 100%; background: var(--accent);
    border-radius: var(--radius-pill); transition: width var(--motion-base) var(--ease);
  }
  /* indeterminate sweep until the first count lands */
  .ai-fill.indet { width: 35%; animation: ai-sweep 1.1s ease-in-out infinite; }
  @keyframes ai-sweep { 0% { margin-left: -35%; } 100% { margin-left: 100%; } }
  .ai-lbl { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .p-warn {
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--warning);
    line-height: 1.5; border-top: 1px solid var(--border-subtle); padding-top: 10px;
  }

  .verdict { border-color: var(--border-default); }
  .v-kicker {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--accent);
  }
  .v-kicker.faint { color: var(--text-muted); }
  .v-summary { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .notes {
    width: 100%; background: var(--surface-inset);
    border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
    color: var(--text-primary); font-size: var(--fs-small); font-family: var(--font-body);
    line-height: var(--lh-snug); padding: var(--space-3);
    resize: vertical; min-height: 4.5rem;
  }
  .notes:focus { outline: none; border-color: var(--accent); }
  .v-actions { display: flex; gap: 9px; }
  .v-actions .grow { flex: 1; }
  .v-note { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); line-height: 1.6; }

  .banner {
    display: flex; flex-direction: column; gap: 8px;
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg); padding: var(--space-3) var(--space-4);
    flex: 0 0 auto;
  }
  .banner p { font-size: var(--fs-small); color: var(--text-muted); line-height: 1.5; }
  .banner.warn { border-color: color-mix(in srgb, var(--warning) 35%, transparent); }
  .banner.ok { border-color: color-mix(in srgb, var(--success) 35%, transparent); }
  .b-pill {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-muted);
  }
  .b-pill.warn { color: var(--warning); }
  .b-pill.ok { color: var(--success); }

  .shortcuts { gap: 0; }
  .shortcuts .v-kicker { margin-bottom: 6px; }
  .sc-row { display: flex; align-items: center; gap: 10px; padding: 8px 0; }
  .sc-row + .sc-row { border-top: 1px solid var(--border-subtle); }
  kbd {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    padding: 3px 7px; border: 1px solid var(--border-default); border-radius: 5px;
    background: var(--surface-inset); color: var(--text-primary); flex: 0 0 auto;
  }
  .sc-row span { font-size: var(--fs-caption); color: var(--text-muted); }

  /* ── grid pane ── */
  .gridbar {
    display: flex; gap: var(--space-4); align-items: center;
    padding-bottom: var(--space-3); flex-wrap: wrap;
  }
  .spacer { flex: 1; }
  .tabs {
    display: flex; gap: 2px; padding: 3px;
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
  }
  .tab {
    font-size: var(--fs-caption); font-weight: var(--fw-medium);
    padding: 7px 12px; border-radius: var(--radius-sm); border: 0; cursor: pointer;
    background: transparent; color: var(--text-muted);
  }
  .tab:hover { color: var(--text-primary); }
  .tab.on { background: var(--surface-overlay); color: var(--text-primary); font-weight: var(--fw-semibold); }
  .t-count { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .tab.on .t-count { color: var(--accent); }

  .switch {
    display: inline-flex; align-items: center; gap: 8px;
    background: none; border: 0; cursor: pointer; padding: 0;
  }
  .switch .knob {
    position: relative; width: 34px; height: 20px; border-radius: var(--radius-pill);
    background: var(--surface-overlay); border: 1px solid var(--border-default);
    transition: background var(--motion-base) var(--ease); flex: 0 0 auto;
  }
  .switch .knob::after {
    content: ''; position: absolute; top: 2px; left: 2px;
    width: 14px; height: 14px; border-radius: 50%;
    background: var(--text-muted); box-shadow: var(--shadow-1);
    transition: left var(--motion-base) var(--ease), background var(--motion-base) var(--ease);
  }
  .switch.on .knob { background: var(--accent); border-color: var(--accent); }
  .switch.on .knob::after { left: 16px; background: var(--on-accent); }
  .sw-label { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .switch:hover .sw-label { color: var(--text-primary); }

  .jump {
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
    padding: 7px 12px; border: 1px solid var(--border-default); border-radius: var(--radius-sm);
    background: transparent; cursor: pointer;
  }
  .jump:hover { color: var(--text-primary); border-color: var(--border-strong); }

  /* The grid scrolls inside its own pane (Aegisub-style): the column header
     is pinned above it, and the rail never leaves view. */
  .gridcard {
    border: 1px solid var(--border-subtle); border-radius: var(--radius-lg);
    background: var(--surface-raised); overflow: hidden;
  }
  .grid {
    max-height: calc(100vh - var(--header-h) - 12rem);
    min-height: 380px;
    overflow-y: auto; overscroll-behavior: contain;
  }
  .row {
    display: grid; grid-template-columns: 3rem 4.2rem 1fr 1fr 1.6rem;
    gap: var(--space-3); align-items: start;
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border-subtle);
    box-shadow: inset 3px 0 0 transparent;
  }
  .row:last-child { border-bottom: 0; }
  .row.playing {
    box-shadow: inset 3px 0 0 var(--accent);
    background: color-mix(in srgb, var(--accent) 5%, transparent);
  }
  .row.header {
    font-size: var(--fs-micro); font-family: var(--font-mono);
    text-transform: uppercase; letter-spacing: 0.1em; color: var(--text-muted);
    border-bottom: 1px solid var(--border-default);
    padding-block: 10px;
  }
  .row-empty { padding: var(--space-7); text-align: center; color: var(--text-muted); font-size: var(--fs-small); }
  .row-more {
    padding: var(--space-3); text-align: center; color: var(--text-muted);
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.06em; text-transform: uppercase;
  }
  .idx { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); padding-top: 7px; }
  .time {
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
    background: none; border: 0; cursor: pointer; padding: 7px 0 0; text-align: left;
  }
  .time:hover { color: var(--accent-hover); }
  .time.active { color: var(--accent); }
  .en {
    font-size: var(--fs-small); color: var(--text-muted);
    white-space: pre-wrap; padding-top: 6px; line-height: var(--lh-snug);
    text-wrap: pretty;
  }
  .ro { min-width: 0; }
  /* underline field, sketch-style: no box until you're in it */
  .ro textarea {
    width: 100%; background: transparent;
    border: 0; border-bottom: 1px solid var(--border-default); border-radius: 0;
    color: var(--text-primary); font-size: var(--fs-small); font-family: var(--font-body);
    line-height: var(--lh-snug); padding: 6px 2px;
    resize: none; overflow: hidden; min-height: 2rem; outline: none;
  }
  .ro textarea:focus { border-bottom-color: var(--accent); }
  .ro textarea:disabled { opacity: 0.6; border-bottom-color: transparent; }
  .ro textarea::placeholder { color: var(--text-faint); }
  .cps {
    display: block; font-family: var(--font-mono); font-size: var(--fs-micro);
    color: var(--warning); margin-top: 5px;
  }
  .fixed-tag {
    display: block; font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.08em; text-transform: uppercase; color: var(--accent); margin-top: 5px;
  }
  .mark { font-family: var(--font-mono); color: var(--accent); padding-top: 7px; font-size: var(--fs-caption); }
  .mark.err { color: var(--danger); }
  /* machine-filled row, not yet reviewed by a human */
  .mark.ai { color: var(--warning); font-size: var(--fs-micro); letter-spacing: 0.05em; }

  .gridnote {
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
    margin-top: 12px; line-height: 1.6;
  }

  /* ── manga flipper (4.6): pages arrive finished, the pane just reads ── */
  .flipper { min-width: 0; }
  .fbar {
    display: flex; align-items: center; justify-content: center; gap: var(--space-4);
    padding-bottom: var(--space-3);
  }
  .fnav {
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
    padding: 7px 12px; border: 1px solid var(--border-default); border-radius: var(--radius-sm);
    background: transparent; cursor: pointer;
  }
  .fnav:hover:not(:disabled) { color: var(--text-primary); border-color: var(--border-strong); }
  .fnav:disabled { opacity: 0.4; cursor: not-allowed; }
  .fpos { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .fpos strong { color: var(--accent); font-weight: var(--fw-medium); }
  .fpages {
    display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--space-3);
    justify-items: center;
  }
  .fpages.double { grid-template-columns: 1fr 1fr; }
  .fpage {
    max-width: 100%; height: auto; border-radius: var(--radius-md);
    border: 1px solid var(--border-subtle); background: var(--surface-raised);
  }
  .reup { font-size: var(--fs-caption); color: var(--text-muted); }
  .reup::file-selector-button {
    border: 1px solid var(--border-default); background: var(--surface-inset);
    color: var(--text-primary); font-weight: var(--fw-semibold); font-size: var(--fs-caption);
    padding: 7px 12px; border-radius: var(--radius-sm); margin-right: 10px; cursor: pointer;
  }

  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 10px 16px; border-radius: var(--radius-md); white-space: nowrap; cursor: pointer;
    transition: background var(--motion-fast) var(--ease);
  }
  .btn.wide { width: 100%; text-align: center; }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-primary); }
  .btn.ghost:hover { background: var(--surface-overlay); }
  .btn.ghost.warn { color: var(--warning); border-color: color-mix(in srgb, var(--warning) 45%, transparent); }
  .btn.ghost.warn:hover { background: color-mix(in srgb, var(--warning) 10%, transparent); }
  .btn:disabled { opacity: 0.5; cursor: not-allowed; }

  .muted { color: var(--text-muted); }
  .center { text-align: center; padding: var(--space-8); }
  .empty { text-align: center; padding: var(--space-8) var(--space-4); color: var(--text-muted); }
  .empty a { color: var(--accent); }

  @media (max-width: 700px) {
    .row { grid-template-columns: 2.2rem 3.5rem 1fr 1.2rem; }
    .en { display: none; }
    .row.header span:nth-child(3) { display: none; }
    .byline, .savestate { display: none; }
    .wb-inner { flex-wrap: wrap; }
  }
</style>
