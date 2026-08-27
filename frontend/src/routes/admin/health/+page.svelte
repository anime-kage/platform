<script lang="ts">
  import { onMount } from 'svelte';
  import api from '$lib/api';
  import { toast } from '$lib/stores/toast';
  import type { AdminEpisodeGap, AdminHealthReport } from '$shared/types';

  // Health dashboard: what the nightly checker found, and the
  // gaps a coordinator should fill next.
  let health = $state<AdminHealthReport | null>(null);
  let loading = $state(true);

  // Disk on the box that serves the API. Staging is the only thing on it that
  // grows without a ceiling, so it gets its own number: running out of room
  // for uploads should never be a surprise.
  let disk = $state<{
    stagingBytes: number;
    stagingDirs: number;
    diskTotalBytes: number;
    diskFreeBytes: number;
  } | null>(null);

  onMount(async () => {
    api
      .getAdminStorage()
      .then((r) => (disk = r.data))
      .catch(() => (disk = null)); // a missing stat must not break the report
    try {
      health = (await api.getAdminHealthReport()).data;
    } catch {
      toast.error('Raportul de sănătate nu a putut fi încărcat.');
    } finally {
      loading = false;
    }
  });

  const gb = (b: number) => `${(b / 1_000_000_000).toFixed(1)} GB`;
  const usedPct = $derived(
    disk && disk.diskTotalBytes > 0
      ? Math.round(((disk.diskTotalBytes - disk.diskFreeBytes) / disk.diskTotalBytes) * 100)
      : 0
  );

  // The two gap lists run to thousands of rows on a fresh catalog, so the
  // report ships a preview and everything past it is fetched a page at a time.
  const PREVIEW = 6;
  const PAGE = 50;

  type GapKind = 'source' | 'rosub';
  type GapView = { open: boolean; offset: number; episodes: AdminEpisodeGap[]; busy: boolean };
  const gaps = $state<Record<GapKind, GapView>>({
    source: { open: false, offset: 0, episodes: [], busy: false },
    rosub: { open: false, offset: 0, episodes: [], busy: false }
  });

  const gapTotal = (k: GapKind) =>
    k === 'source' ? (health?.missingSource.total ?? 0) : (health?.missingRoSub.total ?? 0);

  /** Rows on screen: the report's preview while collapsed, the fetched page after. */
  function shown(k: GapKind): AdminEpisodeGap[] {
    const g = gaps[k];
    if (g.open) return g.episodes;
    const src = k === 'source' ? health?.missingSource.episodes : health?.missingRoSub.episodes;
    return (src ?? []).slice(0, PREVIEW);
  }

  async function loadGap(k: GapKind, offset: number) {
    const g = gaps[k];
    g.busy = true;
    try {
      const r = await api.getAdminHealthGaps(k, { limit: PAGE, offset });
      g.episodes = r.data.episodes;
      g.offset = offset;
      g.open = true;
    } catch {
      toast.error('Lista nu a putut fi încărcată.');
    } finally {
      g.busy = false;
    }
  }

  function collapse(k: GapKind) {
    gaps[k].open = false;
    gaps[k].offset = 0;
    gaps[k].episodes = [];
  }

  const fmtChecked = (ts?: string) => (ts ? new Date(ts).toLocaleString('ro-RO') : 'niciodată');
</script>

