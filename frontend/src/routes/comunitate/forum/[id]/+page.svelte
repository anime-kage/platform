<script lang="ts">
  import { goto } from '$app/navigation';
  import api from '$lib/api';
  import { authStore } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import { reltime } from '$lib/reltime';
  import { nameHue } from '$lib/avatar';
  import type { ForumReply } from '$shared/types';

  let { data } = $props();
  const auth = $derived($authStore);

  // thread flags can change (pin/lock) — keep a local copy
  let pinned = $state(data.thread.isPinned);
  let locked = $state(data.thread.isLocked);
  let replies = $state<ForumReply[]>(data.replies);
  let draft = $state('');
  let sending = $state(false);

  const isMod = $derived(['admin', 'moderator', 'coordinator', 'verifier'].includes(auth.user?.role ?? ''));
  const isAuthor = $derived(auth.user?.username === data.thread.author.username);
  const canDelete = $derived(isMod || isAuthor);

  async function sendReply() {
    if (!auth.isAuthenticated) {
      toast.info('Autentifică-te ca să răspunzi.');
      return;
    }
    const body = draft.trim();
    if (!body) return;
    sending = true;
    try {
      const res = await api.createForumReply(data.thread.id, body);
      replies = [...replies, res.data];
      draft = '';
    } catch (e) {
      toast.error((e as { error?: string })?.error ?? 'Nu am putut trimite răspunsul.');
    } finally {
      sending = false;
    }
  }

  async function togglePin() {
    try {
      await api.pinForumThread(data.thread.id, !pinned);
      pinned = !pinned;
      toast.success(pinned ? 'Subiect fixat.' : 'Fixare anulată.');
    } catch {
      toast.error('Acțiune eșuată.');
    }
  }
  async function toggleLock() {
    try {
      await api.lockForumThread(data.thread.id, !locked);
      locked = !locked;
      toast.success(locked ? 'Subiect blocat.' : 'Subiect deblocat.');
    } catch {
      toast.error('Acțiune eșuată.');
    }
  }
  async function remove() {
    if (!confirm('Ștergi definitiv subiectul?')) return;
    try {
      await api.deleteForumThread(data.thread.id);
      toast.success('Subiect șters.');
      goto('/comunitate?tab=forum');
    } catch {
      toast.error('Nu am putut șterge subiectul.');
    }
  }
</script>

<svelte:head><title>{data.thread.title} · Forum · Anime-Kage</title></svelte:head>

