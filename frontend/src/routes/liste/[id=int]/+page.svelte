<script lang="ts">
  import PagePicker from '$lib/components/PagePicker.svelte';
  import { mediaUrl } from '$lib/media';
  // A real member list (custom lists API) — the seeded community lists live
  // at /liste/[slug]. Client-rendered: private lists need the owner's token,
  // which exists only in localStorage.
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import api from '$lib/api';
  import { authStore } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import { displayName } from '$lib/types';
  import type { Anime, Manga, UserList, UserListItem } from '$shared/types';

  const listId = $derived(Number(page.params.id));
  const auth = $derived($authStore);

  let list = $state<UserList | null>(null);
  let items = $state<UserListItem[]>([]);
  let error = $state('');
  const isOwner = $derived(list !== null && auth.user?.id === list.userId);

  let loadedFor = -1;
  $effect(() => {
    if (auth.isLoading || loadedFor === listId) return;
    loadedFor = listId;
    load();
  });

  async function load() {
    error = '';
    try {
      const res = await api.getList(listId);
      list = res.data;
      items = res.items;
    } catch (err) {
      error = (err as { error?: string }).error ?? 'Lista nu a putut fi încărcată.';
    }
  }

  const errMsg = (err: unknown, fallback: string) =>
    (err as { error?: string }).error ?? (err as { message?: string }).message ?? fallback;

  // ── likes ───────────────────────────────────────────────────────────────────
  let likeBusy = $state(false);
  async function toggleLike() {
    if (!list) return;
    if (!auth.isAuthenticated) {
      toast.info('Autentifică-te ca să apreciezi liste.');
      goto(`/login?redirect=/liste/${listId}`);
      return;
    }
    likeBusy = true;
    try {
      const res = list.liked ? await api.unlikeList(list.id) : await api.likeList(list.id);
      list = { ...list, liked: res.liked, likeCount: res.likeCount };
    } catch (err) {
      toast.error(errMsg(err, 'Acțiunea a eșuat.'));
    } finally {
      likeBusy = false;
    }
  }

  // ── item pagination ─────────────────────────────────────────────────────────
  const ITEMS_PER = 20;
  let itemPage = $state(1);
  const itemPages = $derived(Math.max(1, Math.ceil(items.length / ITEMS_PER)));
  const pagedItems = $derived(items.slice((itemPage - 1) * ITEMS_PER, itemPage * ITEMS_PER));
  $effect(() => {
    if (itemPage > itemPages) itemPage = itemPages;
  });

  // ── owner: edit list meta ──────────────────────────────────────────────────
  let editing = $state(false);
  let eTitle = $state('');
  let eDesc = $state('');
  let ePublic = $state(true);
  let busy = $state(false);

  function startEdit() {
    if (!list) return;
    eTitle = list.title;
    eDesc = list.description ?? '';
    ePublic = list.isPublic;
    editing = true;
  }

  async function saveEdit(e: SubmitEvent) {
    e.preventDefault();
    if (!list || !eTitle.trim()) return;
    busy = true;
    try {
      await api.updateList(list.id, {
        title: eTitle.trim(),
        description: eDesc.trim() || undefined,
        isPublic: ePublic
      });
      list.title = eTitle.trim();
      list.description = eDesc.trim() || undefined;
      list.isPublic = ePublic;
      editing = false;
      toast.success('Listă actualizată.');
    } catch (err) {
      toast.error(errMsg(err, 'Salvarea a eșuat.'));
    } finally {
      busy = false;
    }
  }

  async function removeList() {
    if (!list || !confirm(`Ștergi lista „${list.title}”?`)) return;
    try {
      await api.deleteList(list.id);
      toast.success('Listă ștearsă.');
      goto('/liste');
    } catch (err) {
      toast.error(errMsg(err, 'Ștergerea a eșuat.'));
    }
  }

  // ── owner: add titles ──────────────────────────────────────────────────────
  let adding = $state(false);
  let medium = $state<'anime' | 'manga'>('anime');
  let query = $state('');
  let results = $state<(Anime | Manga)[]>([]);
  let addBusy = $state<number | null>(null);
  let searchTimer: ReturnType<typeof setTimeout>;

  function onQuery() {
    clearTimeout(searchTimer);
    if (query.trim().length < 2) {
      results = [];
      return;
    }
    searchTimer = setTimeout(async () => {
      try {
        results =
          medium === 'manga'
            ? (await api.searchManga(query.trim())).data.slice(0, 6)
            : (await api.searchAnime(query.trim())).data.slice(0, 6);
      } catch {
        results = [];
      }
    }, 250);
  }

  function switchMedium(m: 'anime' | 'manga') {
    if (medium === m) return;
    medium = m;
    results = [];
    if (query.trim().length >= 2) onQuery();
  }

  const onList = $derived(
    new Set(items.map((it) => `${it.animeId ? 'anime' : 'manga'}:${it.animeId ?? it.mangaId}`))
  );

  async function addTitle(t: Anime | Manga) {
    if (!list) return;
    addBusy = t.id;
    try {
      const res = await api.addListItem(list.id, { mediaType: medium, mediaId: t.id });
      items = [...items, res.data];
      list.itemCount = items.length;
      toast.success(`„${displayName(t)}” adăugat pe listă.`);
    } catch (err) {
      toast.error(errMsg(err, 'Adăugarea a eșuat.'));
    } finally {
      addBusy = null;
    }
  }

  // ── owner: per-item notes + removal ────────────────────────────────────────
  let noteFor = $state<number | null>(null);
  let noteDraft = $state('');

  function openNote(it: UserListItem) {
    noteFor = noteFor === it.id ? null : it.id;
    noteDraft = it.note ?? '';
  }

  async function saveNote(it: UserListItem) {
    if (!list) return;
    try {
      await api.updateListItem(list.id, it.id, noteDraft.trim() || null);
      it.note = noteDraft.trim() || undefined;
      noteFor = null;
      toast.success('Notă salvată.');
    } catch (err) {
      toast.error(errMsg(err, 'Salvarea notei a eșuat.'));
    }
  }

  async function removeItem(it: UserListItem) {
    if (!list) return;
    try {
      await api.removeListItem(list.id, it.id);
      items = items.filter((x) => x.id !== it.id);
      list.itemCount = items.length;
      toast.success('Titlu scos de pe listă.');
    } catch (err) {
      toast.error(errMsg(err, 'Eliminarea a eșuat.'));
    }
  }

  const itemHref = (it: UserListItem) => (it.animeId ? `/anime/${it.animeId}` : `/manga/${it.mangaId}`);
  const ownerHue = $derived(((list?.userId ?? 0) * 47) % 360);
