<script lang="ts">
  import { goto } from '$app/navigation';
  import { api } from '$lib/api';
  import { authStore as auth } from '$lib/stores/auth';
  import Person from '$lib/components/Person.svelte';
  import { reltime, waitedFor } from '$lib/reltime';
  import { toast } from '$lib/stores/toast';
  import type { Release, VerifierOption } from '$shared/types';

  // The review gate: the queue in
  // order of age — oldest first, as the next-up card — plus what the team
  // decided recently. Small fixes happen in the editor; this page only routes.

  let releases = $state<Release[]>([]);
  let loading = $state(true);

  const allowed = $derived(
    $auth.isAuthenticated && ['verifier', 'coordinator', 'moderator', 'admin'].includes($auth.user?.role ?? '')
  );
  // verifiers live in their own queue (routed to them + unrouted); the
  // pipeline-wide view is the coordinator/moderator/admin expander
  const canSeeAll = $derived(['coordinator', 'moderator', 'admin'].includes($auth.user?.role ?? ''));
  const canReassign = $derived(['coordinator', 'admin'].includes($auth.user?.role ?? ''));
  let showAll = $state(false);
  let verifiers = $state<VerifierOption[]>([]);
  const roleLabel = $derived(
    $auth.user?.role === 'admin' ? 'ești administrator'
    : $auth.user?.role === 'moderator' ? 'ești moderator'
    : $auth.user?.role === 'coordinator' ? 'ești coordonator'
    : 'ești verificator'
  );

  $effect(() => {
    if ($auth.isLoading) return;
    if (!$auth.isAuthenticated) goto('/login?redirect=/verify');
  });

  $effect(() => {
    if (!allowed) return;
    void showAll; // reload when the view toggles
    load();
  });

  async function load() {
    loading = true;
    try {
      const wantAll = showAll || !$auth.user; // safety: assigned needs a user id
      releases = (await api.getReleases(wantAll ? { all: true } : { assigned: true })).data;
    } catch {
      releases = [];
    } finally {
      loading = false;
    }
    if (canReassign && verifiers.length === 0) {
      try {
        verifiers = (await api.getVerifiers()).data;
      } catch {
        verifiers = [];
      }
    }
  }

  async function reassign(rel: Release, e: Event) {
    const v = (e.currentTarget as HTMLSelectElement).value;
    const id = v ? Number(v) : null;
    try {
      await api.assignVerifier(rel.id, id);
      rel.assignedVerifierId = id ?? undefined;
      rel.assignedVerifierName = id ? verifiers.find((x) => x.id === id)?.username : undefined;
      toast.success('Verificator actualizat.');
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Eroare la reatribuire.');
    }
  }

  const mine = $derived($auth.user?.id);
  // Everything in review, oldest first — including your own uploads. On a
  // small team the verifier often IS the translator; hiding own work made a
  // one-person submission simply vanish. Own rows are labeled instead.
  const queue = $derived(releases.filter((r) => r.state === 'in_review').slice().reverse());
  const nextUp = $derived(queue[0] ?? null);
  const rest = $derived(queue.slice(1));
  const inFix = $derived(releases.filter((r) => r.state === 'changes_requested'));
  const decided = $derived(
    releases.filter((r) => ['approved', 'changes_requested', 'published'].includes(r.state))
  );
  const week = 7 * 24 * 3_600_000;
  const month = 30 * 24 * 3_600_000;
  const decidedWeek = $derived(
    decided.filter((r) => Date.now() - new Date(r.updatedAt).getTime() < week).length
  );
  const published = $derived(releases.filter((r) => r.state === 'approved' || r.state === 'published'));
  const approvedMonth = $derived(
    published.filter((r) => Date.now() - new Date(r.updatedAt).getTime() < month).length
  );
  const returnedMonth = $derived(
    inFix.filter((r) => Date.now() - new Date(r.updatedAt).getTime() < month).length
  );

  // the series' real cover; gradient stand-in when the anime has none or the
  // release only carries a proposed title
  const poster = (r: Release) => {
    const img = r.animeImage ?? r.mangaImage;
    if (img)
      return `background-image: url('${img}'); background-size: cover; background-position: center`;
    const hue = ((r.animeId ?? r.mangaId ?? r.id * 13) * 47) % 360;
    return `background: linear-gradient(158deg, oklch(0.5 0.08 ${hue}) 0%, oklch(0.3 0.06 ${hue}) 46%, oklch(0.16 0.03 ${hue}) 100%)`;
  };
  const seriesName = (r: Release) => r.animeTitle ?? r.mangaTitle ?? r.proposedTitle ?? '—';
  const numLabel = (r: Release) =>
    r.medium === 'manga' ? `capitolul ${r.chapterNumber}` : `episodul ${r.episodeNumber}`;
  const sizeLabel = (r: Release) =>
    r.medium === 'manga' ? `${r.pageCount ?? 0} pagini` : `${r.totalEvents} replici`;
  const waitsLong = (r: Release) => Date.now() - new Date(r.updatedAt).getTime() > 2 * 24 * 3_600_000;

  const checklist = [
    'Diacritice complete — ă î ș ț â',
    'Timpii se potrivesc cu redarea',
    'Maxim 42 de caractere pe rând',
    'Ton consecvent cu restul seriei',
    'Nume și onorifice conform glosarului'
  ];
