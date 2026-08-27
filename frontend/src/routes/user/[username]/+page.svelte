<script lang="ts">
  import RichText from '$lib/components/RichText.svelte';
  import { mediaUrl } from '$lib/media';
  import PosterCard from '$lib/components/PosterCard.svelte';
  import api from '$lib/api';
  import { nameHue } from '$lib/avatar';
  import { authStore } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import { displayName } from '$lib/types';

  let { data } = $props();
  const auth = $derived($authStore);

  const p = $derived(data.profile);
  const isSelf = $derived(auth.user?.username === data.handle);
  const isReal = $derived(data.kind === 'real');

  /* SSR ships guest-view counts (rendered immediately); the viewer's own
     relation (isFollowing) needs their token, which only exists client-side. */
  let followers = $state(data.profile.followers ?? 0);
  let followingCount = $state(data.profile.following ?? 0);
  let following = $state(false);
  let busy = $state(false);
  let hydratedFor = $state('');

  $effect(() => {
    if (hydratedFor === data.handle) return;
    hydratedFor = data.handle;
    followers = p.followers ?? 0;
    followingCount = p.following ?? 0;
    following = false;
    if (isReal && auth.isAuthenticated && !isSelf) {
      api
        .getPublicUser(data.handle)
        .then((res) => {
          followers = res.network.followers;
          followingCount = res.network.following;
          following = res.network.isFollowing;
        })
        .catch(() => {});
    }
  });

  async function toggleFollow() {
    if (!auth.isAuthenticated) {
      toast.info('Autentifică-te ca să urmărești membri.');
      return;
    }
    if (!isReal) {
      // seeded demo members live outside the real social graph
      following = !following;
      followers += following ? 1 : -1;
      return;
    }
    busy = true;
    try {
      const res = following ? await api.unfollowUser(data.handle) : await api.followUser(data.handle);
      following = res.data.isFollowing;
      followers = res.data.followers;
    } catch {
      toast.error('Nu am putut actualiza urmărirea.');
    } finally {
      busy = false;
    }
  }

  const memberSince = $derived(
    p.memberSince ? new Date(p.memberSince).toLocaleDateString('ro-RO', { year: 'numeric', month: 'long' }) : null
  );

  const revDate = (d: Date | string) =>
    new Date(d).toLocaleDateString('ro-RO', { day: 'numeric', month: 'long', year: 'numeric' });
  const revTitle = (t: { title: string; titleRomanian?: string }) => t.titleRomanian || t.title;

  const actMax = $derived(Math.max(1, ...(data.activity ?? [1])));
  const DAYS_RO = ['D', 'L', 'Ma', 'Mi', 'J', 'V', 'S'];
  const dayLabel = (i: number) => {
    const d = new Date();
    d.setDate(d.getDate() - (13 - i));
    return DAYS_RO[d.getDay()];
  };

  /* ── real activity + tastes, mirroring /profile ──────────────────────────
     A member's page used to stop at favourites and reviews, so visiting
     someone else told you much less than your own page told you. Lists,
     ratings and history are already public (Letterboxd-style), so the same
     panels belong here. */
  const realActivity = $derived.by(() => {
    const byDate = new Map((data.history ?? []).map((h) => [h.date, h]));
    return Array.from({ length: 14 }, (_, i) => {
      const d = new Date();
      d.setHours(0, 0, 0, 0);
      d.setDate(d.getDate() - (13 - i));
      const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
      const h = byDate.get(key);
      const ep = h?.episodes ?? 0;
      const ch = h?.chapters ?? 0;
      return { key: d.getTime(), day: DAYS_RO[d.getDay()], date: d.getDate(), ep, ch, n: ep + ch };
    });
  });
  const realActMax = $derived(Math.max(1, ...realActivity.map((a) => a.n)));
  const epTotal = $derived(realActivity.reduce((s, a) => s + a.ep, 0));
  const chTotal = $derived(realActivity.reduce((s, a) => s + a.ch, 0));
  const hasRealActivity = $derived(epTotal + chTotal > 0);

  const statusBars = $derived.by(() => {
    const order: [string, string][] = [
      ['watching', 'În vizionare'],
      ['completed', 'Finalizate'],
      ['plan-to-watch', 'Planificate'],
      ['on-hold', 'În așteptare'],
      ['dropped', 'Abandonate']
    ];
    const wl = data.trackedAnime ?? [];
    const max = Math.max(1, ...order.map(([k]) => wl.filter((e) => e.status === k).length));
    return order
      .map(([k, label]) => ({ label, n: wl.filter((e) => e.status === k).length }))
      .filter((b) => b.n > 0)
      .map((b) => ({ ...b, pct: Math.round((b.n / max) * 100) }));
  });

  const genreBars = $derived.by(() => {
    const counts = new Map<string, number>();
    for (const e of data.trackedAnime ?? []) for (const g of e.anime?.genres ?? []) counts.set(g, (counts.get(g) ?? 0) + 1);
    for (const e of data.trackedManga ?? []) for (const g of e.manga?.genres ?? []) counts.set(g, (counts.get(g) ?? 0) + 1);
    const top = [...counts.entries()].sort((a, b) => b[1] - a[1]).slice(0, 5);
    const max = top[0]?.[1] ?? 1;
    return top.map(([label, n]) => ({ label, n, pct: Math.round((n / max) * 100) }));
  });

  const trackedCount = $derived((data.trackedAnime ?? []).length + (data.trackedManga ?? []).length);
