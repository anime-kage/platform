<script lang="ts">
  import { onMount } from 'svelte';
  import api from '$lib/api';
  import { mediaUrl } from '$lib/media';
  import { authStore as auth } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import { displayName } from '$lib/types';
  import type { Anime, ScheduleSlot } from '$shared/types';

  // "Programul săptămânii" — the team decides which episode lands when. This
  // replaced a schedule derived from MAL broadcast days, which described when a
  // series airs in Japan rather than when our subtitle goes live.
  const canEdit = $derived(
    $auth.isAuthenticated && ['admin', 'coordinator'].includes($auth.user?.role ?? '')
  );

  let slots = $state<ScheduleSlot[]>([]);
  let loading = $state(true);
  let busy = $state(false);

  // ── new slot ────────────────────────────────────────────────────────────────
  // Search the catalog, not Jikan: a slot points at an anime row, so the series
  // has to be imported already (that's /admin/catalog).
  let q = $state('');
  let results = $state<Anime[]>([]);
  let searching = $state(false);
  let picked = $state<Anime | null>(null);
  let episodeNumber = $state(1);
  let when = $state(''); // <input type="datetime-local"> — local wall time
  let note = $state('');

  // Editing an existing row happens in place; only one at a time.
  let editingId = $state<number | null>(null);
  let editEpisode = $state(1);
  let editWhen = $state('');
  let editNote = $state('');

  onMount(load);

  async function load() {
    loading = true;
    try {
      slots = (await api.getSchedule({ upcoming: true })).data;
    } catch {
      toast.error('Nu am putut încărca programul.');
    } finally {
      loading = false;
    }
  }

  async function search(e?: SubmitEvent) {
    e?.preventDefault();
    searching = true;
    try {
      const r = q.trim()
        ? await api.searchAnime(q, { limit: 12 })
        : await api.getAnime({ limit: 12 } as never);
      results = r.data;
    } catch {
      toast.error('Căutarea a eșuat.');
    } finally {
      searching = false;
    }
  }

  function pick(a: Anime) {
    picked = a;
    results = [];
    q = '';
    // Next unscheduled episode as a starting guess — the common case is
    // "the one after the last I scheduled".
    const mine = slots.filter((s) => s.animeId === a.id).map((s) => s.episodeNumber);
    episodeNumber = mine.length ? Math.max(...mine) + 1 : 1;
  }

  /**
   * `datetime-local` gives a wall-clock string with no zone. Constructing a
   * Date from it interprets it in the browser's zone — which is what the
   * coordinator meant — and toISOString() then sends the instant with an
   * explicit offset, which is what the API requires.
   */
  const toInstant = (local: string) => new Date(local).toISOString();

  /** The reverse, for pre-filling the input when editing. */
  function toLocalInput(iso: string) {
    const d = new Date(iso);
    const p = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
  }

  async function add() {
    if (!picked) return toast.error('Alege un anime.');
    if (!when) return toast.error('Alege data și ora.');
    busy = true;
    try {
      const res = await api.createScheduleSlot({
        animeId: picked.id,
        episodeNumber,
        scheduledAt: toInstant(when),
        note: note.trim()
      });
      // Upsert on the server: the same episode rescheduled replaces its row,
      // so reconcile by id rather than always prepending.
      const existing = slots.findIndex((s) => s.id === res.data.id);
      slots =
        existing === -1
          ? [...slots, res.data].sort(byWhen)
          : slots.map((s) => (s.id === res.data.id ? res.data : s)).sort(byWhen);
      toast.success('Programat.');
      picked = null;
      note = '';
      when = '';
    } catch (e) {
      toast.error(errText(e));
    } finally {
      busy = false;
    }
  }

  function startEdit(s: ScheduleSlot) {
    editingId = s.id;
    editEpisode = s.episodeNumber;
    editWhen = toLocalInput(s.scheduledAt);
    editNote = s.note ?? '';
  }

  async function saveEdit() {
    if (editingId === null) return;
    busy = true;
    try {
      const res = await api.updateScheduleSlot(editingId, {
        episodeNumber: editEpisode,
        scheduledAt: toInstant(editWhen),
        note: editNote.trim()
      });
      slots = slots.map((s) => (s.id === res.data.id ? res.data : s)).sort(byWhen);
      editingId = null;
      toast.success('Actualizat.');
    } catch (e) {
      toast.error(errText(e));
    } finally {
      busy = false;
    }
  }

  async function remove(s: ScheduleSlot) {
    if (!confirm(`Ștergi programarea pentru ${displayName(s)} — episodul ${s.episodeNumber}?`)) return;
    busy = true;
    try {
      await api.deleteScheduleSlot(s.id);
      slots = slots.filter((x) => x.id !== s.id);
      if (editingId === s.id) editingId = null;
      toast.success('Șters.');
    } catch (e) {
      toast.error(errText(e));
    } finally {
      busy = false;
    }
  }

  const byWhen = (a: ScheduleSlot, b: ScheduleSlot) =>
    new Date(a.scheduledAt).getTime() - new Date(b.scheduledAt).getTime();

  const stamp = (iso: string) =>
    new Date(iso).toLocaleString('ro-RO', {
      weekday: 'short',
      day: 'numeric',
      month: 'short',
      hour: '2-digit',
      minute: '2-digit'
    });

  /** The next 7 days are what /home shows — worth marking in the list. */
  const onHome = (iso: string) => {
    const at = new Date(iso).getTime();
    const now = Date.now();
    return at >= now - 86_400_000 && at < now + 7 * 86_400_000;
  };

  const errText = (e: unknown) =>
    (e as { error?: string; message?: string })?.error ??
    (e as { message?: string })?.message ??
    'Ceva n-a mers.';
