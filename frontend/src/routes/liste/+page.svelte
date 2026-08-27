<script lang="ts">
  import PagePicker from '$lib/components/PagePicker.svelte';
  import { goto } from '$app/navigation';
  import { mediaUrl } from '$lib/media';
  import api from '$lib/api';
  import { authStore } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import type { UserList } from '$shared/types';

  let { data } = $props();
  const auth = $derived($authStore);

  let tab = $state<'populare' | 'prieteni' | 'mele'>('populare');

  const TABS: [typeof tab, string][] = [
    ['populare', 'Populare'],
    ['prieteni', 'De la prieteni'],
    ['mele', 'Ale mele']
  ];

  const shown = $derived.by(() => {
    if (tab === 'mele') return [];
    const sorted = [...data.lists].sort((a, b) => b.likes - a.likes);
    return tab === 'prieteni' ? sorted.slice(0, 3) : sorted;
  });

  const featured = $derived(tab === 'populare' ? data.featured : null);
  const gridAll = $derived(featured ? shown.filter((l) => l.slug !== featured.slug) : shown);

  // paginate the browse grid
  const GRID_PER = 8;
  let gridPage = $state(1);
  $effect(() => {
    void tab;
    gridPage = 1;
  });
  const gridPages = $derived(Math.max(1, Math.ceil(gridAll.length / GRID_PER)));
  const gridLists = $derived(gridAll.slice((gridPage - 1) * GRID_PER, gridPage * GRID_PER));

  // ── "Ale mele": real lists from the API ────────────────────────────────────
  let mine = $state<UserList[]>([]);
  let mineLoading = $state(false);
  let mineLoaded = false;

  $effect(() => {
    if (tab !== 'mele' || mineLoaded || auth.isLoading || !auth.isAuthenticated) return;
    mineLoaded = true;
    mineLoading = true;
    api
      .getMyLists()
      .then((res) => (mine = res.data))
      .catch(() => (mine = []))
      .finally(() => (mineLoading = false));
  });

  // create form
  let creating = $state(false);
  let newTitle = $state('');
  let newDesc = $state('');
  let newPublic = $state(true);
  let saving = $state(false);

  function startCreate() {
    if (!auth.isAuthenticated) {
      toast.info('Autentifică-te ca să îți creezi liste.');
      goto('/login?redirect=/liste');
      return;
    }
    tab = 'mele';
    creating = true;
  }

  async function createList(e: SubmitEvent) {
    e.preventDefault();
    const title = newTitle.trim();
    if (!title) return;
    saving = true;
    try {
      const res = await api.createList({
        title,
        description: newDesc.trim() || undefined,
        isPublic: newPublic
      });
      toast.success('Listă creată — adaugă primele titluri!');
      goto(`/liste/${res.data.id}`);
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Crearea listei a eșuat.');
    } finally {
      saving = false;
    }
  }

  async function removeList(l: UserList) {
    if (!confirm(`Ștergi lista „${l.title}”?`)) return;
    try {
      await api.deleteList(l.id);
      mine = mine.filter((x) => x.id !== l.id);
      toast.success('Listă ștearsă.');
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Ștergerea a eșuat.');
    }
  }
</script>

<svelte:head><title>Liste · Anime-Kage</title></svelte:head>