</script>

<svelte:head><title>{list?.title ?? 'Listă'} · Liste · Anime-Kage</title></svelte:head>

<div class="container detail">
  <a class="back" href="/liste">← Toate listele</a>

  {#if error}
    <div class="empty">
      <p class="empty-t">{error}</p>
      <p class="empty-p">Lista poate fi privată sau ștearsă.</p>
    </div>
  {:else if !list}
    <p class="loading">Se încarcă…</p>
  {:else}
    <header class="head">
      <div class="head-main">
        <div class="byline">
          <a class="avatar sm monogram" href={`/user/${list.ownerName}`} style={`--mg-hue:${ownerHue}`}>{list.ownerName.charAt(0)}</a>
          <span class="by-t">o listă de <a class="plink" href={`/user/${list.ownerName}`}><strong>{list.ownerName}</strong></a></span>
          {#if !list.isPublic}<span class="privpill">privată</span>{/if}
        </div>

        {#if editing}
          <form class="editform" onsubmit={saveEdit}>
            <input class="e-title" type="text" maxlength="120" bind:value={eTitle} />
            <textarea class="e-desc" rows="2" placeholder="Descriere…" bind:value={eDesc}></textarea>
            <div class="e-foot">
              <label class="e-pub">
                <input type="checkbox" bind:checked={ePublic} />
                <span>publică</span>
              </label>
              <button class="btn" type="button" onclick={() => (editing = false)}>Anulează</button>
              <button class="btn fill" type="submit" disabled={busy || !eTitle.trim()}>Salvează</button>
            </div>
          </form>
        {:else}
          <h1>{list.title}</h1>
          {#if list.description}<p class="desc">{list.description}</p>{/if}
          <div class="actions">
            <span class="stats">{items.length} titluri</span>
            <button class="btn like" class:on={list.liked} disabled={likeBusy} onclick={toggleLike}>
              {list.liked ? '♥' : '♡'}
              {list.likeCount}
            </button>
            {#if isOwner}
              <button class="btn" onclick={startEdit}>Editează</button>
              <button class="btn" onclick={() => (adding = !adding)}>{adding ? '× Închide' : '+ Adaugă titluri'}</button>
              <button class="btn danger" onclick={removeList}>Șterge lista</button>
            {/if}
          </div>
        {/if}
      </div>
      {#if items.length}
        <div class="fan" aria-hidden="true">
          {#each items.slice(0, 5).filter((it) => it.imageUrl) as it (it.id)}
            <span class="fan-cover media-tone" style={`background-image:url(${mediaUrl(it.imageUrl)})`}></span>
          {/each}
        </div>
      {/if}
    </header>

    {#if isOwner && adding}
      <div class="addpanel">
        <div class="ap-head">
          <div class="mtoggle" role="tablist" aria-label="Tip titlu">
            <button type="button" class="mopt" class:on={medium === 'anime'} onclick={() => switchMedium('anime')}>Anime</button>
            <button type="button" class="mopt" class:on={medium === 'manga'} onclick={() => switchMedium('manga')}>Manga</button>
          </div>
          <input
            class="ap-q"
            type="text"
            placeholder="Caută după titlu…"
            bind:value={query}
            oninput={onQuery}
            autocomplete="off"
          />
        </div>
        {#if results.length}
          <div class="ap-hits">
            {#each results as t (t.id)}
              {@const key = `${medium}:${t.id}`}
              <div class="ap-hit">
                {#if t.imageUrl}
                  <span class="ap-thumb media-tone" style={`background-image:url(${mediaUrl(t.imageUrl)})`}></span>
                {:else}
                  <span class="ap-thumb"></span>
                {/if}
                <span class="ap-main">
                  <span class="ap-t">{displayName(t)}</span>
                  <span class="ap-m">{t.year ?? '—'}</span>
                </span>
                <button
                  class="add"
                  class:added={onList.has(key)}
                  disabled={addBusy === t.id || onList.has(key)}
                  title={onList.has(key) ? 'Deja pe listă' : 'Adaugă pe listă'}
                  onclick={() => addTitle(t)}
                >{onList.has(key) ? '✓' : '+'}</button>
              </div>
            {/each}
          </div>
        {:else if query.trim().length >= 2}
          <p class="ap-none">Niciun rezultat în catalog.</p>
        {/if}
      </div>
    {/if}

    <div class="items">
      {#each pagedItems as it, i (it.id)}
        <div class="item">
          <span class="rank">{(itemPage - 1) * ITEMS_PER + i + 1}</span>
          <a class="thumb media-tone" href={itemHref(it)} style={it.imageUrl ? `background-image:url(${mediaUrl(it.imageUrl)})` : ''}></a>
          <div class="item-main">
            <a class="item-t" href={itemHref(it)}>{it.titleRomanian ?? it.title}</a>
            <p class="item-m">{it.year ?? '—'} · {it.animeId ? 'anime' : 'manga'}{it.genres.length ? ` · ${it.genres.slice(0, 3).join(', ')}` : ''}</p>
            {#if noteFor === it.id}
              <div class="noteedit">
                <input
                  type="text"
                  maxlength="500"
                  placeholder="De ce e pe listă…"
                  bind:value={noteDraft}
                  onkeydown={(e) => e.key === 'Enter' && saveNote(it)}
                />
                <button class="btn sm fill" onclick={() => saveNote(it)}>Salvează</button>
              </div>
            {:else if it.note}
              <p class="note">„{it.note}”</p>
            {/if}
          </div>
          {#if it.score}<span class="score">★ {it.score.toFixed(2)}</span>{/if}
          {#if isOwner}
            <button class="ibtn" title="Notă pe titlu" onclick={() => openNote(it)}>✎</button>
            <button class="ibtn del" title="Scoate de pe listă" onclick={() => removeItem(it)}>×</button>
          {/if}
        </div>
      {:else}
        <div class="empty">
          <p class="empty-t">Lista e goală</p>
          <p class="empty-p">
            {isOwner ? 'Apasă „+ Adaugă titluri” și umple-o.' : 'Autorul nu a adăugat încă nimic.'}
          </p>
        </div>
      {/each}
    </div>
    {#if itemPages > 1}
      <nav class="pager">
        <button class="btn" disabled={itemPage === 1} onclick={() => (itemPage -= 1)}>← Anterior</button>
        <PagePicker page={itemPage} pages={itemPages} onselect={(n) => (itemPage = n)} />
        <button class="btn" disabled={itemPage === itemPages} onclick={() => (itemPage += 1)}>Următor →</button>
      </nav>
    {/if}
  {/if}
</div>

<style>
  .detail { padding-block: var(--space-6) var(--space-8); max-width: 900px; }
  .back { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .back:hover { color: var(--text-primary); }
  .loading { color: var(--text-muted); padding: 40px 0; }

  .head {
    display: flex; align-items: flex-start; justify-content: space-between;
    gap: var(--space-5); margin-top: 22px; padding-bottom: 26px;
    border-bottom: 2px solid var(--text-primary);
  }
  .head-main { flex: 1; min-width: 0; }
  .byline { display: flex; align-items: center; gap: 9px; margin-bottom: 12px; }
  .by-t { font-size: 0.84375rem; color: var(--text-muted); }
  .plink strong { color: var(--text-primary); font-weight: var(--fw-semibold); }
  .plink:hover strong { color: var(--accent); }
  .privpill {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.08em; text-transform: uppercase; color: var(--warning);
    border: 1px solid color-mix(in srgb, var(--warning) 45%, transparent);
    border-radius: var(--radius-pill); padding: 2px 9px;
  }
  h1 {
    font-family: var(--font-display); font-size: clamp(1.7rem, 1.4rem + 1.5vw, 2.25rem);
    letter-spacing: -0.015em; line-height: 1.1;
  }
  .desc { font-size: 0.90625rem; line-height: 1.55; color: var(--text-muted); margin-top: 10px; max-width: 56ch; }
  .actions { display: flex; align-items: center; gap: 10px; margin-top: 18px; flex-wrap: wrap; }
  .stats { font-family: var(--font-mono); font-size: 0.75rem; color: var(--text-muted); margin-right: 4px; }

  .avatar {
    width: 30px; height: 30px; border-radius: 50%;
    display: grid; place-items: center;
    font-size: 0.75rem; font-weight: var(--fw-bold); color: #fff;
  }
  .avatar.sm { width: 26px; height: 26px; font-size: var(--fs-micro); }

  .fan { display: flex; padding-right: 22px; flex: 0 0 auto; }
  .fan-cover {
    width: 74px; height: 110px; border-radius: 6px; flex: 0 0 auto;
    margin-left: -22px; border: 2px solid var(--surface-base);
    background-color: var(--surface-overlay);
    background-size: cover; background-position: center 20%;
    box-shadow: 0 8px 18px rgba(0, 0, 0, 0.35);
  }
  .fan-cover:first-child { margin-left: 0; }

  .btn {
    font-weight: var(--fw-semibold); font-size: 0.8125rem;
    padding: 9px 15px; border-radius: 9px; cursor: pointer;
    border: 1px solid var(--border-default); background: transparent; color: var(--text-primary);
  }
  .btn:hover { background: var(--surface-raised); border-color: var(--text-primary); }
  .btn.fill { background: var(--accent); color: var(--on-accent); border-color: var(--accent); }
  .btn.fill:hover { background: var(--accent-hover); }
  .btn.danger { color: var(--danger); border-color: color-mix(in srgb, var(--danger) 45%, transparent); }
  .btn.danger:hover { background: color-mix(in srgb, var(--danger) 10%, transparent); }
  .btn.sm { padding: 6px 12px; font-size: 0.75rem; }
  .btn:disabled { opacity: 0.5; cursor: not-allowed; }
  .btn.like { display: inline-flex; align-items: center; gap: 6px; font-variant-numeric: tabular-nums; }
  .btn.like.on {
    color: var(--accent); border-color: color-mix(in srgb, var(--accent) 55%, transparent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }
  .pager { display: flex; align-items: center; justify-content: center; gap: 16px; margin-top: 28px; }
  .pg-info { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }

  /* edit form */
  .editform { display: flex; flex-direction: column; gap: 12px; }
  .e-title {
    font-family: var(--font-display); font-size: 1.4rem; font-weight: var(--fw-semibold);
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: 9px; color: var(--text-primary); padding: 10px 14px; outline: none;
  }
  .e-desc {
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: 9px; color: var(--text-primary); padding: 10px 14px;
    font-size: var(--fs-small); font-family: var(--font-body); resize: vertical; outline: none;
  }
  .e-title:focus, .e-desc:focus { border-color: var(--accent); }
  .e-foot { display: flex; align-items: center; gap: 10px; }
  .e-pub {
    display: flex; align-items: center; gap: 7px; cursor: pointer; flex: 1;
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
  }
  .e-pub input { accent-color: var(--accent); cursor: pointer; }

  /* add panel */
  .addpanel {
    border: 1px solid var(--border-strong); border-radius: 14px;
    background: var(--surface-raised); padding: 18px; margin-top: 26px;
  }
  .ap-head { display: flex; gap: 12px; align-items: center; flex-wrap: wrap; }
  .mtoggle {
    display: inline-flex; border: 1px solid var(--border-default);
    border-radius: var(--radius-pill); overflow: hidden; flex: 0 0 auto;
  }
  .mopt {
    background: none; border: 0; cursor: pointer; color: var(--text-muted);
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.06em; text-transform: uppercase; padding: 8px 14px;
  }
  .mopt.on { background: var(--accent); color: var(--on-accent); }
  .mopt:not(.on):hover { color: var(--text-primary); background: var(--surface-overlay); }
  .ap-q {
    flex: 1; min-width: 200px;
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: 9px; color: var(--text-primary);
    padding: 10px 14px; font-size: var(--fs-small); outline: none;
  }
  .ap-q:focus { border-color: var(--accent); }
  .ap-hits { margin-top: 14px; border-top: 1px solid var(--border-subtle); }
  .ap-hit { display: flex; align-items: center; gap: 13px; padding: 10px 2px; border-bottom: 1px solid var(--border-subtle); }
  .ap-thumb {
    width: 34px; height: 50px; border-radius: 5px; flex: 0 0 auto;
    background-color: var(--surface-overlay); background-size: cover; background-position: center 20%;
  }
  .ap-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
  .ap-t { font-size: var(--fs-small); font-weight: var(--fw-semibold); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .ap-m { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .ap-none { font-size: var(--fs-caption); color: var(--text-muted); margin-top: 12px; }

  .add {
    width: 34px; height: 34px; border-radius: 50%; flex: 0 0 auto;
    display: grid; place-items: center; cursor: pointer;
    border: 1px solid var(--border-default); background: transparent;
    color: var(--text-primary); font-size: 1rem;
  }
  .add:hover:not(:disabled) { border-color: var(--accent); color: var(--accent); }
  .add.added { background: color-mix(in srgb, var(--accent) 14%, transparent); border-color: var(--accent); color: var(--accent); }
  .add:disabled { cursor: default; opacity: 0.8; }

  /* items */
  .items { margin-top: 26px; }
  .item {
    display: flex; align-items: center; gap: 16px;
    padding: 14px 2px; border-bottom: 1px solid var(--border-subtle);
  }
  .rank {
    font-family: var(--font-mono); font-size: 0.75rem; color: var(--text-muted);
    width: 1.6rem; flex: 0 0 auto; text-align: right;
  }
  .thumb {
    width: 46px; height: 68px; border-radius: 6px; flex: 0 0 auto;
    background-color: var(--surface-overlay); background-size: cover; background-position: center 20%;
  }
  .item-main { flex: 1; min-width: 0; }
  .item-t { font-family: var(--font-display); font-size: 1.0625rem; font-weight: var(--fw-semibold); color: var(--text-primary); }
  .item-t:hover { color: var(--accent); }
  .item-m { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); margin-top: 4px; }
  .note { font-size: 0.8125rem; font-style: italic; color: var(--text-muted); margin-top: 7px; }
  .noteedit { display: flex; gap: 8px; margin-top: 8px; }
  .noteedit input {
    flex: 1; background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: 7px; color: var(--text-primary); padding: 7px 11px;
    font-size: var(--fs-caption); outline: none;
  }
  .noteedit input:focus { border-color: var(--accent); }
  .score { font-family: var(--font-mono); font-size: 0.75rem; color: var(--accent); flex: 0 0 auto; }
  .ibtn {
    background: none; border: 0; cursor: pointer; color: var(--text-muted);
    font-size: 0.95rem; padding: 4px 7px; border-radius: 6px; flex: 0 0 auto; line-height: 1;
  }
  .ibtn:hover { color: var(--text-primary); background: var(--surface-overlay); }
  .ibtn.del:hover { color: var(--danger); background: color-mix(in srgb, var(--danger) 10%, transparent); }

  .empty {
    text-align: center; padding: 60px 20px; margin-top: 26px;
    border: 1px dashed var(--border-default); border-radius: 16px;
  }
  .empty-t { font-family: var(--font-display); font-size: 1.25rem; font-weight: var(--fw-semibold); }
  .empty-p { font-size: var(--fs-small); color: var(--text-muted); margin-top: 8px; }

  @media (max-width: 640px) {
    .fan { display: none; }
  }
</style>
