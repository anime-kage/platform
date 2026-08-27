<script lang="ts">
  import { mediaUrl } from '$lib/media';
  import api from '$lib/api';
  import { toast } from '$lib/stores/toast';
  import { displayName } from '$lib/types';
  import type { Anime, Episode, Subtitle } from '$shared/types';

  // Grab a published RO subtitle as .srt: find the series, pick the episode,
  // download. We store the published track as .vtt at publish time; the server
  // converts it to SubRip on the way out.
  let q = $state('');
  let results = $state<Anime[]>([]);
  let searching = $state(false);
  let searchedOnce = $state(false);

  let picked = $state<Anime | null>(null);
  let episodes = $state<Episode[]>([]);
  let loadingEps = $state(false);
  let epId = $state<number | ''>('');
  let subs = $state<Subtitle[]>([]);
  let loadingSubs = $state(false);

  async function search(e?: SubmitEvent) {
    e?.preventDefault();
    if (!q.trim()) return;
    searching = true;
    try {
      results = (await api.searchAnime(q, { limit: 18 })).data;
      searchedOnce = true;
    } catch {
      toast.error('Căutarea a eșuat.');
    } finally {
      searching = false;
    }
  }

  async function pick(a: Anime) {
    picked = a;
    results = [];
    episodes = [];
    epId = '';
    subs = [];
    loadingEps = true;
    try {
      episodes = (await api.getEpisodes(a.id)).data
        .slice()
        .sort((x, y) => x.episodeNumber - y.episodeNumber);
      if (episodes.length === 0) toast.info('Seria nu are episoade încă.');
    } catch {
      toast.error('Nu am putut încărca episoadele.');
    } finally {
      loadingEps = false;
    }
  }

  async function loadSubs() {
    subs = [];
    if (!epId) return;
    loadingSubs = true;
    try {
      subs = (await api.getEpisodeSubtitles(Number(epId))).data;
    } catch {
      toast.error('Nu am putut încărca subtitrările.');
    } finally {
      loadingSubs = false;
    }
  }

  function reset() {
    picked = null;
    episodes = [];
    epId = '';
    subs = [];
    subFile = null;
  }

  // ── direct attach ─────────────────────────────────────────────────────────
  let subFile = $state<File | null>(null);
  let upLang = $state('ro');
  let uploading = $state(false);
  let busyId = $state<number | null>(null);

  const errMsg = (err: unknown, fallback: string) =>
    (err as { error?: string }).error ?? (err as { message?: string }).message ?? fallback;

  async function upload(e: SubmitEvent) {
    e.preventDefault();
    if (!epId || !subFile) return;
    uploading = true;
    try {
      const r = await api.uploadEpisodeSubtitle(Number(epId), subFile, { language: upLang });
      toast.success(`Subtitrare atașată — ${r.cues} replici.`);
      subFile = null;
      // Clear the file input so the same filename can be picked again after a fix.
      const el = document.querySelector<HTMLInputElement>('.upl input[type=file]');
      if (el) el.value = '';
      await loadSubs();
    } catch (err) {
      toast.error(errMsg(err, 'Încărcarea a eșuat.'));
    } finally {
      uploading = false;
    }
  }

  async function removeTrack(s: Subtitle) {
    if (!confirm(`Ștergi pista ${langLabel(s.language)}?`)) return;
    busyId = s.id;
    try {
      await api.deleteSubtitle(s.id);
      toast.success('Pistă ștearsă.');
      await loadSubs();
    } catch (err) {
      toast.error(errMsg(err, 'Ștergerea a eșuat.'));
    } finally {
      busyId = null;
    }
  }

  const LANG: Record<string, string> = { ro: 'Română', en: 'Engleză', ja: 'Japoneză' };
  const langLabel = (l: string) => LANG[l] ?? l.toUpperCase();
  const srtUrl = (l: string) => api.fileUrl(`/api/episodes/${epId}/subtitles.srt?lang=${l}`);
  const selectedEp = $derived(episodes.find((e) => e.id === Number(epId)) ?? null);
</script>

