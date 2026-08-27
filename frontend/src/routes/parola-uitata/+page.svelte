<script lang="ts">
  import { api } from '$lib/api';

  let email = $state('');
  let busy = $state(false);
  let err = $state<string | null>(null);
  // The server answers identically for addresses that exist and ones that
  // don't, so this page can only ever say "check your inbox". Showing
  // anything more specific would leak who has an account here.
  let sent = $state(false);

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    err = null;
    busy = true;
    try {
      await api.forgotPassword(email);
      sent = true;
    } catch (e2) {
      // api.ts throws a plain ApiError carrying .error, not an Error
      err =
        (e2 as { error?: string })?.error ||
        (e2 instanceof Error ? e2.message : '') ||
        'Nu am putut trimite emailul. Încearcă din nou.';
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head><title>Ai uitat parola · Anime-Kage</title></svelte:head>

<div class="auth-wrap">
  <div class="card">
    <div class="brand">
      <img src="/logo.png" alt="" width="40" height="40" />
      <span class="wordmark">Anime<span class="dot">·</span>Kage</span>
    </div>

    {#if sent}
      <h1>Verifică-ți emailul</h1>
      <p class="sub">
        Dacă există un cont pentru <strong>{email}</strong>, ți-am trimis un link de
        resetare. Este valabil o oră și poate fi folosit o singură dată.
      </p>
      <p class="note">
        Nu a ajuns? Verifică folderul spam, apoi
        <button class="linky" onclick={() => (sent = false)}>încearcă din nou</button>.
      </p>
      <p class="alt"><a href="/login">Înapoi la autentificare</a></p>
    {:else}
      <h1>Ai uitat parola?</h1>
      <p class="sub">Scrie adresa contului și îți trimitem un link de resetare.</p>

      <form onsubmit={submit}>
        <label>
          <span class="lbl">Email</span>
          <input
            type="email"
            bind:value={email}
            required
            autocomplete="email"
            autocapitalize="none"
            autocorrect="off"
            spellcheck="false"
            placeholder="tu@exemplu.ro"
          />
        </label>

        {#if err}<p class="err" role="alert">{err}</p>{/if}

        <button class="go" type="submit" disabled={busy}>
          {busy ? 'Se trimite…' : 'Trimite linkul'}
        </button>
      </form>

      <p class="alt">Ți-ai amintit-o? <a href="/login">Autentifică-te</a></p>
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
  .note { color: var(--text-muted); font-size: var(--fs-small); margin-top: var(--space-4); }

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

  /* a button because it acts on this page, styled as a link because that is
     what "încearcă din nou" reads as mid-sentence */
  .linky {
    background: none; border: none; padding: 0; cursor: pointer;
    color: var(--accent); font: inherit; text-decoration: underline;
  }

  .alt { text-align: center; color: var(--text-muted); font-size: var(--fs-small); margin-top: var(--space-5); }
</style>
