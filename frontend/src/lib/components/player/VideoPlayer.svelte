<script lang="ts">
  // Our own player — plays the manifest returned by
  // /api/episodes/:id/stream.
  //: video.js → HLS.js on a native
  // <video> so our <track> subtitles (3.5) and skip marks (3.6) work.
  import { onMount } from 'svelte';
  import Hls from 'hls.js';

  interface SkipRange {
    start: number;
    end: number;
  }
  interface SubTrack {
    url: string;
    language: string;
    label?: string;
  }

  let {
    src,
    kind = 'hls',
    title = '',
    poster = '',
    subtitles = [],
    skipIntro = null,
    skipOutro = null,
    startAt = 0,
    onTimeUpdate,
    onEnded
  }: {
    src: string;
    kind?: 'hls' | 'mp4';
    title?: string;
    poster?: string;
    subtitles?: SubTrack[];
    skipIntro?: SkipRange | null;
    skipOutro?: SkipRange | null;
    /** resume position in seconds, applied once when metadata loads */
    startAt?: number;
    onTimeUpdate?: (currentTime: number, duration: number) => void;
    onEnded?: () => void;
  } = $props();

  let videoEl = $state<HTMLVideoElement>();
  let wrapperEl = $state<HTMLDivElement>();
  let hls: Hls | null = null;

  let loading = $state(true);
  let error = $state('');
  let isPlaying = $state(false);
  // The poster only stands in while there is genuinely nothing to show. The
  // moment the video has decoded a frame it is showing that frame — including
  // after a reload that seeks straight to a resume point — and covering it
  // would hide the picture until the user pressed play.
  let hasFrame = $state(false);
  let isBuffering = $state(false);
  let duration = $state(0);
  let currentTime = $state(0);
  let bufferedPct = $state(0);
  let volume = $state(1);
  let isMuted = $state(false);
  let isFullscreen = $state(false);
  let showControls = $state(true);
  let showSettings = $state(false);
  let playbackSpeed = $state(1);
  let qualities = $state<{ label: string; level: number }[]>([]);
  let currentLevel = $state(-1);
  let autoSkip = $state(false);
  // subtitles: index into `subtitles`, -1 = off
  let subIndex = $state(-1);
  let showCC = $state(false);
  let cueBox = $state<HTMLDivElement>();
  // height of one letterbox bar, so cues sit above the picture and not above
  // the black band under it when the source doesn't match the box
  let letterbox = $state(0);

  const showSkipIntro = $derived(
    !!skipIntro && currentTime >= skipIntro.start && currentTime < skipIntro.end
  );
  const showSkipOutro = $derived(
    !!skipOutro && currentTime >= skipOutro.start && currentTime < skipOutro.end
  );
  const controlsVisible = $derived(showControls || !isPlaying || showSettings || showCC);

  // ── subtitles ──────────────────────────────────────────────────────────────
  // We render cues ourselves instead of letting the browser draw them. Native
  // cue boxes are pinned to the bottom of the <video> ELEMENT, which puts them
  // in the letterbox band and under the control bar, and `::cue` cannot move
  // them — position/line are only settable per-cue, from inside the file.
  // Rendering them means we also control where they sit when the controls come
  // up. Trade-off: iOS hands fullscreen to the system player, which draws its
  // own cue layer and ignores this one.
  const subLabel = (t: SubTrack) => t.label ?? t.language.toUpperCase();
  const SUB_PREF = 'ak-player-subs';

  // Pick the starting track whenever the list changes: the remembered choice,
  // else Romanian — that's the track we exist to ship.
  $effect(() => {
    const list = subtitles;
    if (!list.length) {
      subIndex = -1;
      return;
    }
    const saved = typeof localStorage !== 'undefined' ? localStorage.getItem(SUB_PREF) : null;
    if (saved === 'off') {
      subIndex = -1;
      return;
    }
    const byLang = (l: string) => list.findIndex((t) => t.language === l);
    const i = saved ? byLang(saved) : -1;
    subIndex = i >= 0 ? i : Math.max(0, byLang('ro'));
  });

  function selectSub(i: number) {
    subIndex = i;
    localStorage.setItem(SUB_PREF, i < 0 ? 'off' : subtitles[i].language);
    showCC = false;
    pokeControls();
  }

  function renderCues() {
    const box = cueBox;
    if (!box) return;
    box.replaceChildren();
    const el = videoEl;
    if (!el || subIndex < 0) return;
    const cues = el.textTracks[subIndex]?.activeCues;
    if (!cues) return;
    for (let i = 0; i < cues.length; i++) {
      const cue = cues[i] as VTTCue;
      const line = document.createElement('div');
      line.className = 'cue-line';
      // getCueAsHTML is the browser's own VTT parse: it keeps the <i>/<b> the
      // file asked for and cannot yield anything else, which is exactly what
      // dropping cue.text into {@html} would fail to guarantee
      line.appendChild(
        typeof cue.getCueAsHTML === 'function'
          ? cue.getCueAsHTML()
          : document.createTextNode(cue.text)
      );
      box.appendChild(line);
    }
  }

  // 'hidden' keeps cuechange firing while the browser draws nothing; every
  // other track goes 'disabled' so it stops parsing.
  $effect(() => {
    const el = videoEl;
    const idx = subIndex;
    subtitles.length; // re-run when the <track> list changes
    if (!el) return;
    const tracks = el.textTracks;
    const onChange = () => renderCues();
    for (let i = 0; i < tracks.length; i++) {
      tracks[i].mode = i === idx ? 'hidden' : 'disabled';
      tracks[i].addEventListener('cuechange', onChange);
    }
    renderCues();
    return () => {
      for (let i = 0; i < tracks.length; i++) tracks[i].removeEventListener('cuechange', onChange);
    };
  });

  // object-fit: contain letterboxes the picture; measure the band so cues can
  // be placed against the image rather than the element.
  function recomputeLetterbox() {
    const el = videoEl;
    if (!el?.videoWidth || !el.videoHeight || !el.clientHeight) {
      letterbox = 0;
      return;
    }
    const scale = Math.min(el.clientWidth / el.videoWidth, el.clientHeight / el.videoHeight);
    letterbox = Math.max(0, (el.clientHeight - el.videoHeight * scale) / 2);
  }

  $effect(() => {
    if (!wrapperEl || typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(recomputeLetterbox);
    ro.observe(wrapperEl);
    return () => ro.disconnect();
  });

  function destroyHls() {
    if (hls) {
      hls.destroy();
      hls = null;
    }
  }

  // (Re)load the source whenever it changes.
  $effect(() => {
    const el = videoEl;
    const url = src;
    const k = kind;
    if (!el || !url) return;

    loading = true;
    error = '';
    hasFrame = false;
    qualities = [];
    currentLevel = -1;
    destroyHls();

    if (k === 'hls' && Hls.isSupported()) {
      hls = new Hls({ maxBufferLength: 30 });
      hls.loadSource(url);
      hls.attachMedia(el);
      hls.on(Hls.Events.MANIFEST_PARSED, (_ev, data) => {
        loading = false;
        const levels = data.levels
          .map((l, i) => ({ label: l.height ? `${l.height}p` : `Nivel ${i + 1}`, level: i }))
          .sort((a, b) => b.level - a.level);
        qualities = levels.length > 1 ? [{ label: 'Auto', level: -1 }, ...levels] : [];
      });
      hls.on(Hls.Events.ERROR, (_ev, data) => {
        if (!data.fatal) return;
        if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
          hls?.startLoad();
        } else if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
          hls?.recoverMediaError();
        } else {
          loading = false;
          error = 'Nu s-a putut încărca fluxul video.';
        }
      });
    } else {
      // mp4, or native HLS (Safari)
      el.src = url;
    }

    return () => destroyHls();
  });

  onMount(() => {
    const saved = localStorage.getItem('ak-player-volume');
    if (saved !== null) {
      volume = Math.max(0, Math.min(1, parseFloat(saved)));
    }
    autoSkip = localStorage.getItem('ak-player-autoskip') === '1';
    if (videoEl) {
      videoEl.volume = volume;
      isMuted = volume === 0;
      videoEl.muted = isMuted;
    }

    document.addEventListener('keydown', handleKeydown);
    document.addEventListener('fullscreenchange', handleFullscreenChange);
    return () => {
      document.removeEventListener('keydown', handleKeydown);
      document.removeEventListener('fullscreenchange', handleFullscreenChange);
      clearTimeout(hideTimer);
    };
  });

  // ── controls auto-hide ─────────────────────────────────────────────────────
  let hideTimer: ReturnType<typeof setTimeout>;

  function pokeControls() {
    showControls = true;
    clearTimeout(hideTimer);
    if (isPlaying && !showSettings) {
      hideTimer = setTimeout(() => {
        if (!showSettings) showControls = false;
      }, 2500);
    }
  }

  function handleMouseLeave() {
    clearTimeout(hideTimer);
    if (isPlaying && !showSettings) {
      hideTimer = setTimeout(() => {
        if (!showSettings) showControls = false;
      }, 800);
    }
  }

  // ── video events ───────────────────────────────────────────────────────────
  function handleTimeUpdate() {
    if (!videoEl) return;
    currentTime = videoEl.currentTime;
    duration = videoEl.duration || 0;
    // auto-skip jumps once, right as playback enters the range — the +1s
    // window keeps a manual seek back into the intro from re-triggering it
    if (autoSkip && skipIntro && currentTime >= skipIntro.start && currentTime < skipIntro.start + 1) {
      videoEl.currentTime = skipIntro.end;
    }
    onTimeUpdate?.(currentTime, duration);
  }

  function toggleAutoSkip() {
    autoSkip = !autoSkip;
    localStorage.setItem('ak-player-autoskip', autoSkip ? '1' : '0');
  }

  function handleProgress() {
    if (!videoEl || !videoEl.duration) return;
    const b = videoEl.buffered;
    if (b.length > 0) bufferedPct = (b.end(b.length - 1) / videoEl.duration) * 100;
  }

  function handleLoadedMetadata() {
    loading = false;
    duration = videoEl?.duration || 0;
    recomputeLetterbox();
    // resume, unless the saved position is essentially the end
    if (videoEl && startAt > 0 && (!duration || startAt < duration * 0.95)) {
      videoEl.currentTime = startAt;
    }
  }

  // ── actions ────────────────────────────────────────────────────────────────
  function togglePlay() {
    if (!videoEl) return;
    if (videoEl.paused) videoEl.play();
    else videoEl.pause();
  }

  function seekBy(delta: number) {
    if (!videoEl) return;
    videoEl.currentTime = Math.max(0, Math.min(duration, videoEl.currentTime + delta));
    pokeControls();
  }

  function handleSeek(e: Event) {
    if (!videoEl) return;
    videoEl.currentTime = parseFloat((e.currentTarget as HTMLInputElement).value);
  }

  function setVolume(v: number) {
    volume = Math.max(0, Math.min(1, v));
    if (videoEl) {
      videoEl.volume = volume;
      videoEl.muted = volume === 0;
    }
    isMuted = volume === 0;
    localStorage.setItem('ak-player-volume', String(volume));
  }

  function toggleMute() {
    if (isMuted) {
      const prev = parseFloat(localStorage.getItem('ak-player-prev-volume') ?? '0.5');
      setVolume(prev > 0 ? prev : 0.5);
    } else {
      localStorage.setItem('ak-player-prev-volume', String(volume));
      setVolume(0);
    }
  }

  function toggleFullscreen() {
    if (!document.fullscreenElement) {
      // iPhone Safari does not implement the Fullscreen API on ordinary
      // elements: div.requestFullscreen is undefined, so `?.` did nothing at
      // all and the button looked broken. The <video> element does have
      // Apple's own webkitEnterFullscreen, which hands off to the system
      // player -- the only route to fullscreen on that device.
      //
      // Trade-off, already noted above: the system player draws its own cues,
      // so our subtitle styling is replaced by Apple's. Fullscreen that works
      // beats fullscreen that is merely styled the way we want.
      if (wrapperEl?.requestFullscreen) {
        wrapperEl.requestFullscreen().catch(() => {});
        return;
      }
      const v = videoEl as (HTMLVideoElement & { webkitEnterFullscreen?: () => void }) | undefined;
      v?.webkitEnterFullscreen?.();
    } else {
      document.exitFullscreen().catch(() => {});
    }
  }

  function handleFullscreenChange() {
    isFullscreen = !!document.fullscreenElement;
    recomputeLetterbox();
    pokeControls();
  }

  function setSpeed(speed: number) {
    if (videoEl) videoEl.playbackRate = speed;
    playbackSpeed = speed;
  }

  function setQuality(level: number) {
    if (hls) hls.currentLevel = level;
    currentLevel = level;
  }

  function doSkip(range: SkipRange | null) {
    if (videoEl && range) videoEl.currentTime = range.end;
  }

  // ── keyboard ───────────────────────────────────────────────────────────────
  function handleKeydown(e: KeyboardEvent) {
    const t = e.target as HTMLElement;
    if (
      t.tagName === 'INPUT' ||
      t.tagName === 'TEXTAREA' ||
      t.tagName === 'SELECT' ||
      t.isContentEditable
    ) {
      return;
    }
    switch (e.code) {
      case 'Space':
        togglePlay();
        break;
      case 'ArrowRight':
        seekBy(10);
        break;
      case 'ArrowLeft':
        seekBy(-10);
        break;
      case 'ArrowUp':
        setVolume(volume + 0.1);
        break;
      case 'ArrowDown':
        setVolume(volume - 0.1);
        break;
      case 'KeyM':
        toggleMute();
        break;
      case 'KeyF':
        toggleFullscreen();
        break;
      case 'KeyC':
        // cycle the subtitle track, off included — the shortcut every player has
        if (subtitles.length) selectSub(subIndex + 1 >= subtitles.length ? -1 : subIndex + 1);
        break;
      case 'Escape':
        if (!showCC && !showSettings) return;
        showCC = false;
        showSettings = false;
        break;
      default:
        return;
    }
    e.preventDefault();
    pokeControls();
  }

  // ── mobile double-tap: left/right thirds seek, middle toggles ─────────────
  let lastTap = 0;
  function handleTouch(e: TouchEvent) {
    const now = Date.now();
    if (now - lastTap < 400 && wrapperEl) {
      const x = e.touches[0]?.clientX ?? 0;
      const { left, width } = wrapperEl.getBoundingClientRect();
      const rel = (x - left) / width;
      if (rel < 1 / 3) seekBy(-10);
      else if (rel > 2 / 3) seekBy(10);
      else togglePlay();
      e.preventDefault();
    } else {
      showControls = !showControls;
    }
    lastTap = now;
  }

  function formatTime(s: number): string {
    if (!Number.isFinite(s)) return '0:00';
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    const sec = Math.floor(s % 60);
    return h > 0
      ? `${h}:${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`
      : `${m}:${String(sec).padStart(2, '0')}`;
  }

  function pct(t: number): number {
    return duration > 0 ? Math.max(0, Math.min(100, (t / duration) * 100)) : 0;
  }
