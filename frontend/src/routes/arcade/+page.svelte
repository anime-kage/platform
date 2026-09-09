<script lang="ts">
  import { onMount } from 'svelte';
  import api from '$lib/api';
  import { authStore } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import { mediaUrl } from '$lib/media';
  import type { GameLeaderRow } from '$shared/types';
  import { genreRo, seasonRo, displayName } from '$lib/types';
  import type { GameBoard, GamePuzzle, GameFaction } from '$shared/types';

  /**
   * The arcade.
   *
   * Restructured around one full-width content area with tabs, after the
   * sidebar version put four unrelated things on screen at once. Identity is a
   * thin strip at the top, and factions and badges are destinations rather than
   * permanent furniture — they are read occasionally, not while playing.
   */

  const TABS = [
    { code: 'title', label: 'Titlu', icon: '✎', game: true, ready: true },
    { code: 'poster', label: 'Poster', icon: '▧', game: true, ready: true },
    { code: 'theme', label: 'Opening & Ending', icon: '♪', game: true, ready: true },
    { code: 'character', label: 'Personaje', icon: '☻', game: true, ready: true },
    { code: 'groups', label: 'Grupe', icon: '◫', game: true, ready: true },
    { code: 'factions', label: 'Facțiuni', icon: '⚑', game: false, ready: true },
    { code: 'badges', label: 'Insigne', icon: '❖', game: false, ready: true },
    { code: 'leaderboard', label: 'Clasament', icon: '≡', game: false, ready: true }
  ] as const;

  let board = $state<GameBoard | null>(null);

  /* Fetched when the tab is first opened, not with the board: the board is
     already the heaviest request in the arcade and most visits never open this. */
  let leaders = $state<GameLeaderRow[] | null>(null);
  let leadersBusy = $state(false);
  async function loadLeaders() {
    if (leaders || leadersBusy) return;
    leadersBusy = true;
    try { leaders = (await api.getGameLeaderboard()).data ?? []; }
    catch { leaders = []; }
    finally { leadersBusy = false; }
  }
  let loading = $state(true);
  let tab = $state<string>('title');
  let lastGame = $state<string>('title');
  let pickedDate = $state<string | null>(null);
  let guess = $state('');
  let busy = $state(false);
  let suggestions = $state<{ id: number; title: string }[]>([]);
  let showSug = $state(false);
  let expanded = $state<string | null>(null);
  let lastAward = $state<{ xp: number; gold: number } | null>(null);
  let rejected = $state('');
  let chestWin = $state<{ gold: number; streak: number } | null>(null);

  /* XP won before choosing a faction is banked, not lost, so this asks once a
     session rather than after every win -- there is nothing urgent to warn
     about, only something waiting to be collected. */
  let factionPrompt = $state<number | null>(null);
  let promptedOnce = false;
  function maybeAskFaction() {
    const pending = board?.profile.pendingXp ?? 0;
    if (promptedOnce || board?.profile.faction || pending <= 0) return;
    promptedOnce = true;
    factionPrompt = pending;
  }

  // The chest rolls over on the server's date, so the countdown runs off the
  // timestamp the board hands us rather than the viewer's own midnight. The
  // ticker only exists while the chest is spent -- once it is claimable again
  // there is nothing left to count down to.
  let now = $state(Date.now());
  let lastRoll = 0;
  $effect(() => {
    if (board?.chest.canClaim !== false) return;
    const t = setInterval(() => (now = Date.now()), 1000);
    return () => clearInterval(t);
  });
  const chestLeft = $derived(
    board ? Math.max(0, new Date(board.chest.nextAt).getTime() - now) : 0
  );
  const chestClock = $derived.by(() => {
    const total = Math.floor(chestLeft / 1000);
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${pad(Math.floor(total / 3600))}:${pad(Math.floor(total / 60) % 60)}:${pad(total % 60)}`;
  });
  // At zero the new chest is already waiting server-side; pick it up instead of
  // making the player reload. Retried at most every 10s, in case our clock
  // crossed midnight a moment before the server's did.
  $effect(() => {
    if (!board || board.chest.canClaim || chestLeft > 0) return;
    if (now - lastRoll > 10000) { lastRoll = now; load(); }
  });

  /* Audio player. Native controls carry the browser's own chrome, which looks
     nothing like the rest of the page. */
  let audioEl = $state<HTMLAudioElement | null>(null);
  let playing = $state(false);
  let cur = $state(0);
  let dur = $state(0);
  /* Video buffering, so the paid clue does not look like it did nothing. */
  let vidLoad = $state(0);
  let vidReady = $state(false);
  let vidEl = $state<HTMLVideoElement | null>(null);
  let vidPlaying = $state(false);
  let vidMuted = $state(false);

  /* Character round. One submission for the whole board, so the answers are
     held locally until the player presses Trimite. */
  type Slot = { charId: number; charName: string; animeId: number; animeTitle: string };
  let slots = $state<Slot[]>([]);
  let openBox = $state<string | null>(null);   // "<index>-name" | "<index>-show"
  let boxHits = $state<{ id: number; label: string; animeId?: number }[]>([]);
  let boxTimer: ReturnType<typeof setTimeout> | null = null;
  let charScore = $state<{ names: number; series: number; xp: number; gold: number } | null>(null);

  /* Sized in an effect, not from the template. Calling a state setter during
     render re-enters the render it was called from; an effect runs after it
     settles and only when the round actually changes.

     Keyed on the puzzle id, NOT on the slot count. Every character round has
     the same five boxes, so a length test never fires when you switch day --
     and yesterday's half-typed answers stayed sitting in today's boxes.

     slotsFor is a plain let rather than $state on purpose: this effect writes
     it, and a reactive one would re-run the effect that had just set it. */
  let roundFor: number | null = null;
  $effect(() => {
    const p = current;
    if (!p || roundFor === p.id) return;
    roundFor = p.id;
    // Everything below is what the player typed into the round they just left.
    // The day buttons only cleared lastAward, so a half-typed guess followed
    // by Enter was submitted against the newly selected day and cost a real
    // attempt on it.
    guess = '';
    suggestions = [];
    showSug = false;
    rejected = '';
    charScore = null;
    openBox = null;
    boxHits = [];
    const n = p.chars?.length ?? 0;
    slots = n
      ? Array.from({ length: n }, () => ({ charId: 0, charName: '', animeId: 0, animeTitle: '' }))
      : [];
  });

  function boxInput(i: number, field: 'name' | 'show', value: string) {
    openBox = `${i}-${field}`;
    if (field === 'name') { slots[i].charName = value; slots[i].charId = 0; }
    else { slots[i].animeTitle = value; slots[i].animeId = 0; }
    if (boxTimer) clearTimeout(boxTimer);
    if (value.trim().length < 2) { boxHits = []; return; }
    boxTimer = setTimeout(async () => {
      try {
        if (field === 'name') {
          const r = await api.searchGameCharacters(value.trim());
          boxHits = (r.data ?? []).map((c) => ({ id: c.id, label: c.name }));
        } else {
          const r = await api.searchAnime(value.trim(), { limit: 8 } as never);
          boxHits = (r.data ?? []).slice(0, 8).map((a) => ({ id: a.id, label: displayName(a) }));
        }
      } catch { boxHits = []; }
    }, 180);
  }

  function pickBox(i: number, field: 'name' | 'show', hit: { id: number; label: string }) {
    if (field === 'name') { slots[i].charId = hit.id; slots[i].charName = hit.label; }
    else { slots[i].animeId = hit.id; slots[i].animeTitle = hit.label; }
    openBox = null; boxHits = [];
  }

  async function submitChars() {
    const p = current;
    if (!p || busy || p.scored) return;
    busy = true;
    try {
      const res = await api.submitCharRound(
        p.id, slots.map((s) => ({ charId: s.charId, animeId: s.animeId }))
      );
      replace(res.data);
      charScore = res.score;
      await refreshProfile();
      maybeAskFaction();
    } catch (e) {
      toast.error((e as { error?: string })?.error ?? 'Nu am putut trimite răspunsurile.');
    } finally { busy = false; }
  }

  function toggleVid() {
    if (!vidEl) return;
    if (vidEl.paused) vidEl.play(); else vidEl.pause();
  }
  function toggleMute() {
    if (!vidEl) return;
    vidEl.muted = !vidEl.muted;
    vidMuted = vidEl.muted;
  }

  function togglePlay() {
    if (!audioEl) return;
    if (audioEl.paused) audioEl.play(); else audioEl.pause();
  }
  function seek(e: MouseEvent) {
    if (!audioEl || !dur) return;
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    audioEl.currentTime = ((e.clientX - r.left) / r.width) * dur;
  }
  function onVidProgress(e: Event) {
    const v = e.currentTarget as HTMLVideoElement;
    if (!v.duration || !v.buffered.length) return;
    vidLoad = Math.min(100, Math.round((v.buffered.end(v.buffered.length - 1) / v.duration) * 100));
  }
  const mmss = (t: number) =>
    !t || !isFinite(t) ? '0:00' : `${Math.floor(t / 60)}:${String(Math.floor(t % 60)).padStart(2, '0')}`;
  let sugTimer: ReturnType<typeof setTimeout> | null = null;

  const auth = $derived($authStore);
  const isGame = $derived(TABS.find((t) => t.code === tab)?.game ?? false);
  const inMode = $derived(board && isGame ? board.puzzles.filter((p) => p.mode === tab) : []);
  const current = $derived.by(() => {
    if (!inMode.length) return null;
    if (pickedDate) return inMode.find((p) => p.playDate === pickedDate) ?? inMode[0];
    return inMode[0];
  });
  const faction = $derived.by(() =>
    board?.factions.find((f) => f.code === board?.profile.faction) ?? null
  );

  onMount(load);

  async function load() {
    loading = true;
    try {
      board = (await api.getGameBoard()).data;
    } catch {
      toast.error('Nu am putut încărca arcade-ul.');
    } finally {
      loading = false;
    }
  }

  /* Only the profile moved, so keep the puzzle the player is reading. */
  async function refreshProfile() {
    try {
      const fresh = (await api.getGameBoard()).data;
      if (board) board = { ...board, profile: fresh.profile };
    } catch { /* what is on screen is still correct */ }
  }

  function pickTab(code: string, ready: boolean) {
    if (!ready) return;
    // Clicking the tab you are already on normally does nothing, and closing it
    // would leave an empty page. But Facțiuni and Insigne are places you visit
    // and leave, so pressing them again returns to the game you came from --
    // the shortcut people expect from "close", without the empty state.
    const meta = TABS.find((t) => t.code === code);
    if (tab === code && meta && !meta.game) {
      tab = lastGame;
      return;
    }
    if (meta?.game) lastGame = code;
    tab = code;
    if (code === 'leaderboard') loadLeaders();
    pickedDate = null;
    guess = '';
    suggestions = [];
    lastAward = null;
    charScore = null;
    slots = [];
  }

  /* Suggestions come from the catalogue: a guess has to resolve to a real
     series for the grid to compare anything, so picking beats typing. */
  function onGuessInput() {
    showSug = true;
    if (sugTimer) clearTimeout(sugTimer);
    const q = guess.trim();
    if (q.length < 2) { suggestions = []; return; }
    sugTimer = setTimeout(async () => {
      try {
        const res = await api.searchAnime(q, { limit: 8 } as never);
        // English/Romanian first: a romaji title is not what people search by.
        suggestions = (res.data ?? []).slice(0, 8).map((a) => ({ id: a.id, title: displayName(a) }));
      } catch { suggestions = []; }
    }, 180);
  }

  /* Grupe: which tiles are currently picked. Cleared after every submission,
     right or wrong, because leaving a wrong set selected invites sending it
     again and spending a second mistake on the same idea. */
  let picked = $state<number[]>([]);
  let sendingGroup = $state(false);


  /* Said on the board rather than in a toast: a toast slides in at the edge,
     away from where the eye already is, and is gone before you have read it.
     The near-miss line is the useful half -- three of four means the idea was
     right and one title was wrong. */
  let groupMsg = $state<{ text: string; tone: 'hit' | 'near' | 'miss' } | null>(null);

  function togglePick(id: number) {
    groupMsg = null;
    if (picked.includes(id)) picked = picked.filter((x) => x !== id);
    else if (picked.length < 3) picked = [...picked, id];
  }

  /* What is still on the board: anything not inside a group already found. */
  function openTiles(p: GamePuzzle) {
    const taken = new Set((p.found ?? []).flatMap((g) => g.animeIds));
    return (p.tiles ?? []).filter((t) => !taken.has(t.animeId));
  }

  /* The catalogue decides how a genre reads in Romanian, including which ones
     deliberately stay English (Mecha, Isekai, Slice of Life). The server sends
     the raw value so the arcade can use that same helper and never drift from
     the rest of the site. Studio and year need no translation. */
  function groupLabel(g: { kind: string; value: string; label: string }) {
    if (g.kind === 'genre' && g.value) return `Gen: ${genreRo(g.value)}`;
    return g.label;
  }

  function titlesOf(p: GamePuzzle, ids: number[]) {
    return ids
      .map((id) => (p.tiles ?? []).find((t) => t.animeId === id)?.title ?? '')
      .filter(Boolean)
      .join(' · ');
  }

  async function sendGroup(p: GamePuzzle) {
    if (picked.length !== 3 || sendingGroup) return;
    sendingGroup = true;
    try {
      const { data } = await api.submitGroup(p.id, picked);
      const before = (p.found ?? []).length;
      replace({
        ...p,
        found: data.found,
        mistakes: data.mistakes,
        maxMistakes: data.maxMistakes,
        finished: data.finished,
        solved: data.solved,
        awardedXp: data.award.xp
      });
      if (data.found.length > before) {
        groupMsg = { text: 'Grupă găsită', tone: 'hit' };
      } else {
        groupMsg =
          data.closest === 2
            ? { text: 'Aproape, două din trei sunt din aceeași grupă', tone: 'near' }
            : { text: 'Nicio legătură între ele', tone: 'miss' };
      }
      if (data.finished) groupMsg = null;
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Nu am putut trimite grupa');
    } finally {
      picked = [];
      sendingGroup = false;
    }
  }

  function replace(next: GamePuzzle) {
    if (!board) return;
    board = { ...board, puzzles: board.puzzles.map((p) => (p.id === next.id ? next : p)) };
  }

  async function submit(text?: string) {
    const p = current;
    const value = (text ?? guess).trim();
    if (!p || p.finished || !value || busy) return;
    busy = true; showSug = false; rejected = '';
    try {
      const res = await api.guessPuzzle(p.id, value);
      replace(res.data);
      guess = ''; suggestions = [];
      // No toast for right or wrong — the grid and the result panel say it.
      if (res.data.solved) {
        lastAward = { xp: res.award?.xp ?? 0, gold: res.award?.gold ?? 0 };
        await refreshProfile();
        maybeAskFaction();
      }
    } catch (e) {
      // Inline, not a toast: a rejected guess costs no attempt, and the reason
      // belongs where the player just typed.
      rejected = (e as { error?: string })?.error ?? 'Nu am putut trimite răspunsul.';
    } finally { busy = false; }
  }

  async function takeHint() {
    const p = current;
    if (!p || !p.hintReady || busy) return;
    busy = true;
    try { replace((await api.hintPuzzle(p.id)).data); }
    catch (e) { toast.error((e as { error?: string })?.error ?? 'Nu am putut arăta descrierea.'); }
    finally { busy = false; }
  }

  async function giveUp() {
    const p = current;
    if (!p || p.finished || busy) return;
    busy = true;
    try { replace((await api.guessPuzzle(p.id, '', true)).data); }
    catch { toast.error('Nu am putut renunța.'); }
    finally { busy = false; }
  }

  async function openChest() {
    if (!board?.chest.canClaim || busy) return;
    busy = true;
    try {
      const res = await api.claimChest();
      chestWin = res.data;
      await load();
    } catch (e) {
      toast.error((e as { error?: string })?.error ?? 'Nu am putut deschide cufărul.');
    } finally { busy = false; }
  }

  async function chooseFaction(f: GameFaction) {
    if (board?.profile.faction === f.code) return;
    try {
      const res = await api.setFaction(f.code);
      await load();
      expanded = null;
      factionPrompt = null;
      if (res.claimedXp > 0) toast.success(`Ai revendicat ${res.claimedXp} XP pentru ${f.name}.`);
    }
    catch (e) { toast.error((e as { error?: string })?.error ?? 'Nu am putut schimba facțiunea.'); }
  }

  /* Mirrors games.XPForLevel — 100*L^1.5 — so the picker can show what any
     faction would return you to. */
  function levelOf(xp: number): number {
    if (xp <= 0) return 1;
    let l = Math.max(1, Math.floor(Math.pow(xp / 100, 1 / 1.5)));
    while (100 * Math.pow(l + 1, 1.5) <= xp) l++;
    while (l > 1 && 100 * Math.pow(l, 1.5) > xp) l--;
    return l;
  }
  function rankOf(f: GameFaction, xp: number): string {
    const lvl = levelOf(xp);
    let t = f.ranks[0]?.title ?? '';
    for (const r of f.ranks) if (lvl >= r.minLevel) t = r.title;
    return t;
  }

  const dayLabel = (iso: string) =>
    new Date(iso + 'T00:00:00').toLocaleDateString('ro-RO', { day: 'numeric', month: 'short' });
  const pct = (p: { xpInto: number; xpSpan: number }) =>
    p.xpSpan > 0 ? Math.min(100, Math.round((p.xpInto / p.xpSpan) * 100)) : 0;
  /* OP1/ED2 -> the right word, so the theme mode does not announce itself as
     the poster game. */
  function gameTitle(p: GamePuzzle): string {
    if (p.mode === 'title') return 'Ghicește anime-ul';
    if (p.mode === 'poster') return 'Ghicește posterul';
    if (p.mode === 'character') return 'Ghicește personajele';
    if (p.mode === 'groups') return 'Găsește grupele';
    const ed = (p.themeSlug ?? '').toUpperCase().startsWith('ED');
    return ed ? 'Ghicește ending-ul' : 'Ghicește opening-ul';
  }

  /* Art is optional. Every image falls back to the glyph it replaced, so a
     missing or half-finished asset set degrades one icon at a time rather than
     leaving holes in the page. See static/arcade/README.md for the filenames. */
  let missingArt = $state<string[]>([]);
  const artTried = new Set<string>();
  const hasArt = (k: string) => !missingArt.includes(k);
  const noArt = (k: string) => { if (!missingArt.includes(k)) missingArt = [...missingArt, k]; };

  /* A dropped request -- a redeploy mid-load, a flaky connection -- used to
     retire the artwork for the whole session, which is why the chest sometimes
     only appeared after a few refreshes. Give each image one retry before
     falling back to the glyph. */
  function artFailed(key: string, e: Event) {
    const img = e.currentTarget as HTMLImageElement;
    if (artTried.has(key)) { noArt(key); return; }
    artTried.add(key);
    const base = img.src.split('?')[0];
    setTimeout(() => { img.src = `${base}?retry=${Date.now()}`; }, 500);
  }

  const arrow = (dir?: string) => (dir === 'up' ? '↑' : dir === 'down' ? '↓' : '');

  /* The catalogue owns these labels (lib/types.ts). Using its helpers rather
     than a second map keeps the arcade reading exactly like every anime page,
     including the deliberate choice to leave anime-native genres in English. */
  const cellText = (key: string, v: string) =>
    key === 'season' ? seasonRo(v) : key === 'genres' ? genreRo(v) : v;
</script>

<svelte:window onkeydown={(e) => {
  if (e.key !== 'Escape') return;
  if (chestWin) chestWin = null;
  else if (factionPrompt !== null) factionPrompt = null;
}} />

<svelte:head><title>Arcade · Anime-Kage</title></svelte:head>

<div class="wrap">
  {#if loading}
    <div class="panel skel">Se încarcă…</div>
  {:else if !board}
    <div class="panel skel">Arcade-ul nu este disponibil acum.</div>
  {:else}
    <!-- ══ identity strip ═══════════════════════════════════════════════ -->
    <section class="strip">
      <div class="avatar" aria-hidden="true">
        {#if auth.user?.avatarUrl}
          <img src={mediaUrl(auth.user.avatarUrl)} alt="" />
        {:else}<span>{board.profile.username.charAt(0).toUpperCase()}</span>{/if}
      </div>
      <div class="who">
        <span class="name">{board.profile.username}</span>
        <span class="rank">
          {#if faction}
            {#if hasArt(`fac:${faction.code}`)}
              <img class="fac-logo sm" data-faction={faction.code} src={`/arcade/factions/${faction.code}.png`} alt=""
                   onerror={(e) => artFailed(`fac:${faction.code}`, e)} />
            {:else}{faction.emoji}{/if}
            {board.profile.rank}
          {:else}
            <button type="button" class="no-fac" onclick={() => pickTab('factions', true)}>
              fără facțiune{#if (board.profile.pendingXp ?? 0) > 0}
                <em>· {board.profile.pendingXp} XP în așteptare</em>{/if}
            </button>
          {/if}
        </span>
      </div>
      <div class="lvl">
        <div class="lvl-top">
          <span>Nivel {board.profile.level}</span>
          <span class="mono">{board.profile.xpInto}/{board.profile.xpSpan} XP</span>
        </div>
        <div class="bar"><span style={`width:${pct(board.profile)}%`}></span></div>
      </div>
      <div class="stat">
        <span class="s-num gold">
          {#if hasArt('coin')}
            <img class="coin" src="/arcade/aur.png" alt="" onerror={(e) => artFailed('coin', e)} />
          {:else}
            <svg class="coin" viewBox="0 0 20 20" aria-hidden="true">
              <defs>
                <linearGradient id="akCoinFace" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stop-color="#F8DE84" />
                  <stop offset="52%" stop-color="#E0A51C" />
                  <stop offset="100%" stop-color="#A96D08" />
                </linearGradient>
              </defs>
              <circle cx="10" cy="10" r="9" fill="url(#akCoinFace)" stroke="#7C4E06" stroke-width="1" />
              <circle cx="10" cy="10" r="6.2" fill="none" stroke="#FBEAAE" stroke-width="0.9" opacity="0.45" />
              <ellipse cx="7.1" cy="6.4" rx="2.7" ry="1.5" fill="#FFF6D2" opacity="0.5"
                       transform="rotate(-30 7.1 6.4)" />
            </svg>
          {/if}{board.profile.gold.toLocaleString('ro-RO')}
        </span>
        <span class="s-lbl">gold</span>
      </div>
      <button
        type="button"
        class="chest"
        class:ready={board.chest.canClaim}
        onclick={openChest}
        disabled={!board.chest.canClaim || busy}
        title={board.chest.canClaim ? 'Deschide cufărul zilnic' : 'Revino mâine'}
      >
        <span class="ch-ico" aria-hidden="true">
          {#if hasArt('chest')}
            <img src={board.chest.canClaim ? '/arcade/chest-closed.png' : '/arcade/chest-open.png'}
                 alt="" onerror={(e) => artFailed('chest', e)} />
          {:else}{board.chest.canClaim ? '🎁' : '📦'}{/if}
        </span>
        <span class="ch-txt">
          {#if board.chest.canClaim}<strong>Deschide</strong>
          {:else}<span class="ch-clock">{chestClock}</span>{/if}
        </span>
      </button>

      <button type="button" class="stat as-btn" onclick={() => pickTab('badges', true)}>
        <span class="s-num">{board.profile.badges.length}<em>/{board.badgeCatalogue.length}</em></span>
        <span class="s-lbl">insigne</span>
      </button>
    </section>

    {#if factionPrompt !== null}
      <div
        class="cw-backdrop"
        role="presentation"
        onclick={(e) => { if (e.target === e.currentTarget) factionPrompt = null; }}
      >
        <div class="cw-box" role="dialog" aria-modal="true"
             aria-label="Alege o facțiune" tabindex="-1">
          <h3 class="cw-h">Ai ghicit!</h3>
          <p class="cw-line">
            Ai strâns <strong>{factionPrompt} XP</strong>, dar încă nu ai o facțiune.
          </p>
          <p class="cw-streak">
            Nu se pierde nimic — XP-ul te așteaptă și intră direct în facțiunea pe care o alegi.
          </p>
          <div class="cw-row">
            <button class="btn fill" onclick={() => { factionPrompt = null; pickTab('factions', true); }}>
              Alege o facțiune
            </button>
            <button class="btn" onclick={() => (factionPrompt = null)}>Mai târziu</button>
          </div>
        </div>
      </div>
    {/if}

    {#if chestWin}
      <!-- A claim is a moment; an inline strip under the header was easy to
           miss entirely. Modal, dismissed by the button, the backdrop or Esc. -->
      <div
        class="cw-backdrop"
        role="presentation"
        onclick={(e) => { if (e.target === e.currentTarget) chestWin = null; }}
      >
        <div class="cw-box" role="dialog" aria-modal="true" aria-label="Cufăr deschis" tabindex="-1">
          {#if hasArt('chest')}
            <img class="cw-chest" src="/arcade/chest-open.png" alt=""
                 onerror={(e) => artFailed('chest', e)} />
          {:else}<span class="cw-emoji">🎁</span>{/if}
          <h3>Felicitări!</h3>
          <p class="cw-line">
            Ai câștigat <strong>{chestWin.gold} gold</strong>
          </p>
          <p class="cw-streak">Ziua {chestWin.streak} la rând</p>
          <button class="btn fill" onclick={() => (chestWin = null)}>Continuă</button>
        </div>
      </div>
    {/if}
  
    <!-- ══ tabs ═════════════════════════════════════════════════════════ -->
    <nav class="tabs" aria-label="Arcade">
      {#each TABS as t (t.code)}
        <button type="button" class="tab" class:active={tab === t.code} class:soon={!t.ready}
                class:sep={t.code === 'factions'}
                disabled={!t.ready} onclick={() => pickTab(t.code, t.ready)}>
          <span class="t-ico" aria-hidden="true">{t.icon}</span>{t.label}
          {#if !t.ready}<span class="soon-tag">în curând</span>{/if}
        </button>
      {/each}
    </nav>

    <!-- ══ games ════════════════════════════════════════════════════════ -->
    {#if isGame}
      {#if current}
        {@const p = current}
        <section class="panel game">
          <div class="game-head">
            <h2>{gameTitle(p)}</h2>
            <span class="when" class:today={p.isToday}>
              {p.isToday ? 'Azi' : dayLabel(p.playDate)}{#if !p.isToday}<em> · 50% XP</em>{/if}
            </span>
          </div>

          {#if p.mode === 'groups' && p.tiles?.length}
            <p class="cr-help">
              <span>Douăsprezece titluri ascund patru grupe de câte trei.</span>
              <span>Grupele pot fi după <b>studio</b>, <b>an</b> sau <b>gen</b>. Alege trei și trimite.</span>
            </p>
            {#if p.finished}
              {@const won = (p.found ?? []).filter((g) => g.solved).length}
              <div class="gr-result" class:all={won === 4}>
                <span class="gr-score">{won}<i>/4</i></span>
                <span class="gr-lines">
                  <strong>{won === 4 ? 'Tablă completă' : won === 0 ? 'Niciuna' : 'Rundă încheiată'}</strong>
                  <em>{won === 4
                        ? 'Le-ai găsit pe toate.'
                        : `Grupele rămase sunt marcate cu ✕ mai jos.`}</em>
                </span>
                {#if p.awardedXp}<span class="gr-xp">+{p.awardedXp} XP</span>{/if}
              </div>
            {/if}
            {#if (p.found ?? []).length}
              <ul class="gr-found">
                {#each p.found ?? [] as g (g.label)}
                  <li class:missed={!g.solved}>
                    <strong><span class="gr-mark">{g.solved ? '✓' : '✕'}</span>{groupLabel(g)}</strong>
                    <div class="gr-mini">
                      {#each g.animeIds as id (id)}
                        {@const t = (p.tiles ?? []).find((x) => x.animeId === id)}
                        {#if t}
                          <span class="gr-mini-item">
                            {#if t.image}<img src={t.image} alt="" loading="lazy" />{/if}
                            <em>{t.title}</em>
                          </span>
                        {/if}
                      {/each}
                    </div>
                  </li>
                {/each}
              </ul>
            {/if}
            {#if !p.finished}
              {#if groupMsg}
                <p class="gr-msg" class:hit={groupMsg.tone === 'hit'} class:near={groupMsg.tone === 'near'}
       data-mark={groupMsg.tone === 'hit' ? '✓' : groupMsg.tone === 'near' ? '!' : '✕'}>
                  {groupMsg.text}
                </p>
              {/if}
              <div class="gr-grid">
                {#each openTiles(p) as t (t.animeId)}
                  <button type="button" class="gr-tile" class:on={picked.includes(t.animeId)}
                          aria-pressed={picked.includes(t.animeId)}
                          onclick={() => togglePick(t.animeId)}>
                    {#if t.image}
                      <img class="gr-img" src={t.image} alt="" loading="lazy" />
                    {:else}
                      <span class="gr-img gr-noimg" aria-hidden="true">◫</span>
                    {/if}
                    <span class="gr-name">{t.title}</span>
                  </button>
                {/each}
              </div>
              <div class="gr-bar">
                <span class="gr-miss" class:last={(p.maxMistakes ?? 4) - p.mistakes === 1}
                      role="img" aria-label={`${p.mistakes} din ${p.maxMistakes ?? 4} greșeli`}>
                  <span class="gr-miss-lbl">Greșeli</span>
                  {#each Array(p.maxMistakes ?? 4) as _, i}
                    <i class="gr-pip" class:spent={i < p.mistakes}>{i < p.mistakes ? '✕' : '○'}</i>
                  {/each}
                </span>
                <span class="gr-count">{picked.length} din 3 alese</span>
                <span class="gr-actions">
                  <button type="button" class="btn ghost" disabled={!picked.length || sendingGroup}
                          onclick={() => (picked = [])}>Șterge</button>
                  <button type="button" class="btn fill"
                          disabled={picked.length !== 3 || sendingGroup}
                          onclick={() => sendGroup(p)}>Trimite</button>
                </span>
              </div>
            {/if}
          {:else if p.mode === 'character' && p.chars?.length}
            <p class="cr-help">
              <span>Cinci personaje. Scrie numele și seria, lasă gol ce nu știi.</span>
              <span>Se trimite o singură dată. Fiecare răspuns corect contează.</span>
            </p>
            <div class="cr-grid">
              {#each p.chars as c, i (c.id)}
                <div class="cr-card" class:done={p.scored}>
                  <img class="cr-img" src={c.image} alt="" loading="lazy" />
                  {#if p.scored}
                    <div class="cr-res">
                      <span class="cr-line" class:hit={c.gaveName}>
                        <span class="cr-mark">{c.gaveName ? '✓' : '✕'}</span>{c.name}
                      </span>
                      <span class="cr-line" class:hit={c.gaveSeries}>
                        <span class="cr-mark">{c.gaveSeries ? '✓' : '✕'}</span>{c.series}
                      </span>
                    </div>
                  {:else}
                    <div class="cr-fields">
                      {#each [{ f: 'name', ph: 'Nume personaj' }, { f: 'show', ph: 'Din ce serie?' }] as fld (fld.f)}
                        <div class="cr-wrap">
                          <input
                            type="text" placeholder={fld.ph} autocomplete="off" disabled={busy}
                            value={fld.f === 'name' ? slots[i]?.charName ?? '' : slots[i]?.animeTitle ?? ''}
                            class:locked={fld.f === 'name' ? slots[i]?.charId : slots[i]?.animeId}
                            oninput={(e) => boxInput(i, fld.f as 'name' | 'show', e.currentTarget.value)}
                          />
                          {#if openBox === `${i}-${fld.f}` && boxHits.length}
                            <ul class="sug">
                              {#each boxHits as hit (hit.id)}
                                <li><button type="button"
                                      onmousedown={(e) => { e.preventDefault(); pickBox(i, fld.f as 'name' | 'show', hit); }}
                                    >{hit.label}</button></li>
                              {/each}
                            </ul>
                          {/if}
                        </div>
                      {/each}
                    </div>
                  {/if}
                </div>
              {/each}
            </div>

            {#if p.scored}
              <div class="result won">
                <div class="r-main">
                  <span class="r-tag">Rezultat</span>
                  <strong class="r-title">
                    {charScore ? `${charScore.names}/${p.chars.length} personaje · ${charScore.series}/${p.chars.length} serii` : 'Runda s-a încheiat'}
                  </strong>
                  {#if charScore}<span class="earned">+{charScore.xp} XP · +{charScore.gold} gold</span>{/if}
                </div>
              </div>
            {:else}
              <button class="btn fill cr-send" onclick={submitChars} disabled={busy}>
                Trimite răspunsurile
              </button>
            {/if}
          {/if}

          {#if p.mode === 'theme' && p.audio}
            <div class="theme-play">
              <!-- Audio only while guessing: a single frame of the video names
                   the series outright, so the picture is the paid clue. -->
              <audio
                bind:this={audioEl}
                src={p.audio}
                preload="metadata"
                onplay={() => (playing = true)}
                onpause={() => (playing = false)}
                ontimeupdate={() => (cur = audioEl?.currentTime ?? 0)}
                onloadedmetadata={() => (dur = audioEl?.duration ?? 0)}
              ><track kind="captions" /></audio>

              <div class="player">
                <button class="pp" onclick={togglePlay} aria-label={playing ? 'Pauză' : 'Redă'}>
                  {#if playing}<span class="ic">❚❚</span>{:else}<span class="ic play">▶</span>{/if}
                </button>
                <div class="track" onclick={seek} role="presentation">
                  <div class="fill" style={`width:${dur ? (cur / dur) * 100 : 0}%`}></div>
                </div>
                <span class="time">{mmss(cur)} / {mmss(dur)}</span>
              </div>
              <p class="tp-note">{p.themeSlug ?? ''} · ascultă și ghicește seria</p>
            </div>
          {/if}

          {#if p.video}
            <div class="theme-video">
              <span class="h-lbl">Video {#if !vidReady && vidLoad > 0}· se încarcă {vidLoad}%{/if}</span>
              <!-- Blurred until the puzzle is over. Unblurred it is not a clue,
                   it is the answer: one clear frame names the series. -->
              <!-- The blur is on the video element, and a CSS filter blurs the
                   element's own controls with it. So while it is blurred the
                   native controls are off and ours sit outside the filtered
                   box; once the puzzle ends the picture clears and the real
                   controls come back. -->
              <video
                bind:this={vidEl}
                class="clip" class:clear={p.finished}
                controls={p.finished}
                preload="auto" src={p.video} playsinline
                onprogress={onVidProgress}
                onplay={() => (vidPlaying = true)}
                onpause={() => (vidPlaying = false)}
                oncanplay={() => { vidReady = true; vidLoad = 100; }}
              ><track kind="captions" /></video>

              {#if !p.finished}
                <div class="vctl">
                  <button class="pp sm" onclick={toggleVid} aria-label={vidPlaying ? 'Pauză' : 'Redă'}>
                    {#if vidPlaying}<span class="ic">❚❚</span>{:else}<span class="ic play">▶</span>{/if}
                  </button>
                  <button class="vbtn" onclick={toggleMute} aria-pressed={vidMuted}>
                    {vidMuted ? '🔇 Sunet oprit' : '🔊 Sunet pornit'}
                  </button>
                </div>
              {/if}
            </div>
          {/if}

          {#if p.mode === 'poster' && p.image}
            <div class="poster-wrap">
              <img class="poster" data-stage={p.finished ? 5 : p.stage} src={mediaUrl(p.image)}
                   alt={p.finished ? (p.answer?.title ?? '') : 'Poster neclar'} />
            </div>
          {/if}

          {#if p.hintUsed && p.hint}
            <div class="hint"><span class="h-lbl">Descriere</span><p>{p.hint}</p></div>
          {/if}

          {#if p.finished && p.mode !== 'character' && p.mode !== 'groups'}
            <div class="result" class:won={p.solved}>
              {#if p.answer?.image}
                <img class="ans-poster" src={mediaUrl(p.answer.image)} alt="" />
              {/if}
              <div class="r-main">
                <span class="r-tag">{p.solved ? 'Corect' : 'Răspunsul era'}</span>
                <strong class="r-title">{p.answer?.title}</strong>
                {#if p.themeSong}<span class="r-song">{p.themeSlug} · {p.themeSong}</span>{/if}
                {#if p.solved && lastAward}
                  <span class="earned">+{lastAward.xp} XP · +{lastAward.gold} gold</span>
                {:else if p.solved && p.awardedXp}
                  <span class="earned">+{p.awardedXp} XP</span>
                {/if}
              </div>
              {#if p.answer?.slug}<a class="go" href={`/anime/${p.answer.slug}`}>Vezi pagina →</a>{/if}
            </div>
          {:else if p.mode !== 'character' && p.mode !== 'groups'}
            <div class="entry">
              <div class="input-wrap">
                <input type="text" bind:value={guess} oninput={onGuessInput}
                       onkeydown={(e) => e.key === 'Enter' && submit()}
                       onblur={() => setTimeout(() => (showSug = false), 150)}
                       placeholder="Caută o serie…" autocomplete="off" disabled={busy} />
                {#if showSug && suggestions.length}
                  <ul class="sug">
                    {#each suggestions as sg (sg.id)}
                      <!-- mousedown, not click: the input's blur handler hides
                           this list 150ms after focus leaves, and on a slow
                           frame that ran BEFORE the click landed, so the guess
                           was silently dropped. preventDefault stops the blur. -->
                      <li><button type="button"
                            onmousedown={(e) => { e.preventDefault(); submit(sg.title); }}
                          >{sg.title}</button></li>
                    {/each}
                  </ul>
                {/if}
              </div>
              <button class="btn fill" onclick={() => submit()} disabled={busy || !guess.trim()}>Ghicește</button>
              <button class="btn ghost" onclick={giveUp} disabled={busy}>Renunț</button>
            </div>
            {#if rejected}<p class="rejected">{rejected}</p>{/if}
            <div class="under">
              <span class="left">{p.remaining} din {board.maxGuesses} încercări</span>
            </div>
            {#if p.hintReady}
              <button class="hint-offer" onclick={takeHint} disabled={busy}>
                <span class="ho-ico" aria-hidden="true">💡</span>
                <span class="ho-txt">
                  <strong>{p.mode === 'theme' ? 'Arată clipul video' : 'Arată descrierea seriei'}</strong>
                  <span class="ho-sub">
                  {#if busy && p.mode === 'theme'}se încarcă…
                  {:else}Te ajută să ghicești — dar câștigi cu 25% mai puțin XP{/if}
                </span>
                </span>
              </button>
            {/if}
          {/if}

          <!-- comparison grid -->
          {#if p.grid.length}
            <div class="grid-scroll">
              <table class="grid">
                <thead>
                  <tr>
                    <th class="c-title">Serie</th>
                    {#each board.columns as c (c.key)}<th>{c.label}</th>{/each}
                  </tr>
                </thead>
                <tbody>
                  {#if p.summary.length}
                    <tr class="sum-row">
                      <th class="c-title">Ce știm</th>
                      {#each p.summary as s (s.key)}
                        <td class={`st-${s.state}`}>{s.value.split(', ').map((x) => cellText(s.key, x)).join(', ')}</td>
                      {/each}
                    </tr>
                  {/if}
                  <!-- Keyed on the guess position, not the series: the same
                       series can legitimately appear twice, and duplicate keys
                       crash Svelte's keyed-each reconciler mid-flush, freezing
                       the whole page until a reload. Guesses only ever append,
                       so the original index is a stable identity. -->
                  {#each p.grid.map((row, gi) => ({ row, gi })).reverse() as { row, gi } (gi)}
                    <tr class:correct={row.correct}>
                      <th class="c-title">{row.title}</th>
                      {#each row.cells as c (c.key)}
                        <td class={`st-${c.state}`}>
                          {#if c.parts?.length}
                            <span class="parts">
                              {#each c.parts as part, i (i)}
                                <span class={`part st-${c.partStates?.[i] ?? 'miss'}`}>{cellText(c.key, part)}</span>
                              {/each}
                            </span>
                          {:else}
                            <span class="v">{cellText(c.key, c.value)}</span>
                            {#if c.dir}<span class="arr">{arrow(c.dir)}</span>{/if}
                          {/if}
                        </td>
                      {/each}
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </section>

        <section class="week">
          <h3>Săptămâna aceasta</h3>
          <div class="days">
            {#each inMode as d (d.id)}
              <button type="button" class="day" class:sel={d.playDate === p.playDate}
                      class:done={d.solved} class:lost={d.finished && !d.solved}
                      onclick={() => { pickedDate = d.playDate; lastAward = null; }}>
                <span class="d-when">{d.isToday ? 'Azi' : dayLabel(d.playDate)}</span>
                <span class="d-mark">{d.solved ? '✓' : d.finished ? '✕' : '·'}</span>
              </button>
            {/each}
          </div>
        </section>
      {:else}
        <div class="panel skel">Nu există puzzle-uri pentru acest joc încă.</div>
      {/if}

    <!-- ══ factions ═════════════════════════════════════════════════════ -->
    {:else if tab === 'factions'}
      <section class="panel">
        <h2 class="sec-h">Facțiuni</h2>
        <p class="note">Poți schimba oricând. Fiecare facțiune își ține propriul progres —
          dacă te întorci, îți găsești rangul neatins. Goldul este comun.</p>
        <ul class="fac-grid">
          {#each board.factions as f (f.code)}
            {@const fxp = board.profile.progress?.[f.code] ?? 0}
            {@const isActive = board.profile.faction === f.code}
            <li class="fac" class:active={isActive}>
              <button type="button" class="fac-head"
                      onclick={() => (expanded = expanded === f.code ? null : f.code)}
                      aria-expanded={expanded === f.code}>
                <span class="fac-emoji">
                  {#if hasArt(`fac:${f.code}`)}
                    <img class="fac-logo" data-faction={f.code} src={`/arcade/factions/${f.code}.png`} alt=""
                         onerror={(e) => artFailed(`fac:${f.code}`, e)} />
                  {:else}{f.emoji}{/if}
                </span>
                <span class="fac-main">
                  <span class="fac-name">{f.name}</span>
                  <span class="fac-sub">
                    {#if fxp > 0}Nivel {levelOf(fxp)} · {rankOf(f, fxp)}{:else}neînceput{/if}
                  </span>
                </span>
                {#if isActive}<span class="pip">activă</span>{/if}
                <span class="chev" aria-hidden="true">{expanded === f.code ? '▾' : '▸'}</span>
              </button>
              {#if expanded === f.code}
                <ol class="ladder">
                  {#each f.ranks as r (r.minLevel)}
                    <li class:reached={levelOf(fxp) >= r.minLevel}>
                      <span class="lv">{r.minLevel}</span><span class="ttl">{r.title}</span>
                    </li>
                  {/each}
                </ol>
                {#if !isActive}
                  <button class="btn fill sm" onclick={() => chooseFaction(f)}>
                    {fxp > 0 ? 'Revino la' : 'Alege'} {f.name}
                  </button>
                {/if}
              {/if}
            </li>
          {/each}
        </ul>
      </section>

    <!-- ══ leaderboard ══════════════════════════════════════════════════ -->
    {:else if tab === 'leaderboard'}
      <section class="panel">
        <h2 class="sec-h">Clasament</h2>
        <p class="note">După nivelul facțiunii active. La egalitate decide XP-ul, apoi goldul.</p>
        {#if leadersBusy && !leaders}
          <p class="note">Se încarcă…</p>
        {:else if !leaders?.length}
          <p class="note">Încă nu a jucat nimeni. Fii primul.</p>
        {:else}
          <ol class="lb">
            {#each leaders as row (row.username)}
              {@const me = row.username === board.profile.username}
              <li class="lb-row" class:me class:top={row.rank <= 3}>
                <span class="lb-pos" class:medal={row.rank <= 3}>{row.rank}</span>
                <span class="lb-ava">
                  {#if row.avatarUrl}
                    <img src={mediaUrl(row.avatarUrl)} alt="" loading="lazy" />
                  {:else}<span class="lb-mono">{row.username.charAt(0).toUpperCase()}</span>{/if}
                </span>
                <span class="lb-who">
                  <span class="lb-name">{row.username}{#if me}<em>tu</em>{/if}</span>
                  <span class="lb-sub">
                    {#if row.faction && hasArt(`fac:${row.faction}`)}
                      <img class="fac-logo sm" data-faction={row.faction} src={`/arcade/factions/${row.faction}.png`} alt=""
                           onerror={(e) => artFailed(`fac:${row.faction}`, e)} />
                    {/if}{row.rankTitle ?? 'fără facțiune'}
                  </span>
                </span>
                <span class="lb-lvl"><em>Nivel</em>{row.level}</span>
                <span class="lb-num mono"><em>XP</em>{row.xp.toLocaleString('ro-RO')}</span>
                <span class="lb-num mono gold"><em>gold</em>{row.gold.toLocaleString('ro-RO')}</span>
              </li>
            {/each}
          </ol>
        {/if}
      </section>

    <!-- ══ badges ═══════════════════════════════════════════════════════ -->
    {:else if tab === 'badges'}
      <section class="panel">
        <h2 class="sec-h">Insigne
          <span class="mono count">{board.profile.badges.length}/{board.badgeCatalogue.length}</span>
        </h2>
        <p class="note">Niciuna nu se ia din prima zi — fiecare cere fie precizie, fie răbdare.</p>
        <ul class="badge-grid">
          {#each board.badgeCatalogue as b (b.code)}
            {@const got = board.profile.badges.includes(b.code)}
            <li class:got>
              <span class="b-ico" class:art={hasArt(`badge:${b.code}`)}>
                {#if hasArt(`badge:${b.code}`)}
                  <img src={`/arcade/icons/${b.code}.png`} alt=""
                       onerror={(e) => artFailed(`badge:${b.code}`, e)} />
                {:else}{b.icon}{/if}
              </span>
              <span class="b-txt">
                <span class="b-name">{b.name}</span>
                <span class="b-desc">{b.desc}</span>
              </span>
            </li>
          {/each}
        </ul>
      </section>
    {/if}
  {/if}
</div>

<style>
  .wrap {
    max-width: var(--container); margin: 0 auto;
    padding: var(--space-5) var(--space-4) var(--space-9);
    display: flex; flex-direction: column; gap: var(--space-4);
  }
  .panel { background: var(--surface-raised); border: 1px solid var(--border-subtle); border-radius: var(--radius-lg); padding: var(--space-5); }
  .skel { color: var(--text-muted); text-align: center; }
  .sec-h { margin: 0 0 var(--space-2); font-size: var(--fs-h3); display: flex; align-items: baseline; gap: var(--space-3); }
  .note { margin: 0 0 var(--space-4); color: var(--text-muted); font-size: var(--fs-small); max-width: 68ch; }
  .mono { font-family: var(--font-mono); font-variant-numeric: tabular-nums; }
  .count { font-size: var(--fs-small); color: var(--text-faint); }

  /* ── identity strip: one line, not a column ──────────────────────────── */
  .strip {
    display: flex; align-items: center; gap: var(--space-4); flex-wrap: wrap;
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg); padding: var(--space-3) var(--space-4);
  }
  .avatar { width: 40px; height: 40px; border-radius: var(--radius-pill); overflow: hidden; flex: 0 0 auto; background: var(--surface-inset); border: 1px solid var(--border-default); display: grid; place-items: center; font-family: var(--font-display); color: var(--text-muted); }
  .avatar img { width: 100%; height: 100%; object-fit: cover; }
  .who { display: flex; flex-direction: column; min-width: 0; }
  .name { font-weight: var(--fw-semibold); font-size: var(--fs-small); }
  .rank { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--accent); text-transform: uppercase; letter-spacing: 0.06em; }
  .lvl { flex: 1 1 200px; min-width: 160px; }
  .lvl-top { display: flex; justify-content: space-between; font-size: var(--fs-micro); color: var(--text-muted); margin-bottom: 4px; }
  .bar { height: 6px; border-radius: var(--radius-pill); background: var(--surface-inset); overflow: hidden; }
  .bar span { display: block; height: 100%; background: var(--accent); transition: width var(--motion-base) var(--ease); }
  /* In the identity strip, not a modal and not a card above the puzzle: it is
     visible on every tab, sits in the "your stuff" zone, and goes quiet the
     moment it is claimed instead of nagging for the other 23 hours. */
  .chest {
    display: flex; align-items: center; gap: var(--space-2);
    padding: var(--space-2) var(--space-3); border-radius: var(--radius-md);
    border: 1px solid var(--border-subtle); background: var(--surface-inset);
    color: var(--text-faint); cursor: default; text-align: left;
  }
  .chest.ready {
    border-color: var(--warning); color: var(--text-primary); cursor: pointer;
    animation: chest-breathe 2.4s ease-in-out infinite;
  }
  .chest.ready:hover { background: var(--surface-overlay); }
  .chest:disabled { cursor: default; }
  .ch-ico { font-size: 1.1rem; }
  .ch-txt { display: flex; flex-direction: column; line-height: 1.15; }
  .ch-txt strong { font-size: var(--fs-small); font-weight: var(--fw-medium); }
  .ch-clock {
    font-family: var(--font-mono); font-size: var(--fs-small);
    font-variant-numeric: tabular-nums; color: var(--text-faint);
  }
  @keyframes chest-breathe {
    0%, 100% { box-shadow: 0 0 0 0 rgba(224, 165, 28, 0); }
    50%      { box-shadow: 0 0 0 4px rgba(224, 165, 28, 0.14); }
  }
  @media (prefers-reduced-motion: reduce) { .chest.ready { animation: none; } }

  .cw-backdrop {
    position: fixed; inset: 0; z-index: var(--z-overlay);
    background: rgba(0, 0, 0, 0.62);
    display: grid; place-items: center; padding: var(--space-4);
    animation: cw-fade 0.16s var(--ease);
  }
  .cw-box {
    background: var(--surface-raised); border: 1px solid var(--warning);
    border-radius: var(--radius-lg); box-shadow: var(--shadow-3);
    padding: var(--space-6) var(--space-7); text-align: center;
    max-width: 340px; width: 100%;
    display: flex; flex-direction: column; align-items: center; gap: var(--space-2);
    animation: cw-pop 0.2s var(--ease);
  }
  .cw-chest { width: 96px; height: 96px; object-fit: contain; image-rendering: pixelated; }
  .cw-row { display: flex; gap: var(--space-2); justify-content: center; flex-wrap: wrap; }
  .no-fac {
    background: none; border: 0; padding: 0; cursor: pointer; color: inherit;
    font: inherit; text-decoration: underline dotted; text-underline-offset: 2px;
  }
  .no-fac:hover { color: var(--text-primary); }
  .no-fac em { font-style: normal; color: var(--warning); }
  .cw-emoji { font-size: 3rem; line-height: 1; }
  .cw-box h3 { margin: var(--space-2) 0 0; font-family: var(--font-display); font-size: var(--fs-h2); }
  .cw-line { margin: 0; font-size: var(--fs-body); color: var(--text-muted); }
  .cw-line strong { color: var(--warning); font-family: var(--font-mono); }
  .cw-streak { margin: 0 0 var(--space-3); font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-faint); text-transform: uppercase; letter-spacing: 0.08em; }
  @keyframes cw-fade { from { opacity: 0 } to { opacity: 1 } }
  @keyframes cw-pop { from { opacity: 0; transform: translateY(8px) scale(0.97) } to { opacity: 1; transform: none } }
  @media (prefers-reduced-motion: reduce) {
    .cw-backdrop, .cw-box { animation: none; }
  }
  .coin.big { width: 30px; height: 30px; }

  .stat { display: flex; flex-direction: column; align-items: flex-end; background: none; border: 0; padding: 0; }
  .stat.as-btn { cursor: pointer; }
  .s-num { font-family: var(--font-mono); font-size: var(--fs-body); color: var(--text-primary); font-variant-numeric: tabular-nums; }
  .s-num.gold { color: var(--warning); display: inline-flex; align-items: center; gap: 6px; }
  /* Drawn rather than an emoji: emoji render differently on every platform and
     will not sit on the type baseline. Fixed golds rather than currentColor --
     a coin has to read as metal, which needs a rim, a face gradient and a
     specular highlight, and those cannot all be one inherited colour. */
  .coin { width: 24px; height: 24px; flex: 0 0 auto; }
  /* Pixel art: the browser's default smoothing turns a crisp sprite to mush at
     these sizes, so nearest-neighbour everywhere art can appear. */
  img.coin, .b-ico img, .ch-ico img { image-rendering: pixelated; }
  .b-ico img, .ch-ico img { width: 100%; height: 100%; object-fit: contain; display: block; }
  .ch-ico { width: 30px; height: 30px; display: block; }
  /* With real art the ring and tint would fight the sprite. */
  .b-ico.art { background: none; border-color: transparent; }
  .rejected { margin: var(--space-3) 0 0; padding: var(--space-2) var(--space-3); font-size: var(--fs-small); color: var(--danger); background: var(--surface-inset); border-left: 2px solid var(--danger); border-radius: var(--radius-sm); }
  .s-num em { font-style: normal; color: var(--text-faint); font-size: var(--fs-small); }
  .s-lbl { font-size: var(--fs-micro); color: var(--text-faint); text-transform: uppercase; letter-spacing: 0.08em; }

  /* ── tabs ────────────────────────────────────────────────────────────── */
  .tabs { display: flex; gap: var(--space-2); flex-wrap: wrap; }
  .tab { display: inline-flex; align-items: center; gap: var(--space-2); padding: var(--space-2) var(--space-4); border-radius: var(--radius-pill); border: 1px solid var(--border-subtle); background: var(--surface-raised); color: var(--text-muted); font-size: var(--fs-small); cursor: pointer; transition: color var(--motion-fast) var(--ease), border-color var(--motion-fast) var(--ease); }
  .tab:hover:not(:disabled) { color: var(--text-primary); border-color: var(--border-default); }
  .tab.active { background: var(--accent); border-color: var(--accent); color: var(--on-accent); }
  .tab.soon { opacity: 0.5; cursor: not-allowed; }
  /* factions/badges are destinations, not games — set them apart */
  .tab.sep { margin-left: auto; }
  .soon-tag { font-family: var(--font-mono); font-size: var(--fs-micro); text-transform: uppercase; letter-spacing: 0.06em; }

  /* ── game ────────────────────────────────────────────────────────────── */
  .game-head { display: flex; align-items: baseline; justify-content: space-between; gap: var(--space-3); margin-bottom: var(--space-4); }
  .game-head h2 { margin: 0; font-size: var(--fs-h3); }
  .when { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-faint); text-transform: uppercase; letter-spacing: 0.06em; }
  .when.today { color: var(--success); }
  .when em { font-style: normal; color: var(--warning); }

  /* Five portraits abreast on a wide screen, wrapping to two rows on a laptop.
     minmax keeps the face readable rather than shrinking to fit the count. */
  .cr-help { margin: 0 0 var(--space-4); font-size: var(--fs-small); color: var(--text-muted); max-width: 68ch; }
  .cr-help span { display: inline-block; }
  /* ── Grupe ─────────────────────────────────────────────────────────────── */
  /* Posters rather than titles alone: a wall of romaji is unreadable at a
     glance, and recognising a show by its art is most of what makes the
     grouping feel possible. */
  .gr-grid {
    display: grid; grid-template-columns: repeat(4, 1fr);
    gap: var(--space-2); margin-bottom: var(--space-3);
  }
  .gr-tile {
    display: flex; flex-direction: column; gap: 6px; padding: 6px;
    background: var(--surface-raised); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); cursor: pointer; text-align: center;
    color: var(--text-primary); overflow: hidden;
  }
  .gr-tile:hover { border-color: var(--border-strong); }
  /* Picked never leans on colour alone: the border thickens and the card lifts,
     so the state survives the sakura ground and a reader who cannot separate
     the hues. */
  .gr-tile.on {
    border-color: var(--text-primary); border-width: 2px; padding: 5px;
    background: var(--surface-overlay);
  }
  /* A fixed, modest poster height rather than filling the card: full-bleed art
     made each tile tall enough that sixteen of them did not fit on screen, and
     the whole point is comparing them at a glance. */
  .gr-img {
    height: 152px; width: auto; aspect-ratio: 2 / 3; object-fit: cover;
    margin: 0 auto; border-radius: var(--radius-sm);
    background: var(--surface-inset); display: block;
  }
  .gr-noimg { display: grid; place-items: center; font-size: 1.6rem; color: var(--text-muted); }
  .gr-name {
    font-size: var(--fs-micro); line-height: 1.2; font-weight: 600;
    display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2;
    -webkit-box-orient: vertical; overflow: hidden;
  }
  .gr-found { list-style: none; padding: 0; margin: 0 0 var(--space-3); display: grid; gap: var(--space-2); }
  .gr-found li {
    padding: var(--space-2) var(--space-3); border-radius: var(--radius-md);
    background: var(--surface-inset); border: 1px solid var(--border-subtle);
  }
  .gr-found strong {
    display: block; margin-bottom: 6px;
    font-size: var(--fs-micro); letter-spacing: .04em; text-transform: uppercase;
  }
  .gr-mini { display: flex; gap: var(--space-2); flex-wrap: wrap; }
  .gr-mini-item { display: flex; align-items: center; gap: 6px; font-size: var(--fs-micro); color: var(--text-muted); }
  .gr-mini-item img { width: 22px; height: 33px; object-fit: cover; border-radius: 3px; }
  .gr-mini-item em { font-style: normal; }
  .gr-bar { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); flex-wrap: wrap; }
  /* Mistakes as marks rather than "1/4": how many are left reads faster as
     shape than as arithmetic, and a spent one is an actual ✕ in red rather
     than a filled dot that could mean anything. */
  .gr-miss { display: inline-flex; align-items: center; gap: 4px; font-size: var(--fs-micro); color: var(--text-muted); }
  .gr-miss-lbl { margin-right: 2px; }
  .gr-pip {
    font-style: normal; font-size: 0.95rem; line-height: 1;
    color: var(--border-strong); font-weight: 700;
  }
  .gr-pip.spent { color: var(--danger); }
  /* On the last life the whole row warns, so the stake is visible before the
     mistake rather than after it. */
  .gr-miss.last .gr-miss-lbl { color: var(--warning); font-weight: 600; }
  .gr-miss.last .gr-pip:not(.spent) { color: var(--warning); }
  .gr-count { font-size: var(--fs-micro); color: var(--text-muted); font-variant-numeric: tabular-nums; }
  .gr-actions { display: flex; gap: var(--space-2); }
  /* The result says what happened in one line, because "3 din 4" is the thing
     a player wants and counting the ticks below is work. */
  /* The verdict a player wants is "how many did I get", so the score is the
     largest thing in the panel and the words explain it rather than carry it. */
  .gr-result {
    display: flex; align-items: center; gap: var(--space-3);
    padding: var(--space-3); margin-bottom: var(--space-3);
    border-radius: var(--radius-md);
    background: var(--surface-inset); border: 1px solid var(--border-default);
  }
  .gr-result.all { border-color: var(--text-primary); }
  .gr-score { font-size: 2rem; font-weight: 700; line-height: 1; font-variant-numeric: tabular-nums; }
  .gr-score i { font-size: 1rem; font-style: normal; color: var(--text-muted); }
  .gr-lines { display: flex; flex-direction: column; gap: 2px; }
  .gr-lines strong { font-size: var(--fs-small); }
  .gr-lines em { font-style: normal; font-size: var(--fs-micro); color: var(--text-muted); }
  .gr-xp { margin-left: auto; font-size: var(--fs-small); font-weight: 700; white-space: nowrap; }
  /* Centred over the board, so the answer appears where you were already
     looking instead of at the edge of the screen. */
  .gr-msg {
    margin: 0 0 var(--space-2); padding: var(--space-2) var(--space-3);
    text-align: center; font-size: var(--fs-small); font-weight: 600;
    border-radius: var(--radius-md);
    background: var(--surface-inset); border: 1px solid var(--border-default);
  }
  /* Colour carries the verdict here, but never alone: the border weight and a
     leading mark change too, so the line still reads on the sakura ground and
     for anyone who cannot separate the hues. */
  .gr-msg.hit {
    color: var(--success); border-color: var(--success); border-width: 2px;
    background: color-mix(in srgb, var(--success) 12%, var(--surface-inset));
  }
  .gr-msg.near {
    color: var(--warning); border-color: var(--warning); border-width: 2px;
    background: color-mix(in srgb, var(--warning) 14%, var(--surface-inset));
  }
  .gr-msg:not(.hit):not(.near) {
    color: var(--danger); border-color: var(--danger);
    background: color-mix(in srgb, var(--danger) 10%, var(--surface-inset));
  }
  .gr-msg::before { content: attr(data-mark) ' '; font-weight: 700; }
  .gr-mark { display: inline-block; width: 1.1em; }
  /* The outcome is carried by colour, the tick, and the border together. Green
     is one you found; red is one the board had to show you. Reading the result
     should not require reading any words. */
  .gr-found li {
    border-left-width: 3px; border-left-color: var(--success);
    background: color-mix(in srgb, var(--success) 8%, var(--surface-inset));
  }
  .gr-found li.missed {
    border-left-color: var(--danger);
    background: color-mix(in srgb, var(--danger) 8%, var(--surface-inset));
  }
  .gr-found li.missed .gr-mark { color: var(--danger); }
  .gr-found li:not(.missed) .gr-mark { color: var(--success); }
  @media (max-width: 560px) {
    .gr-grid { grid-template-columns: repeat(2, 1fr); }
    .gr-bar { justify-content: flex-start; }
  }
  .cr-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(148px, 1fr)); gap: var(--space-3); }
  .cr-card { display: flex; flex-direction: column; gap: var(--space-2); }
  .cr-img { width: 100%; aspect-ratio: 2 / 3; object-fit: cover; border-radius: var(--radius-md); border: 1px solid var(--border-subtle); background: var(--surface-inset); }
  .cr-fields { display: flex; flex-direction: column; gap: 4px; }
  .cr-wrap { position: relative; }
  .cr-wrap input {
    width: 100%; padding: var(--space-2); font-size: var(--fs-micro);
    background: var(--surface-inset); color: var(--text-primary);
    border: 1px solid var(--border-default); border-radius: var(--radius-sm);
  }
  .cr-wrap input:focus-visible { outline: var(--focus-ring); border-color: var(--accent); }
  /* A chosen answer looks chosen — typing again clears it, so the border is the
     only signal that the box holds a real id rather than loose text. */
  .cr-wrap input.locked { border-color: var(--accent); }
  .cr-res { display: flex; flex-direction: column; gap: 2px; }
  .cr-line { display: flex; gap: 5px; align-items: baseline; font-size: var(--fs-micro); color: var(--text-faint); }
  .cr-line.hit { color: var(--text-primary); }
  .cr-mark { color: var(--danger); }
  .cr-line.hit .cr-mark { color: var(--success); }
  .cr-send { margin-top: var(--space-4); }

  .theme-play { margin-bottom: var(--space-4); }
  .theme-play audio { display: none; }
  /* Built from tokens rather than native controls, which carry the browser's
     own chrome and look like nothing else on the site. */
  .player {
    display: flex; align-items: center; gap: var(--space-3);
    padding: var(--space-3); background: var(--surface-inset);
    border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
  }
  .pp {
    width: 38px; height: 38px; flex: 0 0 auto; display: grid; place-items: center;
    border-radius: var(--radius-pill); border: 1px solid var(--accent);
    background: var(--accent); color: var(--on-accent); cursor: pointer; padding: 0;
  }
  .pp:hover { filter: brightness(1.08); }
  .ic { font-size: 0.8rem; line-height: 1; }
  .ic.play { margin-left: 2px; }
  /* The groove sat on --surface-raised over --surface-inset, which is two
     neighbouring darks — the bar was there but you could not see where it
     ended. A translucent white reads on either theme without adding a token. */
  .track {
    flex: 1; height: 8px; border-radius: var(--radius-pill);
    background: color-mix(in srgb, var(--text-primary) 14%, transparent);
    border: 1px solid var(--border-subtle);
    cursor: pointer; overflow: hidden;
  }
  .track:hover { background: color-mix(in srgb, var(--text-primary) 20%, transparent); }
  .fill { height: 100%; background: var(--accent); border-radius: var(--radius-pill); }
  .time { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-faint); font-variant-numeric: tabular-nums; }
  .tp-note { margin: var(--space-2) 0 0; font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-faint); text-transform: uppercase; letter-spacing: 0.06em; }
  .theme-video { margin-bottom: var(--space-4); }
  .theme-video video { width: 100%; max-width: 460px; border-radius: var(--radius-md); border: 1px solid var(--border-subtle); display: block; margin-top: 4px; }
  .clip { filter: blur(8px) saturate(1.1); transition: filter var(--motion-base) var(--ease); }
  .clip.clear { filter: none; }
  .vctl { display: flex; align-items: center; gap: var(--space-2); margin-top: var(--space-2); }
  .pp.sm { width: 32px; height: 32px; }
  .vbtn {
    padding: var(--space-2) var(--space-3); border-radius: var(--radius-md);
    border: 1px solid var(--border-default); background: var(--surface-inset);
    color: var(--text-muted); font-size: var(--fs-micro); cursor: pointer;
  }
  .vbtn:hover { color: var(--text-primary); border-color: var(--accent); }
  .r-song { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }

  .poster-wrap { display: grid; place-items: center; margin-bottom: var(--space-4); }
  /* Blurred, never blacked out: a flat silhouette gives a first guess nothing. */
  .poster { width: 230px; max-width: 100%; border-radius: var(--radius-md); border: 1px solid var(--border-subtle); transition: filter var(--motion-base) var(--ease); }
  .poster[data-stage='0'] { filter: blur(14px) saturate(1.1); }
  .poster[data-stage='1'] { filter: blur(10px); }
  .poster[data-stage='2'] { filter: blur(7px); }
  .poster[data-stage='3'] { filter: blur(4px); }
  .poster[data-stage='4'] { filter: blur(2px); }
  .poster[data-stage='5'] { filter: none; }

  .hint { margin: 0 0 var(--space-4); padding: var(--space-3) var(--space-4); background: var(--surface-inset); border: 1px solid var(--border-subtle); border-left: 2px solid var(--accent); border-radius: var(--radius-sm); }
  .h-lbl { font-family: var(--font-mono); font-size: var(--fs-micro); text-transform: uppercase; letter-spacing: 0.08em; color: var(--text-faint); }
  .hint p { margin: 4px 0 0; font-size: var(--fs-small); color: var(--text-muted); line-height: var(--lh-normal); }

  .entry { display: flex; gap: var(--space-2); align-items: stretch; flex-wrap: wrap; }
  .input-wrap { position: relative; flex: 1 1 220px; min-width: 0; }
  .input-wrap input { width: 100%; padding: var(--space-3); background: var(--surface-inset); color: var(--text-primary); border: 1px solid var(--border-default); border-radius: var(--radius-md); font-size: var(--fs-small); }
  .input-wrap input:focus-visible { outline: var(--focus-ring); border-color: var(--accent); }
  .sug { position: absolute; z-index: 5; top: calc(100% + 4px); left: 0; right: 0; margin: 0; padding: 4px; list-style: none; background: var(--surface-overlay); border: 1px solid var(--border-default); border-radius: var(--radius-md); box-shadow: var(--shadow-2); max-height: 260px; overflow-y: auto; }
  .sug button { display: block; width: 100%; text-align: left; padding: var(--space-2) var(--space-3); background: none; border: 0; border-radius: var(--radius-sm); color: var(--text-primary); font-size: var(--fs-small); cursor: pointer; }
  .sug button:hover { background: var(--surface-inset); }

  .btn { padding: var(--space-3) var(--space-5); border-radius: var(--radius-md); font-size: var(--fs-small); cursor: pointer; border: 1px solid transparent; white-space: nowrap; }
  .btn.fill { background: var(--accent); color: var(--on-accent); }
  .btn.fill:disabled { opacity: 0.5; cursor: not-allowed; }
  .btn.ghost { background: none; border-color: var(--border-default); color: var(--text-muted); }
  .btn.ghost:hover { color: var(--text-primary); }
  .btn.sm { width: calc(100% - var(--space-6)); margin: 0 var(--space-3) var(--space-3); padding: var(--space-2); font-size: var(--fs-micro); }
  /* Deliberately loud. As a small ghost button beside the attempt counter it
     read as a footnote and went unnoticed at exactly the moment it is useful. */
  .hint-offer {
    display: flex; align-items: center; gap: var(--space-3); width: 100%;
    margin-top: var(--space-3); padding: var(--space-3) var(--space-4);
    background: var(--surface-inset);
    border: 1px solid var(--warning); border-left: 3px solid var(--warning);
    border-radius: var(--radius-md); cursor: pointer; text-align: left; color: inherit;
    transition: background var(--motion-fast) var(--ease);
  }
  .hint-offer:hover { background: var(--surface-overlay); }
  .hint-offer:disabled { opacity: 0.6; cursor: not-allowed; }
  .ho-ico { font-size: 1.15rem; }
  .ho-txt { display: flex; flex-direction: column; gap: 1px; }
  .ho-txt strong { font-size: var(--fs-small); }
  .ho-sub { font-size: var(--fs-micro); color: var(--warning); }

  .under { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); margin-top: var(--space-3); flex-wrap: wrap; }
  .left { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-faint); }

  .result { display: flex; align-items: center; gap: var(--space-4); flex-wrap: wrap; padding: var(--space-4); border-radius: var(--radius-md); background: var(--surface-inset); border: 1px solid var(--border-default); }
  .result.won { border-color: var(--success); }
  /* The poster is the payoff on both modes — seeing what it was is the point. */
  .ans-poster { width: 64px; border-radius: var(--radius-sm); border: 1px solid var(--border-subtle); flex: 0 0 auto; }
  .r-main { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
  .r-tag { font-family: var(--font-mono); font-size: var(--fs-micro); text-transform: uppercase; letter-spacing: 0.08em; color: var(--text-faint); }
  .result.won .r-tag { color: var(--success); }
  .r-title { font-size: var(--fs-body); }
  .earned { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--success); }
  .go { margin-left: auto; color: var(--accent); font-size: var(--fs-small); }

  /* ── comparison grid ─────────────────────────────────────────────────── */
  .grid-scroll { margin-top: var(--space-5); overflow-x: auto; }
  .grid { width: 100%; border-collapse: collapse; font-size: var(--fs-small); }
  .grid th, .grid td { padding: var(--space-2) var(--space-3); text-align: center; vertical-align: middle; }
  .grid thead th { font-family: var(--font-mono); font-size: var(--fs-micro); text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-faint); font-weight: var(--fw-medium); border-bottom: 1px solid var(--border-subtle); }
  .grid .c-title { text-align: left; font-weight: var(--fw-regular); color: var(--text-primary); max-width: 190px; }
  .grid tbody tr { border-bottom: 1px solid var(--border-subtle); }
  .grid tbody tr.correct .c-title { color: var(--success); font-weight: var(--fw-semibold); }
  .sum-row { background: var(--surface-inset); }
  .sum-row .c-title { font-family: var(--font-mono); font-size: var(--fs-micro); text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-faint); }
  .sum-row td { font-family: var(--font-mono); font-size: var(--fs-micro); }

  /* Colour carries the meaning, so each state also differs in weight — a
     red/green pair alone is unreadable for a good share of players. */
  .st-hit { color: var(--success); font-weight: var(--fw-semibold); }
  .st-near { color: var(--warning); }
  .st-miss { color: var(--text-faint); }
  .arr { margin-left: 4px; color: var(--text-muted); }
  .parts { display: flex; flex-direction: column; gap: 1px; align-items: center; }
  .part { font-size: var(--fs-micro); }

  /* ── factions ────────────────────────────────────────────────────────── */
  /* Leaderboard. A ranked list rather than a table: the row is scannable at a
     glance and it collapses to one column on a phone without a horizontal
     scroller, which a table of six columns would need. */
  .lb { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 2px; }
  .lb-row {
    display: grid; align-items: center; gap: var(--space-2);
    grid-template-columns: 2.2rem 34px minmax(0, 1fr) auto auto auto;
    padding: var(--space-2) var(--space-3);
    border: 1px solid transparent; border-radius: var(--radius-md);
    background: var(--surface-inset);
  }
  .lb-row.top { background: var(--surface-raised); }
  .lb-row.me { border-color: var(--accent); }
  .lb-pos {
    font-family: var(--font-mono); font-size: var(--fs-small);
    color: var(--text-faint); text-align: center; font-variant-numeric: tabular-nums;
  }
  /* Gold, silver, bronze — the only place colour carries meaning here, so the
     rest of the row stays neutral and the podium reads instantly. */
  .lb-row:nth-child(1) .lb-pos.medal { color: #e0a51c; }
  .lb-row:nth-child(2) .lb-pos.medal { color: #b9c1c9; }
  .lb-row:nth-child(3) .lb-pos.medal { color: #c8823c; }
  .lb-pos.medal { font-weight: var(--fw-medium); }
  .lb-ava {
    width: 34px; height: 34px; border-radius: 50%; overflow: hidden;
    display: grid; place-items: center; background: var(--surface-overlay); flex: 0 0 auto;
  }
  .lb-ava img { width: 100%; height: 100%; object-fit: cover; display: block; }
  .lb-mono { font-size: var(--fs-small); color: var(--text-faint); }
  .lb-who { display: flex; flex-direction: column; min-width: 0; }
  .lb-name { font-size: var(--fs-small); font-weight: var(--fw-medium); }
  .lb-name em {
    font-style: normal; font-size: var(--fs-micro); color: var(--accent);
    border: 1px solid var(--accent); border-radius: var(--radius-sm);
    padding: 0 4px; margin-left: 6px; vertical-align: 1px;
  }
  .lb-sub {
    font-size: var(--fs-micro); color: var(--text-faint);
    display: flex; align-items: center; gap: 4px; min-width: 0;
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .lb-lvl, .lb-num { display: flex; flex-direction: column; align-items: flex-end; line-height: 1.1; }
  .lb-lvl { font-size: var(--fs-small); font-weight: var(--fw-medium); }
  .lb-num { font-size: var(--fs-small); font-variant-numeric: tabular-nums; }
  .lb-num.gold { color: var(--warning); }
  .lb-lvl em, .lb-num em {
    font-style: normal; font-size: var(--fs-micro); color: var(--text-faint);
    text-transform: uppercase; letter-spacing: 0.04em;
  }
  @media (max-width: 640px) {
    /* Gold is the first thing to go: XP is what the ranking is actually on. */
    .lb-row { grid-template-columns: 1.8rem 30px minmax(0, 1fr) auto auto; }
    .lb-num.gold { display: none; }
  }

  .fac-grid { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: var(--space-3); align-items: start; }
  .fac { border: 1px solid var(--border-subtle); border-radius: var(--radius-md); background: var(--surface-inset); overflow: hidden; }
  .fac.active { border-color: var(--accent); }
  .fac-head { display: flex; align-items: center; gap: var(--space-2); width: 100%; padding: var(--space-3); background: none; border: 0; cursor: pointer; text-align: left; color: inherit; }
  .fac-head:hover { background: var(--surface-overlay); }
  .fac-emoji { font-size: 1.7rem; display: flex; align-items: center; justify-content: center; width: 92px; height: 58px; flex: 0 0 auto; }
  /* Logos are artwork, not pixel sprites, so they scale smoothly. contain keeps
     the wide ones (Naruto, HxH, AoT) from being stretched to a square. */
  .fac-logo { max-width: 92px; max-height: 58px; width: auto; height: auto; object-fit: contain; display: block; }
  .fac-logo.sm { width: auto; height: 26px; max-width: 62px; display: inline-block; vertical-align: -8px; margin-right: 2px; }
  /* The Code Geass sigil is solid black with no colour in it at all, so it
     disappears against the default and sakura grounds -- both are dark, sakura
     being a deep plum rather than a pale theme. Inverting it there paints it
     white; the light theme is the one ground it already reads on, so that one
     is put back. Any future crest that is dark and monochrome joins these
     selectors; a crest with real colour in it does not need this. */
  .fac-logo[data-faction='codegeass'] { filter: invert(1); }
  :root[data-theme='light'] .fac-logo[data-faction='codegeass'] { filter: none; }
  .fac-main { display: flex; flex-direction: column; min-width: 0; flex: 1; }
  .fac-name { font-size: var(--fs-small); font-weight: var(--fw-medium); }
  .fac-sub { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-faint); }
  .pip { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--accent); text-transform: uppercase; letter-spacing: 0.06em; }
  .chev { color: var(--text-faint); font-size: var(--fs-micro); }
  .ladder { list-style: none; margin: 0; padding: 0 var(--space-3) var(--space-3); display: flex; flex-direction: column; gap: 2px; }
  .ladder li { display: flex; gap: var(--space-2); align-items: baseline; font-size: var(--fs-micro); color: var(--text-faint); }
  .ladder li.reached { color: var(--text-primary); }
  .ladder .lv { font-family: var(--font-mono); min-width: 22px; }

  /* ── badges ──────────────────────────────────────────────────────────── */
  .badge-grid { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: var(--space-3); }
  .badge-grid li { display: flex; gap: var(--space-3); align-items: flex-start; padding: var(--space-3); border: 1px solid var(--border-subtle); border-radius: var(--radius-md); background: var(--surface-inset); opacity: 0.5; }
  .badge-grid li.got { opacity: 1; border-color: var(--accent); }
  .b-ico { width: 30px; height: 30px; flex: 0 0 auto; display: grid; place-items: center; border-radius: var(--radius-pill); background: var(--surface-raised); border: 1px solid var(--border-subtle); color: var(--text-faint); }
  .badge-grid li.got .b-ico { color: var(--accent); border-color: var(--accent); }
  .b-txt { display: flex; flex-direction: column; min-width: 0; }
  .b-name { font-size: var(--fs-small); }
  .b-desc { font-size: var(--fs-micro); color: var(--text-faint); }

  /* ── week ────────────────────────────────────────────────────────────── */
  .week h3 { margin: 0 0 var(--space-3); font-family: var(--font-mono); font-size: var(--fs-micro); text-transform: uppercase; letter-spacing: 0.08em; color: var(--text-faint); font-weight: var(--fw-medium); }
  .days { display: flex; gap: var(--space-2); flex-wrap: wrap; }
  .day { display: flex; flex-direction: column; align-items: center; gap: 2px; min-width: 58px; padding: var(--space-2) var(--space-3); background: var(--surface-raised); border: 1px solid var(--border-subtle); border-radius: var(--radius-md); cursor: pointer; color: var(--text-muted); }
  .day:hover { border-color: var(--border-strong); }
  .day.sel { border-color: var(--accent); color: var(--text-primary); }
  .day.done .d-mark { color: var(--success); }
  .day.lost .d-mark { color: var(--danger); }
  .d-when { font-size: var(--fs-micro); }
  .d-mark { font-size: var(--fs-small); }

  @media (max-width: 720px) {
    .tab.sep { margin-left: 0; }
    .strip { gap: var(--space-3); }
  }
</style>
