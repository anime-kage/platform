<script lang="ts">
  import { mediaUrl } from '$lib/media';
  import PosterCard from '$lib/components/PosterCard.svelte';
  // import TestNotice from '$lib/components/TestNotice.svelte';
  import api from '$lib/api';
  import { authStore } from '$lib/stores/auth';
  import { displayName, studioOf, displaySynopsis} from '$lib/types';
  import { reltime } from '$lib/reltime';
  import { markdownExcerpt } from '$lib/markdown';
  import { nameHue } from '$lib/avatar';
  import type { ContinueEntry } from '$shared/types';

  let { data } = $props();
  const spot = $derived(data.spotlight);
  const auth = $derived($authStore);

  // Leaderboard time windows. All three now rank by the same thing — episode
  // views, counted once per member per episode — so they differ only in how far
  // back they look. "Din totdeauna" used to rank by tracker count instead, which
  // meant the tabs were measuring different quantities and could not be compared.
  type LWin = 'today' | 'month' | 'all';
  const BOARD_TABS: { key: LWin; label: string }[] = [
    { key: 'today', label: 'Azi' },
    { key: 'month', label: '30 zile' },
    { key: 'all', label: 'Din totdeauna' }
  ];
  let boardWin = $state<LWin>('month');
  const board = $derived(data.leaderboard[boardWin]);
  const boardIcon = $derived(boardWin === 'all' ? '◉' : '▲');

  // Latest published releases: title object for PosterCard, its link, and the
  // "Ep N" / "Cap N" ribbon.
  const relTitle = (r: (typeof data.latest)[number]) => r.anime ?? r.manga!;
  // Entries flagged `upcoming` are the airing-series fallback shown before the
  // first real publish. They have no episode, so they link to the series and
  // the ribbon names the season rather than inventing an episode number.
  const relHref = (r: (typeof data.latest)[number]) =>
    'upcoming' in r && r.upcoming
      ? `/anime/${r.anime!.id}`
      : r.medium === 'anime'
        ? `/anime/${r.anime!.id}/episode/${r.episodeNumber}`
        : `/manga/${r.manga!.id}/chapter/${Number(r.chapterNumber)}`;
  // Filler cards get no ribbon at all. PosterCard shows the score in its place
  // when the ribbon is empty, which says more than a season label would.
  const relRibbon = (r: (typeof data.latest)[number]) =>
    'upcoming' in r && r.upcoming
      ? ''
      : r.medium === 'anime'
        ? `Ep ${r.episodeNumber}`
        : `Cap ${Number(r.chapterNumber)}`;

  // The server resolves each card to the exact episode and second to open —
  // it reads playback, which the watchlist alone can't tell us. Series with
  // nothing left to play are already filtered out server-side.
  let continueWatching = $state<ContinueEntry[]>([]);

  $effect(() => {
    if (auth.isLoading || !auth.isAuthenticated) return;
    api
      .getContinueWatching(6)
      .then((res) => (continueWatching = res.data))
      .catch(() => {});
  });

  /* Netflix's bar: how far into THIS episode you are, not how far through the
     series. A fresh episode has no bar at all rather than a 0%-wide one. */
  const episodePct = (e: ContinueEntry) =>
    e.durationS && e.durationS > 0 ? Math.min(100, (e.positionS / e.durationS) * 100) : 0;

  const remaining = (e: ContinueEntry) =>
    e.durationS && e.durationS > 0
      ? `${Math.max(1, Math.round((e.durationS - e.positionS) / 60))} min rămase`
      : null;

  // spot is whatever survived `curated ?? highest-scored ?? byScore[0]` in the
  // load, and on a catalog with nothing in it that is undefined — a fresh deploy
  // before the first populate, or any moment the query comes back thin. This
  // used to dereference it straight away and take the whole page down with a
  // 500, which is the worst possible place for it: /home is where a member
  // lands the instant they finish registering.
  /* ── Programme labels ──────────────────────────────────────────────────────
     A slot's `scheduledAt` is an instant, so the day it falls on depends on who
     is looking. Formatting happens here, in the browser, rather than in the
     load: the server has its own timezone and would hand every member the same
     answer, which is how the old MAL-derived strip ended up only able to say
     "23:30 JST". */
  const DAYS_SHORT = ['Dum', 'Lun', 'Mar', 'Mie', 'Joi', 'Vin', 'Sâm'];
  const MONTHS_RO = ['ian', 'feb', 'mar', 'apr', 'mai', 'iun', 'iul', 'aug', 'sep', 'oct', 'nov', 'dec'];

  /** Same calendar day, in the viewer's own timezone. */
  const sameDay = (a: Date, b: Date) =>
    a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();

  const isToday = (iso: string) => sameDay(new Date(iso), new Date());

  const dayLabel = (iso: string) => {
    const d = new Date(iso);
    const now = new Date();
    if (sameDay(d, now)) return 'Azi';
    const tomorrow = new Date(now);
    tomorrow.setDate(now.getDate() + 1);
    if (sameDay(d, tomorrow)) return 'Mâine';
    return DAYS_SHORT[d.getDay()];
  };

  const dateLabel = (iso: string) => {
    const d = new Date(iso);
    return `${d.getDate()} ${MONTHS_RO[d.getMonth()]}`;
  };

  const timeLabel = (iso: string) =>
    new Date(iso).toLocaleTimeString('ro-RO', { hour: '2-digit', minute: '2-digit' });

  const spotMeta = $derived(
    spot
      ? [
          spot.score ? `★ ${spot.score.toFixed(2)}` : null,
          spot.year,
          (spot.type ?? 'TV').toUpperCase(),
          spot.episodes ? `${spot.episodes} EP` : null,
          studioOf(spot)
        ]
          .filter(Boolean)
          .join(' · ')
      : ''
  );