</script>

<svelte:head><title>{p.name} · Anime-Kage</title></svelte:head>

<div class="container prof" class:has-backdrop={!!p.banner}>
  <!-- Backdrop: the series this member chose (PLAN 8.17), same treatment as
       /profile so the two pages read as one design. -->
  {#if p.banner}
    <div class="backdrop" aria-hidden="true">
      <div class="backdrop-art" style={`background-image:url(${p.banner.bannerUrl})`}></div>
    </div>
  {/if}

  <!-- MASTHEAD -->
  <header class="mast">
    <div class="mast-id">
      {#if p.avatarUrl}
        <img class="ava-img" src={api.resolveUrl(p.avatarUrl)} alt={p.name} />
      {:else if p.hue != null}
        <span class="ava monogram" style={`--mg-hue:${p.hue}`}>{p.name.charAt(0)}</span>
      {:else}
        <span class="ava monogram" style={`--mg-hue:${nameHue(p.name)}`}>{p.name.charAt(0).toUpperCase()}</span>
      {/if}
      <div class="mast-who">
        <p class="kick">
          Membru
          {#if p.role && p.role !== 'user'}<span class="role">{p.role}</span>{/if}
        </p>
        <h1>{p.name}</h1>
        <p class="net">
          <a class="ni" href={`/user/${data.handle}/urmaritori`}><strong>{followers}</strong> urmăritori</a>
          <a class="ni" href={`/user/${data.handle}/urmariti`}><strong>{followingCount}</strong> urmărește</a>
          <a class="ni" href={`/user/${data.handle}/recenzii`}><strong>{data.userReviewCount || data.reviews.length}</strong> recenzii</a>
          {#if data.ratedCount}<a class="ni" href={`/user/${data.handle}/note`}><strong>{data.ratedCount}</strong> note</a>{/if}
          {#if memberSince}<span class="ni since">membru din {memberSince}</span>{/if}
        </p>
      </div>
    </div>
    <div class="mast-side">
      {#if isSelf}
        <a class="btn ghost" href="/profile/edit">Editează profilul</a>
      {:else}
        <button class="btn" class:fill={!following} class:ghost={following} onclick={toggleFollow} disabled={busy}>
          {following ? '✓ Urmărit' : '+ Urmărește'}
        </button>
      {/if}
    </div>
  </header>

  {#if p.bio}
    <p class="bio">{p.bio}</p>
  {/if}

  <!-- STAT STRIP -->
  <!-- Shown for every real account, zeros included. It used to be hidden when
       every number was 0, which made a new member's profile a different shape
       from everyone else's — and made the strip look like something only your
       own profile had. `data.stats` is still guarded because the seeded demo
       members have no account behind them and so no real numbers. -->
  {#if data.stats}
    <div class="strip" role="list">
      <div class="cell" role="listitem"><span class="v">{data.stats.totalAnimeWatched}</span><span class="l">anime văzute</span></div>
      <div class="cell" role="listitem"><span class="v">{data.stats.totalEpisodesWatched}</span><span class="l">episoade</span></div>
      <div class="cell" role="listitem"><span class="v">{data.stats.totalHoursWatched}</span><span class="l">ore</span></div>
      <div class="cell" role="listitem"><span class="v">{data.stats.totalMangaRead}</span><span class="l">manga citite</span></div>
      <!-- stored 1–10 (stars × 2) but rated in stars — halve it or it reads
           as a 10-point score next to a 5-star widget -->
      <div class="cell" role="listitem">
        <span class="v">{data.stats.averageAnimeScore > 0 ? (data.stats.averageAnimeScore / 2).toFixed(1) : '—'}</span>
        <span class="l">scor mediu / 5</span>
      </div>
    </div>
  {:else}
    <div class="strip-rule"></div>
  {/if}

  <!-- FAVORITES -->
  {#if data.favorites.length}
    <section class="sec">
      <div class="sec-head"><h2 class="sect">Favorite</h2></div>
      <div class="favs">
        {#each data.favorites as f (f.type + f.item.id)}
          <PosterCard a={f.item} href={`/${f.type}/${f.item.id}`} />
        {/each}
      </div>
    </section>
  {/if}

  <!-- WATCHLIST (public, Letterboxd-style) -->
  {#if data.watchlistPeek.length}
    <section class="sec">
      <div class="sec-head">
        <h2 class="sect">Watchlist</h2>
        <a class="sec-link" href={`/user/${data.handle}/watchlist`}>
          {data.watchlistCount > data.watchlistPeek.length ? `toate (${data.watchlistCount}) →` : 'vezi lista →'}
        </a>
      </div>
      <div class="favs">
        {#each data.watchlistPeek as w (w.kind + w.item.id)}
          <PosterCard a={w.item} href={`/${w.kind}/${w.item.id}`} />
        {/each}
      </div>
    </section>
  {/if}

  <!-- ACTIVITY (seed profiles only) -->
  {#if data.activity}
    <section class="sec">
      <div class="sec-head"><h2 class="sect">Activitate · ultimele 14 zile</h2></div>
      <div class="act">
        {#each data.activity as n, i (i)}
          <div class="act-col" title={`${n} episoade`}>
            <span class="act-n" class:zero={n === 0}>{n || ''}</span>
            <span class="act-bar" style={`height:${Math.max(3, Math.round((n / actMax) * 64))}px`} class:off={n === 0}></span>
          </div>
        {/each}
      </div>
      <div class="act-days">
        {#each data.activity as _, i (i)}
          <span class="act-d">{dayLabel(i)}</span>
        {/each}
      </div>
    </section>
  {/if}

  <!-- REVIEWS: real accounts -->
  {#if data.userReviews.length}
    <section class="sec">
      <div class="sec-head">
        <h2 class="sect">Recenzii recente</h2>
        <a class="sec-link" href={`/user/${data.handle}/recenzii`}>toate ({data.userReviewCount}) →</a>
      </div>
      <div class="revs">
        {#each data.userReviews as r (r.kind + r.entryId)}
          <article class="rev">
            <a
              class="rev-thumb media-tone"
              href={`/${r.kind}/${r.title.id}`}
              style={r.title.imageUrl ? `background-image:url(${mediaUrl(r.title.imageUrl)})` : ''}
              aria-label={revTitle(r.title)}
            ></a>
            <div class="rev-main">
              <div class="rev-head">
                <a class="rev-t" href={`/${r.kind}/${r.title.id}`}><em>{revTitle(r.title)}</em>{#if r.title.year}<span class="rev-y">{r.title.year}</span>{/if}</a>
                {#if r.score}
                  <span class="stars">{'★'.repeat(Math.round(r.score / 2))}<span class="stars-off">{'★'.repeat(5 - Math.round(r.score / 2))}</span></span>
                {/if}
              </div>
              <p class="rev-text"><RichText text={r.notes} /></p>
              <p class="rev-meta">
                {revDate(r.updatedAt)}
                · <a class="rev-go" href={`/${r.kind}/${r.title.id}/review/${r.entryId}`}>💬 {r.replyCount ? `${r.replyCount} comentarii` : 'Comentează'} →</a>
              </p>
            </div>
          </article>
        {/each}
      </div>
    </section>
  {/if}

  <!-- REVIEWS: seed profiles -->
  {#if data.reviews.length}
    <section class="sec">
      <div class="sec-head">
        <h2 class="sect">Recenzii recente</h2>
        <a class="sec-link" href={`/user/${data.handle}/recenzii`}>toate →</a>
      </div>
      <div class="revs">
        {#each data.reviews as r, i (i)}
          <article class="rev">
            <a class="rev-thumb media-tone" href={`/anime/${r.anime.id}`} style={r.anime.imageUrl ? `background-image:url(${mediaUrl(r.anime.imageUrl)})` : ''} aria-label={displayName(r.anime)}></a>
            <div class="rev-main">
              <div class="rev-head">
                <a class="rev-t" href={`/anime/${r.anime.id}`}><em>{displayName(r.anime)}</em></a>
                <span class="stars">{'★'.repeat(r.rating)}<span class="stars-off">{'★'.repeat(5 - r.rating)}</span></span>
              </div>
              <p class="rev-text">{r.text}</p>
              <p class="rev-meta">{r.date} · ♥ {r.likes}</p>
            </div>
          </article>
        {/each}
      </div>
    </section>
  {/if}

  <!-- LISTS -->
  {#if data.lists.length}
    <section class="sec">
      <div class="sec-head"><h2 class="sect">Listele lui {p.name}</h2></div>
      <div class="lists">
        {#each data.lists as l (l.slug)}
          <a class="lrow" href={`/liste/${l.slug}`}>
            <span class="fan">
              {#each l.covers as c, i (c)}
                <span class="fan-c media-tone" style={`background-image:url(${c});z-index:${3 - i}`}></span>
              {/each}
            </span>
            <span class="lmain">
              <span class="lt">{l.title}</span>
              <span class="lm">{l.count} titluri</span>
            </span>
            <span class="lgo">→</span>
          </a>
        {/each}
      </div>
    </section>
  {/if}

  <!-- ACTIVITY (real accounts) — the same 14-day chart as /profile -->
  {#if isReal && hasRealActivity}
    <section class="sec">
      <div class="sec-head">
        <h2 class="sect">Activitate · ultimele 14 zile</h2>
        <span class="sec-meta">
          <span class="leg"><span class="dot ep"></span>{epTotal} episoade</span>
          <span class="leg"><span class="dot ch"></span>{chTotal} capitole</span>
        </span>
      </div>
      <div class="act">
        {#each realActivity as a (a.key)}
          <div class="act-col" title={`${a.ep} episoade · ${a.ch} capitole`}>
            <span class="act-n" class:zero={a.n === 0}>{a.n || ''}</span>
            <span
              class="act-stack"
              class:off={a.n === 0}
              style={a.n === 0 ? '' : `height:${Math.max(4, Math.round((a.n / realActMax) * 72))}px`}
            >
              {#if a.ch}<span class="act-seg ch" style={`flex-grow:${a.ch}`}></span>{/if}
              {#if a.ep}<span class="act-seg ep" style={`flex-grow:${a.ep}`}></span>{/if}
            </span>
          </div>
        {/each}
      </div>
      <div class="act-days">
        {#each realActivity as a (a.key)}
          <span class="act-d"><span class="act-dw">{a.day}</span>{a.date}</span>
        {/each}
      </div>
    </section>
  {/if}

  <!-- TASTES (real accounts) -->
  {#if isReal && (statusBars.length || genreBars.length)}
    <section class="sec">
      <div class="sec-head"><h2 class="sect">Gusturi</h2></div>
      <div class="taste-cols">
        {#if statusBars.length}
          <div class="taste">
            <p class="taste-t">Anime, după status</p>
            {#each statusBars as b (b.label)}
              <div class="trow">
                <span class="trow-l">{b.label}</span>
                <span class="trow-track"><span class="trow-fill" style={`width:${b.pct}%`}></span></span>
                <span class="trow-n">{b.n}</span>
              </div>
            {/each}
          </div>
        {/if}
        {#if genreBars.length}
          <div class="taste">
            <p class="taste-t">Genuri, anime & manga</p>
            {#each genreBars as g (g.label)}
              <div class="trow">
                <span class="trow-l">{g.label}</span>
                <span class="trow-track"><span class="trow-fill" style={`width:${g.pct}%`}></span></span>
                <span class="trow-n">{g.n}</span>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </section>
  {/if}

  <!-- TRACKING: the same three doors /profile offers, pointing at their pages -->
  {#if isReal && trackedCount > 0}
    <section class="sec">
      <div class="sec-head"><h2 class="sect">Urmărire</h2></div>
      <div class="tracks">
        <a class="track-row" href={`/user/${data.handle}/watchlist`}>
          <span class="track-t">Watchlist</span>
          <span class="track-m">{data.watchlistCount} titluri de văzut</span>
          <span class="track-go">→</span>
        </a>
        <a class="track-row" href={`/user/${data.handle}/note`}>
          <span class="track-t">Note</span>
          <span class="track-m">{data.ratedCount} titluri notate</span>
          <span class="track-go">→</span>
        </a>
        <a class="track-row" href={`/user/${data.handle}/recenzii`}>
          <span class="track-t">Recenzii</span>
          <span class="track-m">{data.userReviewCount || data.reviews.length} scrise</span>
          <span class="track-go">→</span>
        </a>
      </div>
    </section>
  {/if}

  {#if data.kind === 'real' && !data.favorites.length && !data.userReviews.length}
    <section class="sec">
      <p class="muted">
        {p.name} nu și-a completat încă vitrina de favorite.
        {#if isSelf}<a class="inline-link" href="/profile/edit#favorite">Alege-ți favoritele →</a>{/if}
      </p>
    </section>
  {/if}
</div>

<style>
  .prof { max-width: var(--container-narrow); padding-block: var(--space-6) var(--space-8); position: relative; }

  /* ---- backdrop (PLAN 8.17) — mirrors /profile ----
     Full-bleed but faded to nothing before the content, desaturated so white
     text stays readable over any art, with a horizontal vignette so the edges
     don't cut off as hard rectangles on wide screens. */
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
    filter: saturate(0.7) contrast(1.02) brightness(0.55);
    -webkit-mask-image: linear-gradient(to bottom, rgba(0, 0, 0, 0.95) 0%, rgba(0, 0, 0, 0.55) 45%, transparent 100%);
    mask-image: linear-gradient(to bottom, rgba(0, 0, 0, 0.95) 0%, rgba(0, 0, 0, 0.55) 45%, transparent 100%);
  }
  .backdrop::after {
    content: ''; position: absolute; inset: 0;
    background: linear-gradient(to right, var(--surface-base), transparent 18%, transparent 82%, var(--surface-base));
  }
  .prof > :global(*:not(.backdrop)) { position: relative; z-index: 1; }

  /* ---- masthead ---- */
  .mast {
    display: flex; align-items: flex-end; justify-content: space-between;
    gap: var(--space-4); flex-wrap: wrap;
    padding-bottom: 22px; border-bottom: 2px solid var(--text-primary);
  }
  /* `flex: 1 1 260px` rather than auto-width: with an extra item in the meta
     row ("N note"), the identity block's one-line width grew past the space
     available and pushed the follow button onto its own row. Letting it
     shrink wraps the meta text instead, which is the part that should give. */
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
  .kick {
    display: flex; align-items: center; gap: 10px;
    font-size: var(--fs-caption); font-weight: var(--fw-bold); color: var(--accent);
  }
  .role {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-medium);
    letter-spacing: 0.08em; text-transform: uppercase;
    padding: 2px 8px; border: 1px solid color-mix(in srgb, var(--accent) 40%, transparent);
    border-radius: var(--radius-pill); color: var(--accent);
  }
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

  /* never the thing that gets pushed off the row — the follow button is the
     one action on this page */
  .mast-side { display: flex; gap: 9px; flex: 0 0 auto; }
  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-caption);
    padding: 9px 18px; border-radius: var(--radius-md); cursor: pointer; white-space: nowrap;
  }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-primary); }
  .btn.ghost:hover { background: var(--surface-raised); }
  .btn:disabled { opacity: 0.6; cursor: wait; }

  .bio {
    margin-top: 20px; max-width: 62ch;
    font-family: var(--font-display); font-style: italic;
    font-size: var(--fs-body); line-height: 1.6; color: var(--text-muted);
  }

  /* ---- stat strip ---- */
  .strip {
    display: grid; grid-template-columns: repeat(5, minmax(0, 1fr));
    margin-top: 26px; border-bottom: 1px solid var(--border-subtle); padding-bottom: 24px;
  }
  .strip-rule { margin-top: 26px; border-bottom: 1px solid var(--border-subtle); }
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

  .muted { color: var(--text-muted); font-size: var(--fs-small); }
  .inline-link { color: var(--accent); font-weight: var(--fw-semibold); }

  /* ---- sections ---- */
  .sec { padding-top: 34px; }
  .sec + .sec { margin-top: 34px; border-top: 1px solid var(--border-subtle); }
  .sec-head { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; margin-bottom: 20px; }
  .sect {
    font-family: var(--font-mono); font-size: var(--fs-caption); font-weight: var(--fw-medium);
    letter-spacing: 0.12em; text-transform: uppercase; color: var(--text-muted);
  }
  .sec-link { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .sec-link:hover { color: var(--accent); }

  .favs { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 16px; }

  /* ---- activity ---- */
  .act {
    display: flex; align-items: flex-end; gap: 8px;
    border-bottom: 1px solid var(--border-default);
  }
  .act-col { flex: 1; display: flex; flex-direction: column; align-items: center; gap: 6px; min-width: 0; }
  .act-n { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); height: 1.1em; }
  .act-n.zero { color: transparent; }
  .act-bar { width: 100%; max-width: 22px; background: var(--accent); }
  .act-bar.off { background: var(--surface-overlay); height: 3px !important; }
  .act-days { display: flex; gap: 8px; margin-top: 8px; }
  .act-d {
    flex: 1; text-align: center; min-width: 0;
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
  }

  /* ---- reviews: flat entries on hairlines ---- */
  .revs { display: flex; flex-direction: column; }
  .rev { display: flex; gap: 18px; padding: 22px 0; border-bottom: 1px solid var(--border-subtle); }
  .rev:first-child { padding-top: 0; }
  .rev-thumb {
    width: 58px; height: 86px; border-radius: 6px; flex: 0 0 auto;
    background-color: var(--surface-overlay); background-size: cover; background-position: center;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
  }
  .rev-main { flex: 1; min-width: 0; }
  .rev-head { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
  .rev-t em {
    font-family: var(--font-display); font-size: var(--fs-h3);
    font-weight: var(--fw-semibold); font-style: italic; color: var(--text-primary);
  }
  .rev-t:hover em { color: var(--accent); }
  .rev-y { margin-left: 8px; font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .stars { color: var(--accent); font-size: 0.8125rem; letter-spacing: 1.5px; }
  .stars-off { color: var(--surface-overlay); }
  .rev-text { margin-top: 8px; font-size: var(--fs-small); line-height: 1.65; color: var(--text-muted); }
  .rev-meta { margin-top: 10px; font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .rev-go { color: var(--text-muted); }
  .rev-go:hover { color: var(--accent); }

  /* ---- lists: flat rows ---- */
  .lists { display: flex; flex-direction: column; }
  .lrow { display: flex; align-items: center; gap: 16px; padding: 14px 0; border-bottom: 1px solid var(--border-subtle); }
  .lrow:first-child { padding-top: 0; }
  .fan { display: flex; flex: 0 0 auto; }
  .fan-c {
    width: 34px; height: 48px; border-radius: 5px;
    border: 1px solid var(--border-default);
    background-color: var(--surface-overlay); background-size: cover; background-position: center;
    position: relative;
  }
  .fan-c + .fan-c { margin-left: -14px; }
  .lmain { flex: 1; min-width: 0; display: flex; flex-direction: column; }
  .lt { font-family: var(--font-display); font-weight: var(--fw-semibold); color: var(--text-primary); }
  .lrow:hover .lt { color: var(--accent); }
  .lm { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); margin-top: 4px; }
  .lgo { color: var(--text-muted); }
  .lrow:hover .lgo { color: var(--accent); }

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
    .act, .act-days { gap: 4px; }
  }

  /* ---- real activity / tastes / tracking (mirrors /profile) ----
     .act, .act-col, .act-n and .act-days are already defined above for the
     seeded bar chart and are shared; only the stacked-bar pieces are new. */
  .sec-meta {
    display: flex; gap: 16px; align-items: center;
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
  }
  .leg { display: inline-flex; align-items: center; gap: 6px; }
  .dot { width: 8px; height: 8px; border-radius: 2px; }
  .dot.ep { background: var(--accent); }
  .dot.ch { background: var(--accent-strong); }
  .act-stack {
    display: flex; flex-direction: column;
    width: 100%; max-width: 24px; overflow: hidden; border-radius: 2px 2px 0 0;
  }
  .act-stack.off { background: var(--surface-overlay); height: 4px; }
  .act-seg.ep { background: var(--accent); }
  .act-seg.ch { background: var(--accent-strong); }
  .act-dw { color: var(--text-faint); }

  .taste-cols { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--space-5); }
  @media (max-width: 620px) { .taste-cols { grid-template-columns: minmax(0, 1fr); } }
  .taste-t {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-muted);
    margin-bottom: 12px;
  }
  .trow { display: grid; grid-template-columns: 96px 1fr 28px; align-items: center; gap: 10px; margin-bottom: 8px; }
  .trow-l { font-size: var(--fs-caption); color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .trow-track { height: 6px; border-radius: 3px; background: var(--surface-overlay); overflow: hidden; }
  .trow-fill { display: block; height: 100%; background: var(--accent); border-radius: 3px; }
  .trow-n { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-primary); text-align: right; }

  .tracks { display: flex; flex-direction: column; gap: 8px; }
  .track-row {
    display: grid; grid-template-columns: 1fr auto auto; align-items: center; gap: 12px;
    padding: 13px 16px; border-radius: var(--radius-md);
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
  }
  .track-row:hover { border-color: var(--accent); }
  .track-t { font-weight: var(--fw-semibold); }
  .track-m { font-size: var(--fs-caption); color: var(--text-muted); }
  .track-go { color: var(--accent); }
</style>
