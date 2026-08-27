<script lang="ts">
  import { invalidateAll } from '$app/navigation';
  import { mediaUrl } from '$lib/media';
  import CommentSection from '$lib/components/CommentSection.svelte';
  import VideoPlayer from '$lib/components/player/VideoPlayer.svelte';
  import { api } from '$lib/api';
  import { authStore as auth } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import { displayName, displaySynopsis, studioOf, titleRef } from '$lib/types';
  import { sourceName } from '$lib/providers';
  import { nameHue } from '$lib/avatar';
  import type {
    ResolvedStream,
    StreamSourceInfo,
    EpisodeSkipMarks,
    ReleaseCredits
  } from '$shared/types';

  let { data } = $props();
  const a = $derived(data.anime);
  const ep = $derived(data.episode);

  /* ── Episode editing ───────────────────────────────────────────────────────
     Same gate as the series description on /anime/[id] (editRole on the
     server): admin, coordinator, translator. */
  const canEditEpisode = $derived(
    ['admin', 'coordinator', 'translator'].includes($auth.user?.role ?? '')
  );
  let epEditOpen = $state(false);
  let editTitle = $state('');
  let editSyn = $state('');
  let editFiller = $state(false);
  let editRecap = $state(false);
  let epSaving = $state(false);

  function openEpEdit() {
    editTitle = ep.title ?? '';
    editSyn = ep.synopsis ?? '';
    editFiller = ep.isFiller;
    editRecap = ep.isRecap;
    epEditOpen = true;
  }

  async function saveEpEdit() {
    epSaving = true;
    try {
      // synopsis always travels, including as '' — that is how the description
      // gets cleared (the API turns a blank string into NULL). Title only when
      // non-empty: a blank there means "no title", which the API's coalesce
      // reads as "leave alone", and clearing a title is not a thing anyone
      // wants to do from here.
      await api.updateEpisode(a.id, ep.episodeNumber, {
        ...(editTitle.trim() ? { title: editTitle.trim() } : {}),
        synopsis: editSyn.trim(),
        isFiller: editFiller,
        isRecap: editRecap
      });
      toast.success('Episod actualizat.');
      epEditOpen = false;
      await invalidateAll();
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Eroare la salvare.');
    } finally {
      epSaving = false;
    }
  }

  // Active embed sources only; extract sources are played via the stream
  // endpoint in our own player, never in an iframe.
  const embeds = $derived(
    (ep.links ?? []).filter((l) => l.isActive !== false && l.kind !== 'extract')
  );
  const epMeta = $derived([studioOf(a), ep.airDate].filter(Boolean) as string[]);
  let sourceIdx = $state(0);
  // Our player when the episode has a resolvable extract source;
  // stream stays null (404) for embed-only episodes and we fall back to iframes.
  let stream = $state<{ stream: ResolvedStream; source: StreamSourceInfo } | null>(null);
  let useStream = $state(true);
  let skip = $state<EpisodeSkipMarks>({ intro: null, outro: null });
  let credits = $state<ReleaseCredits | null>(null);
  let resumeAt = $state(0);
  $effect(() => {
    // reset the selected source when the episode changes, then probe for a
    // stream, its skip marks (ours → AniSkip, resolved server-side), and —
    // for logged-in users — the saved resume position
    const epId = ep.id;
    sourceIdx = 0;
    useStream = true;
    showAllSources = false;
    reportOpen = false;
    reportText = '';
    stream = null;
    skip = { intro: null, outro: null };
    credits = null;
    resumeAt = 0;
    lastSavedAt = 0;
    api
      .getEpisodeCredits(a.id, ep.episodeNumber)
      .then((r) => {
        if (ep.id === epId) credits = r.data;
      })
      .catch(() => {});
    api
      .getEpisodeStream(epId)
      .then((r) => {
        if (ep.id === epId) stream = r.data;
      })
      .catch(() => {});
    api
      .getEpisodeSkip(epId)
      .then((r) => {
        if (ep.id === epId) skip = r.data;
      })
      .catch(() => {});
    if ($auth.isAuthenticated) {
      api
        .getPlaybackProgress(epId)
        .then((r) => {
          if (ep.id === epId && r.data) resumeAt = r.data.position;
        })
        .catch(() => {});
      // Count the view. Fire-and-forget on purpose: the leaderboards are not
      // worth a visible error, and the server drops repeats by primary key, so
      // a refresh or a re-watch adds nothing. Not guarded by the source type —
      // an iframe episode counts exactly like one played in our own player,
      // which is the point.
      api.recordEpisodeView(epId).catch(() => {});
    }
  });

  // Progress saves, throttled to ~10s of playback; the server flips the
  // watchlist to watched past 90%.
  let lastSavedAt = 0;
  let knownDuration = 0;
  function onPlayerTime(position: number, duration: number) {
    playhead = position;
    if (!$auth.isAuthenticated || duration <= 0) return;
    knownDuration = duration;
    if (Math.abs(position - lastSavedAt) < 10) return;
    lastSavedAt = position;
    api.savePlaybackProgress(ep.id, { position, duration }).catch(() => {});
  }
  function onPlayerEnded() {
    if (!$auth.isAuthenticated || knownDuration <= 0) return;
    api
      .savePlaybackProgress(ep.id, { position: knownDuration, duration: knownDuration })
      .catch(() => {});
  }
  const current = $derived(embeds[sourceIdx] ?? null);
  /** True when the iframe — not our own player — is what's on screen.
   *  Mirrors the `{#if stream && useStream}` / `{:else if current}` split
   *  exactly, so the source card marked as playing is always the one playing.
   *  `useStream` alone is not enough: it stays true on an embed-only episode,
   *  where `stream` is null and an iframe is showing regardless. */
  const embedPlaying = $derived(!(stream && useStream));

  /** How many hosts the row shows before collapsing the rest behind "+N". */
  const SRC_COLLAPSED = 4;
  let showAllSources = $state(false);
  // A prefix slice, so an index into this is also the index into `embeds` —
  // `sourceIdx` stays meaningful whether the row is collapsed or expanded.
  const shownEmbeds = $derived(showAllSources ? embeds : embeds.slice(0, SRC_COLLAPSED));

  // ── report a problem with this episode ────────────────────────────────────
  let reportOpen = $state(false);
  let reportText = $state('');
  let reportBusy = $state(false);

  async function sendReport() {
    const body = reportText.trim();
    if (!body || reportBusy) return;
    reportBusy = true;
    try {
      await api.reportEpisode(ep.id, body);
      toast.success('Mulțumim! Raportul a ajuns la echipă.');
      reportOpen = false;
      reportText = '';
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Nu am putut trimite raportul.');
    } finally {
      reportBusy = false;
    }
  }

  const epTitle = $derived(ep.title ? `${ep.title}` : `Episodul ${ep.episodeNumber}`);

  // ── skip-mark editor — content roles only ────────────────────────
  const canEditSkips = $derived(!!$auth.user && ['admin', 'translator'].includes($auth.user.role));
  const skipKinds = ['intro', 'outro'] as const;
  let playhead = $state(0);
  let showSkipEditor = $state(false);
  // Held as mm:ss text, not seconds: marks are read off a timeline, and
  // "1:28" is the number the editor is looking at. Parsed on save.
  let skipEdit = $state({
    intro: { start: '', end: '' },
    outro: { start: '', end: '' }
  });
  let savingSkip = $state(false);

  /** mm:ss / h:mm:ss / plain seconds → seconds. null when it isn't a time. */
  function parseTime(v: string): number | null {
    const t = v.trim().replace(',', '.');
    if (!t) return null;
    if (!/^(\d+:)?(\d+:)?\d+(\.\d+)?$/.test(t)) return null;
    return t
      .split(':')
      .reduce((acc, part) => acc * 60 + parseFloat(part), 0);
  }

  const fmtS = (s: number) => {
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    const sec = Math.floor(s % 60);
    const mm = h > 0 ? String(m).padStart(2, '0') : String(m);
    return `${h > 0 ? `${h}:` : ''}${mm}:${String(sec).padStart(2, '0')}`;
  };

  function fillSkipFields() {
    skipEdit = {
      intro: {
        start: skip.intro ? fmtS(skip.intro.start) : '',
        end: skip.intro ? fmtS(skip.intro.end) : ''
      },
      outro: {
        start: skip.outro ? fmtS(skip.outro.start) : '',
        end: skip.outro ? fmtS(skip.outro.end) : ''
      }
    };
  }

  function toggleSkipEditor() {
    showSkipEditor = !showSkipEditor;
    if (showSkipEditor) fillSkipFields();
  }

  const atPlayhead = () => fmtS(playhead);

  async function saveSkip(kind: 'intro' | 'outro') {
    const start = parseTime(skipEdit[kind].start);
    const end = parseTime(skipEdit[kind].end);
    if (start === null || end === null) {
      toast.error('Folosește mm:ss, de exemplu 1:30.');
      return;
    }
    if (end <= start) {
      toast.error('Sfârșitul trebuie să fie după început.');
      return;
    }
    savingSkip = true;
    try {
      await api.setSkipMark(ep.id, { kind, start, end });
      skip = (await api.getEpisodeSkip(ep.id)).data;
      fillSkipFields();
      toast.success(kind === 'intro' ? 'Intro salvat.' : 'Outro salvat.');
    } catch {
      toast.error('Salvarea marcajului a eșuat.');
    } finally {
      savingSkip = false;
    }
  }

  async function removeSkip(kind: 'intro' | 'outro') {
    savingSkip = true;
    try {
      await api.deleteSkipMark(ep.id, kind);
      skip = { ...skip, [kind]: null };
      skipEdit[kind] = { start: '', end: '' };
      toast.success('Marcaj șters.');
    } catch {
      toast.error('Ștergerea a eșuat.');
    } finally {
      savingSkip = false;
    }
  }

