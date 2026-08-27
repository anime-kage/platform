<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { authStore as auth } from '$lib/stores/auth';
  import type { Snippet } from 'svelte';

  // One shell for the whole panel: guard, editorial header,
  // section tabs. The pages under it stay small and single-purpose.
  // /admin itself is the pipeline overview (admin/moderator); translators
  // land in the catalog, which is the part they can act on.
  let { children }: { children: Snippet } = $props();

  // Coordinators belong here too: they own the weekly programme, the emotes and
  // catalog imports. Without them in this list those tabs rendered for a role
  // that could never reach the panel to see them.
  const allowed = $derived(
    $auth.isAuthenticated &&
      ['admin', 'moderator', 'translator', 'coordinator'].includes($auth.user?.role ?? '')
  );
  const canModerate = $derived(
    $auth.isAuthenticated && ['admin', 'moderator'].includes($auth.user?.role ?? '')
  );
  const canTranslate = $derived(
    $auth.isAuthenticated && ['admin', 'translator'].includes($auth.user?.role ?? '')
  );
  const canReview = $derived(
    $auth.isAuthenticated && ['admin', 'moderator', 'verifier'].includes($auth.user?.role ?? '')
  );
  // who decides what the front page shows — mirrors curateRole on the server
  const canCurate = $derived(
    $auth.isAuthenticated && ['admin', 'coordinator', 'moderator'].includes($auth.user?.role ?? '')
  );
  // who sets the weekly programme — mirrors importRole on the server
  const canSchedule = $derived(
    $auth.isAuthenticated && ['admin', 'coordinator'].includes($auth.user?.role ?? '')
  );
  const roleLabel = $derived(
    $auth.user?.role === 'admin' ? 'ești administrator'
    : $auth.user?.role === 'moderator' ? 'ești moderator'
    : $auth.user?.role === 'coordinator' ? 'ești coordonator'
    : 'ești traducător'
  );

  $effect(() => {
    if ($auth.isLoading) return;
    if (!$auth.isAuthenticated) goto('/login?redirect=/admin');
  });

  const path = $derived(page.url.pathname);
  const section = $derived(
    path.startsWith('/admin/curated') ? 'curated'
    : path.startsWith('/admin/anunturi') ? 'announcements'
    : path.startsWith('/admin/program') ? 'programme'
    : path.startsWith('/admin/emote') ? 'emotes'
    : path.startsWith('/admin/health') ? 'health'
    : path.startsWith('/admin/moderation') ? 'moderation'
    : path.startsWith('/admin/team') ? 'team'
    : path.startsWith('/admin/subtitles') ? 'subtitles'
    : path === '/admin' ? 'pipeline'
    : 'catalog'
  );

  // the pipeline overview needs the full release list — verifier-class only;
  // translators go straight to the catalog
  $effect(() => {
    if (allowed && !canModerate && path === '/admin') goto('/admin/catalog');
  });
</script>

<svelte:head><title>Administrare · Anime-Kage</title></svelte:head>

