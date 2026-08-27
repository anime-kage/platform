<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { toast } from '$lib/stores/toast';
  import { api } from '$lib/api';

  // The token arrives in the query string, straight from the email link.
  const token = $derived(page.url.searchParams.get('token') ?? '');

  let password = $state('');
  let confirm = $state('');
  let busy = $state(false);
  let err = $state<string | null>(null);
  let done = $state(false);

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    err = null;
    if (password !== confirm) {
      err = 'Parolele nu coincid.';
      return;
    }
    if (password.length < 8) {
      err = 'Parola trebuie să aibă cel puțin 8 caractere.';
      return;
    }
    busy = true;
    try {
      await api.resetPassword(token, password);
      done = true;
      toast.success('Parola a fost schimbată.');
      // Redemption proves control of the inbox, not of the account, so the
      // server issues no token — they log in with the new password.
      setTimeout(() => goto('/login'), 1600);
    } catch (e2) {
      err =
        (e2 as { error?: string })?.error ||
        (e2 instanceof Error ? e2.message : '') ||
        'Nu am putut schimba parola.';
      busy = false;
    }
  }
</script>

<svelte:head><title>Resetare parolă · Anime-Kage</title></svelte:head>

<div class="auth-wrap">
  <div class="card">
    <div class="brand">
      <img src="/logo.png" alt="" width="40" height="40" />
      <span class="wordmark">Anime<span class="dot">·</span>Kage</span>
    </div>

    {#if !token}
      <!-- reached without a token: a bare visit, or a link an email client
           mangled. Nothing to submit, so don't show a form that can't work. -->
      <h1>Link incomplet</h1>
      <p class="sub">
        Linkul de resetare pare rupt sau incomplet. Cere unul nou și
        deschide-l direct din email.
      </p>
      <p class="alt"><a href="/parola-uitata">Cere un link nou</a></p>
    {:else if done}
      <h1>Gata!</h1>
      <p class="sub">Parola a fost schimbată. Te ducem la autentificare…</p>
      <p class="alt"><a href="/login">Mergi acum</a></p>
    {:else}
      <h1>Alege o parolă nouă</h1>
      <p class="sub">Minim 8 caractere. Fără reguli de majuscule sau simboluri.</p>

      <form onsubmit={submit}>
        <label>
          <span class="lbl">Parolă nouă</span>
          <input
            type="password"
            bind:value={password}
            required
            minlength="8"
            autocomplete="new-password"
            placeholder="minim 8 caractere"
          />
        </label>
        <label>
          <span class="lbl">Confirmă parola</span>
          <input
            type="password"
            bind:value={confirm}
            required
            autocomplete="new-password"
            placeholder="••••••••"
          />
        </label>

        {#if err}<p class="err" role="alert">{err}</p>{/if}

        <button class="go" type="submit" disabled={busy}>
          {busy ? 'Se salvează…' : 'Schimbă parola'}
        </button>
      </form>

      <p class="alt"><a href="/login">Înapoi la autentificare</a></p>
    {/if}
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
