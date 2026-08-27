<script lang="ts">
  import { mediaUrl } from '$lib/media';
  import { goto } from '$app/navigation';
  import api from '$lib/api';
  import { nameHue } from '$lib/avatar';
  import { authStore } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import { displayName, type Anime, type Manga } from '$lib/types';
  import type { FavoriteRef } from '$shared/types';

  const auth = $derived($authStore);

  let username = $state('');
  let bio = $state('');
  let favs = $state<{ type: 'anime' | 'manga'; item: Anime | Manga }[]>([]);

  // Favourites are a ranked showcase — the order is what gets saved, so it has
  // to be changeable without removing and re-adding every title.
  let dragIdx = $state<number | null>(null);
  let overIdx = $state<number | null>(null);

  function moveFav(from: number, to: number) {
    if (from === to || from < 0 || to < 0 || from >= favs.length || to >= favs.length) return;
    const next = [...favs];
    const [moved] = next.splice(from, 1);
    next.splice(to, 0, moved);
    favs = next;
  }

  function onDrop(to: number) {
    if (dragIdx !== null) moveFav(dragIdx, to);
    dragIdx = null;
    overIdx = null;
  }

  /** Arrow keys move a focused favourite, so reordering is not mouse-only. */
  function onFavKey(e: KeyboardEvent, i: number) {
    const dir = e.key === 'ArrowLeft' ? -1 : e.key === 'ArrowRight' ? 1 : 0;
    if (!dir) return;
    e.preventDefault();
    const to = i + dir;
    if (to < 0 || to >= favs.length) return;
    moveFav(i, to);
    // keep focus on the card that just moved
    queueMicrotask(() => {
      const el = document.querySelectorAll<HTMLElement>('.fav')[to];
      el?.focus();
    });
  }
  let hydrated = $state(false);
  let saving = $state(false);
  let uploading = $state(false);

  // favorites picker
  let pickType = $state<'anime' | 'manga'>('anime');
  let pickQuery = $state('');
  let pickResults = $state<(Anime | Manga)[]>([]);
  let searching = $state(false);
  let searchSeq = 0;

  $effect(() => {
    if (auth.isLoading) return;
    if (!auth.isAuthenticated) {
      goto('/login?redirect=/profile/edit');
      return;
    }
    if (!hydrated && auth.user) {
      hydrated = true;
      username = auth.user.username;
      bio = auth.user.bio ?? '';
      // fetch favorites from the full profile — the auth session payload may omit them,
      // and hydrating from an empty list would wipe them on save
      api
        .getMyProfile()
        .then((res) => loadFavorites(res.user.favorites ?? []))
        .catch(() => loadFavorites(auth.user?.favorites ?? []));
    }
  });

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

  async function search() {
    const q = pickQuery.trim();
    if (q.length < 2) {
      pickResults = [];
      return;
    }
    const seq = ++searchSeq;
    searching = true;
    try {
      const res = pickType === 'anime' ? await api.searchAnime(q) : await api.searchManga(q);
      if (seq === searchSeq) pickResults = (res.data as (Anime | Manga)[]).slice(0, 6).map(numScore);
    } catch {
      if (seq === searchSeq) pickResults = [];
    } finally {
      if (seq === searchSeq) searching = false;
    }
  }

  let searchTimer: ReturnType<typeof setTimeout>;
  function onQueryInput() {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(search, 250);
  }

  function addFav(item: Anime | Manga) {
    if (favs.length >= 5) {
      toast.info('Maxim 5 favorite — scoate unul întâi.');
      return;
    }
    if (favs.some((f) => f.type === pickType && f.item.id === item.id)) return;
    favs = [...favs, { type: pickType, item }];
    pickQuery = '';
    pickResults = [];
  }

  function removeFav(i: number) {
    favs = favs.filter((_, idx) => idx !== i);
  }

  async function onAvatarChange(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    if (file.size > 2 * 1024 * 1024) {
      toast.error('Avatarul trebuie să fie sub 2MB.');
      return;
    }
    uploading = true;
    try {
      await api.uploadAvatar(file);
      await authStore.init();
      toast.success('Avatar actualizat.');
    } catch {
      toast.error('Nu am putut încărca avatarul.');
    } finally {
      uploading = false;
      input.value = '';
    }
  }

  async function save() {
    saving = true;
    try {
      await api.updateProfile({
        username: username.trim(),
        bio: bio.trim(),
        favorites: favs.map((f) => ({ type: f.type, id: f.item.id }))
      });
      await authStore.init();
      toast.success('Profil salvat.');
      goto('/profile');
    } catch {
      toast.error('Nu am putut salva profilul. Numele de utilizator e valid și liber?');
    } finally {
      saving = false;
    }
  }

  const initial = $derived(auth.user?.username?.[0]?.toUpperCase() ?? '?');