</script>

<div
  class="player-root"
  class:fullscreen={isFullscreen}
  bind:this={wrapperEl}
  onmousemove={pokeControls}
  onmouseleave={handleMouseLeave}
  ontouchstart={handleTouch}
  role="presentation"
>
  <!-- The poster is OUR layer, not the <video poster> attribute. A video
       element with crossorigin="anonymous" fetches its poster with CORS too,
       and the catalog art comes from MAL/AniList CDNs that send no
       Access-Control-Allow-Origin — so the attribute silently fails on every
       such title. The crossorigin flag itself has to stay: our <track>
       subtitles are served from the API origin and would not load without it.
       A plain <img> is not subject to the video's CORS mode. -->
  {#if poster && !hasFrame}
    <img class="poster" src={poster} alt="" />
  {/if}

  <!-- svelte-ignore a11y_media_has_caption -->
  <video
    bind:this={videoEl}
    playsinline
    preload="auto"
    crossorigin="anonymous"
    aria-label={title}
    onclick={() => {
      // a click on the picture dismisses an open menu instead of toggling
      // playback — the same thing every other player does
      if (showCC || showSettings) {
        showCC = false;
        showSettings = false;
        return;
      }
      togglePlay();
    }}
    ontimeupdate={handleTimeUpdate}
    onprogress={handleProgress}
    onloadedmetadata={handleLoadedMetadata}
    onloadeddata={() => (hasFrame = true)}
    onplay={() => {
      isPlaying = true;
      pokeControls();
    }}
    onpause={() => {
      isPlaying = false;
      showControls = true;
    }}
    onwaiting={() => (isBuffering = true)}
    onplaying={() => {
      isBuffering = false;
      hasFrame = true;
    }}
    oncanplay={() => (isBuffering = false)}
    onended={() => onEnded?.()}
    onerror={() => {
      if (!hls) {
        loading = false;
        error = 'Nu s-a putut încărca fluxul video.';
      }
    }}
  >
    {#each subtitles as t (t.url)}
      <track
        kind="subtitles"
        src={t.url}
        srclang={t.language}
        label={t.label ?? t.language.toUpperCase()}
        default={t.language === 'ro'}
      />
    {/each}
  </video>

  <div
    class="cues"
    class:lifted={controlsVisible}
    style={`--lb:${letterbox}px`}
    bind:this={cueBox}
    aria-live="off"
  ></div>

  {#if loading && !error}
    <div class="overlay">
      <div class="spinner"></div>
      <span class="overlay-text">Se încarcă…</span>
    </div>
  {:else if error}
    <div class="overlay overlay-error"><p>{error}</p></div>
  {:else if isBuffering}
    <div class="buffering"><div class="spinner"></div></div>
  {/if}

  {#if !isPlaying && !loading && !error}
    <button class="big-play" onclick={togglePlay} aria-label="Redă">
      <svg width="44" height="44" viewBox="0 0 24 24" fill="currentColor">
        <polygon points="6 3 21 12 6 21 6 3"></polygon>
      </svg>
    </button>
  {/if}

  {#if showSkipIntro}
    <button class="skip-btn" onclick={() => doSkip(skipIntro)}>Sari peste intro →</button>
  {/if}
  {#if showSkipOutro}
    <button class="skip-btn" onclick={() => doSkip(skipOutro)}>Sari peste outro →</button>
  {/if}

  <div class="controls" class:visible={controlsVisible}>
    <!-- progress -->
    <div class="progress">
      <input
        type="range"
        min="0"
        max={duration || 100}
        step="0.1"
        value={currentTime}
        oninput={handleSeek}
        aria-label="Derulează"
      />
      <div class="track">
        {#if skipIntro && duration > 0}
          <div
            class="marker"
            style="left:{pct(skipIntro.start)}%;width:{pct(skipIntro.end) - pct(skipIntro.start)}%"
          ></div>
        {/if}
        {#if skipOutro && duration > 0}
          <div
            class="marker"
            style="left:{pct(skipOutro.start)}%;width:{pct(skipOutro.end) - pct(skipOutro.start)}%"
          ></div>
        {/if}
        <div class="buffer-fill" style="width:{bufferedPct}%"></div>
        <div class="progress-fill" style="width:{pct(currentTime)}%">
          <div class="handle"></div>
        </div>
      </div>
    </div>

    <div class="bar">
      <div class="bar-left">
        <button class="ctl" onclick={togglePlay} aria-label={isPlaying ? 'Pauză' : 'Redă'}>
          {#if isPlaying}
            <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
              <rect x="6" y="4" width="4" height="16"></rect>
              <rect x="14" y="4" width="4" height="16"></rect>
            </svg>
          {:else}
            <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
              <polygon points="6 3 21 12 6 21 6 3"></polygon>
            </svg>
          {/if}
        </button>

        <div class="volume">
          <button class="ctl" onclick={toggleMute} aria-label={isMuted ? 'Activează sunetul' : 'Dezactivează sunetul'}>
            {#if isMuted || volume === 0}
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M11 5L6 9H2v6h4l5 4V5zM23 9l-6 6M17 9l6 6"></path>
              </svg>
            {:else if volume < 0.5}
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M11 5L6 9H2v6h4l5 4V5z M15 8a5 5 0 0 1 0 8"></path>
              </svg>
            {:else}
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M11 5L6 9H2v6h4l5 4V5z M15 8a5 5 0 0 1 0 8 M19 5a9 9 0 0 1 0 14"></path>
              </svg>
            {/if}
          </button>
          <div class="volume-pop">
            <input
              class="volume-slider"
              type="range"
              min="0"
              max="1"
              step="0.01"
              value={volume}
              oninput={(e) => setVolume(parseFloat((e.currentTarget as HTMLInputElement).value))}
              aria-label="Volum"
            />
          </div>
        </div>

        <span class="time">{formatTime(currentTime)} <em>/</em> {formatTime(duration)}</span>
      </div>

      <div class="bar-right">
        {#if subtitles.length}
          <div class="cc-wrap">
            <button
              class="ctl cc"
              class:active={showCC}
              class:off={subIndex < 0}
              onclick={() => {
                showCC = !showCC;
                showSettings = false;
                if (showCC) clearTimeout(hideTimer);
              }}
              aria-label="Subtitrări"
              aria-expanded={showCC}
            >
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                <rect x="2" y="5" width="20" height="14" rx="3"></rect>
                <path d="M10 10.2a2.4 2.4 0 0 0-3.6 2 2.4 2.4 0 0 0 3.6 2" stroke-linecap="round"></path>
                <path d="M17.6 10.2a2.4 2.4 0 0 0-3.6 2 2.4 2.4 0 0 0 3.6 2" stroke-linecap="round"></path>
              </svg>
            </button>
            {#if showCC}
              <div class="cc-menu" role="menu">
                {#each subtitles as t, i (t.url)}
                  <button class="cc-opt" class:on={subIndex === i} onclick={() => selectSub(i)}>
                    {subLabel(t)}
                  </button>
                {/each}
                <button class="cc-opt" class:on={subIndex < 0} onclick={() => selectSub(-1)}>
                  Dezactivat
                </button>
              </div>
            {/if}
          </div>
        {/if}

        <!-- ±10s. One glyph each: a three-quarter arc with the arrowhead at
             the gap and the seconds inside it — same stroke weight and box as
             every other control, so the row reads as one set. -->
        <button class="ctl seek back" onclick={() => seekBy(-10)} aria-label="Înapoi 10 secunde">
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none">
            <path
              d="M12 5.4a8 8 0 1 1-8 8"
              stroke="currentColor"
              stroke-width="1.8"
              stroke-linecap="round"
            ></path>
            <path d="M13.1 2.2 8.3 5.4l4.8 3.2z" fill="currentColor"></path>
            <text
              x="12"
              y="16.6"
              text-anchor="middle"
              font-size="7.6"
              font-weight="700"
              fill="currentColor"
              stroke="none">10</text
            >
          </svg>
        </button>
        <button class="ctl seek fwd" onclick={() => seekBy(10)} aria-label="Înainte 10 secunde">
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none">
            <path
              d="M12 5.4a8 8 0 1 0 8 8"
              stroke="currentColor"
              stroke-width="1.8"
              stroke-linecap="round"
            ></path>
            <path d="M10.9 2.2 15.7 5.4l-4.8 3.2z" fill="currentColor"></path>
            <text
              x="12"
              y="16.6"
              text-anchor="middle"
              font-size="7.6"
              font-weight="700"
              fill="currentColor"
              stroke="none">10</text
            >
          </svg>
        </button>

        <button
          class="ctl"
          class:active={showSettings}
          onclick={() => {
            showSettings = !showSettings;
            showCC = false;
            if (showSettings) clearTimeout(hideTimer);
          }}
          aria-label="Setări"
        >
          <svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="3"></circle>
            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
          </svg>
        </button>
        <button class="ctl" onclick={toggleFullscreen} aria-label={isFullscreen ? 'Ieși din ecran complet' : 'Ecran complet'}>
          {#if isFullscreen}
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M8 3v3a2 2 0 0 1-2 2H3m18 0h-3a2 2 0 0 1-2-2V3m0 18v-3a2 2 0 0 1 2-2h3M3 16h3a2 2 0 0 1 2 2v3"></path>
            </svg>
          {:else}
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M8 3H5a2 2 0 0 0-2 2v3m18 0V5a2 2 0 0 0-2-2h-3m0 18h3a2 2 0 0 0 2-2v-3M3 16v3a2 2 0 0 0 2 2h3"></path>
            </svg>
          {/if}
        </button>
      </div>
    </div>
  </div>

  {#if showSettings}
    <div class="settings" role="menu" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
      <div class="settings-section">
        <h4>Viteză</h4>
        <div class="opts">
          {#each [0.5, 0.75, 1, 1.25, 1.5, 2] as speed (speed)}
            <button class="opt" class:on={playbackSpeed === speed} onclick={() => setSpeed(speed)}>
              {speed === 1 ? 'Normal' : `${speed}x`}
            </button>
          {/each}
        </div>
      </div>
      {#if qualities.length > 0}
        <div class="settings-section">
          <h4>Calitate</h4>
          <div class="opts">
            {#each qualities as q (q.level)}
              <button class="opt" class:on={currentLevel === q.level} onclick={() => setQuality(q.level)}>
                {q.label}
              </button>
            {/each}
          </div>
        </div>
      {/if}
      {#if skipIntro || skipOutro}
        <div class="settings-section">
          <h4>Intro / Outro</h4>
          <div class="opts opts-wide">
            <button class="opt" class:on={autoSkip} onclick={toggleAutoSkip}>
              {autoSkip ? 'Sari automat: pornit' : 'Sari automat: oprit'}
            </button>
          </div>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .player-root {
    position: relative;
    width: 100%;
    height: 100%;
    background: #000;
    overflow: hidden;
    /* cue text sizes off the player, not the viewport — a page-embedded
       player and a fullscreen one need very different pixel sizes for the
       same apparent caption size */
    container-type: inline-size;
  }
  video {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: contain;
    cursor: pointer;
  }
  /* stands in for the native `poster` attribute — see the comment on the
     markup. Behind every overlay, and gone once the first frame paints. */
  .poster {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: contain;
    z-index: 1;
    pointer-events: none;
  }

  /* Our own cue layer (see the comment on `subLabel`). `--lb` is the
     letterbox band, so the baseline follows the picture; 7% above it is the
     broadcast-ish safe area, and the whole block rises clear of the control
     bar while that's on screen. */
  .cues {
    position: absolute;
    left: 6%;
    right: 6%;
    bottom: calc(var(--lb, 0px) + 7%);
    z-index: 3;
    text-align: center;
    pointer-events: none;
    transition: bottom 0.18s ease;
  }
  .cues.lifted {
    bottom: calc(var(--lb, 0px) + 92px);
  }
  /* :global — these nodes are built in renderCues(), so they never get the
     component's scoping class */
  .cues :global(.cue-line) {
    display: inline-block;
    max-width: 100%;
    color: #fff;
    font-family: var(--font-body);
    font-size: clamp(14px, 3cqw, 34px);
    font-weight: var(--fw-medium, 500);
    line-height: 1.35;
    text-wrap: balance;
    text-shadow:
      0 0 3px rgba(0, 0, 0, 0.95),
      0 2px 5px rgba(0, 0, 0, 0.85),
      0 0 10px rgba(0, 0, 0, 0.6);
  }
  .cues :global(i) {
    font-style: italic;
  }
  .cues :global(b) {
    font-weight: var(--fw-bold);
  }

  /* overlays */
  .overlay {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 12px;
    background: rgba(0, 0, 0, 0.6);
    color: #fff;
    z-index: 2;
    pointer-events: none;
  }
  .overlay-error p {
    color: #ff6b6b;
    font-size: var(--fs-small);
    padding: 20px;
    text-align: center;
  }
  .overlay-text {
    font-size: var(--fs-small);
    color: rgba(255, 255, 255, 0.85);
  }
  .buffering {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    z-index: 2;
    pointer-events: none;
  }
  .spinner {
    width: 44px;
    height: 44px;
    border: 3px solid rgba(255, 255, 255, 0.25);
    border-top-color: #fff;
    border-radius: 50%;
    animation: spin 0.9s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .big-play {
    position: absolute;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    width: 78px;
    height: 78px;
    border-radius: 50%;
    border: 1px solid rgba(255, 255, 255, 0.3);
    background: rgba(0, 0, 0, 0.55);
    backdrop-filter: blur(6px);
    color: #fff;
    display: grid;
    place-items: center;
    padding-left: 6px;
    cursor: pointer;
    z-index: 2;
    transition: background 0.2s, transform 0.2s;
  }
  .big-play:hover {
    background: var(--accent);
    color: var(--on-accent);
    transform: translate(-50%, -50%) scale(1.07);
  }

  .skip-btn {
    position: absolute;
    right: 26px;
    bottom: 92px;
    z-index: 4;
    padding: 11px 20px;
    font-size: var(--fs-small);
    font-weight: var(--fw-semibold);
    color: #fff;
    background: rgba(0, 0, 0, 0.45);
    border: 1px solid rgba(255, 255, 255, 0.4);
    border-radius: var(--radius-md);
    backdrop-filter: blur(4px);
    cursor: pointer;
    text-shadow: 0 2px 4px rgba(0, 0, 0, 0.5);
    transition: border-color 0.2s, transform 0.2s;
  }
  .skip-btn:hover {
    border-color: #fff;
    transform: translateY(-1px);
  }

  /* controls */
  .controls {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 3;
    padding: 14px 16px 12px;
    background: linear-gradient(transparent, rgba(0, 0, 0, 0.75));
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.25s ease;
  }
  .controls.visible {
    opacity: 1;
    pointer-events: auto;
  }

  .progress {
    position: relative;
    height: 22px;
    cursor: pointer;
  }
  .progress input[type='range'] {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    opacity: 0;
    cursor: pointer;
    z-index: 2;
    margin: 0;
  }
  .track {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 8px;
    height: 4px;
    border-radius: 2px;
    background: rgba(255, 255, 255, 0.14);
    transition: height 0.15s ease;
  }
  .progress:hover .track {
    height: 6px;
  }
  .buffer-fill {
    position: absolute;
    top: 0;
    left: 0;
    height: 100%;
    background: rgba(255, 255, 255, 0.25);
    border-radius: 2px;
  }
  .progress-fill {
    position: absolute;
    top: 0;
    left: 0;
    height: 100%;
    min-width: 2px;
    background: var(--accent);
    border-radius: 2px;
  }
  .handle {
    position: absolute;
    top: 50%;
    right: -6px;
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: #fff;
    border: 2px solid var(--accent);
    transform: translateY(-50%) scale(0);
    opacity: 0;
    transition: transform 0.15s, opacity 0.15s;
  }
  .progress:hover .handle {
    transform: translateY(-50%) scale(1);
    opacity: 1;
  }
  .marker {
    position: absolute;
    top: 0;
    height: 100%;
    min-width: 2px;
    background: rgba(255, 77, 77, 0.8);
    border-radius: 2px;
    pointer-events: none;
  }

  .bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-top: 2px;
  }
  .bar-left,
  .bar-right {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .ctl {
    background: none;
    border: none;
    color: rgba(255, 255, 255, 0.85);
    padding: 6px;
    display: grid;
    place-items: center;
    cursor: pointer;
    border-radius: var(--radius-sm);
    transition: color 0.15s;
  }
  .ctl:hover {
    color: #fff;
    background: rgba(255, 255, 255, 0.12);
  }
  .ctl:active {
    background: rgba(255, 255, 255, 0.18);
  }
  .ctl.active {
    color: var(--accent);
  }
  /* the ±10s pair spins its arc on press, so the direction of the jump is
     legible without reading the number */
  .ctl.seek svg {
    transition: transform 0.25s var(--ease, ease);
  }
  .ctl.seek.back:active svg {
    transform: rotate(-40deg);
  }
  .ctl.seek.fwd:active svg {
    transform: rotate(40deg);
  }
  @media (prefers-reduced-motion: reduce) {
    .ctl.seek svg,
    .ctl.seek:active svg {
      transition: none;
      transform: none;
    }
  }

  .time {
    font-family: var(--font-mono);
    font-size: var(--fs-caption);
    color: rgba(255, 255, 255, 0.85);
    margin-left: 6px;
    white-space: nowrap;
  }
  .time em {
    font-style: normal;
    opacity: 0.6;
    margin-inline: 3px;
  }

  /* volume popup (CSS hover) */
  .volume {
    position: relative;
    display: flex;
    align-items: center;
  }
  .volume-pop {
    position: absolute;
    bottom: calc(100% + 8px);
    left: 50%;
    transform: translateX(-50%);
    background: rgba(20, 20, 20, 0.92);
    border-radius: var(--radius-md);
    padding: 12px 10px;
    opacity: 0;
    visibility: hidden;
    transition: opacity 0.15s, visibility 0.15s;
  }
  .volume:hover .volume-pop,
  .volume-pop:hover {
    opacity: 1;
    visibility: visible;
  }
  .volume-slider {
    writing-mode: vertical-lr;
    direction: rtl;
    width: 20px;
    height: 84px;
    cursor: pointer;
    accent-color: var(--accent);
  }

  /* settings menu */
  /* CC: a small menu anchored to its own button, not the settings sheet —
     switching subtitle language mid-scene shouldn't cost two taps */
  .cc-wrap {
    position: relative;
    display: inline-flex;
  }
  .ctl.cc.off {
    opacity: 0.55;
  }
  .cc-menu {
    position: absolute;
    right: 0;
    bottom: calc(100% + 10px);
    z-index: 6;
    min-width: 132px;
    display: flex;
    flex-direction: column;
    gap: 3px;
    padding: 5px;
    background: rgba(20, 20, 20, 0.95);
    backdrop-filter: blur(10px);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: var(--radius-md);
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
  }
  .cc-opt {
    padding: 7px 11px;
    border: none;
    border-radius: 7px;
    background: transparent;
    color: rgba(255, 255, 255, 0.85);
    cursor: pointer;
    text-align: left;
    font-family: inherit;
    font-size: var(--fs-caption);
    white-space: nowrap;
  }
  .cc-opt:hover {
    background: rgba(255, 255, 255, 0.12);
    color: #fff;
  }
  .cc-opt.on {
    background: var(--accent);
    color: var(--on-accent);
    font-weight: var(--fw-semibold);
  }

  .settings {
    position: absolute;
    right: 12px;
    bottom: 64px;
    z-index: 5;
    min-width: 220px;
    background: rgba(20, 20, 20, 0.95);
    backdrop-filter: blur(10px);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: var(--radius-lg);
    padding: 14px;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
  }
  .settings-section + .settings-section {
    margin-top: 16px;
  }
  .settings h4 {
    font-size: var(--fs-caption);
    font-weight: var(--fw-semibold);
    color: rgba(255, 255, 255, 0.7);
    margin-bottom: 8px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .opts {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 6px;
  }
  .opts-wide {
    grid-template-columns: minmax(0, 1fr);
  }
  .opt {
    background: rgba(255, 255, 255, 0.08);
    border: 1px solid rgba(255, 255, 255, 0.1);
    color: rgba(255, 255, 255, 0.9);
    padding: 7px 10px;
    border-radius: var(--radius-sm);
    font-size: var(--fs-caption);
    cursor: pointer;
    transition: background 0.15s;
  }
  .opt:hover {
    background: rgba(255, 255, 255, 0.16);
  }
  .opt.on {
    background: var(--accent);
    border-color: transparent;
    color: var(--on-accent);
    font-weight: var(--fw-semibold);
  }

  .fullscreen .skip-btn {
    right: 40px;
    bottom: 110px;
  }
</style>
