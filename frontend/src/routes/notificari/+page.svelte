<script lang="ts">
  import { goto } from '$app/navigation';
  import { authStore } from '$lib/stores/auth';
  import { notifications } from '$lib/stores/notifications';
  import { nameHue } from '$lib/avatar';
  import { reltime } from '$lib/reltime';
  import { NOTIF_TYPE_META } from '$lib/data/community';
  import type { Notification } from '$shared/types';

  const auth = $derived($authStore);
  const notif = $derived($notifications);

  let filter = $state<'all' | 'unread'>('all');

  const shown = $derived(
    filter === 'unread' ? notif.items.filter((n) => n.unread) : notif.items
  );

  $effect(() => {
    if (auth.isLoading) return;
    if (!auth.isAuthenticated) {
      goto('/login?redirect=/notificari');
      return;
    }
    notifications.load();
  });

  function open(n: Notification) {
    notifications.markRead(n.id);
    if (n.link) goto(n.link);
  }
</script>

<svelte:head><title>Notificări · Anime-Kage</title></svelte:head>

<div class="container inbox">
  <header class="top">
    <div>
      <p class="kicker">Activitate</p>
      <h1>Notificări</h1>
    </div>
    {#if notif.unread > 0}
      <button class="mark-all" onclick={() => notifications.markAllRead()}>
        Marchează toate ca citite
      </button>
    {/if}
  </header>

  <div class="tabs" role="tablist">
    <button class="tab" class:on={filter === 'all'} onclick={() => (filter = 'all')} role="tab" aria-selected={filter === 'all'}>
      Toate <span class="n">{notif.items.length}</span>
    </button>
    <button class="tab" class:on={filter === 'unread'} onclick={() => (filter = 'unread')} role="tab" aria-selected={filter === 'unread'}>
      Necitite <span class="n">{notif.unread}</span>
    </button>
  </div>

  {#if shown.length === 0}
    <div class="empty">
      <span class="empty-ico" aria-hidden="true">🔔</span>
      <p class="empty-title">
        {filter === 'unread' ? 'Nicio notificare necitită' : 'Nicio notificare încă'}
      </p>
      <p class="empty-sub">
        Aici ajung răspunsurile la comentariile tale, urmăririle noi și publicarea traducerilor tale.
      </p>
    </div>
  {:else}
    <ul class="list">
      {#each shown as n (n.id)}
        {@const meta = NOTIF_TYPE_META[n.type] ?? NOTIF_TYPE_META.system}
        <li>
          <button class="row" class:unread={n.unread} class:link={!!n.link} onclick={() => open(n)}>
            {#if n.actor}
              <span class="ava monogram" style="--mg-hue: {nameHue(n.actor)}">{n.actor[0]?.toUpperCase()}</span>
            {:else}
              <span class="ico" style={`color:${meta.color};background:color-mix(in srgb, ${meta.color} 15%, transparent)`}>{meta.icon}</span>
            {/if}
            <span class="body">
              <span class="text">{n.text}</span>
              <span class="time">{reltime(n.createdAt)}</span>
            </span>
            {#if n.unread}<span class="dot" aria-hidden="true"></span>{/if}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .inbox { padding-block: var(--space-6) var(--space-8); }
  .top {
    display: flex; align-items: flex-end; justify-content: space-between;
    gap: var(--space-4); flex-wrap: wrap; margin-bottom: var(--space-5);
  }
  .kicker {
    font-family: var(--font-mono); font-size: var(--fs-micro); letter-spacing: 0.08em;
    text-transform: uppercase; color: var(--accent); margin-bottom: 6px;
  }
  h1 { font-family: var(--font-display); font-size: var(--fs-h1); font-weight: var(--fw-bold); }
  .mark-all {
    font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--accent);
    background: none; border: 1px solid var(--border-default); border-radius: 9px;
    padding: 8px 14px; cursor: pointer;
  }
  .mark-all:hover { border-color: var(--accent); color: var(--accent-hover); }

  .tabs {
    display: flex; gap: 4px; margin-bottom: var(--space-4);
    border-bottom: 1px solid var(--border-subtle);
  }
  .tab {
    display: inline-flex; align-items: center; gap: 7px;
    padding: 10px 14px; background: none; border: none; cursor: pointer;
    font-size: var(--fs-small); font-weight: var(--fw-medium); color: var(--text-muted);
    border-bottom: 2px solid transparent; margin-bottom: -1px;
  }
  .tab:hover { color: var(--text-primary); }
  .tab.on { color: var(--text-primary); border-bottom-color: var(--accent); }
  .tab .n {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    color: var(--text-muted); background: var(--surface-overlay);
    padding: 1px 7px; border-radius: var(--radius-pill);
  }

  .list { list-style: none; display: flex; flex-direction: column; gap: 8px; }
  .row {
    display: flex; align-items: flex-start; gap: 13px; width: 100%; text-align: left;
    padding: 14px 16px; border-radius: var(--radius-md);
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
    cursor: default;
  }
  .row.link { cursor: pointer; }
  .row.link:hover { border-color: var(--border-default); background: var(--surface-overlay); }
  .row.unread { border-left: 3px solid var(--accent); }
  .ico {
    width: 40px; height: 40px; border-radius: 10px; flex: 0 0 auto;
    display: grid; place-items: center; font-size: 0.9375rem;
  }
  .ava { width: 40px; height: 40px; flex: 0 0 auto; font-size: 0.9375rem; border-radius: 50%; }
  .body { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
  .text { font-size: var(--fs-body); line-height: 1.45; color: var(--text-primary); text-wrap: pretty; }
  .time { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .dot { width: 8px; height: 8px; border-radius: 50%; background: var(--accent); flex: 0 0 auto; margin-top: 7px; }

  .empty {
    display: flex; flex-direction: column; align-items: center; gap: 10px;
    padding: var(--space-8) var(--space-4); text-align: center;
  }
  .empty-ico { font-size: 2.5rem; opacity: 0.35; }
  .empty-title { font-family: var(--font-display); font-size: var(--fs-h3); font-weight: var(--fw-semibold); color: var(--text-primary); }
  .empty-sub { font-size: var(--fs-small); color: var(--text-muted); max-width: 380px; line-height: 1.5; }
</style>
