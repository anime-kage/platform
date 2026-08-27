<script lang="ts">
  import { mediaUrl } from '$lib/media';
  import api from '$lib/api';
  import { toast } from '$lib/stores/toast';
  import { displayName } from '$lib/types';
  import type {
    Anime,
    Manga,
    CuratedPick,
    CuratedRef,
    CuratedSlotDef,
    UserList
  } from '$shared/types';

  // Vitrină: choose what the four editorial surfaces show.
  // The slot definitions — how many titles, which media type — come from the
  // API rather than being restated here, so this page cannot disagree with
  // what the server will accept.
  let slots = $state<CuratedSlotDef[]>([]);
  let picks = $state<Record<string, CuratedPick[]>>({});
  let loading = $state(true);

  // which slot's search panel is open; only one at a time keeps the page calm
  let openSlot = $state<string | null>(null);
  let q = $state('');
  let searchMedia = $state<'anime' | 'manga'>('anime');
  let results = $state<(Anime | Manga)[]>([]);
  let searching = $state(false);
  let saving = $state<string | null>(null);

  // A slot whose media is "list" features a member's list, not a title. The
  // public feed already carries owner and item count, so it is filtered in the
  // browser rather than adding a search endpoint for a handful of rows.
  let lists = $state<UserList[]>([]);
  const listResults = $derived(
    (() => {
      const needle = q.trim().toLowerCase();
      if (!needle) return lists;
      return lists.filter(
        (l) =>
          l.title.toLowerCase().includes(needle) ||
          l.ownerName.toLowerCase().includes(needle)
      );
    })()
  );

  async function loadAll() {
    loading = true;
    try {
      const r = await api.getCurated();
      slots = r.slots;
      picks = r.data;
    } catch {
      toast.error('Nu am putut încărca vitrina.');
    } finally {
      loading = false;
    }
  }
  $effect(() => {
    void loadAll();
  });

  function slotDef(key: string) {
    return slots.find((s) => s.key === key);
  }
  function itemsOf(key: string): CuratedPick[] {
    return picks[key] ?? [];
  }
  function titleOf(p: CuratedPick): Anime | Manga {
    return (p.anime ?? p.manga) as Anime | Manga;
  }
  function kindOf(p: CuratedPick): 'anime' | 'manga' | 'list' {
    if (p.list) return 'list';
    return p.anime ? 'anime' : 'manga';
  }
  /** The id to send back for a pick, whichever kind it is. */
  function idOf(p: CuratedPick): number {
    return p.list ? p.list.id : titleOf(p).id;
  }

  function openPicker(key: string) {
    const def = slotDef(key);
    openSlot = openSlot === key ? null : key;
    q = '';
    results = [];
    // a slot locked to one media type shouldn't offer the other
    searchMedia = def?.media === 'manga' ? 'manga' : 'anime';
    if (!openSlot) return;
    if (def?.media === 'list') void loadLists();
    else void search();
  }

  async function loadLists() {
    searching = true;
    try {
      lists = (await api.getPublicLists()).data;
    } catch {
      toast.error('Nu am putut încărca listele.');
    } finally {
      searching = false;
    }
  }

  /** A list slot holds exactly one, so choosing always replaces. */
  async function addList(key: string, l: UserList) {
    await save(key, [{ mediaType: 'list', id: l.id }]);
  }

  async function search(e?: SubmitEvent) {
    e?.preventDefault();
    searching = true;
    try {
      // The catalog, not Jikan: a pick has to render with our own poster and
      // synopsis, so it must already be imported. Import lives in /admin/catalog.
      if (searchMedia === 'anime') {
        const r = q.trim()
          ? await api.searchAnime(q, { limit: 12 })
          : await api.getAnime({ limit: 12 } as never);
        results = r.data;
      } else {
        const r = q.trim()
          ? await api.searchManga(q, { limit: 12 })
          : await api.getManga({ limit: 12 } as never);
        results = r.data;
      }
    } catch {
      toast.error('Căutarea a eșuat.');
    } finally {
      searching = false;
    }
  }

  // Carries imageUrl through: a slot write is a full replace, so dropping it
  // here would wipe the chosen artwork on every reorder or removal.
  function refsOf(key: string): CuratedRef[] {
    return itemsOf(key).map((p) => ({
      mediaType: kindOf(p),
      id: idOf(p),
      imageUrl: p.imageUrl
    }));
  }

  async function save(key: string, items: CuratedRef[]) {
    saving = key;
    try {
      const r = await api.setCurated(key, items);
      picks = { ...picks, [key]: r.data };
      toast.success('Vitrina a fost actualizată.');
    } catch (e) {
      toast.error((e as { error?: string })?.error ?? 'Nu am putut salva.');
    } finally {
      saving = null;
    }
  }

  async function add(key: string, t: Anime | Manga) {
    const def = slotDef(key);
    const current = refsOf(key);
    if (current.some((r) => r.id === t.id && r.mediaType === searchMedia)) {
      toast.error('Titlul este deja în listă.');
      return;
    }
    // A single-pick slot replaces rather than refuses — clicking a new title
    // when one is already chosen obviously means "use this one instead".
    const next =
      def && def.max === 1
        ? [{ mediaType: searchMedia, id: t.id }]
        : [...current, { mediaType: searchMedia, id: t.id }];
    if (def && next.length > def.max) {
      toast.error(`Maxim ${def.max} titluri pentru acest plasament.`);
      return;
    }
    await save(key, next);
  }

  async function remove(key: string, index: number) {
    const next = refsOf(key).filter((_, i) => i !== index);
    await save(key, next);
  }

  async function move(key: string, index: number, delta: number) {
    const next = refsOf(key);
    const to = index + delta;
    if (to < 0 || to >= next.length) return;
    [next[index], next[to]] = [next[to], next[index]];
    await save(key, next);
  }

  // ── per-placement artwork ──────────────────────────────────────────────────
  // Overrides the poster for this block only. The series keeps its own cover
  // everywhere else — cards, lists, the detail page.

  let uploading = $state<string | null>(null); // `${slot}:${index}`

  async function pickImage(key: string, index: number, e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = ''; // so re-picking the same file fires again
    if (!file) return;

    const tag = `${key}:${index}`;
    uploading = tag;
    try {
      const { imageUrl } = await api.uploadCuratedImage(file);
      const next = refsOf(key);
      next[index] = { ...next[index], imageUrl };
      await save(key, next);
    } catch (err) {
      toast.error((err as { error?: string })?.error ?? 'Încărcarea a eșuat.');
    } finally {
      uploading = null;
    }
  }

  async function clearImage(key: string, index: number) {
    const next = refsOf(key);
    next[index] = { ...next[index], imageUrl: undefined };
    await save(key, next);
  }