<div class="container thread-page">
  <a class="back" href="/comunitate?tab=forum">← Forum</a>

  <article class="op">
    <div class="op-head">
      <span class="cat">{data.thread.category}</span>
      {#if pinned}<span class="flag pin">★ Fixat</span>{/if}
      {#if locked}<span class="flag lock">🔒 Blocat</span>{/if}
    </div>
    <h1>{data.thread.title}</h1>
    <div class="byline">
      {#if data.thread.author.avatarUrl}
        <a class="avatar" href={`/user/${data.thread.author.username}`}><img src={api.resolveUrl(data.thread.author.avatarUrl)} alt={data.thread.author.username} /></a>
      {:else}
        <a class="avatar monogram" href={`/user/${data.thread.author.username}`} style={`--mg-hue:${nameHue(data.thread.author.username)}`}>{data.thread.author.username.charAt(0).toUpperCase()}</a>
      {/if}
      <a class="by-name plink" href={`/user/${data.thread.author.username}`}>{data.thread.author.username}</a>
      <span class="by-date">{reltime(data.thread.createdAt)}</span>
    </div>
    <div class="op-body">{data.thread.body}</div>

    {#if isMod || canDelete}
      <div class="mod-bar">
        {#if isMod}
          <button class="modbtn" onclick={togglePin}>{pinned ? 'Anulează fixarea' : 'Fixează'}</button>
          <button class="modbtn" onclick={toggleLock}>{locked ? 'Deblochează' : 'Blochează'}</button>
        {/if}
        {#if canDelete}
          <button class="modbtn danger" onclick={remove}>Șterge</button>
        {/if}
      </div>
    {/if}
  </article>

  <div class="replies-head">
    <span class="rh-count">{replies.length}</span>
    {replies.length === 1 ? 'răspuns' : 'răspunsuri'}
  </div>

  <div class="replies">
    {#each replies as r (r.id)}
      <div class="reply">
        {#if r.author.avatarUrl}
          <a class="avatar" href={`/user/${r.author.username}`}><img src={api.resolveUrl(r.author.avatarUrl)} alt={r.author.username} /></a>
        {:else}
          <a class="avatar monogram" href={`/user/${r.author.username}`} style={`--mg-hue:${nameHue(r.author.username)}`}>{r.author.username.charAt(0).toUpperCase()}</a>
        {/if}
        <div class="reply-main">
          <div class="reply-head">
            <a class="reply-name plink" href={`/user/${r.author.username}`}>{r.author.username}</a>
            <span class="reply-date">{reltime(r.createdAt)}</span>
          </div>
          <p class="reply-body">{r.body}</p>
        </div>
      </div>
    {:else}
      <p class="none">Niciun răspuns încă. Fii primul.</p>
    {/each}
  </div>

  {#if locked}
    <p class="locked-note">🔒 Subiectul e blocat — nu se mai pot trimite răspunsuri.</p>
  {:else if auth.isAuthenticated}
    <div class="composer">
      <textarea bind:value={draft} placeholder="Scrie un răspuns..." maxlength="8000"
        onkeydown={(e) => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) sendReply(); }}></textarea>
      <div class="composer-foot">
        <span class="hint">Ctrl/⌘ + Enter</span>
        <button class="btn fill" onclick={sendReply} disabled={sending || !draft.trim()}>{sending ? 'Se trimite…' : 'Răspunde'}</button>
      </div>
    </div>
  {:else}
    <p class="locked-note"><a href="/login?redirect=/comunitate/forum/{data.thread.id}">Autentifică-te</a> ca să răspunzi.</p>
  {/if}
</div>

<style>
  .thread-page { max-width: 820px; padding-block: var(--space-6) var(--space-8); }
  .back { display: inline-block; margin-bottom: 18px; font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--text-muted); }
  .back:hover { color: var(--accent); }

  .plink { color: inherit; }
  .plink:hover { color: var(--accent); }
  .avatar { width: 38px; height: 38px; border-radius: 50%; flex: 0 0 auto; display: grid; place-items: center; overflow: hidden; font-weight: var(--fw-bold); color: #fff; font-size: 0.9375rem; }
  .avatar img { width: 100%; height: 100%; object-fit: cover; }

  .op { border-bottom: 1px solid var(--border-default); padding-bottom: 24px; margin-bottom: 24px; }
  .op-head { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
  .cat { font-family: var(--font-mono); font-size: var(--fs-micro); text-transform: uppercase; letter-spacing: 0.06em; color: var(--accent); }
  .flag { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .flag.pin { color: var(--accent); }
  .op h1 { font-family: var(--font-display); font-size: clamp(1.5rem, 1.2rem + 1.6vw, 2rem); line-height: 1.15; margin-bottom: 14px; }
  .byline { display: flex; align-items: center; gap: 10px; margin-bottom: 18px; }
  .by-name { font-size: var(--fs-small); font-weight: var(--fw-semibold); }
  .by-date { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .op-body { font-size: var(--fs-body); line-height: 1.7; color: var(--text-primary); white-space: pre-wrap; }

  .mod-bar { display: flex; gap: 8px; margin-top: 20px; flex-wrap: wrap; }
  .modbtn { font-size: var(--fs-micro); font-weight: var(--fw-semibold); color: var(--text-muted); background: none; border: 1px solid var(--border-default); border-radius: 8px; padding: 6px 12px; cursor: pointer; }
  .modbtn:hover { color: var(--text-primary); border-color: var(--accent); }
  .modbtn.danger:hover { color: var(--danger); border-color: var(--danger); }

  .replies-head { font-family: var(--font-display); font-size: var(--fs-h3); font-weight: var(--fw-semibold); margin-bottom: 16px; }
  .rh-count { color: var(--accent); }
  .replies { display: flex; flex-direction: column; gap: 20px; margin-bottom: 28px; }
  .reply { display: flex; gap: 12px; }
  .reply-main { flex: 1; min-width: 0; }
  .reply-head { display: flex; align-items: center; gap: 9px; margin-bottom: 4px; }
  .reply-name { font-size: var(--fs-small); font-weight: var(--fw-semibold); }
  .reply-date { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .reply-body { font-size: var(--fs-small); line-height: 1.6; color: var(--text-muted); white-space: pre-wrap; }
  .none { color: var(--text-muted); font-size: var(--fs-small); }

  .composer { border: 1px solid var(--border-default); border-radius: var(--radius-lg); background: var(--surface-raised); padding: 16px; }
  .composer textarea {
    width: 100%; min-height: 90px; resize: vertical;
    background: var(--surface-overlay); border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
    padding: 12px 14px; color: var(--text-primary); outline: none; font-size: var(--fs-small); line-height: 1.5;
  }
  .composer textarea:focus { border-color: var(--accent); }
  .composer-foot { display: flex; align-items: center; justify-content: space-between; margin-top: 12px; }
  .hint { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-faint); }
  .btn { font-weight: var(--fw-semibold); font-size: var(--fs-small); padding: 9px 18px; border-radius: 9px; cursor: pointer; }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); }
  .btn.fill:disabled { opacity: 0.55; cursor: default; }

  .locked-note { color: var(--text-muted); font-size: var(--fs-small); padding: 16px 0; }
  .locked-note a { color: var(--accent); }
</style>