{#if disk && disk.diskTotalBytes > 0}
  <div class="disk" class:low={disk.diskFreeBytes < disk.diskTotalBytes * 0.1}>
    <span class="d-label">Stocare</span>
    <span class="d-bar" aria-hidden="true"><span class="d-fill" style="width:{usedPct}%"></span></span>
    <span class="d-num">{gb(disk.diskTotalBytes - disk.diskFreeBytes)} / {gb(disk.diskTotalBytes)}</span>
    <span class="d-free">{gb(disk.diskFreeBytes)} liberi</span>
    <span class="d-stg">
      staging {gb(disk.stagingBytes)} · {disk.stagingDirs}
      {disk.stagingDirs === 1 ? 'release' : 'release-uri'}
    </span>
  </div>
{/if}

{#if loading}
  <p class="muted">Se încarcă raportul…</p>
{:else if health}
  <div class="stats">
    <div class="stat" class:bad={health.deadSources.length > 0}>
      <span class="stat-n">{health.deadSources.length}</span>
      <span class="stat-l">surse moarte</span>
    </div>
    <div class="stat" class:bad={health.missingSource.total > 0}>
      <span class="stat-n">{health.missingSource.total}</span>
      <span class="stat-l">episoade fără sursă</span>
    </div>
    <div class="stat" class:warn={health.missingRoSub.total > 0}>
      <span class="stat-n">{health.missingRoSub.total}</span>
      <span class="stat-l">episoade fără sub RO</span>
    </div>
  </div>

  <section class="block">
    <span class="kicker">Surse moarte</span>
    {#if health.deadSources.length === 0}
      <div class="empty"><p class="muted">Nicio sursă activă nu a picat ultima verificare.</p></div>
    {:else}
      <ul class="list">
        {#each health.deadSources as d (d.id)}
          <li>
            <a class="row" href={`/anime/${d.animeId}/episode/${d.episodeNumber}`}>
              <span class="row-main">
                <span class="row-t">{d.animeTitle} <span class="dim">— ep. {d.episodeNumber}</span></span>
                <span class="row-m">{d.provider ?? d.kind} · verificat {fmtChecked(d.lastCheckedAt)}</span>
              </span>
              <span class="pill dead">moartă</span>
            </a>
          </li>
        {/each}
      </ul>
      <p class="hint">Repar-o din pagina titlului (Catalog → titlu → Surse) sau șterge-o.</p>
    {/if}
  </section>

{#snippet gapSection(kind: GapKind, title: string, emptyText: string)}
    <section class="block">
      <span class="kicker">{title}</span>
      {#if gapTotal(kind) === 0}
        <div class="empty"><p class="muted">{emptyText}</p></div>
      {:else}
        <ul class="list" class:dim-list={gaps[kind].busy}>
          {#each shown(kind) as e (e.episodeId)}
            <li>
              <a class="row" href={`/anime/${e.animeId}/episode/${e.episodeNumber}`}>
                <span class="row-main">
                  <span class="row-t">{e.animeTitle} <span class="dim">— ep. {e.episodeNumber}</span></span>
                </span>
              </a>
            </li>
          {/each}
        </ul>

        {#if !gaps[kind].open}
          {#if gapTotal(kind) > PREVIEW}
            <div class="more">
              <button class="btn ghost sm" disabled={gaps[kind].busy} onclick={() => loadGap(kind, 0)}>
                {gaps[kind].busy ? 'Se încarcă…' : `Vezi toate (${gapTotal(kind)})`}
              </button>
            </div>
          {/if}
        {:else}
          <div class="more">
            <span class="pg-info">
              {gaps[kind].offset + 1}–{Math.min(gaps[kind].offset + gaps[kind].episodes.length, gapTotal(kind))}
              din {gapTotal(kind)}
            </span>
            <span class="pg-sp"></span>
            <button
              class="btn ghost sm"
              disabled={gaps[kind].busy || gaps[kind].offset === 0}
              onclick={() => loadGap(kind, Math.max(0, gaps[kind].offset - PAGE))}>← Înapoi</button
            >
            <button
              class="btn ghost sm"
              disabled={gaps[kind].busy || gaps[kind].offset + PAGE >= gapTotal(kind)}
              onclick={() => loadGap(kind, gaps[kind].offset + PAGE)}>Înainte →</button
            >
            <button class="btn ghost sm" onclick={() => collapse(kind)}>Restrânge</button>
          </div>
        {/if}
      {/if}
    </section>
  {/snippet}

  {@render gapSection(
    'source',
    'Episoade fără sursă funcțională',
    'Toate episoadele au cel puțin o sursă care răspunde.'
  )}

{@render gapSection(
    'rosub',
    'Episoade fără subtitrare română',
    'Fiecare episod are o subtitrare română publicată.'
  )}
{/if}

<style>
  .more {
    display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
    margin-top: var(--space-4);
  }
  .pg-sp { flex: 1; }
  .pg-info {
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
  }
  .dim-list { opacity: 0.5; transition: opacity var(--motion-fast) var(--ease); }
  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 10px 18px; border-radius: var(--radius-md); white-space: nowrap; cursor: pointer;
  }
  .btn.sm { padding: 7px 13px; font-size: var(--fs-caption); }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-primary); }
  .btn.ghost:hover:not(:disabled) { background: var(--surface-overlay); border-color: var(--border-strong); }
  .btn:disabled { opacity: 0.45; cursor: default; }

  .disk {
    display: flex; align-items: center; gap: var(--space-3); flex-wrap: wrap;
    margin-bottom: var(--space-5); padding: 9px 14px;
    border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
    background: var(--surface-raised);
    font-family: var(--font-mono); font-size: var(--fs-micro);
  }
  .d-label { letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-muted); }
  .d-bar {
    flex: 1; min-width: 90px; max-width: 200px; height: 5px;
    background: var(--surface-inset); border-radius: 3px; overflow: hidden;
  }
  .d-fill { display: block; height: 100%; background: var(--accent); }
  .disk.low .d-fill { background: var(--danger); }
  .d-num { color: var(--text-primary); font-weight: var(--fw-semibold); }
  .d-free { color: var(--text-muted); }
  .disk.low .d-free { color: var(--danger); font-weight: var(--fw-semibold); }
  .d-stg { color: var(--text-muted); margin-left: auto; }
  @media (max-width: 700px) { .d-stg { margin-left: 0; } }

  .stats {
    display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: var(--space-3); margin-bottom: var(--space-6);
  }
  .stat {
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg); padding: var(--space-4) var(--space-5);
    display: flex; flex-direction: column; gap: 4px;
  }
  .stat-n { font-family: var(--font-display); font-size: 2rem; font-weight: var(--fw-semibold); line-height: 1; }
  .stat-l {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-muted);
  }
  .stat.bad .stat-n { color: var(--danger); }
  .stat.warn .stat-n { color: var(--warning); }

  .block { margin-bottom: var(--space-6); }
  .block > .kicker { display: block; margin-bottom: var(--space-3); }

  .list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 6px; }
  .row {
    display: flex; align-items: center; gap: var(--space-4);
    padding: 10px 14px; border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
    background: var(--surface-raised); color: var(--text-primary); min-width: 0;
    transition: border-color var(--motion-fast) var(--ease), background var(--motion-fast) var(--ease);
  }
  .row:hover { border-color: var(--border-strong); background: var(--surface-overlay); }
  .row-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
  .row-t { font-size: var(--fs-small); font-weight: var(--fw-semibold); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .dim { color: var(--text-muted); font-weight: var(--fw-regular); }
  .row-m { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }

  .pill {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.06em; text-transform: uppercase;
    padding: 3px 10px; border-radius: var(--radius-pill); white-space: nowrap;
  }
  .pill.dead { background: color-mix(in srgb, var(--danger) 14%, transparent); color: var(--danger); }

  .empty {
    border: 1px dashed var(--border-default); border-radius: var(--radius-md);
    padding: var(--space-4); text-align: center;
  }
  .muted { color: var(--text-muted); }
  .hint { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); margin-top: var(--space-3); }
</style>
