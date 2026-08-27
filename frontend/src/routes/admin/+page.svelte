<script lang="ts">
  import { api } from '$lib/api';
  import { authStore as auth } from '$lib/stores/auth';
  import { reltime, waitedFor } from '$lib/reltime';
  import Person from '$lib/components/Person.svelte';
  import type { AdminOverview, Release } from '$shared/types';

  // The pipeline overview: what's
  // stuck, what's where, and what moved recently — across the whole team.
  // Layout guard: admin/moderator only (translators are redirected to the
  // catalog by the layout).
  const canModerate = $derived(
    $auth.isAuthenticated && ['admin', 'moderator'].includes($auth.user?.role ?? '')
  );

  let releases = $state<Release[]>([]);
  let overview = $state<AdminOverview | null>(null);
  let loading = $state(true);

  let loaded = false;
  $effect(() => {
    if (!canModerate || loaded) return;
    loaded = true;
    load();
  });

  async function load() {
    loading = true;
    try {
      const [rels, ov] = await Promise.all([api.getReleases({ all: true }), api.getAdminOverview()]);
      releases = rels.data;
      overview = ov.data;
    } catch {
      releases = [];
    } finally {
      loading = false;
    }
  }

  const day = 24 * 3_600_000;
  const age = (r: Release) => Date.now() - new Date(r.updatedAt).getTime();
  const pct = (r: Release) => (r.totalEvents ? Math.round((r.doneEvents / r.totalEvents) * 100) : 0);
  // the series' real cover; gradient stand-in when the anime has none (or
  // the release only carries a proposed title, pre-import)
  const posterHue = (r: Release) => ((r.animeId ?? r.mangaId ?? r.id * 13) * 47) % 360;
  const poster = (r: Release) =>
    (r.animeImage ?? r.mangaImage)
      ? `background-image: url('${r.animeImage ?? r.mangaImage}'); background-size: cover; background-position: center`
      : `background: linear-gradient(158deg, oklch(0.5 0.08 ${posterHue(r)}) 0%, oklch(0.3 0.06 ${posterHue(r)}) 46%, oklch(0.16 0.03 ${posterHue(r)}) 100%)`;
  const seriesName = (r: Release) => r.animeTitle ?? r.mangaTitle ?? r.proposedTitle ?? '—';
  const shortNum = (r: Release) => (r.medium === 'manga' ? `c${r.chapterNumber}` : `e${r.episodeNumber}`);
  const numLabel = (r: Release) =>
    r.medium === 'manga' ? `capitolul ${r.chapterNumber}` : `episodul ${r.episodeNumber}`;

  const drafting = $derived(releases.filter((r) => r.state === 'draft'));
  const fixing = $derived(releases.filter((r) => r.state === 'changes_requested'));
  const reviewing = $derived(releases.filter((r) => r.state === 'in_review'));
  const publishedWeek = $derived(
    releases.filter((r) => (r.state === 'approved' || r.state === 'published') && age(r) < 7 * day)
  );
  const inPipeline = $derived(drafting.length + fixing.length + reviewing.length);

  // stuck: reviews nobody picked up, drafts nobody touched
  const stuck = $derived([
    ...reviewing
      .filter((r) => age(r) > 3 * day)
      .map((r) => ({
        rel: r,
        pill: `blocat ${waitedFor(r.updatedAt)}`,
        bad: age(r) > 7 * day,
        why: `așteaptă în coada de verificare de ${waitedFor(r.updatedAt)} — niciun verificator nu l-a preluat`,
        href: `/verify/${r.id}`
      })),
    ...[...drafting, ...fixing]
      .filter((r) => age(r) > 7 * day)
      .map((r) => ({
        rel: r,
        pill: 'ciornă veche',
        bad: false,
        why: `la ${pct(r)}% de ${waitedFor(r.updatedAt)} — ${r.uploaderName ?? '—'} nu a mai lucrat la ea`,
        href: `/verify/${r.id}`
      }))
  ]);

  // every stage item links straight into the editor for that release
  const stages = $derived([
    {
      label: 'în traducere',
      items: drafting.map((r) => ({ rel: r, meta: `${pct(r)}%` }))
    },
    {
      label: 'de corectat',
      items: fixing.map((r) => ({ rel: r, meta: `la ${r.uploaderName ?? '—'}` }))
    },
    {
      label: 'la verificare',
      items: reviewing.map((r) => ({ rel: r, meta: waitedFor(r.updatedAt) }))
    },
    {
      label: 'publicate (7 zile)',
      items: publishedWeek.map((r) => ({ rel: r, meta: reltime(r.updatedAt) }))
    }
  ]);

  function activityLine(r: Release): string {
    const what = `${seriesName(r)} · ${shortNum(r)}`;
    switch (r.state) {
      case 'draft':
        return `lucrează la ${what} — ${pct(r)}%`;
      case 'in_review':
        return `a trimis ${what} la verificare`;
      case 'changes_requested':
        return `a primit ${what} înapoi cu note`;
      case 'approved':
        return `are ${what} aprobat${r.reviewerName ? ` de ${r.reviewerName}` : ''}`;
      case 'published':
        return `are ${what} publicat`;
    }
  }
  const activity = $derived(releases.slice(0, 8));
