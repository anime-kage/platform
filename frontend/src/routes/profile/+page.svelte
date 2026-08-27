<script lang="ts">
  import { goto } from '$app/navigation';
  import PosterCard from '$lib/components/PosterCard.svelte';
  import api from '$lib/api';
  import { nameHue } from '$lib/avatar';
  import { authStore } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import type { Anime, Manga } from '$lib/types';
  import type {
    UserStats, WatchlistEntry, ReadlistEntry, FavoriteRef, HistoryDay, FollowNetwork,
    ProfileBanner, BannerChoice, ImportReport
  } from '$shared/types';

  const auth = $derived($authStore);

  let stats = $state<UserStats | null>(null);
  let watchlist = $state<WatchlistEntry[]>([]);
  let readlist = $state<ReadlistEntry[]>([]);
  let history = $state<HistoryDay[]>([]);
  let favs = $state<{ type: 'anime' | 'manga'; item: Anime | Manga }[]>([]);
  let network = $state<FollowNetwork | null>(null);
  let loading = $state(true);

  // ── profile backdrop ───────────────────────────────────────────
  let banner = $state<ProfileBanner | null>(null);
  let bannerOptions = $state<BannerChoice[]>([]);
  // A full list here is a member's whole watchlist — 160+ wide banners at
  // ~1900px each. Filter first, reveal in pages, and let the browser skip the
  // ones still below the fold.
  const BANNER_PAGE = 24;
  let bannerQ = $state('');
  let bannerShown = $state(BANNER_PAGE);
  const bannerMatches = $derived(
    bannerQ.trim()
      ? bannerOptions.filter((o) => o.title.toLowerCase().includes(bannerQ.trim().toLowerCase()))
      : bannerOptions
  );
  const bannerVisible = $derived(bannerMatches.slice(0, bannerShown));
  let bannerOpen = $state(false);
  let bannerBusy = $state(false);

  async function openBannerPicker() {
    bannerOpen = !bannerOpen;
    bannerQ = '';
    bannerShown = BANNER_PAGE;
    if (!bannerOpen || bannerOptions.length) return;
    try {
      bannerOptions = (await api.getBannerOptions()).data;
    } catch {
      toast.error('Nu am putut încărca opțiunile.');
    }
  }

  async function chooseBanner(mediaType: 'anime' | 'manga', id: number) {
    bannerBusy = true;
    try {
      banner = (await api.setBanner(mediaType, id)).data;
      bannerOpen = false;
    } catch (e) {
      toast.error((e as { error?: string })?.error ?? 'Nu am putut schimba fundalul.');
    } finally {
      bannerBusy = false;
    }
  }

  async function clearBanner() {
    bannerBusy = true;
    try {
      await api.setBanner('anime', 0);
      banner = null;
      bannerOpen = false;
    } finally {
      bannerBusy = false;
    }
  }

  // ── list import ────────────────────────────────────────────────
  let importOpen = $state(false);
  let alUser = $state('');
  let importing = $state(false);
  let importReport = $state<ImportReport | null>(null);

  async function importAniList(e: SubmitEvent) {
    e.preventDefault();
    if (!alUser.trim()) return;
    importing = true;
    importReport = null;
    try {
      importReport = (await api.importAniList(alUser.trim())).data;
      await load();
      toast.success('Lista a fost importată.');
    } catch (err) {
      toast.error((err as { error?: string })?.error ?? 'Importul a eșuat.');
    } finally {
      importing = false;
    }
  }

  async function importMal(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = '';
    if (!file) return;
    importing = true;
    importReport = null;
    try {
      importReport = (await api.importMAL(file)).data;
      await load();
      toast.success('Lista a fost importată.');
    } catch (err) {
      toast.error((err as { error?: string })?.error ?? 'Importul a eșuat.');
    } finally {
      importing = false;
    }
  }

  $effect(() => {
    if (auth.isLoading) return;
    if (!auth.isAuthenticated) {
      goto('/login?redirect=/profile');
      return;
    }
    load();
  });

  async function load() {
    loading = true;
    try {
      const [me, wl, rl, hist, pub] = await Promise.all([
        api.getMyProfile().catch(() => null),
        api.getWatchlist().catch(() => ({ data: [] })),
        api.getReadlist().catch(() => ({ data: [] })),
        api.getMyHistory(14).catch(() => ({ data: [] })),
        auth.user ? api.getPublicUser(auth.user.username).catch(() => null) : Promise.resolve(null)
      ]);
      stats = me?.stats ?? null;
      banner = me?.banner ?? null;
      watchlist = wl.data;
      readlist = rl.data;
      history = hist.data;
      network = pub?.network ?? null;
      await loadFavorites(me?.user?.favorites ?? auth.user?.favorites ?? []);
    } finally {
      loading = false;
    }
  }

  // pg numeric serializes as a string on client fetches
  const numScore = <T extends { score?: number }>(x: T): T => ({
    ...x,
    score: x.score == null ? undefined : Number(x.score)
  });

  async function loadFavorites(refs: FavoriteRef[]) {
    const items = await Promise.all(
      refs.slice(0, 5).map(async (r) => {
        try {
          const res = r.type === 'anime' ? await api.getAnimeById(r.id) : await api.getMangaById(r.id);
          return { type: r.type, item: numScore(res.data) };
        } catch {
          return null;
        }
      })
    );
    favs = items.filter((x): x is { type: 'anime' | 'manga'; item: Anime | Manga } => !!x);
  }

  const initial = $derived(auth.user?.username?.[0]?.toUpperCase() ?? '?');

  const memberSince = $derived(
    auth.user?.createdAt
      ? new Date(auth.user.createdAt).toLocaleDateString('ro-RO', { year: 'numeric', month: 'long' })
      : ''
  );

  const ratedCount = $derived(
    watchlist.filter((e) => e.score).length + readlist.filter((e) => e.score).length
  );

  // Reviews = tracked entries where you actually wrote something
  const reviewCount = $derived(
    watchlist.filter((e) => e.notes).length + readlist.filter((e) => e.notes).length
  );

  /* 14-day activity from the watch_history table: every episode/chapter
     progress update is logged as an event on its real day. */
  const activity = $derived.by(() => {
    const byDate = new Map(history.map((h) => [h.date, h]));
    const DAYS_RO = ['D', 'L', 'Ma', 'Mi', 'J', 'V', 'S'];
    return Array.from({ length: 14 }, (_, i) => {
      const d = new Date();
      d.setHours(0, 0, 0, 0);
      d.setDate(d.getDate() - (13 - i));
      const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
      const h = byDate.get(key);
      const ep = h?.episodes ?? 0;
      const ch = h?.chapters ?? 0;
      return {
        key: d.getTime(),
        day: DAYS_RO[d.getDay()],
        date: d.getDate(),
        ep,
        ch,
        n: ep + ch
      };
    });
  });
  const actMax = $derived(Math.max(1, ...activity.map((a) => a.n)));
  const epTotal = $derived(activity.reduce((s, a) => s + a.ep, 0));
  const chTotal = $derived(activity.reduce((s, a) => s + a.ch, 0));

  // combined tastes: anime genres from the watchlist + manga genres from the readlist
  const genreBars = $derived.by(() => {
    const counts = new Map<string, number>();
    for (const e of watchlist) for (const g of e.anime?.genres ?? []) counts.set(g, (counts.get(g) ?? 0) + 1);
    for (const e of readlist) for (const g of e.manga?.genres ?? []) counts.set(g, (counts.get(g) ?? 0) + 1);
    const top = [...counts.entries()].sort((a, b) => b[1] - a[1]).slice(0, 5);
    const max = top[0]?.[1] ?? 1;
    return top.map(([label, n]) => ({ label, n, pct: Math.round((n / max) * 100) }));
  });

  const statusBars = $derived.by(() => {
    const order: [string, string][] = [
      ['watching', 'În vizionare'],
      ['completed', 'Finalizate'],
      ['plan-to-watch', 'Planificate'],
      ['on-hold', 'În așteptare'],
      ['dropped', 'Abandonate']
    ];
    const max = Math.max(1, ...order.map(([k]) => watchlist.filter((e) => e.status === k).length));
    return order
      .map(([k, label]) => ({ label, n: watchlist.filter((e) => e.status === k).length }))
      .filter((b) => b.n > 0)
      .map((b) => ({ ...b, pct: Math.round((b.n / max) * 100) }));
  });