<div class="container shell">
  {#if !$auth.isLoading && $auth.isAuthenticated && !allowed}
    <div class="denied">
      <h1>Administrare</h1>
      <p>Nu ai acces la panoul de administrare.</p>
      <a class="btn ghost" href="/home">Înapoi acasă</a>
    </div>
  {:else if allowed}
    <header class="top">
      <div>
        <p class="pg-kicker">Echipă · {roleLabel}</p>
        <h1>Administrare</h1>
        <p class="sub">
          Catalogul, echipa și sănătatea pipeline-ului de traduceri — tot ce ține site-ul în
          mișcare, într-un singur loc.
        </p>
      </div>
      <div class="side">
        {#if canTranslate}<a class="jump" href="/translate">Traduceri →</a>{/if}
        {#if canReview}<a class="jump" href="/verify">Verificare →</a>{/if}
      </div>
    </header>

    <nav class="tabs" aria-label="Secțiuni">
      {#if canModerate}
        <a class="tab" class:on={section === 'pipeline'} href="/admin">Pipeline</a>
      {/if}
      <a class="tab" class:on={section === 'catalog'} href="/admin/catalog">Catalog</a>
      {#if canCurate}
        <a class="tab" class:on={section === 'curated'} href="/admin/curated">Vitrină</a>
      {/if}
      {#if canModerate}
        <a class="tab" class:on={section === 'announcements'} href="/admin/anunturi">Anunțuri</a>
      {/if}
      <!-- who decides the weekly programme — mirrors importRole on the server -->
      {#if canSchedule}
        <a class="tab" class:on={section === 'programme'} href="/admin/program">Program</a>
      {/if}
      <!-- emotes: admin/coordinator, same bar as the programme -->
      {#if canSchedule}
        <a class="tab" class:on={section === 'emotes'} href="/admin/emote">Emote</a>
      {/if}
      {#if canTranslate}
        <a class="tab" class:on={section === 'subtitles'} href="/admin/subtitles">Subtitrări</a>
      {/if}
      {#if canModerate}
        <a class="tab" class:on={section === 'team'} href="/admin/team">Echipă</a>
      {/if}
      <a class="tab" class:on={section === 'health'} href="/admin/health">Sănătate</a>
      {#if canModerate}
        <a class="tab" class:on={section === 'moderation'} href="/admin/moderation">Moderare</a>
      {/if}
    </nav>

    {@render children()}
  {/if}
</div>

<style>
  .shell { padding-block: var(--space-6) var(--space-8); }

  .top {
    display: flex; align-items: flex-end; justify-content: space-between;
    flex-wrap: wrap; gap: var(--space-4);
    padding-bottom: 18px; border-bottom: 2px solid var(--text-primary);
  }
  .pg-kicker {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-bold);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--accent);
  }
  h1 {
    font-family: var(--font-display); font-size: var(--fs-h1);
    letter-spacing: -0.015em; line-height: var(--lh-tight); margin-top: 10px;
  }
  .sub { color: var(--text-muted); margin-top: 10px; max-width: 54ch; line-height: 1.55; }
  .side { display: flex; align-items: baseline; gap: var(--space-4); flex-wrap: wrap; }
  .jump { font-size: var(--fs-small); font-weight: var(--fw-semibold); color: var(--accent); }
  .jump:hover { color: var(--accent-hover); }

  .tabs {
    display: flex; gap: var(--space-5);
    border-bottom: 1px solid var(--border-subtle);
    margin-bottom: var(--space-6);
    /* overflow-y must be stated: declaring only overflow-x makes the other
       axis compute to `auto` rather than staying `visible`, and the 1px of
       border overlap below is then enough to raise a vertical scrollbar. */
    overflow-x: auto;
    overflow-y: hidden;
  }
  .tab {
    font-family: var(--font-mono); font-size: var(--fs-caption); font-weight: var(--fw-semibold);
    letter-spacing: 0.1em; text-transform: uppercase;
    color: var(--text-muted); padding: 14px 2px 12px;
    border-bottom: 2px solid transparent; margin-bottom: -1px; white-space: nowrap;
  }
  .tab:hover { color: var(--text-muted); }
  .tab.on { color: var(--text-primary); border-bottom-color: var(--accent); }

  .denied {
    display: flex; flex-direction: column; align-items: center; gap: var(--space-4);
    color: var(--text-muted); padding: var(--space-8) 0; text-align: center;
  }
  .denied h1 { font-family: var(--font-display); font-size: var(--fs-h1); color: var(--text-primary); }
  .btn.ghost {
    border: 1px solid var(--border-default); border-radius: var(--radius-md);
    padding: 10px 18px; font-weight: var(--fw-semibold); font-size: var(--fs-small);
    color: var(--text-primary);
  }
  .btn.ghost:hover { background: var(--surface-overlay); }
</style>
