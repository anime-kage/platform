<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import api from '$lib/api';
  import { authStore } from '$lib/stores/auth';
  import { nameHue } from '$lib/avatar';
  import { theme, toggleTheme } from '$lib/stores/theme';
  import { notifications } from '$lib/stores/notifications';
  import { NOTIF_TYPE_META } from '$lib/data/community';
  import { reltime } from '$lib/reltime';

  const auth = $derived($authStore);
  const initial = $derived(auth.user?.username?.[0]?.toUpperCase() ?? '?');

  let menuOpen = $state(false); // mobile drawer
  let accountOpen = $state(false); // avatar dropdown
  let notifOpen = $state(false); // bell dropdown
  let query = $state('');

  const notif = $derived($notifications);
  const unread = $derived(notif.unread);
  // the bell caps its glyph at 9+ so a big count can't distort the pill
  const unreadLabel = $derived(unread > 9 ? '9+' : String(unread));

  function openNotif() {
    notifOpen = !notifOpen;
    accountOpen = false;
    if (notifOpen) notifications.load();
  }

  function onNotifClick(n: { id: number; link?: string }) {
    notifications.markRead(n.id);
    notifOpen = false;
    if (n.link) goto(n.link);
  }
  // the icon shows the theme you are IN, not the one the next click gives —
  // ☾ dark, ☀ light, ❀ sakura. Click order stays dark → light → sakura.
  const themeIcon = $derived($theme === 'dark' ? '☾' : $theme === 'light' ? '☀' : '❀');
  const themeLabel = $derived(
    $theme === 'dark' ? 'Schimbă pe tema luminoasă' : $theme === 'light' ? 'Schimbă pe tema sakura' : 'Schimbă pe tema întunecată'
  );

  const navItems = [
    { label: 'Home', href: '/home' },
    { label: 'Anime', href: '/anime' },
    { label: 'Manga', href: '/manga' },
    { label: 'Liste', href: '/liste' },
    { label: 'Cereri', href: '/cereri' },
    { label: 'Comunitate', href: '/comunitate' }
  ];

  // team pages, shown only to roles that can actually open them: translators
  // translate, verifiers verify, coordinators publish, admins see everything
  const roleNav = $derived.by(() => {
    const role = auth.user?.role ?? '';
    const items: { label: string; href: string }[] = [];
    if (['translator', 'admin'].includes(role)) {
      items.push({ label: 'Traduceri', href: '/translate' });
    }
    if (['verifier', 'coordinator', 'moderator', 'admin'].includes(role)) {
      items.push({ label: 'Verificare', href: '/verify' });
    }
    if (['coordinator', 'admin'].includes(role)) {
      items.push({ label: 'Publicare', href: '/publish' });
    }
    if (['translator', 'moderator', 'admin'].includes(role)) {
      items.push({ label: 'Admin', href: '/admin' });
    }
    return items;
  });

  const isActive = (href: string) => page.url.pathname.startsWith(href);

  // The landing (public front door) shows a bare nav: logo + auth actions.
  const onLanding = $derived(page.url.pathname === '/');

  function submitSearch(e: SubmitEvent) {
    e.preventDefault();
    const q = query.trim();
    menuOpen = false;
    // /cauta, not /anime?q= : the box says "Caută titlu", and a member
    // searching a manga title used to get an empty anime catalog with no sign
    // the manga existed.
    goto(q ? `/cauta?q=${encodeURIComponent(q)}` : '/anime');
  }

  async function logout() {
    accountOpen = false;
    await authStore.logout();
    goto('/');
  }

  function closeAll() {
    menuOpen = false;
    accountOpen = false;
    notifOpen = false;
  }
</script>

<svelte:window
  onkeydown={(e) => {
    if (e.key === 'Escape') closeAll();
  }}
/>

