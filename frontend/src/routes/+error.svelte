<script lang="ts">
  import { page } from '$app/state';

  const statusCode = $derived(page.status);
  const message = $derived(
    statusCode === 404
      ? 'Pagina pe care o cauți nu a fost găsită.'
      : statusCode === 403
        ? 'Nu ai permisiunea să accesezi această pagină.'
        : 'A apărut o eroare neașteptată. Te rugăm să încerci din nou.'
  );
  const title = $derived(
    statusCode === 404
      ? 'Pagină negăsită'
      : statusCode === 403
        ? 'Acces interzis'
        : 'Eroare'
  );
</script>

<svelte:head>
  <title>{statusCode} {title} · Anime-Kage</title>
</svelte:head>

<div class="error-page">
  <div class="error-content">
    <p class="code">{statusCode}</p>
    <h1>{title}</h1>
    <p class="msg">{message}</p>
    <div class="actions">
      <a href="/" class="btn fill">Acasă</a>
      <a href="/anime" class="btn ghost">Explorează anime</a>
    </div>
  </div>
</div>

<style>
  .error-page {
    min-height: calc(100dvh - 62px);
    display: grid; place-items: center;
    padding: var(--space-6) clamp(1rem, 4vw, 2.5rem);
  }
  .error-content { text-align: center; max-width: 480px; }

  .code {
    font-family: var(--font-display); font-weight: var(--fw-semibold);
    font-size: clamp(5rem, 4rem + 5vw, 8rem); line-height: 1;
    color: var(--accent); letter-spacing: -0.02em;
  }
  h1 { font-size: 1.5rem; margin-top: 8px; }
  .msg { color: var(--text-muted); font-size: var(--fs-body); line-height: 1.6; margin: 12px 0 32px; }

  .actions { display: flex; gap: 12px; justify-content: center; flex-wrap: wrap; }
  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 11px 22px; border-radius: var(--radius-md); white-space: nowrap;
  }
  .btn.fill { background: var(--accent); color: var(--on-accent); }
  .btn.fill:hover { background: var(--accent-hover); color: var(--on-accent); }
  .btn.ghost { border: 1px solid var(--border-default); color: var(--text-primary); }
  .btn.ghost:hover { background: var(--surface-raised); color: var(--text-primary); }
</style>
