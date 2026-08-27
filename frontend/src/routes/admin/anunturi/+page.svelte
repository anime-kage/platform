<script lang="ts">
  import GifPicker from '$lib/components/GifPicker.svelte';
  import { onMount } from 'svelte';
  import api, { type AnnouncementInput } from '$lib/api';
  import { authStore as auth } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import { reltime } from '$lib/reltime';
  import EmojiPicker from '$lib/components/EmojiPicker.svelte';
  import Markdown from '$lib/components/Markdown.svelte';
  import type { Announcement } from '$shared/types';

  // "Știri & anunțuri" — the third column of the home page's community strip.
  // Whatever is published here is what every member sees when they land; the
  // draft toggle exists so a notice can be written before it goes out.
  const canWrite = $derived(
    $auth.isAuthenticated && ['admin', 'moderator'].includes($auth.user?.role ?? '')
  );

  // The tags the home card renders as its accent kicker. Suggestions, not a
  // constraint — the server takes any short label, and the field is free text.
  const TAG_SUGGESTIONS = ['Adăugat', 'Comunitate', 'Funcție', 'Mentenanță', 'Echipă'];

  const HOME_SLOTS = 4; // how many the home page shows

  let items = $state<Announcement[]>([]);
  let loading = $state(true);
  let busy = $state(false);

  // The form doubles as the editor: `editing` is the row being changed, or
  // null when the form creates a new one.
  let editing = $state<Announcement | null>(null);
  let tag = $state('');
  let title = $state('');
  let body = $state('');
  let url = $state('');
  let coverUrl = $state('');
  let published = $state(true);
  let uploading = $state(false);
  let preview = $state(false);

  /** The textarea, so the toolbar can wrap the selection instead of appending. */
  let bodyEl = $state<HTMLTextAreaElement | null>(null);

  /**
   * Wrap the selection in `before`/`after`, or insert a placeholder when nothing
   * is selected. Keeps focus and re-selects the text so a second click on Bold
   * un-clicks nothing — the editor is Markdown, not a rich-text widget, and
   * pretending otherwise is where these things get confusing.
   */
  function wrap(before: string, after = before, placeholder = 'text') {
    const el = bodyEl;
    if (!el) return;
    const start = el.selectionStart ?? body.length;
    const end = el.selectionEnd ?? start;
    const selected = body.slice(start, end) || placeholder;
    body = body.slice(0, start) + before + selected + after + body.slice(end);
    const caret = start + before.length;
    queueMicrotask(() => {
      el.focus();
      el.setSelectionRange(caret, caret + selected.length);
    });
  }

  /** Prefix the current line — headings, quotes, list items. */
  function prefixLine(prefix: string) {
    const el = bodyEl;
    if (!el) return;
    const start = el.selectionStart ?? body.length;
    const lineStart = body.lastIndexOf('\n', start - 1) + 1;
    body = body.slice(0, lineStart) + prefix + body.slice(lineStart);
    queueMicrotask(() => {
      el.focus();
      const caret = start + prefix.length;
      el.setSelectionRange(caret, caret);
    });
  }

  function insert(text: string) {
    const el = bodyEl;
    const start = el?.selectionStart ?? body.length;
    body = body.slice(0, start) + text + body.slice(start);
    queueMicrotask(() => {
      el?.focus();
      el?.setSelectionRange(start + text.length, start + text.length);
    });
  }

  /** Upload an image and drop it into the body as Markdown. */
  async function insertImage(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    uploading = true;
    try {
      const r = await api.uploadAnnouncementImage(file);
      insert(`\n![](${r.imageUrl})\n`);
      toast.success('Imagine adăugată în text.');
    } catch (err) {
      toast.error(errText(err));
    } finally {
      uploading = false;
      input.value = '';
    }
  }

  /** The wide image at the top of the post — separate from body images. */
  async function pickCover(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    uploading = true;
    try {
      coverUrl = (await api.uploadAnnouncementImage(file)).imageUrl;
      toast.success('Copertă încărcată.');
    } catch (err) {
      toast.error(errText(err));
    } finally {
      uploading = false;
      input.value = '';
    }
  }

  const dirty = $derived(tag.trim() !== '' || title.trim() !== '' || body.trim() !== '' || url.trim() !== '');
  const publishedCount = $derived(items.filter((a) => a.isPublished).length);

  onMount(load);

  async function load() {
    loading = true;
    try {
      items = (await api.getAnnouncements({ limit: 50, drafts: true })).data;
    } catch {
      toast.error('Nu am putut încărca anunțurile.');
    } finally {
      loading = false;
    }
  }

  function reset() {
    editing = null;
    tag = '';
    title = '';
    body = '';
    url = '';
    coverUrl = '';
    published = true;
    preview = false;
  }

  function edit(a: Announcement) {
    editing = a;
    tag = a.tag;
    title = a.title;
    body = a.body ?? '';
    url = a.url ?? '';
    coverUrl = a.coverUrl ?? '';
    published = a.isPublished;
    if (typeof window !== 'undefined') window.scrollTo({ top: 0, behavior: 'smooth' });
  }

  async function save() {
    const payload: AnnouncementInput = {
      tag: tag.trim(),
      title: title.trim(),
      body: body.trim(),
      url: url.trim(),
      coverUrl: coverUrl.trim(),
      isPublished: published
    };
    if (payload.tag.length < 2 || payload.title.length < 3) {
      toast.error('Completează eticheta și titlul.');
      return;
    }
    busy = true;
    try {
      if (editing) {
        const res = await api.updateAnnouncement(editing.id, payload);
        items = items.map((a) => (a.id === res.data.id ? res.data : a));
        toast.success('Anunț actualizat.');
      } else {
        const res = await api.createAnnouncement(payload);
        items = [res.data, ...items];
        toast.success(published ? 'Anunț publicat.' : 'Ciornă salvată.');
      }
      reset();
    } catch (e) {
      toast.error(errText(e));
    } finally {
      busy = false;
    }
  }

  async function togglePublished(a: Announcement) {
    busy = true;
    try {
      const res = await api.updateAnnouncement(a.id, {
        tag: a.tag,
        title: a.title,
        body: a.body ?? '',
        url: a.url ?? '',
        coverUrl: a.coverUrl ?? '',
        isPublished: !a.isPublished
      });
      items = items.map((x) => (x.id === res.data.id ? res.data : x));
    } catch (e) {
      toast.error(errText(e));
    } finally {
      busy = false;
    }
  }

  async function remove(a: Announcement) {
    if (!confirm(`Ștergi anunțul „${a.title}”?`)) return;
    busy = true;
    try {
      await api.deleteAnnouncement(a.id);
      items = items.filter((x) => x.id !== a.id);
      if (editing?.id === a.id) reset();
      toast.success('Anunț șters.');
    } catch (e) {
      toast.error(errText(e));
    } finally {
      busy = false;
    }
  }

  const errText = (e: unknown) =>
    (e as { error?: string; message?: string })?.error ??
    (e as { message?: string })?.message ??
    'Ceva n-a mers.';
