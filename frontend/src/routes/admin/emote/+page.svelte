<script lang="ts">
  import { onMount } from 'svelte';
  import api from '$lib/api';
  import { authStore as auth } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import { loadEmotes } from '$lib/stores/emotes';
  import type { Emote } from '$shared/types';

  // Chat emotes, 7TV-style: upload once, everyone can type the code.
  const canManage = $derived(
    $auth.isAuthenticated && ['admin', 'coordinator'].includes($auth.user?.role ?? '')
  );

  /** Must match emoteCodeRe on the server. */
  const CODE_RE = /^[A-Za-z][A-Za-z0-9]{1,23}$/;
  /** Mirrors .emote-img in ChatPanel — the preview has to be the real size. */
  const CHAT_H = 28;

  let items = $state<Emote[]>([]);
  let loading = $state(true);
  let busy = $state(false);

  let code = $state('');
  let file = $state<File | null>(null);
  let previewUrl = $state('');
  let dims = $state<{ w: number; h: number } | null>(null);

  const codeOk = $derived(CODE_RE.test(code.trim()));
  const codeTaken = $derived(
    items.some((e) => e.code.toLowerCase() === code.trim().toLowerCase())
  );
  /** Same rule the server enforces; checked here so it fails before the upload. */
  const aspect = $derived(dims ? Math.max(dims.w, dims.h) / Math.min(dims.w, dims.h) : 1);
  const tooWide = $derived(aspect > 3);
  const tooBig = $derived(!!dims && (dims.w > 1024 || dims.h > 1024));
  const tooSmall = $derived(!!dims && (dims.w < 16 || dims.h < 16));

  onMount(load);

  async function load() {
    loading = true;
    try {
      items = (await api.getEmotes(true)).data;
    } catch {
      toast.error('Nu am putut încărca emote-urile.');
    } finally {
      loading = false;
    }
  }

  function pick(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const f = input.files?.[0] ?? null;
    file = f;
    dims = null;
    if (previewUrl) URL.revokeObjectURL(previewUrl);
    previewUrl = f ? URL.createObjectURL(f) : '';
    if (!f) return;
    // Measure locally so the warnings appear before anything is sent.
    const img = new Image();
    img.onload = () => (dims = { w: img.naturalWidth, h: img.naturalHeight });
    img.src = previewUrl;
    // A sensible default name from the filename, still editable.
    if (!code.trim()) {
      const base = f.name.replace(/\.[^.]+$/, '').replace(/[^A-Za-z0-9]/g, '');
      if (CODE_RE.test(base)) code = base;
    }
  }

  async function upload() {
    if (!file || !codeOk) return;
    busy = true;
    try {
      const r = await api.createEmote(code.trim(), file);
      items = [...items, r.data].sort((a, b) => a.code.localeCompare(b.code));
      toast.success(`:${r.data.code}: adăugat.`);
      reset();
      loadEmotes(true); // the chat picks it up without a reload
    } catch (e) {
      toast.error((e as { error?: string }).error ?? 'Încărcarea a eșuat.');
    } finally {
      busy = false;
    }
  }

  function reset() {
    code = '';
    file = null;
    dims = null;
    if (previewUrl) URL.revokeObjectURL(previewUrl);
    previewUrl = '';
  }

  async function toggle(e: Emote) {
    busy = true;
    try {
      const r = await api.setEmoteActive(e.id, !e.isActive);
      items = items.map((x) => (x.id === r.data.id ? r.data : x));
      loadEmotes(true);
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Eroare.');
    } finally {
      busy = false;
    }
  }

  async function remove(e: Emote) {
    if (!confirm(`Ștergi emote-ul :${e.code}:? Mesajele vechi îl vor arăta ca text.`)) return;
    busy = true;
    try {
      await api.deleteEmote(e.id);
      items = items.filter((x) => x.id !== e.id);
      loadEmotes(true);
      toast.success('Emote șters.');
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Eroare.');
    } finally {
      busy = false;
    }
  }
</script>