</script>

<svelte:head><title>Profil · Anime-Kage</title></svelte:head>

{#if auth.user}
  <div class="container prof" class:has-backdrop={!!banner}>
    <!-- Backdrop: full-bleed art behind the masthead, faded into the page so
         it reads as atmosphere rather than a photo pasted on top. -->
    {#if banner}
      <div class="backdrop" aria-hidden="true">
        <div class="backdrop-art" style={`background-image:url(${banner.bannerUrl})`}></div>
      </div>
    {/if}

    <!-- MASTHEAD -->
    <header class="mast">
      <div class="mast-id">
        {#if auth.user.avatarUrl}
          <img class="ava-img" src={api.resolveUrl(auth.user.avatarUrl)} alt={auth.user.username} />
        {:else}
          <span class="ava monogram" style={`--mg-hue:${nameHue(auth.user.username)}`}>{initial}</span>
        {/if}
        <div class="mast-who">
          <p class="kick">Profilul tău</p>
          <h1>{auth.user.username}</h1>
          <p class="net">
            <a class="ni" href={`/user/${auth.user.username}/urmaritori`}><strong>{network?.followers ?? 0}</strong> urmăritori</a>
            <a class="ni" href={`/user/${auth.user.username}/urmariti`}><strong>{network?.following ?? 0}</strong> urmărești</a>
            <a class="ni" href={`/user/${auth.user.username}/recenzii`}><strong>{reviewCount}</strong> recenzii</a>
            {#if memberSince}<span class="ni since">membru din {memberSince}</span>{/if}
          </p>
        </div>
      </div>
      <div class="mast-side">
        <button class="btn ghost" onclick={openBannerPicker}>🖼 Fundal</button>
        <button class="btn ghost" onclick={() => (importOpen = !importOpen)}>↓ Importă listă</button>
        <a class="btn ghost" href="/profile/edit">✎ Editează profilul</a>
      </div>
    </header>

    {#if bannerOpen}
      <section class="panel">
        <div class="panel-head">
          <div>
            <span class="kick">Fundalul profilului</span>
            <p class="muted">
              Alege o serie din listele tale. Apar doar titlurile care au ilustrație lată.
            </p>
          </div>
          {#if banner}
            <button class="btn ghost" disabled={bannerBusy} onclick={clearBanner}>Scoate fundalul</button>
          {/if}
        </div>

        {#if bannerOptions.length === 0}
          <p class="muted">
            Niciun titlu din listele tale nu are ilustrație. Adaugă câteva serii
            în lista ta și revino.
          </p>
        {:else}
          <div class="bsearch">
            <input
              type="search"
              placeholder="Caută în listele tale…"
              bind:value={bannerQ}
              oninput={() => (bannerShown = BANNER_PAGE)}
            />
            <span class="bcount">{bannerMatches.length} din {bannerOptions.length}</span>
          </div>

          {#if bannerMatches.length === 0}
            <p class="muted">Niciun titlu nu se potrivește cu „{bannerQ}”.</p>
          {:else}
            <ul class="bopts">
              {#each bannerVisible as o (o.mediaType + o.id)}
                <li>
                  <button
                    class="bopt"
                    class:on={banner?.id === o.id && banner?.mediaType === o.mediaType}
                    disabled={bannerBusy}
                    onclick={() => chooseBanner(o.mediaType, o.id)}
                  >
                    <!-- A real <img> rather than a background, so loading="lazy"
                         applies: the grid scrolls, and off-screen banners should
                         not be fetched until they are needed. -->
                    <img class="bopt-art" src={o.bannerUrl} alt="" loading="lazy" decoding="async" />
                    <span class="bopt-t">{o.title}</span>
                  </button>
                </li>
              {/each}
            </ul>
            {#if bannerVisible.length < bannerMatches.length}
              <button class="btn ghost bmore" onclick={() => (bannerShown += BANNER_PAGE)}>
                Arată mai multe ({bannerMatches.length - bannerVisible.length} rămase)
              </button>
            {/if}
          {/if}
        {/if}
      </section>
    {/if}

    {#if importOpen}
      <section class="panel">
        <span class="kick">Importă din AniList sau MyAnimeList</span>
        <p class="muted">
          Aducem statusul, progresul, nota și datele de început/final. Notele
          tale de aici nu sunt suprascrise, iar importul nu apare în istoricul
          de activitate.
        </p>

        <form class="imp-row" onsubmit={importAniList}>
          <label class="imp-lbl" for="al-user">AniList</label>
          <input
            id="al-user"
            bind:value={alUser}
            placeholder="numele tău de utilizator"
            autocomplete="off"
            spellcheck="false"
          />
          <button class="btn fill" type="submit" disabled={importing || !alUser.trim()}>
            {importing ? 'Se importă…' : 'Importă'}
          </button>
        </form>
        <p class="hint">Lista trebuie să fie publică în setările AniList.</p>

        <div class="imp-row">
          <span class="imp-lbl">MyAnimeList</span>
          <label class="btn ghost file">
            {importing ? 'Se importă…' : 'Alege fișierul exportat'}
            <input type="file" accept=".xml,.gz,application/gzip,text/xml" disabled={importing} onchange={importMal} />
          </label>
        </div>
        <p class="hint">
          MAL nu are un API public pentru liste. Descarcă exportul din
          <em>MyAnimeList → Settings → Export</em> și încarcă fișierul aici
          (merge și pentru liste private).
        </p>

        {#if importReport}
          <div class="report">
            {#each Object.entries(importReport) as [kind, r] (kind)}
              {#if r}
                <p>
                  <strong>{kind === 'anime' ? 'Anime' : 'Manga'}:</strong>
                  {r.imported} adăugate, {r.updated} actualizate, {r.skipped} lipsesc din catalog.
                </p>
                {#if r.unmatched?.length}
                  <p class="muted small">
                    Nu avem în catalog, de exemplu: {r.unmatched.join(', ')}…
                  </p>
                {/if}
              {/if}
            {/each}
          </div>
        {/if}
      </section>
    {/if}

    {#if auth.user.bio}
      <p class="bio">{auth.user.bio}</p>
    {/if}

    <!-- STAT STRIP -->
    {#if stats}
      <div class="strip">
        <a class="cell link" href="/istoric?media=anime&status=completed" title="Vezi anime-urile finalizate">
          <span class="v">{stats.totalAnimeWatched}</span><span class="l">anime văzute →</span>
        </a>
        <a class="cell link" href="/istoric?media=anime" title="Vezi istoricul de vizionare">
          <span class="v">{stats.totalEpisodesWatched}</span><span class="l">episoade</span>
        </a>
        <div class="cell"><span class="v">{stats.totalHoursWatched}</span><span class="l">ore</span></div>
        <a class="cell link" href="/istoric?media=manga&status=completed" title="Vezi manga finalizate">
          <span class="v">{stats.totalMangaRead}</span><span class="l">manga citite →</span>
        </a>
        <a class="cell link" href="/istoric?media=manga" title="Vezi istoricul de citire">
          <span class="v">{stats.totalChaptersRead}</span><span class="l">capitole</span>
        </a>
        <!-- scores are stored 1–10 (stars × 2) but rated in stars, so the
             strip has to divide by two or it reads as a 10-point score -->
        <div class="cell">
          <span class="v">{stats.averageAnimeScore ? (stats.averageAnimeScore / 2).toFixed(1) : '—'}</span>
          <span class="l">scor mediu</span>
        </div>
      </div>
    {/if}

    {#if loading}
      <p class="muted loading">Se încarcă…</p>
    {:else}
      <!-- FAVORITES -->
      <section class="sec">
        <div class="sec-head">
          <h2 class="sect kicker">Favorite</h2>
          <a class="sec-link" href="/profile/edit#favorite">modifică →</a>
        </div>
        {#if favs.length}
          <div class="favs">
            {#each favs as f (f.type + f.item.id)}
              <PosterCard a={f.item} href={`/${f.type}/${f.item.id}`} />
            {/each}
          </div>
        {:else}
          <a class="empty-cta" href="/profile/edit#favorite">
            <span class="cta-t">Alege-ți titlurile favorite</span>
            <span class="cta-m">Până la 5 anime sau manga, afișate aici — vitrina ta.</span>
          </a>
        {/if}
      </section>

      <!-- ACTIVITY -->
      <section class="sec">
        <div class="sec-head">
          <h2 class="sect kicker">Activitate · ultimele 14 zile</h2>
          <span class="sec-meta">
            <span class="leg"><span class="dot ep"></span>{epTotal} episoade</span>
            <span class="leg"><span class="dot ch"></span>{chTotal} capitole</span>
          </span>
        </div>
        <div class="act">
          {#each activity as a (a.key)}
            <div class="act-col" title={`${a.ep} episoade · ${a.ch} capitole`}>
              <span class="act-n" class:zero={a.n === 0}>{a.n || ''}</span>
              <span class="act-stack" class:off={a.n === 0} style={a.n === 0 ? '' : `height:${Math.max(4, Math.round((a.n / actMax) * 72))}px`}>
                {#if a.ch}<span class="act-seg ch" style={`flex-grow:${a.ch}`}></span>{/if}
                {#if a.ep}<span class="act-seg ep" style={`flex-grow:${a.ep}`}></span>{/if}
              </span>
            </div>
          {/each}
        </div>
        <div class="act-days">
          {#each activity as a (a.key)}
            <span class="act-d"><span class="act-dw">{a.day}</span>{a.date}</span>
          {/each}
        </div>
      </section>

      <!-- TASTES -->
      <section class="sec">
        <div class="sec-head"><h2 class="sect kicker">Gusturi</h2></div>
        <div class="taste-cols">
          <div class="taste">
            <p class="taste-t">Anime, după status</p>
            {#if statusBars.length}
              {#each statusBars as b (b.label)}
                <div class="trow">
                  <span class="trow-l">{b.label}</span>
                  <span class="trow-track"><span class="trow-fill" style={`width:${b.pct}%`}></span></span>
                  <span class="trow-n">{b.n}</span>
                </div>
              {/each}
            {:else}
              <p class="muted">Lista e goală deocamdată.</p>
            {/if}
          </div>
          <div class="taste">
            <p class="taste-t">Genuri, anime & manga</p>
            {#if genreBars.length}
              {#each genreBars as g (g.label)}
                <div class="trow">
                  <span class="trow-l">{g.label}</span>
                  <span class="trow-track"><span class="trow-fill" style={`width:${g.pct}%`}></span></span>
                  <span class="trow-n">{g.n}</span>
                </div>
              {/each}
            {:else}
              <p class="muted">Adaugă titluri în listă ca să vedem gusturile tale.</p>
            {/if}
          </div>
        </div>
      </section>

      <!-- TRACKING -->
      <section class="sec">
        <div class="sec-head"><h2 class="sect kicker">Urmărire</h2></div>
        <div class="tracks">
          <a class="track-row" href="/lista">
            <span class="track-t">Watchlist</span>
            <span class="track-m">
              {watchlist.filter((e) => e.status === 'plan-to-watch').length} anime ·
              {readlist.filter((e) => e.status === 'plan-to-read').length} manga de văzut
            </span>
            <span class="track-go">→</span>
          </a>
          <a class="track-row" href="/istoric">
            <span class="track-t">Istoric</span>
            <span class="track-m">
              {watchlist.filter((e) => e.status !== 'plan-to-watch').length} anime ·
              {readlist.filter((e) => e.status !== 'plan-to-read').length} manga începute sau văzute
            </span>
            <span class="track-go">→</span>
          </a>
          <a class="track-row" href={`/user/${auth.user.username}/note`}>
            <span class="track-t">Notele mele</span>
            <span class="track-m">{ratedCount} titluri notate</span>
            <span class="track-go">→</span>
          </a>
          <a class="track-row" href={`/user/${auth.user.username}/recenzii`}>
            <span class="track-t">Recenziile mele</span>
            <span class="track-m">{reviewCount} recenzii scrise</span>
            <span class="track-go">→</span>
          </a>
        </div>
      </section>
    {/if}
  </div>
{/if}

<style>
  .prof { max-width: var(--container-narrow); padding-block: var(--space-6) var(--space-8); position: relative; }

  /* ---- backdrop (PLAN 8.17) ----
     Letterboxd puts the art at the very top and lets the page grow out of it.
     The two rules that make it read as atmosphere instead of a pasted photo:
     it fades to nothing before the content starts, and it is desaturated to
     sit inside the palette rather than fight it. */
  /* Letterboxd's proportions: the identity block sits low over the art so the
     backdrop is the thing you see first, not a strip behind a header. Only when
     a banner exists — without one this class is absent and the page keeps its
     normal top padding, with no empty gap. */
  .prof.has-backdrop { padding-top: 260px; }
  .backdrop {
    position: absolute; inset: 0 auto auto 50%;
    width: 100vw; height: 460px;
    transform: translateX(-50%);
    z-index: 0; pointer-events: none; overflow: hidden;
  }
  .backdrop-art {
    width: 100%; height: 100%;
    background-size: cover; background-position: center 28%;
    /* desaturated + dimmed so white text stays readable on any art */
    filter: saturate(0.7) contrast(1.02) brightness(0.55);
    /* fade out downwards; the page background takes over before the stat strip */
    -webkit-mask-image: linear-gradient(to bottom, rgba(0, 0, 0, 0.95) 0%, rgba(0, 0, 0, 0.55) 45%, transparent 100%);
    mask-image: linear-gradient(to bottom, rgba(0, 0, 0, 0.95) 0%, rgba(0, 0, 0, 0.55) 45%, transparent 100%);
  }
  /* horizontal vignette: keeps the edges from cutting off as hard rectangles
     on wide screens, where the art is much wider than the content column */
  .backdrop::after {
    content: ''; position: absolute; inset: 0;
    background: linear-gradient(to right, var(--surface-base), transparent 18%, transparent 82%, var(--surface-base));
  }
  /* everything after the backdrop has to sit above it */
  .prof > :global(*:not(.backdrop)) { position: relative; z-index: 1; }

  /* ---- masthead: identity above the fold, print-style ---- */
  .mast {
    display: flex; align-items: flex-end; justify-content: space-between;
    gap: var(--space-4); flex-wrap: wrap;
    padding-bottom: 22px; border-bottom: 2px solid var(--text-primary);
  }
  /* shrinks so the meta row wraps instead of pushing the action buttons onto
     their own line (same reason as /user/[username]) */
  .mast-id { display: flex; align-items: center; gap: 22px; min-width: 0; flex: 1 1 260px; }
  .mast-who { min-width: 0; }
  .ava, .ava-img {
    width: 84px; height: 84px; border-radius: 50%; flex: 0 0 auto;
    border: 1px solid var(--border-default);
  }
  .ava {
    display: grid; place-items: center;
    background: linear-gradient(135deg, var(--accent), var(--accent-strong));
    font-family: var(--font-display); font-size: 2.125rem; font-weight: var(--fw-semibold); color: #fff;
  }
  .ava-img { object-fit: cover; }
  .kick { font-size: var(--fs-caption); font-weight: var(--fw-bold); color: var(--accent); }
  .mast-who h1 {
    font-size: clamp(1.9rem, 1.5rem + 1.8vw, 2.625rem);
    letter-spacing: -0.02em; line-height: 1.05; margin-top: 6px;
  }
  /* A flex row, not inline text. As inline flow the separator and the item
     after it were independent boxes, so a wrap could strand a "·" at the end
     of one line and put "membru din …" alone on the next — and whether that
     happened at all depended on how many items the profile had, which is why
     some profiles looked fine and others did not. Flex items wrap as whole
     units, and `baseline` keeps the mono "membru din" sitting on the same line
     as the rest despite its smaller size. */
  .net {
    display: flex; flex-wrap: wrap; align-items: baseline; row-gap: 4px;
    margin-top: 10px; font-size: var(--fs-small); color: var(--text-muted);
  }
  /* The separator belongs to the item it precedes, so the two can never split.
     `.ni` rather than `* + *`: Svelte prunes CSS it cannot tie to an element in
     the template, and a universal selector is exactly that — the rule compiled
     away silently and the dots disappeared entirely. */
  .net .ni + .ni::before {
    content: '·'; margin: 0 7px; color: var(--text-faint);
  }
  .net a { color: var(--text-muted); }
  .net a:hover { color: var(--accent); }
  .net strong { color: var(--text-primary); font-weight: var(--fw-semibold); }
  .since { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }

  .mast-side { display: flex; gap: 9px; flex-wrap: wrap; flex: 0 0 auto; justify-content: flex-end; }
  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-caption);
    padding: 9px 16px; border-radius: var(--radius-md); cursor: pointer; white-space: nowrap;
  }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-primary); }
  .btn.ghost:hover { border-color: color-mix(in srgb, var(--accent) 55%, transparent); color: var(--accent); }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); }
  .btn:disabled { opacity: 0.6; cursor: wait; }

  /* ---- backdrop picker + import panels ---- */
  .panel {
    margin-top: var(--space-5);
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg); padding: var(--space-5);
  }
  .panel-head {
    display: flex; align-items: flex-start; justify-content: space-between;
    gap: var(--space-4); flex-wrap: wrap; margin-bottom: var(--space-4);
  }
  .panel .muted { color: var(--text-muted); font-size: var(--fs-small); line-height: 1.55; margin-top: 6px; max-width: 60ch; }
  .panel .small { font-size: var(--fs-caption); }
  .hint { color: var(--text-faint); font-size: var(--fs-caption); margin-top: 6px; }
  .hint em { color: var(--text-muted); font-style: normal; font-family: var(--font-mono); }

  .bopts {
    list-style: none; display: grid; gap: 10px; margin-top: var(--space-4);
    grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
    max-height: 340px; overflow-y: auto;
  }
  .bopt {
    width: 100%; padding: 0; cursor: pointer; font: inherit; text-align: left;
    background: none; border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
    overflow: hidden; color: var(--text-primary);
  }
  .bopt:hover { border-color: var(--accent); }
  .bopt.on { border-color: var(--accent); box-shadow: 0 0 0 1px var(--accent); }
  .bopt:disabled { opacity: 0.5; cursor: wait; }
  /* 3:1 — the shape the art is actually drawn at, so the preview doesn't lie
     about how it will crop in the header */
  .bopt-art {
    display: block; width: 100%; aspect-ratio: 3 / 1;
    object-fit: cover; background: var(--surface-inset);
  }
  .bsearch { display: flex; align-items: center; gap: 10px; margin-top: var(--space-4); }
  .bsearch input {
    flex: 1; min-width: 0;
    background: var(--surface-inset); color: var(--text-primary);
    border: 1px solid var(--border-default); border-radius: var(--radius-sm);
    padding: 8px 12px; font: inherit; font-size: var(--fs-small);
  }
  .bsearch input:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 1px; }
  .bcount { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-faint); }
  .bmore { margin-top: 10px; }
  .bopt-t {
    display: block; padding: 8px 10px; font-size: var(--fs-caption);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }

  .imp-row { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; margin-top: var(--space-4); }
  .imp-lbl {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-muted);
    min-width: 92px;
  }
  .imp-row input:not([type='file']) {
    flex: 1; min-width: 200px; min-height: 40px; padding: 0 12px;
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary); outline: none;
  }
  .imp-row input:focus { border-color: var(--accent); }
  .file { display: inline-flex; align-items: center; }
  .file input { display: none; }

  .report {
    margin-top: var(--space-4); padding-top: var(--space-4);
    border-top: 1px solid var(--border-subtle); font-size: var(--fs-small);
  }
  .report p + p { margin-top: 4px; }

  .bio {
    margin-top: 20px; max-width: 62ch;
    font-family: var(--font-display); font-style: italic;
    font-size: var(--fs-body); line-height: 1.6; color: var(--text-muted);
  }

  /* ---- stat strip: a data line, not cards ---- */
  .strip {
    display: grid; grid-template-columns: repeat(6, minmax(0, 1fr));
    margin-top: 26px; border-bottom: 1px solid var(--border-subtle); padding-bottom: 24px;
  }
  .cell { display: flex; flex-direction: column; min-width: 0; }
  .cell + .cell { border-left: 1px solid var(--border-subtle); padding-left: clamp(14px, 2.5vw, 28px); }
  .cell .v {
    font-family: var(--font-display); font-size: clamp(1.5rem, 1.2rem + 1vw, 1.875rem);
    font-weight: var(--fw-semibold); letter-spacing: -0.015em; line-height: 1.1;
  }
  .cell .l {
    font-family: var(--font-mono); font-size: var(--fs-micro); letter-spacing: 0.06em;
    text-transform: uppercase; color: var(--text-muted); margin-top: 6px;
  }
  .cell.link { color: inherit; }
  .cell.link:hover .v { color: var(--accent); }
  .cell.link:hover .l { color: var(--accent); }

  .loading { padding-top: var(--space-6); }
  .muted { color: var(--text-muted); font-size: var(--fs-small); }

  /* ---- sections: mono kicker + hairline, no boxes ---- */
  .sec { padding-top: 34px; }
  .sec + .sec { margin-top: 34px; border-top: 1px solid var(--border-subtle); }
  .sec-head { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; margin-bottom: 20px; }
  .sect {
    font-family: var(--font-mono); font-size: var(--fs-caption); font-weight: var(--fw-medium);
    letter-spacing: 0.12em; text-transform: uppercase; color: var(--text-muted);
  }
  .sec-link { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .sec-link:hover { color: var(--accent); }
  .sec-meta {
    display: flex; gap: 16px; align-items: center;
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
  }
  .leg { display: inline-flex; align-items: center; gap: 6px; }
  .dot { width: 8px; height: 8px; border-radius: 2px; }
  .dot.ep { background: var(--accent); }
  .dot.ch { background: var(--accent-strong); }

  .favs { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 16px; }
  .empty-cta {
    display: block; padding: 30px 0;
    border-top: 1px solid var(--border-subtle); border-bottom: 1px solid var(--border-subtle);
  }
  .empty-cta:hover .cta-t { color: var(--accent); }
  .cta-t {
    display: block; font-family: var(--font-display); font-style: italic;
    font-size: var(--fs-h3); font-weight: var(--fw-semibold); color: var(--text-primary);
  }
  .cta-m { display: block; margin-top: 6px; font-size: var(--fs-caption); color: var(--text-muted); }

  /* ---- activity: stacked ep/cap bars on a shared baseline rule ---- */
  .act {
    display: flex; align-items: flex-end; gap: 8px;
    border-bottom: 1px solid var(--border-default);
  }
  .act-col { flex: 1; display: flex; flex-direction: column; align-items: center; gap: 7px; min-width: 0; }
  .act-n { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); height: 1.1em; }
  .act-n.zero { color: transparent; }
  .act-stack {
    display: flex; flex-direction: column;
    width: 100%; max-width: 24px; overflow: hidden; border-radius: 2px 2px 0 0;
  }
  .act-stack.off { background: var(--surface-overlay); height: 4px; }
  .act-seg.ep { background: var(--accent); }
  .act-seg.ch { background: var(--accent-strong); }
  .act-days { display: flex; gap: 8px; margin-top: 10px; }
  .act-d {
    flex: 1; text-align: center; min-width: 0;
    display: flex; flex-direction: column; gap: 1px;
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
  }
  .act-dw { font-size: var(--fs-micro); color: var(--text-muted); }

  /* ---- tastes: two flat columns of labeled rules ---- */
  .taste-cols { display: grid; grid-template-columns: 1fr 1fr; gap: clamp(24px, 5vw, 56px); }
  .taste-t {
    font-family: var(--font-display); font-size: var(--fs-h3); font-weight: var(--fw-semibold);
    margin-bottom: 14px;
  }
  .trow { display: flex; align-items: center; gap: 12px; padding: 8px 0; }
  .trow-l { flex: 0 0 132px; font-size: var(--fs-small); color: var(--text-muted); }
  .trow-track { flex: 1; height: 3px; background: var(--surface-overlay); }
  .trow-fill { display: block; height: 100%; background: var(--accent); }
  .trow-n { flex: 0 0 auto; font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); min-width: 2ch; text-align: right; }

  /* ---- tracking rows ---- */
  .tracks { display: flex; flex-direction: column; }
  .track-row {
    display: flex; align-items: baseline; gap: 16px;
    padding: 16px 0; border-bottom: 1px solid var(--border-subtle);
  }
  .track-row:first-child { border-top: 1px solid var(--border-subtle); }
  .track-t {
    font-family: var(--font-display); font-size: var(--fs-h3);
    font-weight: var(--fw-semibold); color: var(--text-primary);
  }
  .track-row:hover .track-t { color: var(--accent); }
  .track-m { flex: 1; font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .track-go { color: var(--text-muted); }
  .track-row:hover .track-go { color: var(--accent); }

  @media (max-width: 720px) {
    /* A phone screen is mostly the offset at 260px — enough banner to set the
       scene, not a screenful of it before the name appears. */
    .prof.has-backdrop { padding-top: 150px; }
    .backdrop { height: 300px; }

    .mast-id { align-items: flex-start; gap: 16px; }
    .ava, .ava-img { width: 64px; height: 64px; }
    .ava { font-size: 1.625rem; }
    .strip { grid-template-columns: repeat(3, minmax(0, 1fr)); row-gap: 20px; }
    .cell + .cell { border-left: none; padding-left: 0; }
    .cell:nth-child(3n + 2), .cell:nth-child(3n) { border-left: 1px solid var(--border-subtle); padding-left: 16px; }
    .favs { grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; }
    .taste-cols { grid-template-columns: minmax(0, 1fr); gap: 28px; }
    .act, .act-days { gap: 4px; }
    .trow-l { flex-basis: 108px; }
    .act-d { font-size: var(--fs-micro); }
  }
</style>
