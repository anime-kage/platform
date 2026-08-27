<script lang="ts">
  import { goto } from '$app/navigation';
  import { authStore } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import PunpunTip from '$lib/components/PunpunTip.svelte';

  import { api } from '$lib/api';

  let username = $state('');
  let email = $state('');
  let password = $state('');
  let confirm = $state('');
  let inviteCode = $state('');
  let busy = $state(false);
  let err = $state<string | null>(null);

  // Whether the code is required is the server's call (INVITE_ONLY),
  // so the field only appears when it actually does something. If the lookup
  // fails we hide it — the server still rejects a missing code with a clear
  // message, which beats demanding one that might not be needed.
  let inviteOnly = $state(false);
  $effect(() => {
    api
      .getPublicConfig()
      .then((r) => (inviteOnly = r.data.inviteOnly))
      .catch(() => {});
  });

  // KAGE-7F2A-9X — upper-case as you type, and re-insert the dashes so a
  // pasted or hand-typed code lands in the shape the server stores.
  function onInviteInput(e: Event) {
    const raw = (e.currentTarget as HTMLInputElement).value
      .toUpperCase()
      .replace(/[^A-Z0-9]/g, '')
      .replace(/^KAGE/, '');
    const parts = [raw.slice(0, 4), raw.slice(4, 6)].filter(Boolean);
    inviteCode = parts.length ? `KAGE-${parts.join('-')}` : '';
  }

  // The mascot comments from the side of the form.
  let punShow = $state(false);
  let punType = $state<'tip' | 'success' | 'error'>('tip');
  let punMsg = $state('');
  let punTimer: ReturnType<typeof setTimeout> | undefined;

  function punpunSay(type: typeof punType, msg: string, hideAfter?: number) {
    clearTimeout(punTimer);
    punType = type;
    punMsg = msg;
    punShow = true;
    if (hideAfter) punTimer = setTimeout(() => (punShow = false), hideAfter);
  }

  function onPasswordFocus() {
    punpunSay('tip', 'Punpun îți recomandă o parolă puternică, pe care nu o folosești nicăieri altundeva!');
  }
  function onPasswordBlur() {
    if (punType === 'tip') punShow = false;
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    err = null;
    punShow = false;
    if (password !== confirm) {
      err = 'Parolele nu coincid.';
      punpunSay('error', err, 5000);
      return;
    }
    if (password.length < 8) {
      err = 'Parola trebuie să aibă cel puțin 8 caractere.';
      punpunSay('error', err, 5000);
      return;
    }
    busy = true;
    try {
      await authStore.register(username, email, password, confirm, inviteCode || undefined);
      toast.success('Cont creat. Bun venit!');
      punpunSay('success', `Bun venit în Anime-Kage, ${username}!`);
      setTimeout(() => goto('/home'), 1400);
    } catch (e2) {
      // api.ts throws a plain ApiError ({ error }), not an Error — reading
      // .message swallowed the server's reason and showed the generic
      // fallback, which matters most exactly here: "codul a fost deja
      // folosit" and "codul a expirat" are different problems for the user.
      err =
        (e2 as { error?: string })?.error ||
        (e2 instanceof Error ? e2.message : '') ||
        'Nu am putut crea contul.';
      punpunSay('error', err, 5000);
      busy = false;
    }
  }
</script>

<svelte:head><title>Înregistrare · Anime-Kage</title></svelte:head>

<!-- success flips auth on, which docks the chat on the right — Punpun steps left -->
<PunpunTip show={punShow} type={punType} message={punMsg} side={punType === 'success' ? 'left' : 'right'} />

<div class="auth-wrap">
  <div class="card">
    <div class="brand">
      <img src="/logo.png" alt="" width="40" height="40" />
      <span class="wordmark">Anime<span class="dot">·</span>Kage</span>
    </div>
    <h1>Creează cont</h1>
    <p class="sub">Liste de vizionare, evaluări și discuții — totul în română.</p>

    <form onsubmit={submit}>
      {#if inviteOnly}
        <label>
          <span class="lbl">Cod de invitație</span>
          <input
            class="code"
            type="text"
            value={inviteCode}
            oninput={onInviteInput}
            required
            autocomplete="off"
            spellcheck="false"
            placeholder="KAGE-7F2A-9X"
          />
        </label>
      {/if}
      <label>
        <span class="lbl">Nume utilizator</span>
        <input type="text" bind:value={username} required minlength="3" autocomplete="username" placeholder="otaku_ro" />
      </label>
      <label>
        <span class="lbl">Email</span>
        <input type="email" bind:value={email} required autocomplete="email" autocapitalize="none" autocorrect="off" spellcheck="false" placeholder="tu@exemplu.ro" />
      </label>
      <label>
        <span class="lbl">Parolă</span>
        <input type="password" bind:value={password} required minlength="8" autocomplete="new-password" placeholder="minim 8 caractere" onfocus={onPasswordFocus} onblur={onPasswordBlur} />
      </label>
      <label>
        <span class="lbl">Confirmă parola</span>
        <input type="password" bind:value={confirm} required autocomplete="new-password" placeholder="••••••••" />
      </label>

      {#if err}<p class="err" role="alert">{err}</p>{/if}

      <button class="go" type="submit" disabled={busy}>
        {busy ? 'Se creează…' : 'Creează contul'}
      </button>
    </form>

    <p class="alt">Ai deja cont? <a href="/login">Autentifică-te</a></p>
  </div>
</div>

<style>
  .auth-wrap {
    min-height: calc(100dvh - 62px);
    display: grid; place-items: center;
    padding: var(--space-6) clamp(1rem, 4vw, 2.5rem);
  }
  .card {
    width: 100%; max-width: 400px;
    background: var(--surface-raised);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-xl);
    padding: clamp(var(--space-5), 4vw, var(--space-6));
  }
  .brand { display: flex; align-items: center; gap: 10px; margin-bottom: var(--space-5); }
  .brand img { width: 40px; height: 40px; object-fit: contain; }
  .wordmark { font-family: var(--font-display); font-weight: var(--fw-semibold); font-size: 1.25rem; }
  .dot { color: var(--accent); }

  h1 { font-size: 1.625rem; letter-spacing: -0.01em; }
  .sub { color: var(--text-muted); font-size: var(--fs-small); margin-top: 6px; }

  form { display: flex; flex-direction: column; gap: var(--space-4); margin-top: var(--space-5); }
  label { display: flex; flex-direction: column; gap: 7px; }

  /* the code is read off a screen and typed — monospace and spaced out so
     the characters can't be mistaken for one another */
  .code {
    font-family: var(--font-mono);
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .lbl {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-muted);
  }
  input {
    min-height: 46px; padding: 0 14px;
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary); outline: none;
  }
  input::placeholder { color: var(--text-faint); }
  input:focus { border-color: var(--accent); box-shadow: 0 0 0 3px var(--focus-ring); }

  .err { color: var(--danger); font-size: var(--fs-small); }

  .go {
    min-height: 48px; margin-top: var(--space-2);
    background: var(--accent); color: var(--on-accent);
    font-weight: var(--fw-semibold); font-size: var(--fs-body);
    border: none; border-radius: var(--radius-md); cursor: pointer;
  }
  .go:hover { background: var(--accent-hover); }
  .go:disabled { opacity: 0.6; cursor: wait; }

  .alt { text-align: center; color: var(--text-muted); font-size: var(--fs-small); margin-top: var(--space-5); }
</style>