</script>

{#if canModerate}
  <div class="strip" role="list">
    <div class="cell" role="listitem">
      <span class="n">{overview ? overview.animeCount + overview.mangaCount : '—'}</span>
      <span class="l">serii în catalog</span>
    </div>
    <div class="cell" role="listitem">
      <span class="n">{overview?.teamCount ?? '—'}</span>
      <span class="l">membri în echipă</span>
    </div>
    <div class="cell" role="listitem">
      <span class="n" class:warn={stuck.length > 0}>{inPipeline}</span>
      <span class="l">în pipeline acum</span>
    </div>
    <div class="cell" role="listitem">
      <span class="n">{publishedWeek.length}</span>
      <span class="l">publicate (7 zile)</span>
    </div>
  </div>

  {#if loading}
    <p class="muted">Se încarcă…</p>
  {:else}
    <section class="sect">
      <h2 class="s-label">Necesită atenție <span class="s-count">· {stuck.length}</span></h2>
      {#if stuck.length === 0}
        <div class="calm">Nimic blocat — pipeline-ul curge.</div>
      {:else}
        <div class="listcard">
          {#each stuck as s (s.rel.id)}
            <div class="lrow">
              <span class="pill" class:bad={s.bad} class:warn={!s.bad}>{s.pill}</span>
              <span class="l-main">
                <span class="l-title">{seriesName(s.rel)} <span class="dim">— {numLabel(s.rel)}</span></span>
                <span class="l-meta">{s.why}</span>
              </span>
              <a class="btn ghost sm" href={s.href}>Deschide →</a>
            </div>
          {/each}
        </div>
      {/if}
    </section>

    <section class="sect">
      <h2 class="s-label">Tot pipeline-ul, pe etape</h2>
      <div class="stages">
        {#each stages as st (st.label)}
          <div class="stage">
            <div class="st-head">
              <span class="st-label">{st.label}</span>
              <span class="st-n" class:zero={st.items.length === 0}>{st.items.length}</span>
            </div>
            {#each st.items.slice(0, 6) as item (item.rel.id)}
              <a class="st-item" href="/verify/{item.rel.id}" title="Deschide în editor">
                <span class="sti-poster" style={poster(item.rel)}></span>
                <span class="sti-main">
                  <span class="sti-title">{seriesName(item.rel)}</span>
                  <span class="sti-meta">{shortNum(item.rel)} · {item.meta}</span>
                </span>
              </a>
            {:else}
              <span class="st-empty">—</span>
            {/each}
            {#if st.items.length > 6}
              <span class="st-empty">încă {st.items.length - 6}…</span>
            {/if}
          </div>
        {/each}
      </div>
    </section>

    {#if activity.length > 0}
      <section class="sect">
        <h2 class="s-label">Activitate recentă</h2>
        <div class="plain">
          {#each activity as r (r.id)}
            <div class="arow">
              <span class="a-when">{reltime(r.updatedAt)}</span>
              <span class="a-what"><Person name={r.uploaderName} /> {activityLine(r)}</span>
              <a class="a-open" href="/verify/{r.id}">deschide →</a>
            </div>
          {/each}
        </div>
      </section>
    {/if}
  {/if}
{/if}

<style>
  .strip {
    display: grid; grid-template-columns: repeat(4, minmax(0, 1fr));
    margin-bottom: var(--space-6);
  }
  .cell { padding: 0 18px 0 0; }
  .cell + .cell { border-left: 1px solid var(--border-subtle); padding-left: 18px; }
  .n { display: block; font-family: var(--font-display); font-size: 1.7rem; font-weight: var(--fw-semibold); line-height: 1; }
  .n.warn { color: var(--warning); }
  .l {
    display: block; margin-top: 6px;
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-muted);
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

  .calm {
    border: 1px dashed var(--border-default); border-radius: var(--radius-md);
    padding: var(--space-5); text-align: center; color: var(--text-muted); font-size: var(--fs-small);
  }

  .listcard {
    border: 1px solid var(--border-subtle); border-radius: var(--radius-lg);
    background: var(--surface-raised); overflow: hidden;
  }
  .lrow { display: flex; align-items: center; gap: var(--space-4); padding: 14px 18px; }
  .lrow + .lrow { border-top: 1px solid var(--border-subtle); }
  .lrow:hover { background: var(--surface-overlay); }
  .l-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
  .l-title {
    font-family: var(--font-display); font-size: var(--fs-body); font-weight: var(--fw-semibold);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .l-meta { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }

  .pill {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.08em; text-transform: uppercase; white-space: nowrap; flex: 0 0 auto;
    width: 7.5rem;
  }
  .pill.warn { color: var(--warning); }
  .pill.bad { color: var(--danger); }

  .stages {
    display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--space-3);
  }
  @media (max-width: 900px) { .stages { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  .stage {
    border: 1px solid var(--border-subtle); border-radius: var(--radius-lg);
    background: var(--surface-raised); padding: var(--space-3) var(--space-3);
    display: flex; flex-direction: column; gap: 6px;
  }
  .st-head {
    display: flex; align-items: center; justify-content: space-between; gap: 8px;
    padding: 2px 6px 8px; border-bottom: 1px solid var(--border-subtle); margin-bottom: 4px;
  }
  .st-label {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.08em; text-transform: uppercase; color: var(--text-muted);
  }
  .st-n { font-family: var(--font-display); font-size: var(--fs-h3); font-weight: var(--fw-semibold); color: var(--accent); }
  .st-n.zero { color: var(--text-muted); }
  .st-item {
    display: flex; align-items: center; gap: 10px;
    padding: 7px 8px; border-radius: var(--radius-sm);
    color: var(--text-primary); min-width: 0;
    transition: background var(--motion-fast) var(--ease);
  }
  .st-item:hover { background: var(--surface-overlay); }
  .sti-poster { width: 26px; height: 38px; border-radius: 4px; flex: 0 0 auto; }
  .sti-main { min-width: 0; display: flex; flex-direction: column; gap: 2px; }
  .sti-title {
    font-size: var(--fs-caption); font-weight: var(--fw-semibold); line-height: 1.3;
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .sti-meta { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .st-empty {
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
    padding: 7px 8px;
  }

  .arow {
    display: flex; align-items: baseline; gap: 14px;
    padding: 12px 4px; border-bottom: 1px solid var(--border-subtle);
  }
  .a-when { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); flex: 0 0 5.5rem; }
  .a-what { font-size: var(--fs-small); color: var(--text-muted); line-height: 1.5; min-width: 0; flex: 1; }
  .a-open {
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--accent);
    white-space: nowrap; opacity: 0; transition: opacity var(--motion-fast) var(--ease);
  }
  .arow:hover .a-open { opacity: 1; }

  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-caption);
    padding: 8px 14px; border-radius: var(--radius-md); white-space: nowrap; cursor: pointer;
  }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-primary); }
  .btn.ghost:hover { background: var(--surface-overlay); border-color: var(--border-strong); }

  .muted { color: var(--text-muted); }

  @media (max-width: 640px) {
    .strip { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .cell:nth-child(3) { border-left: 0; padding-left: 0; }
  }
</style>
