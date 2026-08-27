<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { authStore } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';

  let email = $state('');
  let password = $state('');
  let busy = $state(false);
  let err = $state<string | null>(null);

  const redirectTo = $derived(page.url.searchParams.get('redirect') ?? '/home');

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    err = null;
    busy = true;
    try {
      await authStore.login(email, password);
      toast.success('Bine ai revenit!');
      goto(redirectTo);
    } catch {
      err = 'Email sau parolă incorecte.';
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head><title>Autentificare · Anime-Kage</title></svelte:head>

<div class="auth-wrap">
  <div class="card">
    <div class="brand">
      <img src="/logo.png" alt="" width="40" height="40" />
      <span class="wordmark">Anime<span class="dot">·</span>Kage</span>
    </div>
    <h1>Autentificare</h1>
    <p class="sub">Intră în cont pentru liste, evaluări și discuții.</p>

    <form onsubmit={submit}>
      <label>
        <span class="lbl">Email</span>
        <input type="email" bind:value={email} required autocomplete="email" autocapitalize="none" autocorrect="off" spellcheck="false" placeholder="tu@exemplu.ro" />
      </label>
      <label>
        <span class="lbl">
          Parolă
          <a class="forgot" href="/parola-uitata">Ai uitat parola?</a>
        </span>
        <input type="password" bind:value={password} required autocomplete="current-password" placeholder="••••••••" />
      </label>

      {#if err}<p class="err" role="alert">{err}</p>{/if}

      <button class="go" type="submit" disabled={busy}>
        {busy ? 'Se conectează…' : 'Intră în cont'}
      </button>
    </form>

    <p class="alt">Nu ai cont? <a href="/register">Înregistrează-te</a></p>
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
  .lbl {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-muted);
    display: flex; align-items: baseline; justify-content: space-between; gap: var(--space-3);
  }
  /* sits on the password label's baseline — found where you look for it,
     which is the field it belongs to, not the bottom of the card */
  .forgot { text-transform: none; letter-spacing: 0; color: var(--text-muted); }
  .forgot:hover { color: var(--accent); }
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