<header class="nav" class:bare={onLanding}>
  <div class="inner">
    <a class="brand" href={auth.isAuthenticated ? '/home' : '/'} onclick={closeAll}>
      <img class="mark" src="/logo.png" alt="" width="34" height="34" />
      <span class="wordmark">Anime<span class="dot">·</span>Kage</span>
    </a>

    {#if !onLanding}
      <nav class="links" aria-label="Navigare principală">
        {#each navItems as n}
          <a class="link" class:active={isActive(n.href)} href={n.href}>{n.label}</a>
        {/each}
        {#each roleNav as n}
          <a class="link team" class:active={isActive(n.href)} href={n.href}>{n.label}</a>
        {/each}
      </nav>
    {/if}

    <div class="spacer"></div>

    {#if auth.isAuthenticated && auth.user}
      <form class="search" onsubmit={submitSearch} role="search">
        <span class="search-ico" aria-hidden="true">⌕</span>
        <input type="search" placeholder="Caută titlu, gen..." bind:value={query} />
      </form>

      <button class="icon-btn" title={themeLabel} aria-label={themeLabel} onclick={toggleTheme}>
        {themeIcon}
      </button>

      <div class="popwrap">
        <button
          class="icon-btn"
          class:has-unread={unread > 0}
          onclick={openNotif}
          aria-expanded={notifOpen}
          aria-label={unread > 0 ? `Notificări — ${unread} necitite` : 'Notificări'}
        >
          <span aria-hidden="true">🔔</span>
          {#if unread > 0}<span class="badge">{unreadLabel}</span>{/if}
        </button>
        {#if notifOpen}
          <button class="scrim" onclick={() => (notifOpen = false)} aria-label="Închide"></button>
          <div class="pop notif anim-fade">
            <div class="pop-head">
              <span class="pop-title">Notificări</span>
              {#if unread > 0}
                <button class="mark-read" onclick={() => notifications.markAllRead()}>
                  Marchează toate
                </button>
              {/if}
            </div>
            <div class="notif-list">
              {#if notif.items.length === 0}
                <div class="notif-empty">
                  <span class="empty-ico" aria-hidden="true">🔔</span>
                  <span class="empty-title">Nicio notificare</span>
                  <span class="empty-sub">Aici ajung răspunsurile, urmăririle și publicările tale.</span>
                </div>
              {:else}
                {#each notif.items as n (n.id)}
                  {@const meta = NOTIF_TYPE_META[n.type] ?? NOTIF_TYPE_META.system}
                  <button
                    type="button"
                    class="notif-row"
                    class:unread={n.unread}
                    onclick={() => onNotifClick(n)}
                  >
                    {#if n.actor}
                      <span class="notif-ava monogram" style="--mg-hue: {nameHue(n.actor)}">
                        {n.actor[0]?.toUpperCase()}
                      </span>
                    {:else}
                      <span
                        class="notif-ico"
                        style={`color:${meta.color};background:color-mix(in srgb, ${meta.color} 15%, transparent)`}
                      >{meta.icon}</span>
                    {/if}
                    <span class="notif-body">
                      <span class="notif-text">{n.text}</span>
                      <span class="notif-time">{reltime(n.createdAt)}</span>
                    </span>
                    {#if n.unread}<span class="dot-unread" aria-hidden="true"></span>{/if}
                  </button>
                {/each}
              {/if}
            </div>
            <a class="notif-all" href="/notificari" onclick={() => (notifOpen = false)}>
              Vezi toate notificările
            </a>
          </div>
        {/if}
      </div>

      <div class="popwrap">
        <button
          class="avatar"
          class:monogram={!auth.user.avatarUrl}
          style="--mg-hue: {nameHue(auth.user.username)}"
          onclick={() => { accountOpen = !accountOpen; notifOpen = false; }}
          aria-expanded={accountOpen}
          aria-label="Contul meu"
        >
          {#if auth.user.avatarUrl}
            <img src={api.resolveUrl(auth.user.avatarUrl)} alt={auth.user.username} />
          {:else}
            {initial}
          {/if}
        </button>
        {#if accountOpen}
          <button class="scrim" onclick={() => (accountOpen = false)} aria-label="Închide meniul"></button>
          <div class="pop menu anim-fade" role="menu">
            <div class="menu-head">
              <span class="avatar sm" class:monogram={!auth.user.avatarUrl} style="--mg-hue: {nameHue(auth.user.username)}">
                {#if auth.user.avatarUrl}<img src={api.resolveUrl(auth.user.avatarUrl)} alt={auth.user.username} />{:else}{initial}{/if}
              </span>
              <div class="who">
                <span class="who-name">{auth.user.username}</span>
                <span class="who-mail">{auth.user.email}</span>
              </div>
            </div>
            <a role="menuitem" href="/profile" onclick={closeAll}><span class="mi" aria-hidden="true">☉</span>Profilul meu</a>
            <a role="menuitem" href="/lista" onclick={closeAll}><span class="mi" aria-hidden="true">✓</span>Watchlist</a>
            <a role="menuitem" href="/istoric" onclick={closeAll}><span class="mi" aria-hidden="true">⟳</span>Istoric</a>
            <a role="menuitem" href="/calendar" onclick={closeAll}><span class="mi" aria-hidden="true">▦</span>Calendar</a>
            {#if auth.user.role === 'admin' || auth.user.role === 'moderator' || auth.user.role === 'translator'}
              <a role="menuitem" href="/admin" onclick={closeAll}><span class="mi" aria-hidden="true">⚙</span>Administrare</a>
            {/if}
            <button role="menuitem" class="theme-item" onclick={toggleTheme}><span class="mi" aria-hidden="true">{themeIcon}</span>{themeLabel}</button>
            <button role="menuitem" class="logout" onclick={logout}><span class="mi" aria-hidden="true">↪</span>Deconectare</button>
          </div>
        {/if}
      </div>
    {:else}
      <div class="auth-btns">
        <button class="icon-btn" title={themeLabel} aria-label={themeLabel} onclick={toggleTheme}>
          {themeIcon}
        </button>
        <a class="btn ghost" href="/login">Autentificare</a>
        <a class="btn fill" href="/register">Înregistrare</a>
      </div>
    {/if}

    {#if !onLanding}
      <button
        class="burger"
        onclick={() => (menuOpen = !menuOpen)}
        aria-expanded={menuOpen}
        aria-label="Meniu"
      >
        {menuOpen ? '✕' : '☰'}
      </button>
    {/if}
  </div>

  {#if menuOpen && !onLanding}
    <div class="drawer">
      <form class="search drawer-search" onsubmit={submitSearch} role="search">
        <span class="search-ico" aria-hidden="true">⌕</span>
        <input type="search" placeholder="Caută titlu, gen..." bind:value={query} />
      </form>
      {#each navItems as n}
        <a class="drawer-link" class:active={isActive(n.href)} href={n.href} onclick={closeAll}>
          {n.label}
        </a>
      {/each}
      {#each roleNav as n}
        <a class="drawer-link team" class:active={isActive(n.href)} href={n.href} onclick={closeAll}>
          {n.label}
        </a>
      {/each}
      {#if !auth.isAuthenticated}
        <div class="drawer-auth">
          <a class="btn ghost" href="/login" onclick={closeAll}>Autentificare</a>
          <a class="btn fill" href="/register" onclick={closeAll}>Înregistrare</a>
        </div>
      {/if}
    </div>
  {/if}
</header>

<style>
  .nav {
    position: sticky;
    top: 0;
    z-index: var(--z-header);
    border-bottom: 1px solid var(--border-subtle);
    background: color-mix(in srgb, var(--surface-base) 82%, transparent);
    backdrop-filter: blur(14px);
    -webkit-backdrop-filter: blur(14px);
  }
  .inner {
    display: flex; align-items: center; gap: 18px;
    height: 62px; padding: 0 clamp(1rem, 3vw, 1.75rem);
  }

  .brand { display: flex; align-items: center; gap: 11px; flex: 0 0 auto; margin-right: 6px; }
  .mark { width: 34px; height: 34px; object-fit: contain; }
  .wordmark {
    font-family: var(--font-display); font-weight: var(--fw-semibold);
    font-size: 1.1875rem; color: var(--text-primary); letter-spacing: 0.1px;
  }
  .dot { color: var(--accent); }

  .links { display: flex; align-items: center; gap: 3px; margin-left: 8px; }
  .link {
    font-size: var(--fs-small); font-weight: var(--fw-medium); color: var(--text-muted);
    padding: 8px 12px; border-radius: 9px;
    transition: color var(--motion-fast) var(--ease), background var(--motion-fast) var(--ease);
  }
  .link:hover { color: var(--text-primary); background: var(--surface-raised); }
  .link.active { color: var(--text-primary); background: var(--surface-raised); }
  /* team pages get the accent so staff can spot them instantly */
  .link.team { color: var(--accent); }
  .link.team:hover, .link.team.active { color: var(--accent-hover); }

  .spacer { flex: 1; }

  .search {
    display: flex; align-items: center; gap: 8px;
    width: 230px; padding: 8px 13px;
    border-radius: 9px;
    background: var(--surface-raised); border: 1px solid var(--border-subtle);
  }
  .search:focus-within { border-color: var(--accent); }
  .search-ico { color: var(--text-faint); font-size: 0.875rem; line-height: 1; }
  .search input {
    flex: 1; min-width: 0; background: none; border: none; outline: none;
    color: var(--text-primary); font-size: 0.84375rem;
  }
  .search input::placeholder { color: var(--text-faint); }

  .popwrap { position: relative; flex: 0 0 auto; }

  .icon-btn {
    position: relative; width: 38px; height: 38px; border-radius: 9px;
    border: 1px solid var(--border-subtle); background: var(--surface-raised);
    color: var(--text-muted); display: grid; place-items: center;
    font-size: 0.9375rem; cursor: pointer;
  }
  .icon-btn:hover { border-color: var(--border-default); color: var(--text-primary); }
  /* The badge is a fixed 16px pill regardless of viewport — it must NOT ride
     the fluid type scale, or on a wide screen the count grew past the pill and
     spilled off the bell. Count is capped at "9+" in the markup so it never
     needs more than two glyphs; the font is a fixed 10.5px. */
  .badge {
    position: absolute; top: -4px; right: -4px;
    min-width: 16px; height: 16px; padding: 0 4px;
    box-sizing: border-box;
    border-radius: var(--radius-pill);
    background: var(--accent); color: var(--on-accent);
    font-family: var(--font-mono); font-size: 10.5px; font-weight: var(--fw-bold);
    line-height: 1; letter-spacing: -0.02em;
    display: grid; place-items: center;
    border: 2px solid var(--surface-base);
  }

  .scrim { position: fixed; inset: 0; z-index: 60; background: none; border: none; cursor: default; }
  .pop {
    position: absolute; top: 48px; right: 0; z-index: 61;
    background: var(--surface-raised); border: 1px solid var(--border-default);
    border-radius: var(--radius-lg); overflow: hidden; box-shadow: var(--shadow-3);
  }

  .notif { width: min(360px, calc(100vw - 24px)); }
  .pop-head {
    display: flex; align-items: center; justify-content: space-between;
    padding: 14px 16px; border-bottom: 1px solid var(--border-subtle);
  }
  .pop-title { font-family: var(--font-display); font-size: 1rem; font-weight: var(--fw-semibold); }
  .mark-read {
    font-size: 0.75rem; font-weight: var(--fw-semibold); color: var(--accent);
    background: none; border: none; cursor: pointer;
  }
  .mark-read:hover { color: var(--accent-hover); }
  .notif-list { max-height: 60vh; overflow-y: auto; }
  .notif-row {
    display: flex; gap: 12px; width: 100%; text-align: left;
    padding: 12px 15px; background: none; border: none;
    border-bottom: 1px solid var(--border-subtle); cursor: pointer;
    align-items: flex-start;
  }
  .notif-row:hover { background: var(--surface-overlay); }
  .notif-row.unread { background: color-mix(in srgb, var(--accent) 8%, transparent); }
  .notif-ico {
    width: 34px; height: 34px; border-radius: 9px; flex: 0 0 auto;
    display: grid; place-items: center; font-size: 0.8125rem;
  }
  .notif-ava {
    width: 34px; height: 34px; flex: 0 0 auto;
    font-size: 0.8125rem; border-radius: 50%;
  }
  .notif-body { flex: 1; min-width: 0; display: flex; flex-direction: column; }
  .notif-text {
    font-size: var(--fs-caption); line-height: 1.45; color: var(--text-primary);
    text-wrap: pretty;
  }
  .notif-time {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    color: var(--text-muted); margin-top: 4px;
  }
  .dot-unread {
    width: 7px; height: 7px; border-radius: 50%; background: var(--accent);
    flex: 0 0 auto; margin-top: 6px;
  }

  .notif-empty {
    display: flex; flex-direction: column; align-items: center; gap: 6px;
    padding: 34px 24px; text-align: center;
  }
  .empty-ico { font-size: 1.5rem; opacity: 0.4; }
  .empty-title { font-weight: var(--fw-semibold); color: var(--text-primary); font-size: var(--fs-small); }
  .empty-sub { font-size: var(--fs-caption); color: var(--text-muted); line-height: 1.4; max-width: 220px; }

  .notif-all {
    display: block; text-align: center; padding: 12px;
    font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--accent);
    border-top: 1px solid var(--border-subtle);
  }
  .notif-all:hover { background: var(--surface-overlay); color: var(--accent-hover); }

  .avatar {
    width: 38px; height: 38px; cursor: pointer; font-size: 1rem;
    border-radius: 26%; overflow: hidden; display: grid; place-items: center;
  }
  .avatar img { width: 100%; height: 100%; object-fit: cover; display: block; }
  .avatar.sm { width: 32px; height: 32px; font-size: 0.875rem; }

  .menu { width: 238px; }
  .menu-head {
    display: flex; align-items: center; gap: 11px; padding: 14px 15px;
    border-bottom: 1px solid var(--border-subtle);
  }
  .who { min-width: 0; display: flex; flex-direction: column; }
  .who-name {
    font-family: var(--font-display); font-weight: var(--fw-semibold); font-size: var(--fs-body);
    color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .who-mail {
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
    margin-top: 2px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .menu a,
  .menu .theme-item,
  .menu .logout {
    display: flex; align-items: center; gap: 12px; width: 100%; text-align: left;
    padding: 11px 15px; font-size: var(--fs-small); color: var(--text-primary);
    background: none; border: none; cursor: pointer;
  }
  .mi { width: 16px; text-align: center; color: var(--text-muted); font-size: 0.875rem; }
  .menu a:hover,
  .menu .theme-item:hover,
  .menu .logout:hover { background: var(--surface-overlay); color: var(--text-primary); }
  .menu .logout { color: var(--text-muted); border-top: 1px solid var(--border-subtle); }

  .auth-btns { display: flex; align-items: center; gap: 10px; }
  .btn {
    font-size: var(--fs-small); font-weight: var(--fw-semibold);
    padding: 9px 16px; border-radius: 9px; white-space: nowrap;
  }
  .btn.ghost { border: 1px solid var(--border-default); color: var(--text-primary); font-weight: var(--fw-medium); }
  .btn.ghost:hover { background: var(--surface-raised); color: var(--text-primary); }
  .btn.fill { background: var(--accent); color: var(--on-accent); }
  .btn.fill:hover { background: var(--accent-hover); color: var(--on-accent); }

  .burger {
    display: none; width: 42px; height: 42px; flex: 0 0 auto;
    border: 1px solid var(--border-subtle); border-radius: 9px;
    background: var(--surface-raised); color: var(--text-primary);
    font-size: 1.0625rem; cursor: pointer;
  }

  .drawer {
    display: none; flex-direction: column; gap: 2px;
    padding: var(--space-3) clamp(1rem, 4vw, 2.5rem) var(--space-5);
    border-top: 1px solid var(--border-subtle);
  }
  .drawer-search { width: 100%; margin-bottom: var(--space-3); }
  .drawer-link {
    padding: 12px 10px; border-radius: 9px;
    font-size: var(--fs-body); font-weight: var(--fw-medium); color: var(--text-muted);
  }
  .drawer-link:hover,
  .drawer-link.active { color: var(--text-primary); background: var(--surface-raised); }
  .drawer-link.team { color: var(--accent); }
  .drawer-auth { display: flex; gap: 9px; margin-top: var(--space-3); }
  .drawer-auth .btn { flex: 1; text-align: center; }

  /* Laptop widths: a team member's bar carries up to 10 nav items (the four
     role links are conditional) plus a 230px search, which overran a 1366
     screen by ~96px and pushed the avatar off the edge. Let the search give
     up width first, then tighten the link padding — the nav itself must not
     be the thing that wraps. */
  @media (max-width: 1400px) {
    .inner { gap: 12px; }
    .search { width: clamp(150px, 14vw, 230px); }
    .link { padding: 8px 9px; }
    .links { margin-left: 0; }
  }

  @media (max-width: 1080px) {
    .links,
    .search:not(.drawer-search),
    .auth-btns { display: none; }
    .burger { display: grid; place-items: center; }
    .drawer { display: flex; }
    /* landing has no drawer, so its auth actions stay in the bar */
    .nav.bare .auth-btns { display: flex; }
  }

  /* Phone: the bar held brand + theme + notifications + avatar + burger =
     451px of controls in a 390px viewport, which pushed the burger off-screen
     and made the whole document scroll sideways. Drop the standalone theme
     button (it is also in the avatar menu, so nothing becomes unreachable)
     and tighten the gaps. */
  @media (max-width: 640px) {
    .inner { gap: 10px; padding: 0 var(--space-4); }
    .inner > .icon-btn { display: none; }
    .brand { margin-right: 0; }
    .wordmark { font-size: 1.0625rem; }
  }
  /* very small phones: the wordmark goes, the logo mark carries the brand */
  @media (max-width: 380px) {
    .wordmark { display: none; }
  }

  /* The landing has no drawer, so both auth CTAs stay in the bar — brand +
     theme + two buttons came to 495px of content in a 390px phone. The mark
     carries the brand and the buttons tighten. The theme toggle STAYS: the
     landing page body has no theme control, and for a logged-out visitor on
     a phone this is the only one on the site. */
  @media (max-width: 640px) {
    .nav.bare .wordmark { display: none; }
    .nav.bare .auth-btns { gap: 8px; min-width: 0; }
    .nav.bare .auth-btns .btn {
      padding-inline: 10px;
      font-size: var(--fs-micro);
      white-space: nowrap;
    }
  }
</style>