</script>

<!-- One credit in the byline. A snippet rather than a third copy of the same
     markup — the roles differ only by their label. -->
{#snippet credit(
  who: { username: string; avatarUrl?: string | null } | null | undefined,
  role: string,
  title: string
)}
  {#if who}
    <a class="credit" href={`/user/${who.username}`} {title}>
      {#if who.avatarUrl}
        <span class="cr-av"><img src={api.resolveUrl(who.avatarUrl)} alt={who.username} /></span>
      {:else}
        <span class="cr-av monogram" style={`--mg-hue:${nameHue(who.username)}`}>
          {who.username.charAt(0).toUpperCase()}
        </span>
      {/if}
      <span class="cr-txt">
        <span class="cr-role">{role}</span>
        <span class="cr-name">{who.username}</span>
      </span>
    </a>
  {/if}
{/snippet}

<svelte:head><title>Ep. {ep.episodeNumber} · {displayName(a)} · Anime-Kage</title></svelte:head>

<!-- sticky sub-bar -->
<div class="subbar">
  <div class="container subbar-in">
    <a class="back" href={`/anime/${titleRef(a)}`}>← Detalii</a>
    <span class="vr"></span>
    <span class="show-title">{displayName(a)}</span>
  </div>
</div>

<div class="container watch">
  <div class="layout">
    <main>
      <!-- player -->
      <div class="player">
        {#if stream && useStream}
          <VideoPlayer
            src={stream.stream.manifestUrl}
            kind={stream.stream.kind}
            title={`${displayName(a)} — ${epTitle}`}
            poster={mediaUrl(a.imageUrl ?? '')}
            subtitles={stream.stream.subtitles ?? []}
            skipIntro={skip.intro}
            skipOutro={skip.outro}
            startAt={resumeAt}
            onTimeUpdate={onPlayerTime}
            onEnded={onPlayerEnded}
          />
        {:else if current}
          <!--
            No `sandbox`, deliberately (changed July 2026).

            It used to carry allow-scripts + allow-same-origin + allow-presentation
            and pointedly *not* allow-popups / allow-top-navigation, to keep ad
            scripts from opening tabs or navigating the page away. DoodStream —
            and it will not be the only one — detects any sandboxed context and
            refuses to play at all, rendering "Blocked · Sandbox not allowed"
            instead of the video. Since `sandbox` is the only browser mechanism
            that can block popups and top-level navigation, there is no partial
            version of this: either the attribute goes and the host's ads come
            with it, or the episode does not play. Playing won.

            What that costs, so nobody has to rediscover it: the framed page can
            now open popups and can navigate this tab away from the site. It is
            the host's own player behaving the way their free tier behaves.

            What still holds the line:
              - `allow` grants only autoplay/fullscreen/picture-in-picture, so
                camera, microphone, geolocation and the rest stay denied by
                Permissions Policy — that is independent of sandbox.
              - `referrerpolicy=no-referrer` keeps our URLs out of their logs.
              - content-link URLs are still validated server-side (https, public
                host, optional CONTENT_HOSTS allowlist), so this only ever frames
                a host someone with a content role deliberately added.

            An `extract` source played in our own player needs none of this, and
            is the only path that carries our RO subtitle and skip marks.
          -->
          <iframe
            src={current.hostingUrl}
            title={`${displayName(a)} — ${epTitle}`}
            allowfullscreen
            allow="autoplay; fullscreen; picture-in-picture"
            referrerpolicy="no-referrer"
          ></iframe>
        {:else}
          {#if a.imageUrl}
            <div class="ph-bg" style={`background-image:url(${mediaUrl(a.imageUrl)})`}></div>
          {/if}
          <div class="ph-msg">
            <span class="ph-play">▶</span>
            <p>Nicio sursă video disponibilă încă pentru acest episod.</p>
          </div>
        {/if}
      </div>

      <!-- Source picker: one row of pills, the way a streaming site does a
           server list. Most episodes have a dozen hosts and nobody reads
           twelve — the first few are the ones that get used, so only those are
           shown and the rest sit behind "+N". `sourceIdx` resets to 0 on every
           episode change, so the selected source is always inside the
           collapsed set and can never hide behind the toggle. -->
      {#if (stream ? 1 : 0) + embeds.length > 0}
        <div class="srcrow" role="group" aria-label="Surse video">
          <span class="kicker srcrow-label">Surse</span>

          {#if stream}
            <button
              class="srcpill"
              class:on={!embedPlaying}
              aria-pressed={!embedPlaying}
              onclick={() => (useStream = true)}
            >
              Player AK
            </button>
          {/if}

          {#each shownEmbeds as s, i (s.id)}
            {@const active = embedPlaying && i === sourceIdx}
            <button
              class="srcpill"
              class:on={active}
              aria-pressed={active}
              onclick={() => {
                useStream = false;
                sourceIdx = i;
              }}
            >
              {sourceName(s, i)}
            </button>
          {/each}

          {#if embeds.length > SRC_COLLAPSED}
            <button
              class="srcmore"
              aria-expanded={showAllSources}
              onclick={() => (showAllSources = !showAllSources)}
            >
              {showAllSources ? 'mai puțin' : `+${embeds.length - SRC_COLLAPSED}`}
            </button>
          {/if}

          <!-- Pushed to the far end of the same row with margin-left:auto, so
               it keeps its place whether the pills are collapsed or expanded,
               and drops onto its own line only when the row wraps on a phone. -->
          {#if $auth.user}
            <button
              class="reportbtn"
              title="Raportează o problemă la acest episod"
              aria-label="Raportează o problemă la acest episod"
              onclick={() => (reportOpen = true)}
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" aria-hidden="true">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                  d="M3 21v-4m0 0V5a2 2 0 012-2h6.5l1 1H21l-3 6 3 6h-8.5l-1-1H5a2 2 0 00-2 2z" />
              </svg>
              <span class="reportbtn-t">Raportează</span>
            </button>
          {/if}
        </div>
      {/if}

      <!-- skip-mark editor (PLAN 5.4) — admin/translator only -->
      {#if canEditSkips}
        <div class="skipadmin">
          <button class="sk-toggle" onclick={toggleSkipEditor}>
            Marcaje skip (echipă) {showSkipEditor ? '▴' : '▾'}
          </button>
          {#if showSkipEditor}
            <div class="sk-body">
              {#each skipKinds as kind (kind)}
                <div class="sk-row" class:set={!!skip[kind]}>
                  <span class="sk-name">{kind === 'intro' ? 'Intro' : 'Outro'}</span>

                  <span class="sk-field">
                    <input
                      bind:value={skipEdit[kind].start}
                      placeholder="0:00"
                      inputmode="numeric"
                      spellcheck="false"
                      aria-label={`${kind} start`}
                    />
                    <button
                      class="sk-grab"
                      title={`Preia ${fmtS(playhead)} din player`}
                      onclick={() => (skipEdit[kind].start = atPlayhead())}>⤓</button
                    >
                  </span>

                  <span class="sk-arrow">→</span>

                  <span class="sk-field">
                    <input
                      bind:value={skipEdit[kind].end}
                      placeholder="0:00"
                      inputmode="numeric"
                      spellcheck="false"
                      aria-label={`${kind} final`}
                    />
                    <button
                      class="sk-grab"
                      title={`Preia ${fmtS(playhead)} din player`}
                      onclick={() => (skipEdit[kind].end = atPlayhead())}>⤓</button
                    >
                  </span>

                  <button class="sk-ok" disabled={savingSkip} title="Salvează" onclick={() => saveSkip(kind)}>✓</button>
                  <button
                    class="sk-x"
                    disabled={savingSkip || !skip[kind]}
                    title="Șterge marcajul"
                    onclick={() => removeSkip(kind)}>✕</button
                  >
                </div>
              {/each}
            </div>
          {/if}
        </div>
      {/if}

      <!-- title + nav -->
      <div class="ep-head">
        <div class="ep-head-main">
          <p class="ep-label">
            Episodul {ep.episodeNumber}{data.episodes.length ? ` din ${data.episodes.length}` : ''}
            {#if ep.isFiller}<span class="ep-flag filler">filler</span>
            {:else if ep.isRecap}<span class="ep-flag recap">recap</span>{/if}
          </p>
          <h1 class="ep-title" class:filler={ep.isFiller}>{ep.title ?? displayName(a)}</h1>
          <!-- Built from the parts that exist rather than concatenated with
               leading separators: the line used to open with a fixed
               "Subtitrare RO", which every other part hung off. The whole site
               is Romanian, so saying so on each episode was noise. -->
          {#if epMeta.length}
            <p class="ep-meta">{epMeta.join(' · ')}</p>
          {/if}
          {#if credits && (credits.translator || credits.verifier || credits.coordinator)}
            <div class="credits">
              {@render credit(credits.translator, 'Tradus de', 'Traducător')}
              {@render credit(credits.verifier, 'Verificat de', 'Verificator')}
              {@render credit(credits.coordinator, 'Publicat de', 'Coordonator')}
            </div>
          {/if}
        </div>
        <!-- At the edge of a season these roll into the neighbouring one
             instead of disappearing — the end of a season is the moment you
             most need to be told there is a next one. -->
        <div class="ep-nav">
          {#if data.prev !== null}
            <a class="btn ghost" href={`/anime/${titleRef(a)}/episode/${data.prev}`}>← Anterior</a>
          {:else if data.prevSeason}
            <!-- Same button as "Anterior", only the label differs. The
                 destination series is a tooltip rather than a second line: it
                 made the button twice the height of its neighbour for
                 information you get anyway the moment you land there. -->
            <a
              class="btn ghost"
              href={`/anime/${titleRef(data.prevSeason.anime)}/episode/${data.prevSeason.episodeNumber}`}
              title={displayName(data.prevSeason.anime)}>← Sezonul anterior</a
            >
          {/if}
          {#if data.next !== null}
            <a class="btn fill" href={`/anime/${titleRef(a)}/episode/${data.next}`}>Următorul →</a>
          {:else if data.nextSeason}
            <a
              class="btn fill"
              href={`/anime/${titleRef(data.nextSeason.anime)}/episode/${data.nextSeason.episodeNumber}`}
              title={displayName(data.nextSeason.anime)}>Sezonul următor →</a
            >
          {/if}
        </div>
      </div>

      <!-- Episode description, with the same inline editor the series
           description has on /anime/[id]. Falls back to the series synopsis
           when this episode has none, so the space is never blank — but the
           editor only ever writes the episode's own field.
           displaySynopsis, not a.synopsis: this page was reading the raw
           column, so it showed the English text with its "(Source: …)" tail
           even where a Romanian translation existed. -->
      <div class="syn-block">
        {#if ep.synopsis}
          <p class="syn">{ep.synopsis}</p>
        {:else if displaySynopsis(a)}
          <p class="syn muted">{displaySynopsis(a)}</p>
        {/if}

        {#if canEditEpisode}
          {#if !epEditOpen}
            <button class="edit-btn" onclick={openEpEdit}>
              ✎ {ep.synopsis ? 'Editează descrierea' : 'Adaugă o descriere'}
            </button>
          {:else}
            <div class="edit-panel">
              <label class="ed-field">
                <span>Titlul episodului</span>
                <input bind:value={editTitle} placeholder={`Episodul ${ep.episodeNumber}`} />
              </label>
              <label class="ed-field">
                <span>Descrierea episodului</span>
                <textarea
                  bind:value={editSyn}
                  rows="5"
                  maxlength="2000"
                  placeholder="Ce se întâmplă în episod. Lasă gol ca să ștergi descrierea."
                ></textarea>
              </label>
              <div class="ed-flags">
                <label class="check">
                  <input type="checkbox" bind:checked={editFiller} />
                  <span>Filler</span>
                </label>
                <label class="check">
                  <input type="checkbox" bind:checked={editRecap} />
                  <span>Recap</span>
                </label>
                <span class="ed-hint">
                  Marcajele vin de la MyAnimeList când se sincronizează, dar le poți pune și manual.
                </span>
              </div>
              <div class="ed-actions">
                <button class="btn ghost" onclick={() => (epEditOpen = false)}>Anulează</button>
                <button class="btn fill" onclick={saveEpEdit} disabled={epSaving}>
                  {epSaving ? 'Se salvează…' : 'Salvează'}
                </button>
              </div>
            </div>
          {/if}
        {/if}
      </div>

      <!-- Report dialog. A real <dialog>-style overlay rather than an inline
           panel: the form has to be reachable from the source row without
           pushing the player around, and closing it must not lose the text by
           accident — hence explicit Cancel/Send and Escape, but no
           click-outside-to-dismiss. -->
      {#if reportOpen}
        <div
          class="rep-back"
          role="button"
          tabindex="-1"
          onkeydown={(e) => e.key === 'Escape' && (reportOpen = false)}
        >
          <div class="rep-card" role="dialog" aria-modal="true" aria-labelledby="rep-h">
            <h2 id="rep-h" class="rep-h">Raportează o problemă</h2>
            <!-- Names the episode by number and series, not by its own title:
                 "The Journey's End" alone gives no clue which episode of what
                 you are reporting. -->
            <p class="rep-guide">
              Spune-ne pe scurt ce nu merge la <strong>episodul {ep.episodeNumber}</strong>
              din <strong>{displayName(a)}</strong>{ep.title ? ` („${ep.title}”)` : ''}.
              De exemplu: o sursă care nu pornește sau are alt episod, sunet ori imagine
              stricată, subtitrare nepotrivită, marcaje de intro/outro puse greșit, sau
              orice altceva legat de episod. Dacă e vorba de o sursă anume, scrie-i numele
              (ex. Mp4Upload).
            </p>
            <textarea
              class="rep-in"
              bind:value={reportText}
              maxlength="2000"
              rows="5"
              placeholder="Ex.: sursa DoodStream nu pornește, iar pe Filemoon intro-ul e sărit prea devreme."
            ></textarea>
            <div class="rep-foot">
              <span class="rep-count">{reportText.trim().length}/2000</span>
              <div class="rep-actions">
                <button class="btn ghost" onclick={() => (reportOpen = false)}>Anulează</button>
                <button
                  class="btn fill"
                  onclick={sendReport}
                  disabled={reportBusy || !reportText.trim()}
                >
                  {reportBusy ? 'Se trimite…' : 'Trimite raportul'}
                </button>
              </div>
            </div>
          </div>
        </div>
      {/if}

      <!-- per-episode comments -->
      <div class="ep-comments">
        {#key ep.id}
          <CommentSection animeId={a.id} episodeId={ep.id} heading={`Comentarii · Episodul ${ep.episodeNumber}`} />
        {/key}
      </div>
    </main>

    <!-- episode strip -->
    <aside class="panel">
      <div class="panel-head">
        <span class="panel-title">Episoade</span>
        <span class="kicker">{data.episodes.length} ep</span>
      </div>
      <div class="strip">
        {#each data.episodes as e (e.id)}
          <a
            class="row"
            class:on={e.episodeNumber === ep.episodeNumber}
            href={`/anime/${titleRef(a)}/episode/${e.episodeNumber}`}
            aria-current={e.episodeNumber === ep.episodeNumber ? 'page' : undefined}
          >
            <span class="thumb" style={a.imageUrl ? `background-image:url(${mediaUrl(a.imageUrl)})` : ''}>
              <span class="tp">▶</span>
            </span>
            <span class="row-main">
              <span class="row-t" class:filler={e.isFiller} class:recap={e.isRecap}>
                {e.title ?? `Episodul ${e.episodeNumber}`}
              </span>
              <!-- Source count first, then the filler/recap note in brackets:
                   "8 surse (Filler ep)". The count is the line's subject; the
                   note qualifies the episode, so it reads as an aside. -->
              <span class="row-m">
                <span>{e.links?.length ? `${e.links.length} surse` : 'în curând'}</span>
                {#if e.isFiller}<span class="row-flag">(Filler ep)</span>
                {:else if e.isRecap}<span class="row-flag recap">(Recap ep)</span>{/if}
              </span>
            </span>
            <span class="row-n">{e.episodeNumber}</span>
          </a>
        {/each}
      </div>
    </aside>
  </div>
</div>

<style>
  .subbar {
    position: sticky; top: 62px; z-index: 20;
    background: color-mix(in srgb, var(--surface-raised) 88%, transparent);
    backdrop-filter: blur(10px); -webkit-backdrop-filter: blur(10px);
    border-bottom: 1px solid var(--border-subtle);
  }
  .subbar-in { display: flex; align-items: center; gap: 14px; padding-block: 12px; }
  .back { font-size: var(--fs-small); color: var(--text-muted); }
  .back:hover { color: var(--text-primary); }
  .vr { width: 1px; height: 20px; background: var(--border-default); }
  .show-title {
    font-family: var(--font-display); font-size: var(--fs-body); font-weight: var(--fw-semibold);
    min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }

  .watch { padding-block: var(--space-5) var(--space-8); }
  /* 322px left the episode list about 150px for a title, which cut even short
     ones. The player is still the page — it just gives up the width it was
     wasting on letterbox margins at wide viewports. */
  .layout { display: grid; grid-template-columns: minmax(0, 1fr) 400px; gap: var(--space-6); align-items: start; }

  .player {
    position: relative; aspect-ratio: 16 / 9; overflow: hidden;
    border-radius: var(--radius-lg); background: #05070a;
  }
  .player iframe { position: absolute; inset: 0; width: 100%; height: 100%; border: 0; }
  .ph-bg {
    position: absolute; inset: 0; background-size: cover; background-position: center 20%;
    filter: blur(24px) brightness(0.45); transform: scale(1.15);
  }
  .ph-msg {
    position: absolute; inset: 0; display: grid; place-content: center; justify-items: center;
    gap: var(--space-4); text-align: center; padding: var(--space-5);
  }
  .ph-play {
    width: 74px; height: 74px; border-radius: 50%;
    background: rgba(255, 255, 255, 0.14); backdrop-filter: blur(6px);
    border: 1px solid rgba(255, 255, 255, 0.3);
    display: grid; place-items: center; color: #fff; font-size: 1.5rem; padding-left: 5px;
  }
  .ph-msg p { color: var(--text-muted); font-size: var(--fs-small); max-width: 34ch; }

  /* ── source picker ──────────────────────────────────────────────────────
     One row, no container. The label sets it as a group and the pills carry
     the names; anything more turned a secondary control into a panel that
     competed with the player above it. */
  .srcrow {
    display: flex; align-items: center; flex-wrap: wrap; gap: 6px;
    margin-top: var(--space-4);
  }
  .srcrow-label { margin-right: 2px; }
  .srcpill {
    font-size: var(--fs-micro); color: var(--text-muted);
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
    border-radius: 999px; padding: 5px 12px; cursor: pointer; white-space: nowrap;
    transition: color 120ms ease, background 120ms ease, border-color 120ms ease;
  }
  .srcpill:hover { color: var(--text-primary); border-color: var(--border-default); }
  .srcpill:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 2px; }
  /* Filled, not tinted: the selected pill differs in luminance as well as hue,
     so it still reads as selected without colour vision. */
  .srcpill.on {
    background: var(--accent); border-color: var(--accent);
    color: var(--on-accent); font-weight: var(--fw-semibold);
  }
  .srcmore {
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-faint);
    background: none; border: none; cursor: pointer; padding: 5px 6px;
  }
  .srcmore:hover { color: var(--text-primary); }
  .srcmore:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 2px; }

  /* Report control. margin-left:auto parks it at the end of the source row in
     both collapsed and expanded states; when the row wraps on a narrow screen
     it simply lands on the next line, still right-aligned. */
  .reportbtn {
    margin-left: auto;
    display: inline-flex; align-items: center; gap: 6px;
    font-size: var(--fs-micro); color: var(--text-faint);
    background: none; border: 1px solid transparent; border-radius: 999px;
    padding: 5px 10px; cursor: pointer; white-space: nowrap;
    transition: color 120ms ease, border-color 120ms ease, background 120ms ease;
  }
  .reportbtn:hover {
    color: var(--danger); border-color: color-mix(in srgb, var(--danger) 40%, transparent);
    background: color-mix(in srgb, var(--danger) 8%, transparent);
  }
  .reportbtn:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 2px; }
  .reportbtn svg { width: 14px; height: 14px; flex: none; }
  /* The label is a convenience on a mouse and clutter on a phone, where the
     flag alone is unambiguous and the row is already tight. */
  @media (max-width: 560px) {
    .reportbtn-t { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
    .reportbtn { padding: 0 10px; min-height: var(--tap-min); }
  }

  /* report dialog */
  .rep-back {
    position: fixed; inset: 0; z-index: 60;
    background: rgba(0, 0, 0, 0.62);
    display: grid; place-items: center; padding: var(--space-4);
  }
  .rep-card {
    width: min(560px, 100%);
    background: var(--surface-raised);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    padding: 18px 18px 14px;
    max-height: 90vh; overflow-y: auto;
  }
  .rep-h { font-family: var(--font-display); font-size: var(--fs-h3); margin-bottom: 6px; }
  .rep-guide {
    font-size: var(--fs-caption); color: var(--text-muted);
    line-height: var(--lh-normal); margin-bottom: 12px;
  }
  .rep-in {
    width: 100%; resize: vertical;
    background: var(--surface-inset); color: var(--text-primary);
    border: 1px solid var(--border-default); border-radius: var(--radius-sm);
    padding: 10px 12px; font: inherit; font-size: var(--fs-small);
  }
  .rep-in:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 1px; }
  .rep-foot {
    display: flex; align-items: center; justify-content: space-between;
    gap: 10px; margin-top: 10px; flex-wrap: wrap;
  }
  .rep-count { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-faint); }
  .rep-actions { display: flex; gap: 8px; margin-left: auto; }
  /* Pills are deliberately small on a mouse. On touch they still have to be
     reachable, so grow them there rather than padding them out everywhere. */
  @media (pointer: coarse) {
    .srcpill, .srcmore {
      display: inline-flex; align-items: center;
      min-height: var(--tap-min); padding-block: 0;
    }
  }

  /* skip-mark editor (5.4) */
  .skipadmin { margin-top: var(--space-3); }
  .sk-toggle {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.08em; text-transform: uppercase; color: var(--text-muted);
    background: none; border: none; cursor: pointer; padding: 4px 0;
  }
  .sk-toggle:hover { color: var(--text-muted); }
  .sk-body {
    display: flex; flex-direction: column; gap: 8px;
    border: 1px dashed var(--border-default); border-radius: var(--radius-md);
    background: var(--surface-inset); padding: 12px 14px; margin-top: 6px;
  }
  /* Two identical rows, each one sentence: Intro [from] → [to] ✓ ✕.
     No captions on the fields — the shape says it, and only the team sees it. */
  .sk-row { display: flex; align-items: center; gap: 8px; }
  .sk-name {
    font-size: var(--fs-caption); font-weight: var(--fw-semibold);
    color: var(--text-muted); width: 58px; flex: 0 0 auto;
  }
  .sk-row.set .sk-name { color: var(--text-primary); }
  .sk-field {
    position: relative; display: inline-flex; align-items: center; flex: 0 0 auto;
  }
  .sk-field input {
    width: 104px; min-height: 36px; padding: 0 30px 0 10px;
    font-family: var(--font-mono); font-size: var(--fs-caption);
    background: var(--surface-raised); border: 1px solid var(--border-default);
    border-radius: var(--radius-sm); color: var(--text-primary); outline: none;
  }
  .sk-field input:focus { border-color: var(--accent); }
  /* grab-the-playhead lives inside its own field, so there's no doubt which
     one it fills */
  .sk-grab {
    position: absolute; right: 4px;
    width: 24px; height: 24px; border-radius: 6px; border: none;
    background: transparent; color: var(--text-faint); cursor: pointer;
    font-size: 0.8125rem; line-height: 1; display: grid; place-items: center;
  }
  .sk-grab:hover { background: var(--surface-overlay); color: var(--accent); }
  .sk-arrow { color: var(--text-faint); font-size: var(--fs-caption); flex: 0 0 auto; }
  .sk-ok, .sk-x {
    width: 36px; height: 36px; flex: 0 0 auto;
    display: grid; place-items: center;
    font-size: 0.8125rem; border-radius: var(--radius-sm); cursor: pointer;
    border: 1px solid var(--border-default); background: var(--surface-raised);
  }
  .sk-ok { color: var(--accent); border-color: color-mix(in srgb, var(--accent) 45%, transparent); }
  .sk-ok:hover:not(:disabled) { background: color-mix(in srgb, var(--accent) 12%, transparent); }
  .sk-x { color: var(--danger); border-color: color-mix(in srgb, var(--danger) 40%, transparent); }
  .sk-x:hover:not(:disabled) { background: color-mix(in srgb, var(--danger) 12%, transparent); }
  .sk-ok:disabled, .sk-x:disabled { opacity: 0.35; cursor: default; }
  @media (max-width: 560px) {
    .sk-row { flex-wrap: wrap; }
    .sk-field { flex: 1 1 108px; }
    .sk-field input { width: 100%; }
  }

  .ep-head {
    display: flex; align-items: flex-start; justify-content: space-between;
    gap: var(--space-5); margin-top: 20px; flex-wrap: wrap;
  }
  .ep-head-main { min-width: 0; }
  .ep-label { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--accent); }
  .ep-title { font-size: 1.625rem; letter-spacing: -0.01em; margin-top: 6px; }
  /* Filler in red, with the word next to it — the colour alone would not reach
     a red-green colourblind reader, and "can I skip this" is the point. */
  .ep-title.filler { color: var(--danger); }
  .ep-flag {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.1em; text-transform: uppercase; margin-left: 8px;
    padding: 2px 7px; border-radius: 4px;
  }
  .ep-flag.filler { color: var(--danger); background: color-mix(in srgb, var(--danger) 14%, transparent); }
  .ep-flag.recap { color: var(--text-muted); background: var(--surface-overlay); }
  .ep-meta { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); margin-top: 6px; }

  /* translator / verifier credits (the RO track's people) */
  /* A byline, not a pair of chips. The bordered, pill-shaped, tinted version
     read as UI chrome competing with the episode title; credits are editorial
     information, so they get the same treatment as a magazine credit line —
     a hairline rule between them and no box of their own. */
  .credits {
    display: flex; flex-wrap: wrap; align-items: center; gap: 0 var(--space-4);
    margin-top: 14px; padding-top: 12px;
    border-top: 1px solid var(--border-subtle);
  }
  .credit {
    display: flex; align-items: center; gap: 9px;
    padding: 2px 0; color: inherit;
  }
  /* hairline between credits instead of two separate outlines */
  .credit + .credit {
    padding-left: var(--space-4);
    border-left: 1px solid var(--border-subtle);
  }
  /* the monogram tile, not a gradient circle — matches the avatar convention
     used everywhere else on the site */
  .cr-av {
    width: 26px; height: 26px; border-radius: 26%; flex: 0 0 auto; overflow: hidden;
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

  .ep-nav { display: flex; gap: 10px; flex-shrink: 0; }
  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 11px 18px; border-radius: var(--radius-md); white-space: nowrap;
  }
  .btn.ghost { border: 1px solid var(--border-default); color: var(--text-primary); }
  .btn.ghost:hover { background: var(--surface-raised); color: var(--text-primary); }
  .btn.fill { background: var(--accent); color: var(--on-accent); }
  .btn.fill:hover { background: var(--accent-hover); color: var(--on-accent); }

  .syn { font-size: var(--fs-small); line-height: 1.65; color: var(--text-muted); max-width: 640px; margin-top: 18px; }
  /* the series synopsis standing in for a missing episode one — dimmer, so it
     doesn't read as a description of this episode */
  .syn.muted { color: var(--text-faint); }

  /* ---- episode editor (mirrors the series editor on /anime/[id]) ---- */
  .syn-block { max-width: 640px; }
  .edit-btn {
    margin-top: 12px; padding: 6px 12px; cursor: pointer;
    font: inherit; font-size: var(--fs-caption); font-weight: var(--fw-semibold);
    background: none; border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-muted);
  }
  .edit-btn:hover { color: var(--text-primary); border-color: var(--border-strong); }
  .edit-panel {
    margin-top: 16px; padding: var(--space-4);
    border: 1px solid var(--border-default); border-radius: var(--radius-md);
    background: var(--surface-raised);
    display: flex; flex-direction: column; gap: var(--space-4);
  }
  .ed-field { display: flex; flex-direction: column; gap: 6px; }
  .ed-field span {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.12em; text-transform: uppercase; color: var(--text-muted);
  }
  .ed-field input,
  .ed-field textarea {
    width: 100%; padding: 10px 12px; font: inherit; font-size: var(--fs-small);
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-sm); color: var(--text-primary); outline: none; resize: vertical;
  }
  .ed-field input:focus, .ed-field textarea:focus { border-color: var(--accent); }
  .ed-flags { display: flex; align-items: center; gap: var(--space-4); flex-wrap: wrap; }
  .check { display: flex; align-items: center; gap: 7px; cursor: pointer; font-size: var(--fs-small); color: var(--text-primary); }
  .check input { accent-color: var(--accent); width: 16px; height: 16px; }
  .ed-hint { font-size: var(--fs-micro); color: var(--text-muted); flex: 1; min-width: 12rem; }
  .ed-actions { display: flex; justify-content: flex-end; gap: 9px; }
  .ep-comments { margin-top: var(--space-7); max-width: 780px; }

  /* episode strip */
  .panel {
    border: 1px solid var(--border-subtle); border-radius: var(--radius-lg);
    background: var(--surface-raised); overflow: hidden;
  }
  .panel-head {
    display: flex; align-items: baseline; justify-content: space-between;
    padding: 15px 16px; border-bottom: 1px solid var(--border-subtle);
  }
  .panel-title { font-family: var(--font-display); font-size: var(--fs-body); font-weight: var(--fw-semibold); }
  .strip { max-height: 520px; overflow-y: auto; padding: 10px; display: flex; flex-direction: column; gap: 6px; }
  .row {
    display: flex; align-items: center; gap: 11px;
    padding: 8px 10px; border-radius: var(--radius-md); border: 1px solid transparent;
  }
  .row:hover { background: var(--surface-overlay); }
  .row.on { background: var(--surface-overlay); border-color: color-mix(in srgb, var(--accent) 45%, transparent); }
  .thumb {
    position: relative; width: 58px; height: 34px; border-radius: var(--radius-sm); flex: 0 0 auto;
    background-color: var(--surface-overlay); background-size: cover; background-position: center 20%;
    display: grid; place-items: center; overflow: hidden;
  }
  .tp { color: rgba(255, 255, 255, 0.78); font-size: var(--fs-micro); }
  .row-main { flex: 1; min-width: 0; display: flex; flex-direction: column; }
  /* Two lines instead of one, clamped. `nowrap` + ellipsis meant even short
     titles were cut, because the row only had ~150px of text column; two lines
     of a wider panel fit most of them whole and long ones still degrade
     gracefully. line-clamp needs the flex item to be a block, hence the
     explicit display. */
  .row-t {
    font-size: var(--fs-small); font-weight: var(--fw-semibold); color: var(--text-primary);
    line-height: 1.3; text-wrap: pretty;
    display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2;
    -webkit-box-orient: vertical; overflow: hidden;
  }
  .row-t.filler { color: var(--danger); }
  .row-t.recap { color: var(--text-muted); }
  .row-m {
    display: flex; align-items: baseline; gap: 6px;
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); margin-top: 3px;
  }
  .row-flag { color: var(--danger); font-weight: var(--fw-semibold); }
  .row-flag.recap { color: var(--text-muted); }
  .row-n { font-family: var(--font-display); font-size: var(--fs-body); font-weight: var(--fw-semibold); color: var(--text-muted); }

  @media (max-width: 900px) {
    .layout { grid-template-columns: minmax(0, 1fr); }
    .strip { max-height: 320px; }
    .show-title { display: none; }
    .subbar .vr { display: none; }
  }
</style>