</script>

<!-- One-time "still in testing" popup (see the component for why once). -->
<!-- Commented out for the 1.0 launch: the catalogue has published content now, so the warning is no longer true. Restore this (and the import above) if the site goes back into a testing phase.
<TestNotice scope="home" /> -->

<svelte:head><title>Home · Anime-Kage</title></svelte:head>

<!-- SPOTLIGHT -->
<!-- Nothing to spotlight means no spotlight, not a broken one: with an empty
     catalog the rest of the dashboard (lists, activity, forum) is still worth
     rendering, so the section drops out rather than the page failing. -->
{#if spot}
<section class="container spot-wrap">
  <div class="spot">
    {#if spot.imageUrl}
      <!-- posters are portrait — stretched wide they crop and pixelate, so the
           backdrop is an ambient blur of the same art and the sharp poster sits
           at its native aspect on the right -->
      <div class="spot-bg" style={`background-image:url(${mediaUrl(spot.imageUrl)})`}></div>
    {/if}
    <div class="spot-fade"></div>
    <div class="spot-body">
      <p class="spot-kicker">În centrul atenției</p>
      <h1 class="spot-title">{displayName(spot)}</h1>
      <p class="spot-meta">{spotMeta}</p>
      {#if displaySynopsis(spot)}<p class="spot-syn">{displaySynopsis(spot)}</p>{/if}
      <div class="spot-actions">
        <a class="btn fill" href={`/anime/${spot.id}/episode/1`}>▶ Vizionează</a>
        <a class="btn ghost" href={`/anime/${spot.id}`}>Detalii</a>
      </div>
    </div>
    {#if spot.imageUrl}
      <a class="spot-poster" href={`/anime/${spot.id}`} aria-hidden="true" tabindex="-1">
        <img src={mediaUrl(spot.imageUrl)} alt="" loading="eager" />
      </a>
    {/if}
  </div>
</section>
{/if}

<!-- CONTINUĂ VIZIONAREA -->
{#if continueWatching.length}
  <section class="container block">
    <div class="head">
      <h2>Continuă vizionarea</h2>
      <span class="kicker">{continueWatching.length} titluri</span>
    </div>
    <div class="row">
      {#each continueWatching as e (e.episodeId)}
        {@const a = e.anime}
        {@const pct = episodePct(e)}
        {@const left = remaining(e)}
        <a class="cw" href={`/anime/${a.id}/episode/${e.episodeNumber}`}>
          <span class="cw-media">
            {#if a.imageUrl}
              <!-- Two layers of the same poster: a blurred, cropped copy fills
                   the wide frame, the sharp one sits on top uncropped. MAL
                   only gives us a 2:3 portrait, so a 16:9 card can either cut
                   ~70% of it off or do this. -->
              <span class="cw-blur" style={`background-image:url(${mediaUrl(a.imageUrl)})`}></span>
              <img class="cw-poster media-tone" src={mediaUrl(a.imageUrl)} alt="" loading="lazy" />
            {/if}
            <span class="cw-play"><span>▶</span></span>
            {#if pct > 0}
              <span class="cw-track">
                <span class="cw-fill" style={`width:${pct}%`}></span>
              </span>
            {/if}
          </span>
          <span class="cw-body">
            <span class="cw-t">{displayName(a)}</span>
            <span class="cw-m">
              Ep {e.episodeNumber} / {e.availableEpisodes}{#if left}<span class="cw-left"
                  >{' · '}{left}</span
                >{/if}
            </span>
          </span>
        </a>
      {/each}
    </div>
  </section>
{/if}

<!-- ULTIMELE LANSĂRI -->
{#if data.latest.length}
  <section class="container block">
    <div class="head">
      <h2>Ultimele lansări</h2>
      <a class="mono-link" href="/anime">Tot catalogul →</a>
    </div>
    <div class="row">
      {#each data.latest as r (`${r.medium}-${r.id}`)}
        <div class="cell"><PosterCard a={relTitle(r)} href={relHref(r)} ribbon={relRibbon(r)} /></div>
      {/each}
    </div>
  </section>
{/if}

<!-- COLECȚII ALE COMUNITĂȚII + CELE MAI VIZIONATE -->
<section class="container block split">
  <div>
    <div class="head">
      <h2>Colecții ale comunității</h2>
      <a class="mono-link" href="/liste">Toate listele →</a>
    </div>
    {#if data.collections.length}
      <div class="collections">
        {#each data.collections as c (c.href)}
          <a class="coll" href={c.href}>
            <span class="coll-covers">
              {#each c.covers as cv}
                <span class="coll-cover media-tone" style={`background-image:url(${mediaUrl(cv)})`}></span>
              {/each}
            </span>
            <span class="coll-body">
              <span class="coll-title">{c.title}</span>
              <span class="coll-meta">{c.count} titluri · de {c.curator}</span>
            </span>
          </a>
        {/each}
      </div>
    {:else}
      <p class="strip-empty">
        Nu există încă nicio colecție. <a href="/liste">Fă tu prima listă</a> și apare aici.
      </p>
    {/if}
  </div>

  <aside class="panel">
    <div class="panel-head">
      <span class="panel-title">Clasament</span>
      <!-- Views in every window now — the all-time tab used to say "urmăritori"
           because it ranked by tracker count instead. -->
      <span class="kicker">vizionări</span>
    </div>
    <div class="board-tabs" role="tablist" aria-label="Interval clasament">
      {#each BOARD_TABS as t (t.key)}
        <button
          class="board-tab"
          class:on={boardWin === t.key}
          role="tab"
          aria-selected={boardWin === t.key}
          onclick={() => (boardWin = t.key)}
        >
          {t.label}
        </button>
      {/each}
    </div>
    {#if board.length}
      {#each board as a, i (a.id)}
        <a class="rank" href={`/anime/${a.id}`}>
          <span class="rank-n" class:top={i === 0}>{i + 1}</span>
          <span class="rank-thumb media-tone" style={`background-image:url(${mediaUrl(a.imageUrl)})`}></span>
          <span class="rank-main">
            <span class="rank-t">{displayName(a)}</span>
            <span class="rank-m">{a.year ?? '—'} · {(a.type ?? 'TV').toUpperCase()}</span>
          </span>
          <span class="rank-s">{boardIcon} {a.points}</span>
        </a>
      {/each}
    {:else}
      <p class="board-empty">
        {boardWin === 'today'
          ? 'Niciun episod vizionat azi încă.'
          : boardWin === 'month'
            ? 'Niciun episod vizionat în ultimele 30 de zile.'
            : 'Niciun episod vizionat încă.'}
      </p>
    {/if}
  </aside>
</section>

<!-- ACTIVITATE / FORUM / ȘTIRI -->
<section class="container block pulse">
  <div class="pulse-col">
    <p class="pulse-head kicker with-link">
      <span>Activitate în comunitate</span>
      <a class="mono-link" href="/comunitate?tab=activitate">Tot →</a>
    </p>
    {#each data.activity as ev (ev.type + ev.user.id + ev.target + ev.createdAt)}
      <div class="act">
        {#if ev.user.avatarUrl}
          <a class="act-avatar" href={`/user/${ev.user.username}`}>
            <img src={api.resolveUrl(ev.user.avatarUrl)} alt={ev.user.username} />
          </a>
        {:else}
          <a
            class="act-avatar monogram"
            href={`/user/${ev.user.username}`}
            style={`--mg-hue:${nameHue(ev.user.username)}`}
          >
            {ev.user.username.charAt(0).toUpperCase()}
          </a>
        {/if}
        <span class="act-body">
          <span>
            <a class="plink" href={`/user/${ev.user.username}`}><strong>{ev.user.username}</strong></a>
            {ev.verb}
            {#if ev.link}<a class="act-tgt" href={ev.link}>{ev.target}</a>{:else}<strong>{ev.target}</strong>{/if}
            {#if ev.meta}<span class="act-meta">{ev.meta}</span>{/if}
          </span>
          <span class="act-time">{reltime(ev.createdAt)}</span>
        </span>
      </div>
    {:else}
      <p class="strip-empty">
        Încă nu s-a întâmplat nimic. Notează un anime din <a href="/anime">catalog</a> și
        vei fi primul aici.
      </p>
    {/each}
  </div>

  <div class="pulse-col">
    <p class="pulse-head kicker with-link">
      <span>Pe forum</span>
      <a class="mono-link" href="/comunitate?tab=forum">Tot →</a>
    </p>
    {#each data.threads as t (t.id)}
      <a class="topic" href={`/comunitate/forum/${t.id}`}>
        <span class="topic-t">
          {#if t.isPinned}<span class="topic-pin" title="Fixat">★</span>{/if}{t.title}
        </span>
        <span class="topic-m">
          <span class="topic-cat">{t.category}</span>
          <span class="topic-r">
            {t.replyCount}
            {t.replyCount === 1 ? 'răspuns' : 'răspunsuri'} · activ {reltime(t.lastActivityAt)}
          </span>
        </span>
      </a>
    {:else}
      <p class="strip-empty">
        Nimeni n-a deschis încă un subiect.
        <a href="/comunitate?tab=forum">Deschide tu primul</a>.
      </p>
    {/each}
  </div>

  <div class="pulse-col">
    <p class="pulse-head kicker with-link">
      <span>Știri & anunțuri</span>
      <a class="mono-link" href="/anunturi">Toate →</a>
    </p>
    {#each data.news as n (n.id)}
      <!-- Every card now leads to the post's own page. `n.url` stays as an
           optional extra destination *inside* the post, not as the card's. -->
      <a class="news linked" href={`/anunturi/${n.slug ?? n.id}`}>
        <span class="news-m">
          <span class="news-tag">{n.tag}</span>
          <span class="news-t">{reltime(n.createdAt)}</span>
        </span>
        <span class="news-title">{n.title}</span>
        {#if n.body}<span class="news-body">{markdownExcerpt(n.body, 120)}</span>{/if}
      </a>
    {:else}
      <p class="strip-empty">Niciun anunț deocamdată. Când echipa are ceva de spus, apare aici.</p>
    {/each}
  </div>
</section>

<!-- PROGRAMUL SĂPTĂMÂNII -->
<section class="container block">
  <div class="head">
    <h2>Programul săptămânii</h2>
    <a class="mono-link" href="/calendar">Tot calendarul →</a>
  </div>
  {#if data.schedule.length}
    <div class="sched">
      {#each data.schedule as s (s.id)}
        <!-- Links to the episode once it exists, to the series until then:
             a slot is a promise about the future, so most of the time there is
             nothing to play yet. -->
        <a
          class="slot"
          href={s.published
            ? `/anime/${s.animeId}/episode/${s.episodeNumber}`
            : `/anime/${s.animeId}`}
        >
          <span class="slot-date">
            <span class="slot-day" class:today={isToday(s.scheduledAt)}>{dayLabel(s.scheduledAt)}</span>
            <span class="slot-d">{dateLabel(s.scheduledAt)}</span>
          </span>
          <span class="slot-main">
            <span class="slot-t">{displayName(s)}</span>
            <span class="slot-e">
              Episodul {s.episodeNumber} · {timeLabel(s.scheduledAt)}
              {#if s.note}<span class="slot-note">{s.note}</span>{/if}
            </span>
          </span>
        </a>
      {/each}
    </div>
  {:else}
    <p class="strip-empty wide">
      Nu e programat încă nimic pentru săptămâna asta. Echipa anunță aici
      episoadele pe măsură ce intră la tradus.
    </p>
  {/if}
</section>

<style>
  .block { padding-top: var(--space-7); }
  .block:last-child { padding-bottom: var(--space-7); }

  .head {
    display: flex; align-items: baseline; justify-content: space-between;
    gap: var(--space-4); margin-bottom: var(--space-4);
  }
  .head h2 { font-size: var(--fs-h2); }
  .mono-link { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .mono-link:hover { color: var(--accent); }

  /* ---- spotlight ---- */
  .spot-wrap { padding-top: var(--space-5); }
  .spot {
    position: relative; overflow: hidden; display: flex; align-items: flex-end;
    min-height: 340px; border: 1px solid var(--border-subtle); border-radius: var(--radius-xl);
  }
  /* ambient blur: oversized + blurred so source resolution/crop never shows */
  .spot-bg {
    position: absolute; inset: -40px;
    background-size: cover; background-position: center 30%;
    filter: blur(34px) saturate(0.9) brightness(0.62);
    transform: scale(1.12);
  }
  .spot-fade {
    position: absolute; inset: 0;
    background: linear-gradient(
      90deg,
      color-mix(in srgb, var(--surface-base) 80%, transparent),
      transparent 65%
    ),
    linear-gradient(to top, color-mix(in srgb, var(--surface-base) 55%, transparent), transparent 60%);
  }
  .spot-body { position: relative; padding: clamp(var(--space-5), 4vw, 40px); max-width: 560px; }
  .spot-poster {
    position: relative; flex: 0 0 auto;
    align-self: center; margin-left: auto;
    margin-right: clamp(var(--space-5), 4vw, 48px);
    padding: var(--space-5) 0;
  }
  .spot-poster img {
    display: block; width: clamp(150px, 16vw, 200px); aspect-ratio: 2 / 3;
    object-fit: cover; border-radius: var(--radius-lg);
    border: 1px solid rgba(255, 255, 255, 0.14);
    box-shadow: 0 18px 44px rgba(0, 0, 0, 0.45);
  }
  .spot-kicker { font-size: var(--fs-caption); font-weight: var(--fw-bold); color: var(--accent); }
  .spot-title { font-size: var(--fs-h1); line-height: 1.08; letter-spacing: -0.01em; margin-top: 14px; }
  .spot-meta { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); margin-top: 10px; }
  .spot-syn {
    font-size: var(--fs-body); line-height: 1.6; color: var(--text-muted);
    margin: 14px 0 22px; max-width: 480px;
    display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3;
    -webkit-box-orient: vertical; overflow: hidden;
  }
  .spot-actions { display: flex; gap: 11px; flex-wrap: wrap; }
  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 12px 22px; border-radius: var(--radius-md); white-space: nowrap;
  }
  .btn.fill { background: var(--accent); color: var(--on-accent); }
  .btn.fill:hover { background: var(--accent-hover); color: var(--on-accent); }
  .btn.ghost {
    border: 1px solid var(--border-default); color: var(--text-primary);
    background: color-mix(in srgb, var(--surface-base) 50%, transparent);
  }
  .btn.ghost:hover { background: var(--surface-raised); color: var(--text-primary); }

  /* ---- horizontal rows ---- */
  .row {
    display: flex; gap: 15px; overflow-x: auto;
    padding-bottom: 10px; scroll-snap-type: x proximity;
  }
  /* ---- latest releases (poster cards) ---- */
  .cell { flex: 0 0 160px; scroll-snap-align: start; }

  /* ---- continue watching ---- */
  .cw {
    flex: 0 0 252px; scroll-snap-align: start; display: block;
    border: 1px solid var(--border-subtle); border-radius: 12px; overflow: hidden;
    background: var(--surface-raised);
    transition: border-color var(--motion-base) var(--ease);
  }
  .cw:hover { border-color: var(--border-default); }
  .cw-media {
    position: relative; display: block; aspect-ratio: 16 / 9;
    background: var(--surface-overlay); overflow: hidden;
  }
  .cw-blur {
    position: absolute; inset: 0;
    background-size: cover; background-position: center;
    filter: blur(18px) saturate(1.15) brightness(0.55);
    /* the blur samples past the edges, so overscale to keep them opaque */
    transform: scale(1.25);
  }
  .cw-poster {
    position: absolute; inset: 0; margin: auto;
    height: 100%; width: auto; max-width: 100%;
    object-fit: contain; display: block;
  }
  .cw-play {
    position: absolute; inset: 0; display: grid; place-items: center;
    background: rgba(8, 10, 12, 0.25);
  }
  .cw-play span {
    width: 46px; height: 46px; border-radius: 50%;
    background: rgba(15, 20, 25, 0.6); backdrop-filter: blur(4px);
    display: grid; place-items: center; color: #fff; font-size: 0.9375rem; padding-left: 3px;
  }
  /* the resume bar: position within THIS episode, Netflix-style */
  .cw-track {
    position: absolute; left: 0; right: 0; bottom: 0; height: 4px;
    background: rgba(255, 255, 255, 0.22);
  }
  .cw-fill { display: block; height: 100%; background: var(--accent); }
  .cw-left { color: var(--text-faint); margin-left: 4px; }
  .cw-body { display: block; padding: 11px 13px 13px; }
  .cw-t {
    display: block; font-family: var(--font-display); font-size: 0.90625rem;
    font-weight: var(--fw-semibold); line-height: 1.2; color: var(--text-primary);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .cw-m { display: block; font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); margin-top: 5px; }

  /* ---- collections + sidebar ---- */
  .split {
    display: grid; grid-template-columns: 1fr 340px;
    gap: var(--space-6); align-items: start;
  }
  .collections { display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-4); }
  .coll {
    display: block; overflow: hidden; cursor: pointer;
    border: 1px solid var(--border-subtle); border-radius: var(--radius-lg);
    background: var(--surface-raised);
    transition: border-color var(--motion-base) var(--ease);
  }
  .coll:hover { border-color: var(--border-default); }
  .coll-covers { display: flex; height: 104px; }
  .coll-cover { flex: 1; background-size: cover; background-position: center 20%; }
  .coll-body { display: block; padding: 14px 15px 15px; }
  .coll-title {
    display: block; font-family: var(--font-display);
    font-size: var(--fs-h3); font-weight: var(--fw-semibold);
    line-height: 1.2; color: var(--text-primary);
  }
  .coll-meta {
    display: block; margin-top: 9px;
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
  }

  .panel {
    border: 1px solid var(--border-subtle); border-radius: var(--radius-lg);
    background: var(--surface-raised); overflow: hidden;
  }
  .panel-head {
    display: flex; align-items: center; justify-content: space-between;
    padding: 14px 16px; border-bottom: 1px solid var(--border-subtle);
  }
  .panel-title { font-family: var(--font-display); font-weight: var(--fw-semibold); font-size: var(--fs-body); }
  .board-tabs { display: flex; border-bottom: 1px solid var(--border-subtle); }
  .board-tab {
    flex: 1; font-size: var(--fs-caption); font-weight: var(--fw-semibold);
    color: var(--text-muted); background: none; border: none; cursor: pointer;
    padding: 10px 6px; border-bottom: 2px solid transparent; margin-bottom: -1px;
    white-space: nowrap; transition: color var(--motion-fast) var(--ease);
  }
  .board-tab:hover { color: var(--text-primary); }
  .board-tab.on { color: var(--text-primary); border-bottom-color: var(--accent); }
  .board-empty {
    padding: 22px 16px; font-size: var(--fs-small); color: var(--text-muted); text-align: center;
  }
  .rank-n.top { color: var(--accent); }
  .rank {
    display: flex; align-items: center; gap: 12px;
    padding: 9px 16px; border-bottom: 1px solid var(--border-subtle);
  }
  .rank:last-child { border-bottom: none; }
  .rank:hover { background: var(--surface-overlay); }
  .rank-n {
    font-family: var(--font-display); font-size: 1.0625rem; font-weight: var(--fw-semibold);
    color: var(--text-muted); width: 22px; flex: 0 0 auto;
  }
  .rank-thumb {
    width: 30px; height: 44px; border-radius: 5px; flex: 0 0 auto;
    background-size: cover; background-position: center;
  }
  .rank-main { flex: 1; min-width: 0; display: flex; flex-direction: column; }
  .rank-t {
    font-size: var(--fs-small); font-weight: var(--fw-semibold); color: var(--text-primary);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .rank-m { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); margin-top: 2px; }
  .rank-s {
    font-family: var(--font-mono); font-size: var(--fs-caption);
    font-weight: var(--fw-medium); color: var(--accent); white-space: nowrap;
  }

  /* ---- community pulse strip ---- */
  .pulse {
    display: grid; grid-template-columns: 1.25fr 1fr 1fr;
    gap: 38px; align-items: start;
  }
  .pulse-head {
    display: block; padding-bottom: 12px;
    border-bottom: 1px solid var(--border-default);
  }
  .pulse-head.with-link { display: flex; align-items: center; justify-content: space-between; }

  /* Shared empty state for every strip that can legitimately have nothing in
     it — a fresh install, a week between seasons, a forum nobody has posted
     in yet. Says what is missing and how to fill it. */
  .strip-empty {
    padding: 16px 0 4px;
    font-size: 0.8125rem; line-height: 1.55; color: var(--text-muted); text-wrap: pretty;
  }
  .strip-empty.wide {
    padding: 18px 0; border-top: 2px solid var(--border-default); font-size: var(--fs-small);
  }
  .strip-empty a { color: var(--text-primary); text-decoration: underline; text-underline-offset: 3px; }
  .strip-empty a:hover { color: var(--accent); }

  .act {
    display: flex; gap: 11px; align-items: flex-start;
    padding: 13px 0; border-bottom: 1px solid var(--border-subtle);
  }
  .act-avatar {
    flex: 0 0 30px; height: 30px; border-radius: 50%; overflow: hidden;
    display: grid; place-items: center;
    font-size: 0.75rem; font-weight: var(--fw-bold); color: #fff;
  }
  .act-avatar img { width: 100%; height: 100%; object-fit: cover; display: block; }
  a.act-avatar:hover { color: #fff; }
  .act-tgt { font-family: var(--font-display); font-style: italic; color: var(--text-primary); }
  a.act-tgt:hover { color: var(--accent); }
  .plink { color: inherit; }
  .plink:hover { color: var(--accent); }
  .act-body {
    flex: 1; min-width: 0; display: flex; flex-direction: column;
    font-size: 0.8125rem; line-height: 1.45; color: var(--text-muted);
  }
  .act-body strong { color: var(--text-primary); font-weight: var(--fw-semibold); }
  .act-meta { color: var(--accent); font-family: var(--font-mono); font-size: var(--fs-micro); }
  .act-time { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); margin-top: 3px; }

  .topic { display: block; padding: 13px 0; border-bottom: 1px solid var(--border-subtle); }
  .topic:hover .topic-t { color: var(--accent); }
  .topic-t {
    display: block; font-size: 0.84375rem; font-weight: var(--fw-semibold);
    line-height: 1.35; color: var(--text-primary);
    transition: color var(--motion-fast) var(--ease);
  }
  .topic-pin { color: var(--accent); margin-right: 5px; }
  .topic-m { display: flex; align-items: center; gap: 9px; margin-top: 6px; font-family: var(--font-mono); font-size: var(--fs-micro); }
  .topic-cat { letter-spacing: 0.08em; text-transform: uppercase; color: var(--accent); flex: 0 0 auto; }
  .topic-r { color: var(--text-muted); min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .news { display: block; padding: 13px 0; border-bottom: 1px solid var(--border-subtle); }
  .news-m { display: flex; align-items: center; gap: 9px; margin-bottom: 6px; font-family: var(--font-mono); font-size: var(--fs-micro); }
  .news-tag { letter-spacing: 0.08em; text-transform: uppercase; color: var(--accent); }
  .news-t { color: var(--text-muted); }
  .news-title { display: block; font-size: 0.84375rem; font-weight: var(--fw-semibold); line-height: 1.35; color: var(--text-primary); }
  .news.linked:hover .news-title { color: var(--accent); }
  .news-body {
    display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2;
    -webkit-box-orient: vertical; overflow: hidden;
    margin-top: 5px; font-size: 0.8125rem; line-height: 1.45; color: var(--text-muted);
  }

  /* ---- schedule (dates only, titles wrap to two lines) ---- */
  .sched { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--space-4) var(--space-6); }
  .slot {
    display: flex; align-items: flex-start; gap: 16px;
    padding-top: 14px; border-top: 2px solid var(--border-default);
  }
  .slot-date { display: flex; flex-direction: column; flex: 0 0 auto; min-width: 3.2em; }
  .slot-day {
    font-family: var(--font-display); font-size: 1.1875rem;
    font-weight: var(--fw-semibold); color: var(--text-primary); line-height: 1.1;
  }
  .slot-day.today { color: var(--accent); }
  .slot-d {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.08em; text-transform: uppercase;
    color: var(--text-muted); margin-top: 3px;
  }
  .slot-main { flex: 1; min-width: 0; display: flex; flex-direction: column; }
  .slot-t {
    font-size: var(--fs-body); font-weight: var(--fw-semibold); color: var(--text-primary);
    line-height: 1.35; text-wrap: pretty;
    display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2;
    -webkit-box-orient: vertical; overflow: hidden;
  }
  .slot:hover .slot-t { color: var(--accent); }
  .slot-e { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); margin-top: 4px; }
  .slot-note { display: block; color: var(--accent); margin-top: 3px; }

  /* ---- responsive ---- */
  @media (max-width: 960px) {
    .split { grid-template-columns: minmax(0, 1fr); }
    .sched { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .pulse { grid-template-columns: minmax(0, 1fr); gap: var(--space-5); }
  }
  @media (max-width: 620px) {
    .collections { grid-template-columns: minmax(0, 1fr); }
    .sched { grid-template-columns: minmax(0, 1fr); }
    .spot { min-height: 300px; }
    .spot-poster { display: none; }
  }
</style>