{#if !canManage}
  <p class="calm">Emote-urile se gestionează de administratori și coordonatori.</p>
{:else}
  <section class="sect">
    <p class="s-label">
      Emote nou
      <span class="s-count">· oricine îl poate folosi scriind numele în chat</span>
    </p>

    <div class="listcard form">
      <div class="row">
        <label class="field code-f">
          <span class="lbl">Nume</span>
          <input bind:value={code} placeholder="Kagege" maxlength="24" spellcheck="false" />
          {#if code && !codeOk}
            <span class="warn">Numele începe cu o literă, 2–24 caractere, doar litere și cifre.</span>
          {:else if codeTaken}
            <span class="warn">Numele e deja folosit.</span>
          {/if}
        </label>

        <label class="field">
          <span class="lbl">Imagine</span>
          <input type="file" accept="image/png,image/gif,image/jpeg" onchange={pick} />
          <span class="hint">PNG, GIF (animat merge) sau JPEG · max 1 MB · ideal ~112px înălțime, fundal transparent</span>
        </label>
      </div>

      {#if previewUrl}
        <!-- The preview that matters: exactly the size chat renders at. A rule
             can reject the obviously broken, but "does this read at 28px" is a
             judgement only a person can make. -->
        <div class="prev">
          <div class="prev-chat">
            <span class="prev-line">
              <span class="prev-user">Crefi:</span>
              gata cu episodul <img src={previewUrl} alt="" style:height={`${CHAT_H}px`} />
              hai la următorul <img src={previewUrl} alt="" style:height={`${CHAT_H}px`} />
            </span>
          </div>
          <div class="prev-meta">
            <img class="prev-big" src={previewUrl} alt="" />
            <span>
              {#if dims}{dims.w}×{dims.h}px{/if}
              {#if tooBig}<span class="warn">prea mare (max 1024)</span>{/if}
              {#if tooSmall}<span class="warn">prea mică (min 16)</span>{/if}
              {#if tooWide}<span class="warn">prea alungită — va apărea ca o dungă</span>{/if}
            </span>
          </div>
        </div>
      {/if}

      <div class="form-foot">
        {#if previewUrl}<button class="btn ghost sm" onclick={reset} disabled={busy}>Renunță</button>{/if}
        <button
          class="btn fill sm"
          onclick={upload}
          disabled={busy || !file || !codeOk || codeTaken || tooBig || tooSmall || tooWide}
        >
          {busy ? 'Se încarcă…' : 'Adaugă emote'}
        </button>
      </div>
    </div>
  </section>

  <section class="sect">
    <p class="s-label">
      Emote-uri
      <span class="s-count">· {items.filter((e) => e.isActive).length} active din {items.length}</span>
    </p>

    {#if loading}
      <p class="calm">Se încarcă…</p>
    {:else if items.length === 0}
      <p class="calm">Niciun emote încă.</p>
    {:else}
      <div class="grid">
        {#each items as e (e.id)}
          <div class="cell" class:off={!e.isActive}>
            <img src={e.imageUrl} alt={e.code} />
            <span class="c-code">{e.code}</span>
            <span class="c-dim">{e.width}×{e.height}</span>
            <div class="c-actions">
              <button class="mini" onclick={() => toggle(e)} disabled={busy}
                title={e.isActive ? 'Scoate din picker' : 'Pune înapoi'}>
                {e.isActive ? 'Ascunde' : 'Arată'}
              </button>
              <button class="mini danger" onclick={() => remove(e)} disabled={busy} title="Șterge">✕</button>
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
    padding-bottom: 12px; border-bottom: 1px solid var(--border-default); margin-bottom: var(--space-4);
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
  .form { padding: var(--space-5); display: flex; flex-direction: column; gap: var(--space-4); }
  .row { display: grid; grid-template-columns: 220px minmax(0, 1fr); gap: var(--space-4); }
  .field { display: flex; flex-direction: column; gap: 6px; }
  .lbl {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.12em; text-transform: uppercase; color: var(--text-muted);
  }
  .field input:not([type]) {
    padding: 10px 12px; font: inherit; font-size: var(--fs-small);
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary); outline: none;
  }
  .hint { font-size: var(--fs-micro); color: var(--text-muted); }
  .warn { font-size: var(--fs-micro); color: var(--danger); }

  /* preview */
  .prev { display: flex; flex-direction: column; gap: 10px; }
  .prev-chat {
    padding: 10px 12px; border-radius: var(--radius-md);
    background: var(--surface-inset); border: 1px solid var(--border-subtle);
  }
  .prev-line { font-size: 0.9375rem; line-height: 1.55; color: var(--text-primary); }
  .prev-line img { vertical-align: middle; width: auto; max-width: 112px; margin: -2px 1px; object-fit: contain; }
  .prev-user { font-weight: var(--fw-bold); color: var(--accent); margin-right: 4px; }
  .prev-meta { display: flex; align-items: center; gap: 12px; font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .prev-big { height: 64px; width: auto; max-width: 160px; object-fit: contain; background: var(--surface-overlay); border-radius: var(--radius-sm); }

  .form-foot { display: flex; justify-content: flex-end; gap: 9px; }

  /* list */
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(132px, 1fr)); gap: 10px; }
  .cell {
    display: flex; flex-direction: column; align-items: center; gap: 5px;
    padding: 12px 8px; border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
    background: var(--surface-raised);
  }
  .cell.off { opacity: 0.45; }
  .cell img { height: 40px; width: auto; max-width: 100%; object-fit: contain; }
  .c-code { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-primary); word-break: break-all; text-align: center; }
  .c-dim { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-faint); }
  .c-actions { display: flex; gap: 4px; margin-top: 2px; }
  .mini {
    border: 1px solid var(--border-default); background: none; color: var(--text-muted);
    border-radius: var(--radius-sm); font-size: var(--fs-micro); padding: 3px 7px; cursor: pointer;
  }
  .mini:hover { color: var(--text-primary); background: var(--surface-overlay); }
  .mini.danger { color: var(--danger); }
  .mini:disabled { opacity: 0.5; cursor: wait; }

  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 10px 18px; border-radius: var(--radius-md); white-space: nowrap; cursor: pointer;
  }
  .btn.sm { padding: 7px 13px; font-size: var(--fs-caption); }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-primary); }
  .btn:disabled { opacity: 0.55; cursor: not-allowed; }

  @media (max-width: 700px) { .row { grid-template-columns: minmax(0, 1fr); } }
</style>