</script>

<div class="curated">
  <header class="intro">
    <span class="kicker">Vitrină</span>
    <p class="muted">
      Ce apare pe paginile publice. Un plasament gol se completează automat cu
      titlurile cele mai bine notate — golește-l ca să revii la comportamentul
      automat.
    </p>
    <p class="muted">
      Poți încărca o imagine proprie pentru fiecare plasament — blocurile sunt
      late, iar o copertă verticală arată adesea prost în ele. Imaginea se
      aplică <strong>doar aici</strong>; coperta seriei rămâne neschimbată în
      catalog, în liste și pe pagina ei.
    </p>
  </header>

  {#if loading}
    <p class="muted">Se încarcă…</p>
  {:else}
    {#each slots as def (def.key)}
      {@const chosen = itemsOf(def.key)}
      <section class="slot">
        <div class="s-head">
          <div>
            <h2>{def.label}</h2>
            <p class="muted">{def.hint}</p>
          </div>
          <div class="s-actions">
            <span class="count" class:auto={chosen.length === 0}>
              {chosen.length === 0 ? 'automat' : `${chosen.length}/${def.max}`}
            </span>
            <button class="btn ghost" onclick={() => openPicker(def.key)}>
              {openSlot === def.key ? 'Închide' : 'Alege titluri'}
            </button>
            {#if chosen.length > 0}
              <button
                class="btn ghost danger"
                disabled={saving === def.key}
                onclick={() => save(def.key, [])}
              >
                Golește
              </button>
            {/if}
          </div>
        </div>

        {#if chosen.length === 0}
          <p class="empty">Nimic ales — pagina alege singură.</p>
        {:else}
          <ul class="chosen">
            {#each chosen as p, i (kindOf(p) + idOf(p))}
              <li class="pick">
                {#if p.list}
                  <!-- A list has no single poster; its first item cover stands
                       in, the same art the card fan shows on /liste. -->
                  <span
                    class="p-thumb media-tone"
                    style={`background-image:url(${mediaUrl(p.list.covers?.[0] ?? '')})`}
                  ></span>
                  <span class="p-main">
                    <span class="p-t">{p.list.title}</span>
                    <span class="p-m">
                      Listă · {p.list.ownerName} · {p.list.itemCount} titluri
                    </span>
                  </span>
                {:else}
                  {@const t = titleOf(p)}
                  <span
                    class="p-thumb media-tone"
                    class:custom={!!p.imageUrl}
                    style={`background-image:url(${mediaUrl(p.imageUrl ?? t.imageUrl ?? '')})`}
                  ></span>
                  <span class="p-main">
                    <span class="p-t">{displayName(t)}</span>
                    <span class="p-m">
                      {kindOf(p) === 'anime' ? 'Anime' : 'Manga'} · {t.year ?? '—'}
                      {#if p.imageUrl}<em class="tag">imagine proprie</em>{/if}
                    </span>
                  </span>
                {/if}

                <span class="p-tools">
                  {#if def.max > 1}
                    <button
                      class="ib"
                      aria-label="Mută mai sus"
                      title="Mută mai sus"
                      disabled={i === 0 || saving === def.key}
                      onclick={() => move(def.key, i, -1)}>↑</button
                    >
                    <button
                      class="ib"
                      aria-label="Mută mai jos"
                      title="Mută mai jos"
                      disabled={i === chosen.length - 1 || saving === def.key}
                      onclick={() => move(def.key, i, 1)}>↓</button
                    >
                  {/if}

                  <label class="btn ghost sm upload" class:busy={uploading === `${def.key}:${i}`}>
                    {uploading === `${def.key}:${i}`
                      ? 'Se încarcă…'
                      : p.imageUrl
                        ? 'Schimbă imaginea'
                        : 'Imagine proprie'}
                    <input
                      type="file"
                      accept="image/*"
                      disabled={saving === def.key || uploading !== null}
                      onchange={(e) => pickImage(def.key, i, e)}
                    />
                  </label>

                  {#if p.imageUrl}
                    <button
                      class="btn ghost sm"
                      disabled={saving === def.key}
                      onclick={() => clearImage(def.key, i)}>Revino la copertă</button
                    >
                  {/if}

                  <button
                    class="btn ghost sm danger"
                    disabled={saving === def.key}
                    onclick={() => remove(def.key, i)}>Scoate</button
                  >
                </span>
              </li>
            {/each}
          </ul>
        {/if}

        {#if openSlot === def.key}
          <div class="picker">
            <div class="find">
              {#if def.media === ''}
                <div class="seg" role="tablist" aria-label="Tip de titlu">
                  <button
                    class="seg-b"
                    class:on={searchMedia === 'anime'}
                    onclick={() => {
                      searchMedia = 'anime';
                      search();
                    }}>Anime</button
                  >
                  <button
                    class="seg-b"
                    class:on={searchMedia === 'manga'}
                    onclick={() => {
                      searchMedia = 'manga';
                      search();
                    }}>Manga</button
                  >
                </div>
              {/if}
              <form class="search" onsubmit={def.media === 'list' ? (e) => e.preventDefault() : search}>
                <input
                  type="search"
                  bind:value={q}
                  placeholder={def.media === 'list'
                    ? 'Filtrează după membru sau titlul listei…'
                    : searchMedia === 'anime'
                      ? 'Caută un anime din catalog…'
                      : 'Caută o manga din catalog…'}
                />
                {#if def.media !== 'list'}
                  <button class="btn fill" type="submit" disabled={searching}>
                    {searching ? '…' : 'Caută'}
                  </button>
                {/if}
              </form>
            </div>

            {#if def.media === 'list'}
              <!-- Lists are loaded once and filtered in the browser: there are
                   few of them, and the feed already carries owner and count. -->
              {#if searching && lists.length === 0}
                <p class="muted">Se încarcă listele…</p>
              {:else if listResults.length === 0}
                <p class="muted">
                  Nicio listă publică găsită. Listele se creează de membri în
                  <a href="/liste">Liste</a>.
                </p>
              {:else}
                <ul class="results">
                  {#each listResults as l (l.id)}
                    <li>
                      <button
                        class="result"
                        disabled={saving === def.key}
                        onclick={() => addList(def.key, l)}
                      >
                        <span
                          class="r-thumb media-tone"
                          style={l.covers?.[0] ? `background-image:url(${mediaUrl(l.covers[0])})` : ''}
                        ></span>
                        <span class="r-main">
                          <span class="r-t">{l.title}</span>
                          <span class="r-m">{l.ownerName} · {l.itemCount} titluri</span>
                        </span>
                        <span class="r-go">Alege</span>
                      </button>
                    </li>
                  {/each}
                </ul>
              {/if}
            {:else if searching && results.length === 0}
              <p class="muted">Se caută…</p>
            {:else if results.length === 0}
              <p class="muted">
                Niciun titlu găsit. Titlurile trebuie să fie deja în catalog —
                importă-le din <a href="/admin/catalog">Catalog</a>.
              </p>
            {:else}
              <ul class="results">
                {#each results as r (r.id)}
                  <li>
                    <button class="result" disabled={saving === def.key} onclick={() => add(def.key, r)}>
                      <span
                        class="r-thumb media-tone"
                        style={r.imageUrl ? `background-image:url(${mediaUrl(r.imageUrl)})` : ''}
                      ></span>
                      <span class="r-main">
                        <span class="r-t">{displayName(r)}</span>
                        <span class="r-m">{r.year ?? '—'} · {r.type ?? '—'}</span>
                      </span>
                      <span class="r-go">Alege</span>
                    </button>
                  </li>
                {/each}
              </ul>
            {/if}
          </div>
        {/if}
      </section>
    {/each}
  {/if}
</div>

<style>
  .curated { padding-top: var(--space-5); display: flex; flex-direction: column; gap: var(--space-5); }
  .kicker {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-bold);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--accent);
  }
  .muted { color: var(--text-muted); font-size: var(--fs-small); line-height: 1.55; }
  .intro { display: flex; flex-direction: column; gap: 8px; max-width: 62ch; }

  .slot {
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg); padding: var(--space-5);
  }
  .s-head {
    display: flex; align-items: flex-start; justify-content: space-between;
    gap: var(--space-4); flex-wrap: wrap;
  }
  .s-head h2 { font-size: var(--fs-h4); letter-spacing: -0.01em; }
  .s-head .muted { margin-top: 4px; max-width: 52ch; }
  .s-actions { display: flex; align-items: center; gap: var(--space-3); flex-wrap: wrap; }
  .count {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.08em; text-transform: uppercase; color: var(--accent);
  }
  .count.auto { color: var(--text-faint); }
  .danger { color: var(--danger); }

  .empty { color: var(--text-faint); font-size: var(--fs-small); margin-top: var(--space-4); }

  .chosen { list-style: none; margin-top: var(--space-4); display: flex; flex-direction: column; gap: 8px; }
  .pick {
    display: flex; align-items: center; gap: var(--space-3);
    background: var(--surface-inset); border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md); padding: 8px 12px;
  }
  .p-thumb, .r-thumb {
    width: 34px; height: 48px; flex: none; border-radius: 4px;
    background: var(--surface-overlay) center/cover no-repeat;
  }
  .p-main, .r-main { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; text-align: left; }
  .p-t, .r-t { font-weight: var(--fw-semibold); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .p-m, .r-m { font-size: var(--fs-caption); color: var(--text-muted); }

  /* .btn is per-page in this codebase, not global — without this block the
     buttons render as unstyled browser defaults */
  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 10px 18px; border-radius: var(--radius-md); white-space: nowrap; cursor: pointer;
    transition: background var(--motion-fast) var(--ease), border-color var(--motion-fast) var(--ease);
  }
  .btn.sm { padding: 7px 13px; font-size: var(--fs-caption); }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-primary); }
  .btn.ghost:hover { background: var(--surface-overlay); border-color: var(--border-strong); }
  .btn.ghost.danger { color: var(--danger); border-color: color-mix(in srgb, var(--danger) 40%, transparent); }
  .btn.ghost.danger:hover { background: color-mix(in srgb, var(--danger) 12%, transparent); }
  .btn:disabled { opacity: 0.6; cursor: wait; }

  .p-tools { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; justify-content: flex-end; }
  /* square icon buttons, sized to match .btn.sm's height so the row lines up */
  .ib {
    width: 30px; height: 30px; flex: none; border-radius: var(--radius-md); cursor: pointer;
    background: transparent; border: 1px solid var(--border-default);
    color: var(--text-primary); line-height: 1;
    transition: background var(--motion-fast) var(--ease), border-color var(--motion-fast) var(--ease);
  }
  .ib:hover:not(:disabled) { background: var(--surface-overlay); border-color: var(--border-strong); }
  .ib:disabled { opacity: 0.35; cursor: default; }

  /* a label wrapping a hidden file input — the only way to style the picker */
  .upload { display: inline-flex; align-items: center; }
  .upload input { display: none; }
  .upload.busy { opacity: 0.6; cursor: wait; }

  .tag {
    font-style: normal; font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.06em; text-transform: uppercase; color: var(--accent);
    margin-left: 6px;
  }
  /* a custom image is a deliberate choice — make it visible at a glance */
  .p-thumb.custom { box-shadow: 0 0 0 2px var(--accent); }

  .picker { margin-top: var(--space-5); border-top: 1px solid var(--border-subtle); padding-top: var(--space-4); }
  .find { display: flex; gap: var(--space-3); flex-wrap: wrap; margin-bottom: var(--space-4); }
  .seg {
    display: flex; gap: 2px; padding: 3px;
    background: var(--surface-inset); border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
  }
  .seg-b {
    padding: 6px 14px; border: none; border-radius: 6px; cursor: pointer;
    background: none; color: var(--text-muted); font: inherit; font-size: var(--fs-small);
  }
  .seg-b.on { background: var(--surface-overlay); color: var(--text-primary); }
  .search { display: flex; gap: 8px; flex: 1; min-width: 240px; }
  .search input {
    flex: 1; min-height: 40px; padding: 0 12px;
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary); outline: none;
  }
  .search input:focus { border-color: var(--accent); }

  .results { list-style: none; display: grid; gap: 6px; }
  .result {
    width: 100%; display: flex; align-items: center; gap: var(--space-3);
    padding: 8px 12px; cursor: pointer; font: inherit; text-align: left;
    background: var(--surface-inset); border: 1px solid transparent; border-radius: var(--radius-md);
    color: var(--text-primary);
  }
  .result:hover { border-color: var(--accent); }
  .result:disabled { opacity: 0.5; cursor: wait; }
  .r-go { font-size: var(--fs-small); font-weight: var(--fw-semibold); color: var(--accent); flex: none; }
</style>
