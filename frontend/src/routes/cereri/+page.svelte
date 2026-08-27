<script lang="ts">
  import { mediaUrl } from '$lib/media';
  import { goto } from '$app/navigation';
  import api from '$lib/api';
  import { authStore } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import type { TranslationRequest, RequestStatusKey, MalSearchHit } from '$shared/types';

  type SearchHit = MalSearchHit & { medium: 'anime' | 'manga' };

  const auth = $derived($authStore);
  // coordinators/admins move requests through the queue
  const canModerate = $derived(!!auth.user && ['admin', 'coordinator'].includes(auth.user.role));

  const STATUS: Record<RequestStatusKey, { label: string; color: string }> = {
    pending: { label: 'În așteptare', color: 'var(--text-muted)' },
    in_progress: { label: 'În lucru', color: 'var(--accent)' },
    approved: { label: 'Aprobat', color: 'var(--success)' },
    rejected: { label: 'Respins', color: 'var(--danger)' }
  };
  const STATUS_ORDER: RequestStatusKey[] = ['pending', 'in_progress', 'approved', 'rejected'];

  const CHIPS: [string, string][] = [
    ['', 'Toate'],
    ['pending', 'În așteptare'],
    ['in_progress', 'În lucru'],
    ['approved', 'Aprobate'],
    ['rejected', 'Respinse']
  ];

  const hitMeta = (h: SearchHit) =>
    [h.type?.toUpperCase(), h.year, h.episodes ? `${h.episodes} ep` : h.chapters ? `${h.chapters} cap` : null]
      .filter(Boolean)
      .join(' · ');

  let requests = $state<TranslationRequest[]>([]);
  let pagination = $state({ page: 1, pages: 1, total: 0, perPage: 20 });
  let loading = $state(true);

  let filter = $state<'' | RequestStatusKey>('');
  let sort = $state<'votes' | 'recent'>('votes');
  let page = $state(1);

  $effect(() => {
    void filter;
    void sort;
    void page;
    load();
  });

  async function load() {
    loading = true;
    try {
      const res = await api.getRequests({ status: filter, sort, page });
      requests = res.data;
      pagination = res.pagination;
    } catch {
      requests = [];
    } finally {
      loading = false;
    }
  }

  function setFilter(f: '' | RequestStatusKey) {
    filter = f;
    page = 1;
  }
  function setSort(s: 'votes' | 'recent') {
    sort = s;
    page = 1;
  }

  async function toggleVote(r: TranslationRequest) {
    if (!auth.isAuthenticated) {
      toast.info('Autentifică-te ca să votezi.');
      goto('/login?redirect=/cereri');
      return;
    }
    try {
      const res = r.voted ? await api.unvoteRequest(r.id) : await api.voteRequest(r.id);
      requests = requests.map((x) =>
        x.id === r.id ? { ...x, voted: res.voted, voteCount: res.voteCount } : x
      );
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Votul a eșuat.');
    }
  }

  async function changeStatus(r: TranslationRequest, status: RequestStatusKey) {
    if (status === r.status) return;
    try {
      const res = await api.setRequestStatus(r.id, status);
      requests = requests.map((x) => (x.id === r.id ? { ...x, status: res.data.status } : x));
      toast.success(`„${r.title}” → ${STATUS[status].label}`);
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Schimbarea stării a eșuat.');
    }
  }

  // ── propose: search MAL, pick a series, submit ──────────────────────────────
  const NOTE_MAX = 300;
  let query = $state('');
  let hits = $state<SearchHit[]>([]);
  let searching = $state(false);
  let searched = $state(false);
  let selected = $state<SearchHit | null>(null);
  let draftNote = $state('');
  let submitting = $state(false);

  // Search only on Enter / button — each query hits the external MAL API, so we
  // fire on intent, not on every keystroke.
  async function runSearch() {
    const q = query.trim();
    if (q.length < 2 || searching) return;
    searching = true;
    searched = true;
    try {
      hits = (await api.searchRequests(q)).data;
    } catch {
      hits = [];
    } finally {
      searching = false;
    }
  }

  function pick(h: SearchHit) {
    if (!auth.isAuthenticated) {
      toast.info('Autentifică-te ca să trimiți o cerere.');
      goto('/login?redirect=/cereri');
      return;
    }
    selected = h;
    hits = [];
    query = '';
  }

  function clearSelection() {
    selected = null;
    draftNote = '';
  }

  async function submit() {
    if (!selected) return;
    submitting = true;
    try {
      const res = await api.createRequest({
        medium: selected.medium,
        title: selected.title,
        malId: selected.malId,
        imageUrl: selected.imageUrl,
        note: draftNote.trim() || undefined
      });
      if (res.merged) {
        toast.info(res.message ?? 'Cererea există deja — ți-am adăugat votul.');
      } else {
        toast.success(`„${res.data.title}” a fost adăugat.`);
      }
      clearSelection();
      filter = '';
      sort = res.merged ? 'votes' : 'recent';
      page = 1;
      await load();
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Trimiterea a eșuat.');
    } finally {
      submitting = false;
    }
  }

  const rankOffset = $derived((pagination.page - 1) * pagination.perPage);