<div class="subs">
  {#if !picked}
    <form class="search" onsubmit={search}>
      <input type="search" bind:value={q} placeholder="Caută seria după titlu…" />
      <button class="btn fill" type="submit" disabled={searching}>{searching ? '…' : 'Caută'}</button>
    </form>

    {#if searchedOnce}
      <span class="kicker">Rezultate</span>
      {#if searching && results.length === 0}
        <p class="muted">Se caută…</p>
      {:else if results.length === 0}
        <div class="empty"><p>Niciun anime găsit{q.trim() ? ` pentru „${q}"` : ''}.</p></div>
      {:else}
        <ul class="results">
          {#each results as r (r.id)}
            <li>
              <button class="result" onclick={() => pick(r)}>
                <span class="r-thumb" style={r.imageUrl ? `background-image:url(${mediaUrl(r.imageUrl)})` : ''}></span>
                <span class="r-main">
                  <span class="r-t">{displayName(r)}</span>
                  <span class="r-m">{r.year ?? '—'} · {r.type ?? '—'} · {r.status ?? '—'}</span>
                </span>
                <span class="r-go">Alege →</span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    {:else}
      <div class="hint">
        <span class="hint-ico" aria-hidden="true">⬇</span>
        <p>Caută o serie, alege episodul, apoi descarcă subtitrarea RO ca <code>.srt</code>.</p>
      </div>
    {/if}
  {:else}
    <div class="picked">
      <span class="p-thumb" style={picked.imageUrl ? `background-image:url(${mediaUrl(picked.imageUrl)})` : ''}></span>
      <div class="p-main">
        <span class="p-t">{displayName(picked)}</span>
        <span class="p-m">{episodes.length} {episodes.length === 1 ? 'episod' : 'episoade'} în catalog</span>
      </div>
      <button class="btn ghost" onclick={reset}>Schimbă seria</button>
    </div>

    <div class="pickrow">
      <label class="field">
        <span class="f-label">Episod</span>
        <select bind:value={epId} onchange={loadSubs} disabled={loadingEps || episodes.length === 0}>
          <option value="" disabled selected>{loadingEps ? 'Se încarcă…' : 'Alege un episod…'}</option>
          {#each episodes as e (e.id)}
            <option value={e.id}>Ep. {e.episodeNumber}{e.title ? ` — ${e.title}` : ''}</option>
          {/each}
        </select>
      </label>
    </div>

    {#if epId}
      <div class="dlcard">
        {#if loadingSubs}
          <p class="muted">Se verifică subtitrările…</p>
        {:else if subs.length === 0}
          <div class="empty">
            <p>Nicio subtitrare publicată pentru <strong>Ep. {selectedEp?.episodeNumber}</strong>.</p>
            <p class="muted">Se publică din pipeline-ul de traduceri (Publicare) sau se atașează direct mai jos.</p>
          </div>
        {:else}
          <span class="kicker">Subtitrări publicate · Ep. {selectedEp?.episodeNumber}</span>
          <ul class="tracks">
            {#each subs as s (s.language + s.format)}
              <li class="track">
                <span class="t-lang">{langLabel(s.language)}</span>
                <span class="t-fmt">{s.format.toUpperCase()}</span>
                <span class="t-sp"></span>
                <a class="btn fill sm" href={srtUrl(s.language)}>⬇ Descarcă .srt</a>
                <a class="t-raw" href={api.resolveUrl(s.url)} target="_blank" rel="noreferrer">deschide originalul →</a>
                <button class="t-del" type="button" onclick={() => removeTrack(s)} disabled={busyId === s.id}>
                  {busyId === s.id ? '…' : 'șterge'}
                </button>
              </li>
            {/each}
          </ul>
        {/if}

        <!-- Direct attach: for an episode whose source was linked by hand, or
             whose translation was done outside the pipeline. -->
        <form class="upl" onsubmit={upload}>
          <span class="kicker">Atașează o subtitrare</span>
          <p class="muted small">
            .srt, .ass sau .vtt — se convertește în WebVTT la încărcare, pentru că
            playerul nu poate afișa altceva. Reîncărcarea aceleiași limbi
            înlocuiește pista existentă.
          </p>
          <div class="upl-row">
            <input
              type="file"
              accept=".srt,.ass,.ssa,.vtt"
              onchange={(e) => (subFile = (e.currentTarget as HTMLInputElement).files?.[0] ?? null)}
            />
            <select bind:value={upLang} aria-label="Limbă">
              <option value="ro">Română</option>
              <option value="en">Engleză</option>
              <option value="ja">Japoneză</option>
            </select>
            <button class="btn fill" type="submit" disabled={uploading || !subFile}>
              {uploading ? 'Se încarcă…' : 'Atașează'}
            </button>
          </div>
          <p class="muted small note">
            Se vede doar la sursele redate în playerul nostru. O sursă
            <code>embed</code> (iframe) nu poate purta subtitrarea noastră —
            same-origin.
          </p>
        </form>
      </div>
    {/if}
  {/if}
</div>

<style>
  .subs { max-width: 760px; }

  .search { display: flex; gap: 8px; margin-bottom: var(--space-5); }
  .search input {
    flex: 1; min-height: 44px; padding: 0 14px;
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary); outline: none;
  }
  .search input:focus { border-color: var(--accent); }

  .kicker { display: block; margin-bottom: var(--space-3); }
  .muted { color: var(--text-muted); }

  .hint {
    display: flex; align-items: center; gap: 14px;
    border: 1px dashed var(--border-default); border-radius: var(--radius-md);
    padding: var(--space-5); color: var(--text-muted);
  }
  .hint-ico { font-size: 1.5rem; color: var(--accent); }
  .hint code {
    font-family: var(--font-mono); font-size: 0.85em;
    background: var(--surface-inset); padding: 1px 6px; border-radius: 5px; color: var(--text-primary);
  }

  .results { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
  .result {
    display: flex; align-items: center; gap: var(--space-4); width: 100%; text-align: left;
    padding: 10px 14px; border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
    background: var(--surface-raised); color: var(--text-primary); cursor: pointer;
  }
  .result:hover { border-color: var(--border-strong); background: var(--surface-overlay); }
  .r-thumb {
    width: 38px; height: 56px; border-radius: var(--radius-sm); flex: 0 0 auto;
    background-color: var(--surface-overlay); background-size: cover; background-position: center;
  }
  .r-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
  .r-t { font-weight: var(--fw-semibold); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .r-m { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .r-go { font-size: var(--fs-small); font-weight: var(--fw-semibold); color: var(--accent); white-space: nowrap; }

  .empty {
    border: 1px dashed var(--border-default); border-radius: var(--radius-md);
    padding: var(--space-6); text-align: center; color: var(--text-primary);
  }
  .empty .muted { margin-top: 6px; }

  .picked {
    display: flex; align-items: center; gap: var(--space-4);
    padding: 12px 14px; border: 1px solid var(--border-default); border-radius: var(--radius-md);
    background: var(--surface-raised); margin-bottom: var(--space-4);
  }
  .p-thumb {
    width: 42px; height: 62px; border-radius: var(--radius-sm); flex: 0 0 auto;
    background-color: var(--surface-overlay); background-size: cover; background-position: center;
  }
  .p-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
  .p-t { font-family: var(--font-display); font-size: var(--fs-h3); font-weight: var(--fw-semibold); }
  .p-m { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }

  .pickrow { margin-bottom: var(--space-4); }
  .field { display: flex; flex-direction: column; gap: 7px; }
  .f-label {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.08em; text-transform: uppercase; color: var(--text-muted);
  }
  .field select {
    min-height: 44px; padding: 0 12px; max-width: 520px;
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary); outline: none; cursor: pointer;
  }
  .field select:focus { border-color: var(--accent); }
  .field select:disabled { opacity: 0.6; }

  .dlcard {
    border: 1px solid var(--border-subtle); border-radius: var(--radius-lg);
    background: var(--surface-raised); padding: var(--space-4) var(--space-5) var(--space-5);
  }
  .tracks { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 10px; }
  .track {
    display: flex; align-items: center; gap: 12px; flex-wrap: wrap;
    padding: 12px 14px; border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
    background: var(--surface-inset);
  }
  .t-lang { font-weight: var(--fw-semibold); }
  .t-fmt {
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
    border: 1px solid var(--border-subtle); border-radius: 5px; padding: 1px 7px;
  }
  .t-sp { flex: 1; }
  .t-raw { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .t-raw:hover { color: var(--accent); }

  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 10px 18px; border-radius: var(--radius-md); white-space: nowrap; cursor: pointer;
  }
  .btn.sm { padding: 8px 13px; font-size: var(--fs-caption); }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-primary); }
  .btn.ghost:hover { background: var(--surface-overlay); }
  .btn:disabled { opacity: 0.6; cursor: wait; }

  /* ---- direct attach ---- */
  .upl {
    margin-top: var(--space-5); padding-top: var(--space-5);
    border-top: 1px solid var(--border-subtle);
  }
  .small { font-size: var(--fs-small); line-height: 1.55; }
  .upl-row {
    display: flex; gap: 8px; flex-wrap: wrap; align-items: center;
    margin-top: var(--space-3);
  }
  .upl-row input[type='file'] { flex: 1; min-width: 220px; font-size: var(--fs-small); }
  .upl-row select {
    min-height: 40px; padding: 0 10px; cursor: pointer;
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary);
  }
  .note { margin-top: var(--space-3); }
  .note code {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    padding: 1px 5px; border-radius: 4px;
    background: var(--surface-overlay); color: var(--accent);
  }
  .t-del {
    font-family: var(--font-mono); font-size: var(--fs-caption);
    color: var(--text-muted); cursor: pointer; background: none; border: none;
  }
  .t-del:hover:not(:disabled) { color: var(--danger, #e5484d); }
  .t-del:disabled { cursor: wait; opacity: 0.6; }
</style>