</script>

<svelte:head>
  <title>Verificare · Anime-Kage</title>
</svelte:head>

<div class="container page">
  {#if !$auth.isLoading && $auth.isAuthenticated && !allowed}
    <div class="denied">
      <h1>Verificare</h1>
      <p>Pagina e rezervată verificatorilor și coordonatorilor.</p>
      <a class="btn ghost" href="/home">Înapoi acasă</a>
    </div>
  {:else if allowed}
    <header class="top">
      <div>
        <p class="pg-kicker">Echipă · {roleLabel}</p>
        <h1>Verificare</h1>
        <p class="sub">
          {#if showAll}
            Tot ce e în pipeline, indiferent cui i-a fost rutat — vezi cine ține coada pe loc.
          {:else}
            Release-urile rutate către tine (plus cele nealocate), în ordinea vechimii.
            Deschizi, corectezi pe loc, apoi aprobi sau returnezi cu note.
          {/if}
        </p>
      </div>
      {#if canSeeAll}
        <button class="btn ghost" class:on={showAll} onclick={() => (showAll = !showAll)}>
          {showAll ? '✓ Vezi tot' : 'Vezi tot'}
        </button>
      {/if}
    </header>

    <div class="strip" role="list">
      <div class="cell" role="listitem">
        <span class="n accent">{queue.length}</span>
        <span class="l">în așteptare</span>
      </div>
      <div class="cell" role="listitem">
        <span class="n" class:warn={inFix.length > 0}>{inFix.length}</span>
        <span class="l">în corectură la traducători</span>
      </div>
      <div class="cell" role="listitem">
        <span class="n">{decidedWeek}</span>
        <span class="l">decise săptămâna asta</span>
      </div>
      <div class="cell" role="listitem">
        <span class="n">{published.length}</span>
        <span class="l">publicate</span>
      </div>
    </div>

    <div class="cols">
      <div class="main">
        {#if loading}
          <p class="muted">Se încarcă…</p>
        {:else}
          {#if nextUp}
            <section class="sect">
              <h2 class="s-label">Următorul la rând <span class="s-count">· cel mai vechi din coadă</span></h2>
              <div class="hero">
                <span class="poster lg" style={poster(nextUp)}></span>
                <div class="hero-main">
                  <span class="h-title">{seriesName(nextUp)} <span class="dim">— {numLabel(nextUp)}</span></span>
                  <span class="h-meta">
                    tradus de <Person name={nextUp.uploaderName} self={nextUp.uploaderId === mine} /> · {sizeLabel(nextUp)}
                    {#if showAll && nextUp.assignedVerifierName}
                      · verifică <Person name={nextUp.assignedVerifierName} self={nextUp.assignedVerifierId === mine} />
                    {/if}
                  </span>
                  <span class="pill" class:warn={waitsLong(nextUp)}>
                    <span class="dot" class:warn={waitsLong(nextUp)}></span>așteaptă de {waitedFor(nextUp.updatedAt)}
                  </span>
                </div>
                <a class="btn fill" href="/verify/{nextUp.id}">Începe verificarea →</a>
              </div>
            </section>
          {:else}
            <section class="sect">
              <h2 class="s-label">Coada de verificare</h2>
              <div class="empty-hero">
                <h2>Nimic de verificat</h2>
                <p>Coada e goală — primești de lucru când un traducător trimite un release.</p>
              </div>
            </section>
          {/if}

          {#if rest.length > 0}
            <section class="sect">
              <h2 class="s-label">Coada de verificare <span class="s-count">· {rest.length}</span></h2>
              <div class="listcard">
                {#each rest as rel (rel.id)}
                  <div class="lrow">
                    <span class="poster sm" style={poster(rel)}></span>
                    <span class="l-main">
                      <span class="l-title">{seriesName(rel)} <span class="dim">— {numLabel(rel)}</span></span>
                      <span class="l-meta">de <Person name={rel.uploaderName} self={rel.uploaderId === mine} /> · {sizeLabel(rel)}</span>
                    </span>
                    <span class="l-meta" class:late={waitsLong(rel)}>așteaptă de {waitedFor(rel.updatedAt)}</span>
                    {#if showAll && canReassign}
                      <select
                        class="vsel"
                        value={rel.assignedVerifierId ?? ''}
                        onchange={(e) => reassign(rel, e)}
                        title="Rutează verificarea"
                      >
                        <option value="">nealocat</option>
                        {#each verifiers as v (v.id)}
                          <option value={v.id} disabled={v.id === rel.uploaderId}>{v.username}</option>
                        {/each}
                      </select>
                    {:else if showAll}
                      <span class="l-meta">{rel.assignedVerifierName ? `verifică ${rel.assignedVerifierName}` : 'nealocat'}</span>
                    {:else if !rel.assignedVerifierId}
                      <span class="l-meta">nealocat</span>
                    {/if}
                    <a class="btn ghost sm" href="/verify/{rel.id}">Deschide →</a>
                  </div>
                {/each}
              </div>
            </section>
          {/if}

          {#if decided.length > 0}
            <section class="sect">
              <h2 class="s-label">Decise recent</h2>
              <div class="plain">
                {#each decided.slice(0, 8) as rel (rel.id)}
                  <div class="prow">
                    <div class="p-top">
                      <span class="pill {rel.state === 'changes_requested' ? 'warn' : 'ok'}">
                        {rel.state === 'changes_requested' ? 'returnat' : rel.state === 'published' ? 'publicat' : 'aprobat'}
                      </span>
                      <a class="p-title" href="/verify/{rel.id}">{seriesName(rel)} <span class="dim">— {numLabel(rel)}</span></a>
                      <span class="p-meta">
                        {#if rel.state === 'changes_requested'}
                          returnat către <Person name={rel.uploaderName} self={rel.uploaderId === mine} />
                        {:else if rel.reviewerName}
                          aprobat de <Person name={rel.reviewerName} />
                        {:else}
                          de <Person name={rel.uploaderName} self={rel.uploaderId === mine} />
                        {/if}
                        · {reltime(rel.updatedAt)}
                      </span>
                    </div>
                    {#if rel.state === 'changes_requested' && rel.reviewNotes}
                      <blockquote class="p-quote">„{rel.reviewNotes}”</blockquote>
                    {/if}
                  </div>
                {/each}
              </div>
            </section>
          {/if}
        {/if}
      </div>

      <aside class="rail">
        <section class="r-sect">
          <h2 class="r-label">Lista de verificare</h2>
          {#each checklist as item (item)}
            <div class="rule">
              <span class="box"></span>
              <span class="rule-t">{item}</span>
            </div>
          {/each}
          <p class="r-note">
            Corecturile mici le faci direct în editor — returnezi doar când e nevoie de traducător.
          </p>
        </section>

        <section class="r-sect">
          <h2 class="r-label">Luna asta</h2>
          <div class="stat-row"><span>Aprobate</span><strong>{approvedMonth}</strong></div>
          <div class="stat-row"><span>Returnate</span><strong>{returnedMonth}</strong></div>
        </section>
      </aside>
    </div>
  {/if}
</div>

<style>
  .page { padding-block: var(--space-6) var(--space-8); }

  .top { display: flex; align-items: flex-end; justify-content: space-between; flex-wrap: wrap; gap: var(--space-4); }
  .pg-kicker {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-bold);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--accent);
  }
  h1 {
    font-family: var(--font-display); font-size: var(--fs-h1);
    letter-spacing: -0.015em; line-height: var(--lh-tight); margin-top: 10px;
  }
  .sub { color: var(--text-muted); margin-top: 10px; max-width: 54ch; line-height: 1.55; }

  .strip {
    display: grid; grid-template-columns: repeat(4, minmax(0, 1fr));
    border-top: 2px solid var(--text-primary);
    margin-top: var(--space-5);
  }
  .cell { padding: 14px 18px 0 0; }
  .cell + .cell { border-left: 1px solid var(--border-subtle); padding-left: 18px; }
  .n { display: block; font-family: var(--font-display); font-size: 1.7rem; font-weight: var(--fw-semibold); line-height: 1; }
  .n.accent { color: var(--accent); }
  .n.warn { color: var(--warning); }
  .l {
    display: block; margin-top: 6px;
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-muted);
  }

  .cols {
    display: grid; grid-template-columns: minmax(0, 1fr) 280px;
    gap: var(--space-7); align-items: start; margin-top: var(--space-6);
  }
  @media (max-width: 900px) { .cols { grid-template-columns: minmax(0, 1fr); } }

  .sect { margin-bottom: var(--space-6); }
  .s-label {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--text-muted);
    padding-bottom: 12px; border-bottom: 1px solid var(--border-default);
    margin-bottom: var(--space-4);
  }
  .s-count { color: var(--text-muted); font-weight: var(--fw-regular); letter-spacing: 0.06em; text-transform: none; }
  .dim { color: var(--text-muted); font-weight: var(--fw-regular); }

  .poster { border-radius: var(--radius-sm); flex: 0 0 auto; }
  .poster.lg { width: 54px; height: 80px; border-radius: 8px; box-shadow: var(--shadow-2); }
  .poster.sm { width: 38px; height: 56px; }

  .hero {
    display: flex; align-items: center; gap: var(--space-4);
    border: 1px solid var(--border-default); border-radius: var(--radius-lg);
    background: var(--surface-raised); padding: var(--space-4) var(--space-5);
  }
  .hero-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 7px; }
  .h-title { font-family: var(--font-display); font-size: var(--fs-h3); font-weight: var(--fw-semibold); line-height: var(--lh-snug); }
  .h-meta { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }

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
  .l-meta { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); white-space: nowrap; }
  .l-meta.late { color: var(--warning); }

  .prow { padding: 13px 4px; border-bottom: 1px solid var(--border-subtle); }
  .p-top { display: flex; align-items: center; gap: 14px; min-width: 0; }
  .p-title {
    font-weight: var(--fw-semibold); color: var(--text-primary); min-width: 0;
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .p-title:hover .dim { color: var(--text-primary); }
  .p-meta { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); margin-left: auto; white-space: nowrap; }
  .p-quote {
    font-size: var(--fs-caption); font-style: italic; color: var(--text-muted);
    margin: 8px 0 0; padding-left: 12px; border-left: 2px solid var(--border-default);
    max-width: 64ch;
  }

  .pill {
    display: inline-flex; align-items: center; gap: 6px;
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.08em; text-transform: uppercase; white-space: nowrap; flex: 0 0 auto;
    color: var(--text-muted);
  }
  .pill.warn { color: var(--warning); }
  .pill.ok { color: var(--success); }
  .dot { width: 5px; height: 5px; border-radius: 50%; background: var(--text-muted); }
  .dot.warn { background: var(--warning); animation: pulse 2s ease-in-out infinite; }
  @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.35; } }

  .rail { position: sticky; top: calc(var(--header-h) + var(--space-4)); }
  @media (max-width: 900px) { .rail { position: static; } }
  .r-sect { margin-bottom: var(--space-6); }
  .r-label {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--accent);
    padding-bottom: 12px;
  }
  .rule { display: flex; gap: 12px; padding: 10px 0; border-top: 1px solid var(--border-subtle); }
  .box {
    width: 13px; height: 13px; border: 1px solid var(--border-default);
    border-radius: 3px; flex: 0 0 auto; margin-top: 3px;
  }
  .rule-t { font-size: var(--fs-small); line-height: 1.5; color: var(--text-muted); }
  .r-note { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); margin-top: 12px; line-height: 1.6; }
  .stat-row {
    display: flex; align-items: center; justify-content: space-between;
    padding: 10px 0; border-top: 1px solid var(--border-subtle);
  }
  .stat-row span { font-size: var(--fs-small); color: var(--text-muted); }
  .stat-row strong { font-family: var(--font-display); font-size: var(--fs-h3); font-weight: var(--fw-semibold); }

  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 11px 18px; border-radius: var(--radius-md); white-space: nowrap; cursor: pointer;
    transition: background var(--motion-fast) var(--ease), border-color var(--motion-fast) var(--ease);
  }
  .btn.sm { padding: 8px 14px; font-size: var(--fs-caption); }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-primary); }
  .btn.ghost:hover { background: var(--surface-overlay); border-color: var(--border-strong); }
  .btn.ghost.on {
    color: var(--accent);
    border-color: color-mix(in srgb, var(--accent) 55%, transparent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }

  .vsel {
    background: var(--surface-inset, var(--surface-overlay)); border: 1px solid var(--border-default);
    border-radius: var(--radius-sm); color: var(--text-primary); cursor: pointer;
    padding: 3px 8px; font-size: var(--fs-caption); outline: none; max-width: 150px;
  }
  .vsel:hover, .vsel:focus { border-color: var(--accent); }

  .empty-hero {
    text-align: center; padding: var(--space-7) var(--space-5);
    border: 1px dashed var(--border-default); border-radius: var(--radius-xl);
  }
  .empty-hero h2 { font-family: var(--font-display); font-size: var(--fs-h2); font-weight: var(--fw-semibold); }
  .empty-hero p { color: var(--text-muted); margin: 10px auto 0; max-width: 42ch; line-height: 1.6; }

  .muted { color: var(--text-muted); }
  .denied {
    display: flex; flex-direction: column; align-items: center; gap: var(--space-4);
    color: var(--text-muted); padding: var(--space-8) 0; text-align: center;
  }
  .denied h1 { font-family: var(--font-display); font-size: var(--fs-h1); color: var(--text-primary); }

  @media (max-width: 640px) {
    .strip { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .cell:nth-child(3) { border-left: 0; padding-left: 0; }
    .p-meta { display: none; }
  }
</style>