</script>

<svelte:head><title>Cereri de traducere · Anime-Kage</title></svelte:head>

<div class="container cereri">
  <header class="top">
    <div>
      <p class="kick">Contribuții</p>
      <h1>Cereri de traducere</h1>
      <p class="sub">
        Votează ce vrei să subtitrăm următorul. Titlurile sunt legate de MyAnimeList, așa că
        aceeași serie nu poate fi cerută de două ori.
      </p>
    </div>
    <div class="top-r">
      <span class="kicker">{pagination.total} {pagination.total === 1 ? 'cerere' : 'cereri'}</span>
      <div class="seg">
        <button class:on={sort === 'votes'} onclick={() => setSort('votes')}>Voturi</button>
        <button class:on={sort === 'recent'} onclick={() => setSort('recent')}>Recente</button>
      </div>
    </div>
  </header>

  <div class="chips">
    {#each CHIPS as [key, label] (key)}
      <button class="chip" class:on={filter === key} onclick={() => setFilter(key as '' | RequestStatusKey)}>
        {label}
      </button>
    {/each}
  </div>

  <div class="cols">
    <div class="list">
      {#if loading}
        <p class="empty">Se încarcă…</p>
      {:else if !requests.length}
        <p class="empty">Nicio cerere în această categorie deocamdată.</p>
      {:else}
        {#each requests as r, i (r.id)}
          <div class="req">
            {#if sort === 'votes' && filter === ''}<span class="rank">{rankOffset + i + 1}</span>{/if}
            <button class="votebox" class:on={r.voted} onclick={() => toggleVote(r)} title="Votează">
              <span class="arrow">▲</span>
              <span class="votes">{r.voteCount}</span>
            </button>
            {#if r.imageUrl}
              <span class="poster media-tone" style={`background-image:url(${mediaUrl(r.imageUrl)})`}></span>
            {:else}
              <span class="poster ph">{r.title.charAt(0)}</span>
            {/if}
            <div class="req-main">
              <p class="req-t">{r.title}</p>
              <p class="req-sub">
                <span class="req-type">{r.medium === 'anime' ? 'Anime' : 'Manga'}</span>
                <span class="req-by">cerut de {r.requesterName}</span>
              </p>
              {#if r.note}<p class="req-note">{r.note}</p>{/if}
            </div>

            {#if canModerate}
              <label class="statusdd" style={`--sc:${STATUS[r.status].color}`} title="Schimbă starea">
                <select
                  value={r.status}
                  aria-label="Schimbă starea"
                  onchange={(e) => changeStatus(r, e.currentTarget.value as RequestStatusKey)}
                >
                  {#each STATUS_ORDER as s (s)}
                    <option value={s}>{STATUS[s].label}</option>
                  {/each}
                </select>
                <span class="dd-caret" aria-hidden="true">▾</span>
              </label>
            {:else}
              <span class="statustext" style={`color:${STATUS[r.status].color}`}>{STATUS[r.status].label}</span>
            {/if}
          </div>
        {/each}

        {#if pagination.pages > 1}
          <nav class="pager">
            <button class="pg-btn" disabled={pagination.page === 1} onclick={() => (page = pagination.page - 1)}>
              ← Anterior
            </button>
            <span class="pg-info">Pagina {pagination.page} din {pagination.pages}</span>
            <button class="pg-btn" disabled={pagination.page === pagination.pages} onclick={() => (page = pagination.page + 1)}>
              Următor →
            </button>
          </nav>
        {/if}
      {/if}
    </div>

    <aside class="side">
      <p class="side-head kicker">Propune un titlu</p>

      {#if selected}
        <div class="picked">
          {#if selected.imageUrl}
            <span class="picked-thumb media-tone" style={`background-image:url(${mediaUrl(selected.imageUrl)})`}></span>
          {:else}
            <span class="picked-thumb ph">{selected.title.charAt(0)}</span>
          {/if}
          <span class="picked-main">
            <span class="picked-t">{selected.title}</span>
            <span class="picked-m">
              <span class="hit-badge" class:manga={selected.medium === 'manga'}>{selected.medium === 'anime' ? 'Anime' : 'Manga'}</span>
              {hitMeta(selected)}
            </span>
          </span>
          <button class="picked-x" onclick={clearSelection} title="Alege altceva" aria-label="Alege altceva">×</button>
        </div>
        <textarea
          bind:value={draftNote}
          rows="2"
          maxlength={NOTE_MAX}
          placeholder="De ce vrei subtitrarea? (opțional)"
        ></textarea>
        <span class="note-count" class:near={draftNote.length > NOTE_MAX - 30}>{draftNote.length}/{NOTE_MAX}</span>
        <button class="send" disabled={submitting} onclick={submit}>
          {submitting ? 'Se trimite…' : 'Trimite cererea'}
        </button>
      {:else}
        <p class="side-p">Caută seria pe MyAnimeList și alege exact sezonul — sau manga.</p>
        <div class="searchrow">
          <input
            bind:value={query}
            oninput={() => (searched = false)}
            onkeydown={(e) => e.key === 'Enter' && runSearch()}
            placeholder="Caută: Frieren, One Piece…"
            autocomplete="off"
          />
          <button class="s-btn" disabled={searching || query.trim().length < 2} onclick={runSearch}>
            {searching ? '…' : 'Caută'}
          </button>
        </div>
        {#if searching}
          <p class="s-status">Se caută…</p>
        {:else if hits.length}
          <div class="hits">
            {#each hits as h (h.medium + '-' + h.malId)}
              <button class="hit" onclick={() => pick(h)}>
                {#if h.imageUrl}
                  <span class="hit-thumb media-tone" style={`background-image:url(${mediaUrl(h.imageUrl)})`}></span>
                {:else}
                  <span class="hit-thumb ph">{h.title.charAt(0)}</span>
                {/if}
                <span class="hit-main">
                  <span class="hit-t">{h.title}</span>
                  <span class="hit-m">
                    <span class="hit-badge" class:manga={h.medium === 'manga'}>{h.medium === 'anime' ? 'Anime' : 'Manga'}</span>
                    {hitMeta(h)}
                  </span>
                </span>
              </button>
            {/each}
          </div>
        {:else if searched}
          <p class="s-status">Niciun rezultat pe MyAnimeList.</p>
        {/if}
      {/if}

      <p class="fine">Un vot per membru · cererile duplicate se unesc, iar seriile deja în catalog sunt respinse.</p>
    </aside>
  </div>
</div>

<style>
  .cereri { padding-block: var(--space-6) var(--space-8); max-width: 1120px; }

  .top {
    display: flex; align-items: flex-end; justify-content: space-between;
    flex-wrap: wrap; gap: var(--space-4);
    padding-bottom: 20px; border-bottom: 2px solid var(--text-primary);
  }
  .kick { font-size: var(--fs-caption); font-weight: var(--fw-bold); color: var(--accent); }
  .top h1 { font-size: clamp(2rem, 1.6rem + 2vw, 2.625rem); letter-spacing: -0.02em; line-height: 1; margin-top: 10px; }
  .sub { font-size: 0.90625rem; color: var(--text-muted); margin-top: 12px; max-width: 560px; line-height: 1.55; }
  .top-r { display: flex; align-items: center; gap: 18px; }

  .seg {
    display: flex; gap: 2px; padding: 3px;
    background: var(--surface-raised); border: 1px solid var(--border-subtle); border-radius: 9px;
  }
  .seg button {
    font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--text-muted);
    padding: 6px 13px; border-radius: 7px; border: none; background: none; cursor: pointer;
  }
  .seg button.on { background: var(--surface-overlay); color: var(--text-primary); }

  .chips { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 22px; }
  .chip {
    font-size: var(--fs-caption); font-weight: var(--fw-medium); color: var(--text-muted);
    padding: 5px 12px; border: 1px solid var(--border-subtle); border-radius: var(--radius-pill);
    background: none; cursor: pointer;
  }
  .chip:hover { color: var(--text-primary); }
  .chip.on {
    color: var(--accent); border-color: color-mix(in srgb, var(--accent) 55%, transparent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }

  .cols { display: flex; flex-wrap: wrap; gap: 48px; align-items: flex-start; margin-top: 14px; }
  .list { flex: 3 1 380px; min-width: 0; }
  .empty { font-family: var(--font-mono); font-size: 0.8125rem; color: var(--text-muted); padding: 34px 6px; }

  .req {
    display: flex; gap: 16px; align-items: center;
    padding: 14px 6px; border-bottom: 1px solid var(--border-subtle);
  }
  .rank { font-family: var(--font-mono); font-size: 0.75rem; color: var(--text-muted); width: 18px; flex: 0 0 auto; }
  .votebox {
    display: flex; flex-direction: column; align-items: center; gap: 3px;
    width: 48px; padding: 8px 0; flex: 0 0 auto;
    border: 1px solid var(--border-default); border-radius: var(--radius-md);
    background: none; color: var(--text-muted); cursor: pointer;
    transition: all var(--motion-fast) var(--ease);
  }
  .votebox:hover { border-color: var(--accent); color: var(--text-primary); }
  .votebox.on {
    color: var(--accent);
    border-color: color-mix(in srgb, var(--accent) 55%, transparent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }
  .arrow { font-size: 0.6875rem; line-height: 1; }
  .votes { font-family: var(--font-display); font-size: 1rem; font-weight: var(--fw-semibold); line-height: 1; }

  .poster {
    flex: 0 0 auto; width: 48px; height: 68px; border-radius: 7px;
    background-color: var(--surface-overlay); background-size: cover; background-position: center;
    border: 1px solid var(--border-subtle);
  }
  .poster.ph {
    display: grid; place-items: center;
    font-family: var(--font-display); font-size: 1.4rem; color: rgba(255, 255, 255, 0.14);
  }
  .req-main { flex: 1; min-width: 0; }
  .req-t {
    font-family: var(--font-display); font-size: 1.0625rem; font-weight: var(--fw-semibold);
    color: var(--text-primary); letter-spacing: -0.005em; line-height: 1.25;
  }
  .req-sub {
    display: flex; align-items: center; gap: 10px; margin-top: 5px;
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
  }
  .req-type { letter-spacing: 0.08em; text-transform: uppercase; color: var(--accent); }
  .req-note {
    font-size: 0.8125rem; line-height: 1.5; color: var(--text-muted); margin-top: 6px; max-width: 520px;
    display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3;
    -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere;
  }

  /* ---- status: text-only for members, dropdown for staff ---- */
  .statustext {
    flex: 0 0 auto; font-family: var(--font-mono); font-size: var(--fs-caption);
    font-weight: var(--fw-medium); white-space: nowrap;
  }
  .statusdd {
    position: relative; display: inline-flex; align-items: center; gap: 6px; flex: 0 0 auto;
    padding: 5px 9px 5px 12px; border-radius: 8px; cursor: pointer;
    color: var(--sc); border: 1px solid color-mix(in srgb, var(--sc) 38%, transparent);
    background: color-mix(in srgb, var(--sc) 10%, transparent);
    transition: background var(--motion-fast) var(--ease);
  }
  .statusdd:hover { background: color-mix(in srgb, var(--sc) 18%, transparent); }
  .statusdd select {
    appearance: none; -webkit-appearance: none; -moz-appearance: none;
    border: none; background: none; outline: none; cursor: pointer;
    font-family: var(--font-mono); font-size: var(--fs-caption); font-weight: var(--fw-medium);
    color: var(--sc); padding: 0;
  }
  .statusdd select option { color: var(--text-primary); background: var(--surface-raised); }
  .dd-caret { font-size: 0.5625rem; opacity: 0.8; }

  .pager { display: flex; align-items: center; justify-content: center; gap: 16px; margin-top: 30px; }
  .pg-btn {
    font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--text-primary);
    padding: 8px 16px; border-radius: 9px; cursor: pointer;
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
  }
  .pg-btn:hover:not(:disabled) { border-color: var(--accent); }
  .pg-btn:disabled { opacity: 0.4; cursor: not-allowed; }
  .pg-info { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }

  /* ---- propose panel ---- */
  .side { flex: 1 1 300px; min-width: 260px; }
  .side-head { color: var(--accent); padding: 14px 0 0; display: block; }
  .side-p { font-size: 0.8125rem; color: var(--text-muted); margin: 10px 0 12px; line-height: 1.55; }
  .side input,
  .side textarea {
    width: 100%; padding: 11px 13px;
    border: 1px solid var(--border-default); border-radius: 10px;
    background: var(--surface-inset); color: var(--text-primary); outline: none;
    font-size: 0.875rem; line-height: 1.5; resize: vertical; font-family: var(--font-body);
  }
  .side input:focus,
  .side textarea:focus { border-color: var(--accent); }
  .side input::placeholder,
  .side textarea::placeholder { color: var(--text-faint); }
  .s-status { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); margin-top: 12px; }

  .searchrow { display: flex; gap: 8px; }
  .searchrow input { flex: 1; min-width: 0; }
  .s-btn {
    flex: 0 0 auto; font-weight: var(--fw-semibold); font-size: var(--fs-caption);
    padding: 0 15px; border-radius: 10px; cursor: pointer;
    border: 1px solid var(--border-default); background: var(--surface-raised); color: var(--text-primary);
  }
  .s-btn:hover:not(:disabled) { border-color: var(--accent); color: var(--accent); }
  .s-btn:disabled { opacity: 0.45; cursor: not-allowed; }

  .note-count {
    display: block; text-align: right; margin-top: 5px;
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-faint);
  }
  .note-count.near { color: var(--danger); }

  .hits {
    margin-top: 10px; display: flex; flex-direction: column;
    border: 1px solid var(--border-subtle); border-radius: 12px; overflow: hidden;
    max-height: 420px; overflow-y: auto;
  }
  .hit {
    display: flex; gap: 12px; align-items: center; text-align: left;
    padding: 9px 11px; background: none; border: none; cursor: pointer;
    border-bottom: 1px solid var(--border-subtle);
  }
  .hit:last-child { border-bottom: none; }
  .hit:hover { background: var(--surface-overlay); }
  .hit-thumb {
    flex: 0 0 auto; width: 34px; height: 48px; border-radius: 5px;
    background-color: var(--surface-overlay); background-size: cover; background-position: center;
  }
  .hit-thumb.ph, .picked-thumb.ph {
    display: grid; place-items: center;
    font-family: var(--font-display); color: rgba(255, 255, 255, 0.16);
  }
  .hit-main { min-width: 0; display: flex; flex-direction: column; gap: 4px; }
  .hit-t {
    font-size: var(--fs-small); font-weight: var(--fw-semibold); color: var(--text-primary);
    line-height: 1.2; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .hit-m { display: flex; align-items: center; gap: 7px; font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .hit-badge {
    font-size: 0.5625rem; font-weight: var(--fw-bold); letter-spacing: 0.06em;
    padding: 2px 6px; border-radius: 999px; text-transform: uppercase;
    background: color-mix(in srgb, var(--accent) 16%, transparent); color: var(--accent);
  }
  .hit-badge.manga { background: color-mix(in srgb, var(--blue, #6aa9ff) 18%, transparent); color: var(--blue, #6aa9ff); }

  .picked {
    display: flex; gap: 13px; align-items: center; margin-top: 6px;
    padding: 11px; border: 1px solid var(--border-default); border-radius: 12px;
    background: var(--surface-raised);
  }
  .picked-thumb {
    flex: 0 0 auto; width: 46px; height: 66px; border-radius: 6px;
    background-color: var(--surface-overlay); background-size: cover; background-position: center;
    font-size: 1.3rem;
  }
  .picked-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 6px; }
  .picked-t { font-family: var(--font-display); font-size: 0.9375rem; font-weight: var(--fw-semibold); color: var(--text-primary); line-height: 1.2; }
  .picked-m { display: flex; align-items: center; gap: 7px; font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .picked-x {
    flex: 0 0 auto; width: 26px; height: 26px; border-radius: 50%;
    border: 1px solid var(--border-subtle); background: none; color: var(--text-muted);
    cursor: pointer; font-size: 1rem; line-height: 1;
  }
  .picked-x:hover { color: var(--danger); border-color: color-mix(in srgb, var(--danger) 45%, transparent); }

  .side textarea { margin-top: 12px; }
  .send {
    width: 100%; margin-top: 12px;
    font-weight: var(--fw-semibold); font-size: 0.875rem; padding: 12px;
    border: none; border-radius: 10px;
    background: var(--accent); color: var(--on-accent); cursor: pointer;
  }
  .send:hover:not(:disabled) { background: var(--accent-hover); }
  .send:disabled { opacity: 0.6; cursor: wait; }
  .fine { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); line-height: 1.6; margin-top: 16px; }

  @media (max-width: 720px) {
    .req { flex-wrap: wrap; }
    .req-main { flex-basis: 100%; order: 5; }
    .statusdd, .statustext { order: 6; }
  }
</style>