</script>

<svelte:head><title>Editează profilul · Anime-Kage</title></svelte:head>

{#if auth.user}
  <div class="container edit">
    <header class="top">
      <div>
        <p class="crumb"><a href="/profile">← Profilul tău</a></p>
        <h1>Editează profilul</h1>
      </div>
      <div class="top-actions">
        <button class="btn fill" onclick={save} disabled={saving}>{saving ? 'Se salvează…' : 'Salvează'}</button>
      </div>
    </header>

    <!-- IDENTITY -->
    <section class="sec">
      <h2 class="sect">Identitate</h2>
      <div class="id-grid">
        <div class="ava-col">
          {#if auth.user.avatarUrl}
            <img class="ava-img" src={api.resolveUrl(auth.user.avatarUrl)} alt={auth.user.username} />
          {:else}
            <span class="ava monogram" style={`--mg-hue:${nameHue(auth.user.username)}`}>{initial}</span>
          {/if}
          <label class="upload" class:busy={uploading}>
            {uploading ? 'Se încarcă…' : 'Schimbă avatarul'}
            <input type="file" accept="image/jpeg,image/png,image/webp,image/gif" onchange={onAvatarChange} disabled={uploading} />
          </label>
          <p class="hint">JPEG, PNG, WebP, GIF · max 2MB.<br />Se salvează imediat.</p>
        </div>
        <div class="fields">
          <label class="field">
            <span class="f-l">Nume de utilizator</span>
            <input type="text" bind:value={username} minlength="3" maxlength="50" />
          </label>
          <label class="field">
            <span class="f-l">Bio <span class="f-count">{bio.length}/500</span></span>
            <textarea bind:value={bio} rows="4" maxlength="500" placeholder="Câteva cuvinte despre tine…"></textarea>
          </label>
        </div>
      </div>
    </section>

    <!-- FAVORITES -->
    <section class="sec" id="favorite">
      <h2 class="sect">Favorite <span class="sect-m">· vitrina ta, până la 5 titluri</span></h2>

      {#if favs.length}
        <!-- listbox/option, not list/listitem: the cards take drag and arrow-key
             input, and a listitem is a non-interactive role. A listbox also
             makes arrow-key navigation the expected behaviour rather than a
             surprise. -->
        <div class="fav-row" role="listbox" aria-label="Favorite, în ordinea aleasă" aria-orientation="horizontal">
          {#each favs as f, i (f.type + f.item.id)}
            <div
              class="fav"
              class:dragging={dragIdx === i}
              class:over={overIdx === i && dragIdx !== i}
              draggable="true"
              role="option"
              aria-selected={dragIdx === i}
              tabindex="0"
              aria-label={`${displayName(f.item)} — poziția ${i + 1} din ${favs.length}. Trage sau folosește săgețile pentru a reordona.`}
              ondragstart={(e) => { dragIdx = i; e.dataTransfer?.setData('text/plain', String(i)); }}
              ondragover={(e) => { e.preventDefault(); overIdx = i; }}
              ondragleave={() => { if (overIdx === i) overIdx = null; }}
              ondrop={(e) => { e.preventDefault(); onDrop(i); }}
              ondragend={() => { dragIdx = null; overIdx = null; }}
              onkeydown={(e) => onFavKey(e, i)}
            >
              <span class="fav-rank">{i + 1}</span>
              {#if f.item.imageUrl}
                <img class="fav-img media-tone" src={mediaUrl(f.item.imageUrl)} alt={displayName(f.item)} />
              {:else}
                <span class="fav-img ph"></span>
              {/if}
              <span class="fav-t">{displayName(f.item)}</span>
              <button
                class="fav-x"
                title="Scoate din favorite"
                draggable="false"
                onpointerdown={(e) => e.stopPropagation()}
                onclick={(e) => { e.stopPropagation(); removeFav(i); }}
              >✕</button>
            </div>
          {/each}
        </div>
      {/if}

      {#if favs.length < 5}
        <div class="picker">
          <div class="seg">
            <button class="seg-b" class:on={pickType === 'anime'} onclick={() => { pickType = 'anime'; pickResults = []; if (pickQuery) search(); }}>Anime</button>
            <button class="seg-b" class:on={pickType === 'manga'} onclick={() => { pickType = 'manga'; pickResults = []; if (pickQuery) search(); }}>Manga</button>
          </div>
          <input
            class="pick-q"
            type="search"
            placeholder={`Caută ${pickType} de adăugat…`}
            bind:value={pickQuery}
            oninput={onQueryInput}
          />
        </div>
        {#if searching}
          <p class="hint dim">Se caută…</p>
        {:else if pickResults.length}
          <div class="results">
            {#each pickResults as r (r.id)}
              <button class="res" onclick={() => addFav(r)}>
                <span class="res-thumb media-tone" style={r.imageUrl ? `background-image:url(${mediaUrl(r.imageUrl)})` : ''}></span>
                <span class="res-main">
                  <span class="res-t">{displayName(r)}</span>
                  <span class="res-m">{r.year ?? '—'}{r.score ? ` · ★ ${Number(r.score).toFixed(2)}` : ''}</span>
                </span>
                <span class="res-add">+ adaugă</span>
              </button>
            {/each}
          </div>
        {:else if pickQuery.trim().length >= 2}
          <p class="hint dim">Niciun rezultat.</p>
        {/if}
      {/if}
    </section>

    <div class="actions">
      <button class="btn fill" onclick={save} disabled={saving}>{saving ? 'Se salvează…' : 'Salvează profilul'}</button>
      <a class="btn ghost" href="/profile">Renunță</a>
    </div>
  </div>
{/if}

<style>
  .edit { max-width: 760px; padding-block: var(--space-6) var(--space-8); }

  .top {
    display: flex; align-items: flex-end; justify-content: space-between;
    flex-wrap: wrap; gap: var(--space-4);
    padding-bottom: 18px; border-bottom: 2px solid var(--text-primary);
  }
  .crumb { font-family: var(--font-mono); font-size: var(--fs-caption); }
  .crumb a { color: var(--text-muted); }
  .crumb a:hover { color: var(--text-primary); }
  .top h1 { font-size: clamp(1.7rem, 1.4rem + 1.2vw, 2.125rem); letter-spacing: -0.015em; margin-top: 8px; }

  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-caption);
    padding: 10px 18px; border-radius: var(--radius-md); cursor: pointer; white-space: nowrap;
  }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-primary); }
  .btn.ghost:hover { background: var(--surface-raised); }
  .btn:disabled { opacity: 0.6; cursor: wait; }

  /* ---- sections: mono kickers on hairlines, no boxes ---- */
  .sec { padding-top: 34px; }
  .sec + .sec { margin-top: 34px; border-top: 1px solid var(--border-subtle); }
  .sect {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-medium);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--text-muted);
    margin-bottom: 20px;
  }
  .sect-m { letter-spacing: 0.04em; text-transform: none; }

  .hint { font-size: var(--fs-caption); color: var(--text-muted); line-height: 1.5; }
  .hint.dim { color: var(--text-muted); margin-top: 12px; }

  /* identity */
  .id-grid { display: grid; grid-template-columns: 150px 1fr; gap: clamp(20px, 4vw, 40px); }
  .ava-col { display: flex; flex-direction: column; align-items: flex-start; gap: 12px; }
  .ava, .ava-img { width: 96px; height: 96px; border-radius: 50%; border: 1px solid var(--border-default); }
  .ava {
    display: grid; place-items: center;
    background: linear-gradient(135deg, var(--accent), var(--accent-strong));
    font-family: var(--font-display); font-size: 2.375rem; font-weight: var(--fw-semibold); color: #fff;
  }
  .ava-img { object-fit: cover; }
  .upload {
    position: relative; display: inline-block; overflow: hidden; cursor: pointer;
    font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--accent);
  }
  .upload:hover { color: var(--accent-hover); }
  .upload input { position: absolute; inset: 0; opacity: 0; cursor: pointer; }
  .upload.busy { opacity: 0.6; pointer-events: none; }

  .fields { min-width: 0; }
  .field { display: block; margin-bottom: 18px; }
  .f-l {
    display: flex; justify-content: space-between; align-items: baseline;
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.08em; text-transform: uppercase; color: var(--text-muted); margin-bottom: 8px;
  }
  .f-count { letter-spacing: 0; }
  .field input, .field textarea, .pick-q {
    width: 100%; resize: vertical;
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-sm); padding: 11px 13px; color: var(--text-primary); outline: none;
    font-size: var(--fs-small);
  }
  .field input:focus, .field textarea:focus, .pick-q:focus { border-color: var(--accent); }

  /* favorites */
  .fav { position: relative; cursor: grab; }
  .fav:active { cursor: grabbing; }
  .fav.dragging { opacity: 0.45; }
  /* the card being hovered over gets the insertion edge, so the drop point is
     visible without moving anything until the pointer is released */
  .fav.over { outline: 2px solid var(--accent, #4db6ac); outline-offset: 2px; }
  .fav:focus-visible { outline: 2px solid var(--accent, #4db6ac); outline-offset: 2px; }
  .fav-rank {
    position: absolute; top: 4px; left: 4px; z-index: 2;
    min-width: 18px; height: 18px; padding: 0 4px;
    display: inline-flex; align-items: center; justify-content: center;
    border-radius: 9px; background: rgba(10, 13, 17, 0.78); color: #fff;
    font-size: 11px; font-variant-numeric: tabular-nums; line-height: 1;
  }
  @media (prefers-reduced-motion: reduce) {
    .fav { transition: none; }
  }
  .fav-row { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 14px; margin-bottom: 22px; }
  .fav { position: relative; }
  .fav-img {
    display: block; width: 100%; aspect-ratio: 2 / 3; object-fit: cover;
    border-radius: var(--radius-sm); border: 1px solid var(--border-subtle);
  }
  .fav-img.ph { background: var(--surface-overlay); }
  .fav-t {
    display: -webkit-box; margin-top: 7px; font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    color: var(--text-muted); line-height: 1.3;
    -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden;
  }
  .fav-x {
    position: absolute; top: 6px; right: 6px;
    width: 24px; height: 24px; border-radius: 50%; border: none; cursor: pointer;
    background: rgba(10, 10, 12, 0.65); color: #fff; font-size: 0.75rem; line-height: 1;
  }
  .fav-x:hover { background: var(--danger); }

  .picker { display: flex; gap: 10px; align-items: stretch; }
  .seg { display: flex; flex: 0 0 auto; border: 1px solid var(--border-default); border-radius: var(--radius-sm); overflow: hidden; }
  .seg-b {
    font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--text-muted);
    padding: 0 14px; border: none; background: none; cursor: pointer;
  }
  .seg-b + .seg-b { border-left: 1px solid var(--border-default); }
  .seg-b.on { background: var(--surface-overlay); color: var(--text-primary); }

  /* search results: flat hairline rows */
  .results { margin-top: 14px; display: flex; flex-direction: column; }
  .res {
    display: flex; align-items: center; gap: 14px; text-align: left;
    padding: 10px 0; border: none; border-bottom: 1px solid var(--border-subtle);
    background: none; cursor: pointer;
  }
  .res:first-child { border-top: 1px solid var(--border-subtle); }
  .res-thumb {
    width: 34px; height: 48px; border-radius: 4px; flex: 0 0 auto;
    background-color: var(--surface-overlay); background-size: cover; background-position: center;
  }
  .res-main { flex: 1; min-width: 0; display: flex; flex-direction: column; }
  .res-t {
    font-family: var(--font-display); font-size: var(--fs-small); font-weight: var(--fw-semibold);
    color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .res:hover .res-t { color: var(--accent); }
  .res-m { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); margin-top: 2px; }
  .res-add { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--accent); white-space: nowrap; }

  .actions {
    display: flex; gap: 10px; margin-top: 44px; padding-top: 22px;
    border-top: 1px solid var(--border-subtle);
  }

  @media (max-width: 620px) {
    .id-grid { grid-template-columns: minmax(0, 1fr); }
    .fav-row { grid-template-columns: repeat(3, minmax(0, 1fr)); }
    .picker { flex-direction: column; align-items: stretch; }
    .seg-b { flex: 1; padding-block: 9px; }
  }
</style>