</script>

{#if !canWrite}
  <p class="calm">Anunțurile se scriu de administratori și moderatori.</p>
{:else}
  <section class="sect">
    <p class="s-label">
      {editing ? 'Modifică anunțul' : 'Anunț nou'}
      <span class="s-count">
        · primele {HOME_SLOTS} publicate apar pe pagina principală
      </span>
    </p>

    <div class="listcard form">
      <div class="row two">
        <label class="field">
          <span class="lbl">Etichetă</span>
          <input bind:value={tag} maxlength="24" placeholder="Adăugat" list="tag-suggestions" />
          <datalist id="tag-suggestions">
            {#each TAG_SUGGESTIONS as t}<option value={t}></option>{/each}
          </datalist>
        </label>
        <label class="field">
          <span class="lbl">Link (opțional)</span>
          <input bind:value={url} placeholder="/anime/12 sau https://…" />
        </label>
      </div>

      <label class="field">
        <span class="lbl">Titlu</span>
        <input bind:value={title} maxlength="160" placeholder="Frieren S2 — episodul 5 e disponibil" />
      </label>

      <div class="field">
        <span class="lbl">Coperta (opțional)</span>
        <div class="cover-row">
          {#if coverUrl}
            <img class="cover-prev" src={coverUrl} alt="" />
            <button class="btn ghost sm" onclick={() => (coverUrl = '')}>Elimină</button>
          {:else}
            <label class="btn ghost sm file">
              {uploading ? 'Se încarcă…' : '↑ Încarcă o imagine'}
              <input type="file" accept="image/*" onchange={pickCover} disabled={uploading} />
            </label>
            <span class="hint">Apare lată, în capul postării.</span>
          {/if}
        </div>
      </div>

      <div class="field">
        <span class="lbl">Textul postării</span>

        <!-- Markdown, with buttons for the parts people actually use. It stays
             plain text on purpose: it is what gets stored, it is diffable, and
             it can never carry markup into the page (see lib/markdown.ts). -->
        <div class="toolbar">
          <button class="tb" title="Îngroșat" onclick={() => wrap('**')}><strong>B</strong></button>
          <button class="tb" title="Cursiv" onclick={() => wrap('*')}><em>I</em></button>
          <span class="tb-sep"></span>
          <button class="tb" title="Titlu mare" onclick={() => prefixLine('# ')}>H1</button>
          <button class="tb" title="Titlu mic" onclick={() => prefixLine('## ')}>H2</button>
          <span class="tb-sep"></span>
          <button class="tb" title="Listă" onclick={() => prefixLine('- ')}>• Listă</button>
          <button class="tb" title="Citat" onclick={() => prefixLine('> ')}>❝</button>
          <button class="tb" title="Linie despărțitoare" onclick={() => insert('\n---\n')}>—</button>
          <span class="tb-sep"></span>
          <button class="tb" title="Ascunde ca spoiler" onclick={() => wrap('||', '||', 'spoiler')}>||</button>
          <button class="tb" title="Link" onclick={() => wrap('[', '](https://)', 'text')}>🔗</button>
          <label class="tb file" title="Imagine în text">
            {uploading ? '…' : '🖼'}
            <input type="file" accept="image/*" onchange={insertImage} disabled={uploading} />
          </label>
          <EmojiPicker onPick={(t) => insert(t)} />
          <GifPicker onPick={(url) => insert(`\n![](${url})\n`)} />
          <span class="tb-spacer"></span>
          <button class="tb" class:on={preview} onclick={() => (preview = !preview)}>
            {preview ? 'Editează' : 'Previzualizare'}
          </button>
        </div>

        {#if preview}
          <div class="preview">
            {#if body.trim()}
              <Markdown source={body} />
            {:else}
              <p class="hint">Nimic de previzualizat încă.</p>
            {/if}
          </div>
        {:else}
          <textarea
            bind:this={bodyEl}
            bind:value={body}
            maxlength="20000"
            rows="12"
            placeholder={'Scrie anunțul.\n\n**Îngroșat**, *cursiv*, # titlu, - listă.\nImaginile și emoji se adaugă din bara de sus.'}
          ></textarea>
        {/if}
      </div>

      <div class="form-foot">
        <label class="check">
          <input type="checkbox" bind:checked={published} />
          <span>Publicat</span>
        </label>
        <div class="spacer"></div>
        {#if editing || dirty}
          <button class="btn ghost sm" onclick={reset} disabled={busy}>Renunță</button>
        {/if}
        <button class="btn fill sm" onclick={save} disabled={busy}>
          {busy ? 'Se salvează…' : editing ? 'Salvează' : 'Adaugă'}
        </button>
      </div>
    </div>
  </section>

  <section class="sect">
    <p class="s-label">
      Anunțuri
      <span class="s-count">· {publishedCount} publicate, {items.length - publishedCount} ciorne</span>
    </p>

    {#if loading}
      <p class="calm">Se încarcă…</p>
    {:else if items.length === 0}
      <p class="calm">Niciun anunț încă. Cel de sus e primul.</p>
    {:else}
      <div class="listcard">
        {#each items as a, i (a.id)}
          <div class="ann" class:draft={!a.isPublished}>
            <div class="ann-main">
              <div class="ann-head">
                <span class="tagpill">{a.tag}</span>
                <span class="ann-title">{a.title}</span>
                {#if a.isPublished && i < HOME_SLOTS}<span class="pill on">pe home</span>{/if}
                {#if !a.isPublished}<span class="pill">ciornă</span>{/if}
              </div>
              {#if a.body}<p class="ann-body">{a.body}</p>{/if}
              <p class="ann-meta">
                {reltime(a.createdAt)}{#if a.authorName} · {a.authorName}{/if}{#if a.url}
                  · <a href={a.url}>{a.url}</a>
                {/if}
              </p>
            </div>
            <div class="ann-actions">
              <button class="btn ghost sm" onclick={() => togglePublished(a)} disabled={busy}>
                {a.isPublished ? 'Retrage' : 'Publică'}
              </button>
              <button class="btn ghost sm" onclick={() => edit(a)} disabled={busy}>Modifică</button>
              <button class="btn ghost sm danger" onclick={() => remove(a)} disabled={busy}>Șterge</button>
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
  .row.two { display: grid; grid-template-columns: 200px minmax(0, 1fr); gap: var(--space-4); }
  .field { display: flex; flex-direction: column; gap: 7px; }
  .lbl {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.12em; text-transform: uppercase; color: var(--text-muted);
  }
  .field input,
  .field textarea {
    width: 100%; padding: 11px 13px; font: inherit; font-size: var(--fs-small);
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary); outline: none;
    resize: vertical;
  }
  .field input:focus, .field textarea:focus { border-color: var(--accent); }

  /* ---- editor ---- */
  .toolbar {
    display: flex; align-items: center; gap: 4px; flex-wrap: wrap;
    padding: 6px; border: 1px solid var(--border-default); border-bottom: none;
    border-radius: var(--radius-md) var(--radius-md) 0 0; background: var(--surface-inset);
  }
  .tb {
    font: inherit; font-size: var(--fs-caption); line-height: 1;
    padding: 6px 9px; cursor: pointer; border-radius: var(--radius-sm);
    background: none; border: 1px solid transparent; color: var(--text-muted);
  }
  .tb:hover { background: var(--surface-overlay); color: var(--text-primary); }
  .tb.on { background: var(--accent); border-color: var(--accent); color: var(--on-accent); }
  .tb.file { position: relative; overflow: hidden; display: inline-flex; align-items: center; }
  .tb.file input, .btn.file input { position: absolute; inset: 0; opacity: 0; cursor: pointer; }
  .tb-sep { width: 1px; align-self: stretch; background: var(--border-default); margin: 2px 4px; }
  .tb-spacer { flex: 1; }

  /* the textarea joins the toolbar into one box */
  .field textarea {
    border-radius: 0 0 var(--radius-md) var(--radius-md); font-family: var(--font-mono);
    font-size: var(--fs-small); line-height: 1.6;
  }
  .preview {
    border: 1px solid var(--border-default); border-top: none;
    border-radius: 0 0 var(--radius-md) var(--radius-md);
    padding: var(--space-4); background: var(--surface-base); min-height: 12rem;
  }

  .cover-row { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .cover-prev {
    height: 64px; width: auto; border-radius: var(--radius-sm);
    border: 1px solid var(--border-subtle); display: block;
  }
  .btn.file { position: relative; overflow: hidden; }
  .hint { font-size: var(--fs-caption); color: var(--text-muted); }

  .form-foot { display: flex; align-items: center; gap: 9px; flex-wrap: wrap; }
  .spacer { flex: 1; }
  .check { display: flex; align-items: center; gap: 8px; font-size: var(--fs-small); color: var(--text-muted); cursor: pointer; }
  .check input { accent-color: var(--accent); width: 16px; height: 16px; }

  /* ---- list ---- */
  .ann { display: flex; align-items: flex-start; gap: var(--space-4); padding: var(--space-4) var(--space-5); }
  .ann + .ann { border-top: 1px solid var(--border-subtle); }
  .ann.draft { opacity: 0.62; }
  .ann-main { flex: 1; min-width: 0; }
  .ann-head { display: flex; align-items: baseline; gap: 10px; flex-wrap: wrap; }
  .tagpill {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.08em; text-transform: uppercase; color: var(--accent);
  }
  .ann-title { font-weight: var(--fw-semibold); color: var(--text-primary); }
  .ann-body { font-size: var(--fs-small); color: var(--text-muted); line-height: 1.55; margin-top: 7px; max-width: 64ch; }
  .ann-meta { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); margin-top: 8px; }
  .ann-meta a { color: var(--text-muted); text-decoration: underline; }
  .ann-meta a:hover { color: var(--text-primary); }
  .ann-actions { display: flex; gap: 7px; flex-wrap: wrap; flex: 0 0 auto; }

  .pill {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.08em; text-transform: uppercase; white-space: nowrap; color: var(--text-muted);
  }
  .pill.on { color: var(--success); }

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

  @media (max-width: 700px) {
    .row.two { grid-template-columns: minmax(0, 1fr); }
    .ann { flex-direction: column; }
  }
</style>