<div class="container liste">
  <header class="top">
    <div>
      <p class="kick">Colecții</p>
      <h1>Liste</h1>
    </div>
    <button class="btn ghost" onclick={startCreate}>+ Creează o listă</button>
  </header>

  <div class="tabs">
    {#each TABS as [key, label] (key)}
      <button class="tab" class:on={tab === key} onclick={() => (tab = key)}>{label}</button>
    {/each}
  </div>

  {#if featured}
    <a class="featured" href={`/liste/${featured.slug}`}>
      <span class="f-covers">
        {#each featured.covers as cv, i (i)}
          <span class="f-cover media-tone" style={`background-image:url(${mediaUrl(cv)})`}></span>
        {/each}
        <span class="f-fade"></span>
      </span>
      <span class="f-body">
        <span class="f-flag"><span class="f-dot"></span> Listă remarcată</span>
        <span class="f-title">{featured.title}</span>
        <span class="f-desc">{featured.desc}</span>
        <span class="f-meta">
          {#if featured.authorName}
            <span class="f-author">
              {#if featured.authorAvatarUrl}
                <span class="avatar"><img src={api.resolveUrl(featured.authorAvatarUrl)} alt={featured.authorName} /></span>
              {:else}
                <span class="avatar monogram" style={`--mg-hue:${featured.authorHue}`}>{featured.authorName.charAt(0)}</span>
              {/if}
              o listă de <strong>{featured.authorName}</strong>
            </span>
          {:else}
            <span class="f-author">o listă a platformei</span>
          {/if}
          <span class="f-stats">
            <span>{featured.count} titluri</span>
            <span class="lk">♥ {featured.likes}</span>
            <span>💬 {featured.comments}</span>
          </span>
        </span>
      </span>
    </a>
  {/if}

  {#if tab === 'populare' && data.platform.length}
    <section class="platform">
      <div class="p-head">
        <span class="p-flag"><span class="f-dot"></span> De la Anime-Kage</span>
        <span class="p-sub">Clasamente oficiale, actualizate din catalog</span>
      </div>
      <div class="p-grid">
        {#each data.platform as l (l.slug)}
          <a class="pcard" href={`/liste/${l.slug}`}>
            <span class="pc-covers">
              {#each l.covers as cv, i (i)}
                <span class="pc-cover media-tone" style={`background-image:url(${mediaUrl(cv)})`}></span>
              {/each}
            </span>
            <span class="pc-body">
              <span class="pc-title">{l.title}</span>
              <span class="pc-desc">{l.desc}</span>
              <span class="pc-count">{l.count} titluri →</span>
            </span>
          </a>
        {/each}
      </div>
    </section>
  {/if}

  {#if tab === 'mele'}
    {#if creating}
      <form class="create" onsubmit={createList}>
        <div class="c-field">
          <label for="nl-title">Titlul listei</label>
          <input
            id="nl-title"
            type="text"
            maxlength="120"
            placeholder="Top isekai, De plâns garantat…"
            bind:value={newTitle}
          />
        </div>
        <div class="c-field">
          <label for="nl-desc">Descriere <span class="c-opt">opțional</span></label>
          <textarea id="nl-desc" rows="2" placeholder="Despre ce e lista…" bind:value={newDesc}></textarea>
        </div>
        <div class="c-foot">
          <label class="c-pub">
            <input type="checkbox" bind:checked={newPublic} />
            <span>publică — apare în Liste pentru toată lumea</span>
          </label>
          <button class="btn ghost" type="button" onclick={() => (creating = false)}>Anulează</button>
          <button class="btn fill" type="submit" disabled={saving || !newTitle.trim()}>
            {saving ? 'Se creează…' : 'Creează lista'}
          </button>
        </div>
      </form>
    {/if}

    {#if !auth.isLoading && !auth.isAuthenticated}
      <div class="empty">
        <p class="empty-t">Listele tale te așteaptă</p>
        <p class="empty-p"><a class="loglink" href="/login?redirect=/liste">Autentifică-te</a> ca să îți creezi propriile liste.</p>
      </div>
    {:else if mineLoading}
      <p class="loading">Se încarcă…</p>
    {:else if mine.length === 0 && !creating}
      <div class="empty">
        <p class="empty-t">Nicio listă încă</p>
        <p class="empty-p">Creează-ți prima listă și adună titlurile care merită povestite.</p>
        <button class="btn fill mt" onclick={() => (creating = true)}>+ Prima ta listă</button>
      </div>
    {:else if mine.length > 0}
      <div class="grid">
        {#each mine as l (l.id)}
          <div class="lcard mine">
            <a class="l-link" href={`/liste/${l.id}`}>
              <span class="l-covers">
                {#if l.covers.length}
                  {#each l.covers as cv, i (i)}
                    <span class="l-cover media-tone" style={`background-image:url(${mediaUrl(cv)})`}></span>
                  {/each}
                {:else}
                  <span class="l-cover l-blank">+</span>
                {/if}
              </span>
              <span class="l-title">{l.title}</span>
              {#if l.description}<span class="l-desc">{l.description}</span>{/if}
            </a>
            <span class="l-meta">
              <span class="l-author">{l.itemCount} titluri · {l.isPublic ? 'publică' : 'privată'}</span>
              <span class="l-stats">
                <a class="l-edit" href={`/liste/${l.id}`}>deschide →</a>
                <button class="l-del" title="Șterge lista" onclick={() => removeList(l)}>×</button>
              </span>
            </span>
          </div>
        {/each}
      </div>
    {/if}
  {:else if !shown.length}
    <div class="empty">
      <p class="empty-t">Nimic aici încă</p>
      <p class="empty-p">Urmărește alți membri ca să le vezi listele, sau creează-ți prima listă.</p>
    </div>
  {:else}
    <div class="grid">
      {#each gridLists as l (l.slug)}
        <a class="lcard" href={`/liste/${l.slug}`}>
          <span class="l-covers">
            {#each l.covers as cv, i (i)}
              <span class="l-cover media-tone" style={`background-image:url(${mediaUrl(cv)})`}></span>
            {/each}
          </span>
          <span class="l-title">{l.title}</span>
          <span class="l-desc">{l.desc}</span>
          <span class="l-meta">
            <span class="l-author">
              {#if l.authorAvatarUrl}
                <span class="avatar sm"><img src={api.resolveUrl(l.authorAvatarUrl)} alt={l.authorName} /></span>
              {:else}
                <span class="avatar sm monogram" style={`--mg-hue:${l.authorHue}`}>{l.authorName.charAt(0)}</span>
              {/if}
              {l.authorName}
            </span>
            <span class="l-stats">
              <span>{l.count}</span>
              <span class="lk">♥ {l.likes}</span>
              {#if l.comments}<span>💬 {l.comments}</span>{/if}
            </span>
          </span>
        </a>
      {/each}
    </div>
    {#if gridPages > 1}
      <nav class="pager">
        <button class="pg-btn" disabled={gridPage === 1} onclick={() => (gridPage -= 1)}>← Anterior</button>
        <PagePicker page={gridPage} pages={gridPages} onselect={(n) => (gridPage = n)} />
        <button class="pg-btn" disabled={gridPage === gridPages} onclick={() => (gridPage += 1)}>Următor →</button>
      </nav>
    {/if}
  {/if}
</div>

<style>
  .liste { padding-block: var(--space-6) var(--space-8); max-width: 1180px; }

  .top {
    display: flex; align-items: flex-end; justify-content: space-between;
    flex-wrap: wrap; gap: var(--space-4);
    padding-bottom: 18px; border-bottom: 2px solid var(--text-primary);
  }
  .kick { font-size: var(--fs-caption); font-weight: var(--fw-bold); color: var(--accent); }
  .top h1 { font-size: clamp(2rem, 1.6rem + 2vw, 2.625rem); letter-spacing: -0.02em; line-height: 1; margin-top: 10px; }

  .btn {
    font-weight: var(--fw-semibold); font-size: 0.84375rem;
    padding: 11px 18px; border-radius: 9px; cursor: pointer;
  }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-primary); }
  .btn.ghost:hover { background: var(--surface-raised); border-color: var(--text-primary); }

  .tabs { display: flex; gap: 6px; margin: 18px 0 30px; }
  .tab {
    font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--text-muted);
    padding: 7px 14px; border-radius: var(--radius-pill); border: 1px solid transparent;
    background: none; cursor: pointer;
  }
  .tab:hover { color: var(--text-primary); }
  .tab.on {
    color: var(--accent); border-color: color-mix(in srgb, var(--accent) 55%, transparent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }

  /* ---- platform (official) lists ---- */
  .platform { margin-bottom: 40px; }
  .p-head { display: flex; align-items: baseline; gap: 14px; flex-wrap: wrap; margin-bottom: 18px; }
  .p-flag {
    display: inline-flex; align-items: center; gap: 7px;
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--accent);
  }
  .p-sub { font-size: var(--fs-caption); color: var(--text-muted); }
  .p-grid {
    display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px;
  }
  .pcard {
    display: flex; flex-direction: column; overflow: hidden;
    border: 1px solid var(--border-subtle); border-radius: 14px;
    background: var(--surface-raised);
    transition: border-color var(--motion-base) var(--ease), transform var(--motion-base) var(--ease);
  }
  .pcard:hover { border-color: color-mix(in srgb, var(--accent) 45%, var(--border-subtle)); transform: translateY(-3px); }
  .pc-covers { display: flex; height: 96px; }
  .pc-cover {
    flex: 1; background-color: var(--surface-overlay);
    background-size: cover; background-position: center 20%;
  }
  .pc-body { display: flex; flex-direction: column; gap: 6px; padding: 14px 16px 16px; }
  .pc-title { font-family: var(--font-display); font-size: 1.0625rem; font-weight: var(--fw-semibold); color: var(--text-primary); }
  .pc-desc {
    font-size: 0.8125rem; line-height: 1.45; color: var(--text-muted);
    display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden;
  }
  .pc-count { margin-top: 2px; font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--accent); }

  /* ---- pager ---- */
  .pager { display: flex; align-items: center; justify-content: center; gap: 16px; margin-top: 34px; }
  .pg-btn {
    font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--text-primary);
    padding: 8px 16px; border-radius: 9px; cursor: pointer;
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
  }
  .pg-btn:hover:not(:disabled) { border-color: var(--accent); }
  .pg-btn:disabled { opacity: 0.4; cursor: not-allowed; }
  .pg-info { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }

  /* ---- featured ---- */
  .featured {
    position: relative; display: block; overflow: hidden;
    border: 1px solid var(--border-subtle); border-radius: 16px;
    background: var(--surface-raised); margin-bottom: 38px;
    transition: border-color var(--motion-base) var(--ease);
  }
  .featured:hover { border-color: var(--border-default); }
  .f-covers {
    position: relative; display: flex; gap: 0;
    padding: 26px 28px 0; height: 150px; overflow: hidden;
  }
  .f-cover {
    position: relative; width: 104px; height: 156px; flex: 0 0 auto;
    margin-right: -26px; border-radius: 7px;
    border: 2px solid var(--surface-raised);
    background-color: var(--surface-overlay);
    background-size: cover; background-position: center 20%;
    box-shadow: 0 10px 24px rgba(0, 0, 0, 0.4);
  }
  .f-fade {
    position: absolute; inset: 0;
    background: linear-gradient(180deg, transparent 30%, var(--surface-raised));
  }
  .f-body { display: block; padding: 6px 28px 26px; }
  .f-flag {
    display: inline-flex; align-items: center; gap: 7px;
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--accent);
    margin-bottom: 10px;
  }
  .f-dot { width: 5px; height: 5px; border-radius: 50%; background: var(--accent); }
  .f-title {
    display: block; font-family: var(--font-display);
    font-size: 1.6875rem; font-weight: var(--fw-semibold);
    line-height: 1.12; letter-spacing: -0.01em; color: var(--text-primary);
  }
  .f-desc {
    display: block; font-size: 0.90625rem; line-height: 1.55;
    color: var(--text-muted); margin: 10px 0 18px; max-width: 560px;
  }
  .f-meta { display: flex; align-items: center; gap: 16px; flex-wrap: wrap; }
  .f-author { display: flex; align-items: center; gap: 9px; font-size: 0.84375rem; color: var(--text-primary); }
  .f-author strong { font-weight: var(--fw-semibold); }
  .f-stats {
    display: flex; align-items: center; gap: 16px; margin-left: auto;
    font-family: var(--font-mono); font-size: 0.75rem; color: var(--text-muted);
  }
  .lk { color: var(--accent); }

  .avatar {
    width: 30px; height: 30px; border-radius: 50%;
    display: grid; place-items: center;
    font-size: 0.75rem; font-weight: var(--fw-bold); color: #fff;
  }
  .avatar.sm { width: 24px; height: 24px; font-size: var(--fs-micro); }
  /* A monogram is drawn with a background; a real avatar is an <img> that has
     to be cropped to the same circle. */
  .avatar img { width: 100%; height: 100%; object-fit: cover; border-radius: inherit; display: block; }

  /* ---- grid ---- */
  .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; }
  .lcard {
    display: block; cursor: pointer;
    padding-bottom: 22px; border-bottom: 1px solid var(--border-subtle);
    transition: opacity var(--motion-base) var(--ease);
  }
  .lcard:hover { opacity: 0.82; }
  .l-covers { display: flex; height: 120px; padding-left: 6px; margin-bottom: 16px; }
  .l-cover {
    position: relative; width: 80px; height: 120px; flex: 0 0 auto;
    margin-left: -18px; border-radius: 6px;
    border: 2px solid var(--surface-base);
    background-color: var(--surface-overlay);
    background-size: cover; background-position: center 20%;
    box-shadow: 0 6px 16px rgba(0, 0, 0, 0.35);
  }
  .l-covers .l-cover:first-child { margin-left: 0; }
  .l-title {
    display: block; font-family: var(--font-display);
    font-size: 1.1875rem; font-weight: var(--fw-semibold);
    line-height: 1.18; color: var(--text-primary);
  }
  .l-desc { display: block; font-size: 0.8125rem; line-height: 1.5; color: var(--text-muted); margin: 7px 0 13px; }
  .l-meta { display: flex; align-items: center; gap: 12px; }
  .l-author { display: flex; align-items: center; gap: 8px; font-size: 0.78125rem; color: var(--text-muted); }
  .l-stats {
    display: flex; align-items: center; gap: 13px; margin-left: auto;
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
  }

  /* ---- create form + own lists ---- */
  .create {
    border: 1px solid var(--border-strong); border-radius: 14px;
    background: var(--surface-raised); padding: 22px;
    display: flex; flex-direction: column; gap: 16px; margin-bottom: 30px;
  }
  .c-field { display: flex; flex-direction: column; gap: 7px; }
  .c-field label {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.08em; text-transform: uppercase; color: var(--text-muted);
  }
  .c-opt { text-transform: none; letter-spacing: 0; }
  .c-field input[type='text'], .c-field textarea {
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: 9px; color: var(--text-primary);
    padding: 10px 14px; font-size: var(--fs-small); font-family: var(--font-body);
    outline: none; resize: vertical;
  }
  .c-field input:focus, .c-field textarea:focus { border-color: var(--accent); }
  .c-foot { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
  .c-pub {
    display: flex; align-items: center; gap: 8px; cursor: pointer; flex: 1;
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
  }
  .c-pub input { accent-color: var(--accent); cursor: pointer; }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); }
  .btn:disabled { opacity: 0.5; cursor: not-allowed; }

  .lcard.mine { cursor: default; }
  .lcard.mine:hover { opacity: 1; }
  .l-link { display: block; }
  .l-link:hover { opacity: 0.82; }
  .l-blank {
    display: grid; place-items: center;
    color: var(--text-muted); font-size: 1.4rem;
  }
  .l-edit { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--accent); }
  .l-edit:hover { color: var(--accent-hover); }
  .l-del {
    background: none; border: 0; color: var(--text-muted); font-size: 1rem;
    cursor: pointer; padding: 2px 6px; border-radius: 6px; line-height: 1;
  }
  .l-del:hover { color: var(--danger); background: color-mix(in srgb, var(--danger) 10%, transparent); }
  .loglink { color: var(--accent); }
  .loading { color: var(--text-muted); padding: 30px 0; }
  .mt { margin-top: 18px; }

  .empty {
    text-align: center; padding: 70px 20px;
    border: 1px dashed var(--border-default); border-radius: 16px;
  }
  .empty-t { font-family: var(--font-display); font-size: 1.375rem; font-weight: var(--fw-semibold); }
  .empty-p { font-size: var(--fs-small); color: var(--text-muted); margin-top: 10px; }

  @media (max-width: 760px) {
    .grid { grid-template-columns: minmax(0, 1fr); }
    .f-stats { margin-left: 0; }
  }
</style>