</script>

{#if !canEdit}
  <p class="calm">Programul se stabilește de administratori și coordonatori.</p>
{:else}
  <section class="sect">
    <p class="s-label">
      Programează un episod
      <span class="s-count">· apare în „Programul săptămânii” pe pagina principală</span>
    </p>

    <div class="listcard form">
      {#if !picked}
        <form class="search" onsubmit={search}>
          <input type="search" bind:value={q} placeholder="Caută un anime din catalog…" />
          <button class="btn fill" type="submit" disabled={searching}>
            {searching ? '…' : 'Caută'}
          </button>
        </form>
        {#if results.length}
          <ul class="results">
            {#each results as a (a.id)}
              <li>
                <button class="res" onclick={() => pick(a)}>
                  <span
                    class="res-art media-tone"
                    style={a.imageUrl ? `background-image:url(${mediaUrl(a.imageUrl)})` : ''}
                  ></span>
                  <span class="res-t">{displayName(a)}</span>
                  <span class="res-m">{a.year ?? '—'}</span>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      {:else}
        <div class="chosen">
          <span
            class="res-art media-tone"
            style={picked.imageUrl ? `background-image:url(${mediaUrl(picked.imageUrl)})` : ''}
          ></span>
          <span class="res-t">{displayName(picked)}</span>
          <button class="linklike" onclick={() => (picked = null)}>schimbă</button>
        </div>

        <div class="row three">
          <label class="field">
            <span class="lbl">Episodul</span>
            <input type="number" min="1" bind:value={episodeNumber} />
          </label>
          <label class="field">
            <span class="lbl">Când (ora ta)</span>
            <input type="datetime-local" bind:value={when} />
          </label>
          <label class="field">
            <span class="lbl">Notă (opțional)</span>
            <input bind:value={note} maxlength="120" placeholder="parte 1, întârziat o zi…" />
          </label>
        </div>

        <div class="form-foot">
          <span class="hint">
            Ora se salvează ca moment exact, deci fiecare membru o vede în fusul lui.
          </span>
          <button class="btn fill sm" onclick={add} disabled={busy}>
            {busy ? 'Se salvează…' : 'Programează'}
          </button>
        </div>
      {/if}
    </div>
  </section>

  <section class="sect">
    <p class="s-label">
      Programul
      <span class="s-count">· {slots.length} {slots.length === 1 ? 'episod' : 'episoade'} de acum înainte</span>
    </p>

    {#if loading}
      <p class="calm">Se încarcă…</p>
    {:else if slots.length === 0}
      <p class="calm">Nimic programat. Pagina principală arată un mesaj până adaugi primul episod.</p>
    {:else}
      <div class="listcard">
        {#each slots as s (s.id)}
          <div class="slot">
            <span
              class="res-art media-tone"
              style={s.imageUrl ? `background-image:url(${mediaUrl(s.imageUrl)})` : ''}
            ></span>
            <div class="slot-main">
              <div class="slot-head">
                <span class="slot-t">{displayName(s)}</span>
                <span class="pill">Ep {s.episodeNumber}</span>
                {#if s.published}<span class="pill ok">publicat</span>{/if}
                {#if onHome(s.scheduledAt)}<span class="pill on">pe home</span>{/if}
              </div>

              {#if editingId === s.id}
                <div class="row three edit">
                  <label class="field">
                    <span class="lbl">Episodul</span>
                    <input type="number" min="1" bind:value={editEpisode} />
                  </label>
                  <label class="field">
                    <span class="lbl">Când</span>
                    <input type="datetime-local" bind:value={editWhen} />
                  </label>
                  <label class="field">
                    <span class="lbl">Notă</span>
                    <input bind:value={editNote} maxlength="120" />
                  </label>
                </div>
              {:else}
                <p class="slot-m">
                  {stamp(s.scheduledAt)}{#if s.createdByName} · de {s.createdByName}{/if}
                </p>
                {#if s.note}<p class="slot-note">{s.note}</p>{/if}
              {/if}
            </div>

            <div class="slot-actions">
              {#if editingId === s.id}
                <button class="btn fill sm" onclick={saveEdit} disabled={busy}>Salvează</button>
                <button class="btn ghost sm" onclick={() => (editingId = null)} disabled={busy}>Renunță</button>
              {:else}
                <button class="btn ghost sm" onclick={() => startEdit(s)} disabled={busy}>Mută</button>
                <button class="btn ghost sm danger" onclick={() => remove(s)} disabled={busy}>Șterge</button>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </section>
{/if}

<style>
  .sect { margin-bottom: var(--space-6); }
  .s-label {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--text-muted);
    padding-bottom: 12px; border-bottom: 1px solid var(--border-default);
    margin-bottom: var(--space-4);
  }
  .s-count { color: var(--text-muted); font-weight: var(--fw-regular); letter-spacing: 0.06em; text-transform: none; }

  .calm {
    border: 1px dashed var(--border-default); border-radius: var(--radius-md);
    padding: var(--space-5); text-align: center; color: var(--text-muted); font-size: var(--fs-small);
  }
  .listcard {
    border: 1px solid var(--border-subtle); border-radius: var(--radius-lg);
    background: var(--surface-raised); overflow: hidden;
  }

  /* ---- form ---- */
  .form { padding: var(--space-5); display: flex; flex-direction: column; gap: var(--space-4); }
  .search { display: flex; gap: 8px; }
  .search input { flex: 1; }
  .row.three { display: grid; grid-template-columns: 7rem 14rem minmax(0, 1fr); gap: var(--space-4); }
  .field { display: flex; flex-direction: column; gap: 7px; }
  .lbl {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.12em; text-transform: uppercase; color: var(--text-muted);
  }
  input {
    width: 100%; padding: 10px 13px; font: inherit; font-size: var(--fs-small);
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary); outline: none;
  }
  input:focus { border-color: var(--accent); }

  .form-foot { display: flex; align-items: center; justify-content: space-between; gap: var(--space-4); flex-wrap: wrap; }
  .hint { font-size: var(--fs-caption); color: var(--text-muted); }

  .results { display: flex; flex-direction: column; gap: 2px; }
  .res {
    display: flex; align-items: center; gap: 11px; width: 100%; text-align: left;
    background: none; border: none; cursor: pointer; padding: 7px 9px;
    border-radius: var(--radius-sm); color: var(--text-primary); font: inherit;
  }
  .res:hover { background: var(--surface-overlay); }
  .res-art {
    flex: 0 0 30px; height: 44px; border-radius: 4px;
    background-size: cover; background-position: center; background-color: var(--surface-overlay);
  }
  .res-t { flex: 1; min-width: 0; font-size: var(--fs-small); font-weight: var(--fw-semibold); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .res-m { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }

  .chosen { display: flex; align-items: center; gap: 11px; }
  .chosen .res-t { flex: 0 1 auto; }
  .linklike {
    background: none; border: none; padding: 0; cursor: pointer;
    font: inherit; font-size: var(--fs-caption); color: var(--accent); text-decoration: underline;
  }

  /* ---- list ---- */
  .slot { display: flex; align-items: flex-start; gap: 13px; padding: var(--space-4) var(--space-5); }
  .slot + .slot { border-top: 1px solid var(--border-subtle); }
  .slot-main { flex: 1; min-width: 0; }
  .slot-head { display: flex; align-items: baseline; gap: 9px; flex-wrap: wrap; }
  .slot-t { font-weight: var(--fw-semibold); color: var(--text-primary); }
  .slot-m { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); margin-top: 6px; }
  .slot-note { font-size: var(--fs-caption); color: var(--accent); margin-top: 4px; }
  .slot-actions { display: flex; gap: 7px; flex-wrap: wrap; flex: 0 0 auto; }
  .row.three.edit { margin-top: 10px; }

  .pill {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.08em; text-transform: uppercase; white-space: nowrap; color: var(--text-muted);
  }
  .pill.on { color: var(--accent); }
  .pill.ok { color: var(--success); }

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

  @media (max-width: 760px) {
    .row.three { grid-template-columns: minmax(0, 1fr); }
    .slot { flex-wrap: wrap; }
  }
</style>
