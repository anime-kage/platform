<script lang="ts">
  import { api } from '$lib/api';
  import { toast } from '$lib/stores/toast';
  import { nameHue } from '$lib/avatar';
  import { ROLE_BADGES } from '$lib/data/community';
  import type { User, UserStats, FollowNetwork, ChatRestriction, ProfileBanner } from '$shared/types';

  // The Twitch-style card behind a chat username: who this is, when they
  // joined, and — for staff — the room's own timeout/ban controls.
  //
  // Positioned by the caller (fixed coordinates from the clicked name), so it
  // escapes the message list's overflow instead of being clipped by it.
  interface Props {
    username: string;
    role: string;
    /** viewport coordinates of the name that was clicked */
    anchor: { x: number; y: number };
    /** true when the viewer may hand out timeouts/bans to THIS member */
    canModerate: boolean;
    onMention: (u: string) => void;
    onReply: () => void;
    onClose: () => void;
  }
  let { username, role, anchor, canModerate, onMention, onReply, onClose }: Props = $props();

  const WIDTH = 292;

  let user = $state<Omit<User, 'email'> | null>(null);
  let stats = $state<UserStats | null>(null);
  let network = $state<FollowNetwork | null>(null);
  // the member's chosen series backdrop; null for most people
  let banner = $state<ProfileBanner | null>(null);
  let restriction = $state<ChatRestriction | null>(null);
  let loading = $state(true);
  let failed = $state(false);
  let busy = $state(false);
  let panel = $state<HTMLDivElement | null>(null);
  let modOpen = $state(false);
  let reason = $state('');

  const badge = $derived(ROLE_BADGES[role]);
  const hue = $derived(nameHue(username));

  // Clamp into the viewport: the chat sits at the right edge, so a card the
  // width of the panel would otherwise hang off-screen on every message.
  //
  // The height is *measured*, not assumed — the card grows by ~150px when the
  // moderation drawer opens, and a guessed height puts the ban button below
  // the fold exactly when someone needs it.
  let height = $state(0);
  const pos = $derived.by(() => {
    if (typeof window === 'undefined') return { left: anchor.x, top: anchor.y };
    const left = Math.min(Math.max(8, anchor.x), window.innerWidth - WIDTH - 8);
    const top = Math.min(Math.max(8, anchor.y), Math.max(8, window.innerHeight - height - 8));
    return { left, top };
  });

  $effect(() => {
    if (!panel || typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(() => (height = panel?.offsetHeight ?? 0));
    ro.observe(panel);
    return () => ro.disconnect();
  });

  $effect(() => {
    let stopped = false;
    (async () => {
      try {
        const r = await api.getPublicUser(username);
        if (stopped) return;
        user = r.user;
        stats = r.stats;
        network = r.network;
        banner = r.banner ?? null;
      } catch {
        if (!stopped) failed = true;
      } finally {
        if (!stopped) loading = false;
      }
      // staff-only, and a separate call so a 403 here never blanks the profile
      if (!canModerate || stopped) return;
      try {
        const r = await api.getChatRestriction(username);
        if (!stopped) restriction = r.data;
      } catch {
        /* the card is still useful without it */
      }
    })();
    return () => {
      stopped = true;
    };
  });

  // focus the card so Escape and tab-away land somewhere sensible
  $effect(() => {
    panel?.focus();
  });

  const joined = (d: string | Date) =>
    new Date(d).toLocaleDateString('ro-RO', { month: 'long', year: 'numeric' });

  const restrictionLabel = $derived.by(() => {
    if (!restriction) return null;
    if (!restriction.expiresAt) return 'Blocat permanent din chat';
    const left = new Date(restriction.expiresAt).getTime() - Date.now();
    if (left <= 0) return null;
    const mins = Math.ceil(left / 60000);
    if (mins < 60) return `Timeout · încă ${mins} min`;
    const hours = Math.ceil(mins / 60);
    if (hours < 24) return `Timeout · încă ${hours} h`;
    return `Timeout · încă ${Math.ceil(hours / 24)} zile`;
  });

  // Twitch's ladder, in seconds; 0 is the ban
  const TIMEOUTS: { s: number; label: string }[] = [
    { s: 60, label: '1 min' },
    { s: 300, label: '5 min' },
    { s: 600, label: '10 min' },
    { s: 3600, label: '1 oră' },
    { s: 86400, label: '24 h' },
    { s: 604800, label: '7 zile' }
  ];

  async function restrict(seconds: number) {
    if (busy) return;
    busy = true;
    try {
      const r = await api.setChatRestriction(username, seconds, reason.trim() || undefined);
      toast.success(r.message);
      const fresh = await api.getChatRestriction(username);
      restriction = fresh.data;
      reason = '';
      modOpen = false;
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Acțiunea a eșuat.');
    } finally {
      busy = false;
    }
  }

  async function lift() {
    if (busy) return;
    busy = true;
    try {
      const r = await api.clearChatRestriction(username);
      toast.success(r.message);
      restriction = null;
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Acțiunea a eșuat.');
    } finally {
      busy = false;
    }
  }
</script>

<svelte:window
  onkeydown={(e) => {
    if (e.key === 'Escape') onClose();
  }}
/>

<!-- click-outside: a transparent full-screen button under the card -->
<button class="uc-scrim" aria-label="Închide cardul" onclick={onClose}></button>

<div
  class="uc"
  bind:this={panel}
  tabindex="-1"
  role="dialog"
  aria-label={`Profil ${username}`}
  style={`left:${pos.left}px; top:${pos.top}px; width:${WIDTH}px`}
>
  <!-- Backdrop: same art and the same treatment as the profile page, so the
       card reads as a small version of that profile rather than its own
       thing. Rendered only once loaded, so the card never reserves space for
       a banner most members don't have. -->
  {#if banner}
    <div class="uc-banner" title={banner.title}>
      <div class="uc-banner-art" style={`background-image:url(${banner.bannerUrl})`}></div>
    </div>
  {/if}

  <div class="uc-head" class:on-banner={!!banner}>
    <span class="uc-ava" class:monogram={!user?.avatarUrl} style={`--mg-hue:${hue}`}>
      {#if user?.avatarUrl}
        <img src={api.resolveUrl(user.avatarUrl)} alt="" />
      {:else}
        {username.charAt(0)}
      {/if}
    </span>
    <div class="uc-id">
      <div class="uc-name-row">
        <a
          class="uc-name"
          href={`/user/${encodeURIComponent(username)}`}
          style={`color:hsl(${hue} 62% 62%)`}
          title={`Vezi profilul lui ${username}`}
          onclick={onClose}>{username}</a
        >
        {#if badge}
          <span class="uc-badge" title={badge.title} style={`background:${badge.bg}`}>{badge.glyph}</span>
        {/if}
      </div>
      {#if badge}<span class="uc-role">{badge.title}</span>{/if}
    </div>
    <button class="uc-x" title="Închide" onclick={onClose}>×</button>
  </div>

  {#if loading}
    <p class="uc-note">Se încarcă…</p>
  {:else if failed}
    <p class="uc-note">Profilul nu a putut fi încărcat.</p>
  {:else if user}
    <p class="uc-joined">Membru din {joined(user.createdAt)}</p>
    {#if user.bio}<p class="uc-bio">{user.bio}</p>{/if}

    <div class="uc-stats">
      <div><b>{stats?.totalAnimeWatched ?? 0}</b><span>anime</span></div>
      <div><b>{stats?.totalEpisodesWatched ?? 0}</b><span>episoade</span></div>
      <div><b>{stats?.totalChaptersRead ?? 0}</b><span>capitole</span></div>
      <div><b>{network?.followers ?? 0}</b><span>urmăritori</span></div>
    </div>
  {/if}

  <div class="uc-actions">
    <button class="uc-btn" onclick={() => { onMention(username); onClose(); }}>Menționează</button>
    <button class="uc-btn" onclick={() => { onReply(); onClose(); }}>Răspunde</button>
    <a class="uc-btn" href={`/user/${encodeURIComponent(username)}`} onclick={onClose}>Profil</a>
  </div>

  {#if canModerate}
    <div class="uc-mod">
      {#if restrictionLabel}
        <div class="uc-live">
          <span class="uc-live-txt">
            {restrictionLabel}{#if restriction?.byName}{' · de '}{restriction.byName}{/if}
          </span>
          <button class="uc-lift" disabled={busy} onclick={lift}>Ridică</button>
        </div>
        {#if restriction?.reason}<p class="uc-reason">„{restriction.reason}”</p>{/if}
      {/if}

      {#if !modOpen}
        <button class="uc-modtoggle" onclick={() => (modOpen = true)}>Moderare chat</button>
      {:else}
        <!-- the wording matters: staff must not think this suspends the account -->
        <p class="uc-modnote">Se aplică doar în chatul live, nu pe cont.</p>
        <input class="uc-reason-in" bind:value={reason} maxlength={200} placeholder="Motiv (opțional)" />
        <div class="uc-grid">
          {#each TIMEOUTS as t}
            <button class="uc-to" disabled={busy} onclick={() => restrict(t.s)}>{t.label}</button>
          {/each}
        </div>
        <button class="uc-ban" disabled={busy} onclick={() => restrict(0)}>Ban permanent din chat</button>
      {/if}
    </div>
  {/if}
</div>

<style>
  /* Above the chat drawer in BOTH modes — it's 80 docked but 210 as a phone
     sheet, and at anything lower the composer sits on top of the card and
     eats its clicks. */
  .uc-scrim {
    position: fixed; inset: 0; z-index: 219;
    border: none; background: transparent; cursor: default;
  }
  .uc {
    position: fixed; z-index: 220;
    background: var(--surface-raised);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-lg, 12px);
    box-shadow: 0 18px 46px rgba(0, 0, 0, 0.45);
    padding: 13px; outline: none;
    /* last resort: on a short viewport even a top-clamped card can be taller
       than the screen, and scrolling it beats hiding the ban button */
    max-height: calc(100vh - 16px); overflow-y: auto;
  }

  /* ---- backdrop (PLAN 8.17) ----
     Full-bleed to the card edges — the negative margins cancel the card's
     13px padding — with the avatar overlapping its lower edge, the usual
     profile-header shape. Same desaturate + fade as the profile page so the
     two read as one design, and so the name stays legible over any art. */
  .uc-banner {
    height: 116px; margin: -13px -13px 0;
    border-radius: var(--radius-lg, 12px) var(--radius-lg, 12px) 0 0;
    overflow: hidden; position: relative;
  }
  .uc-banner-art {
    width: 100%; height: 100%;
    background-size: cover; background-position: center 30%;
    filter: saturate(0.82) contrast(1.02) brightness(0.72);
  }
  /* Reaches solid card colour at 84%, not at the very bottom: the head row is
     pulled up into that band, so the username and the close button always sit
     on flat surface rather than on whatever the art happens to be there.
     Raised from 76% along with the height, so more of the art is actually
     visible while the text keeps a flat surface underneath it. */
  .uc-banner::after {
    content: ''; position: absolute; inset: 0;
    background: linear-gradient(to bottom, transparent 45%, var(--surface-raised) 84%);
  }

  /* The name is a link now, so the underline and inherited link colour have to
     be undone -- it still uses the per-user hue set inline. */
  a.uc-name { text-decoration: none; }
  a.uc-name:hover { text-decoration: underline; }
  a.uc-name:focus-visible { outline: 2px solid currentColor; outline-offset: 2px; border-radius: 3px; }

  .uc-head { display: flex; align-items: flex-start; gap: 10px; }
  /* pull the row up so the avatar straddles the banner's lower edge */
  .uc-head.on-banner { margin-top: -17px; position: relative; }
  .uc-head.on-banner .uc-ava { box-shadow: 0 0 0 2px var(--surface-raised); }
  .uc-ava {
    flex: 0 0 auto; width: 42px; height: 42px; border-radius: 26%;
    overflow: hidden; font-size: 1.125rem;
  }
  .uc-ava img { width: 100%; height: 100%; object-fit: cover; display: block; }
  .uc-id { flex: 1; min-width: 0; }
  .uc-name-row { display: flex; align-items: center; gap: 6px; }
  .uc-name {
    font-weight: var(--fw-bold); font-size: 0.9375rem;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .uc-badge {
    flex: 0 0 auto; display: inline-grid; place-items: center;
    width: 17px; height: 17px; border-radius: 5px;
    font-size: 0.625rem; color: #fff;
  }
  .uc-role { font-size: var(--fs-micro); color: var(--text-muted); }
  .uc-x {
    flex: 0 0 auto; width: 24px; height: 24px; border-radius: 6px; border: none;
    background: transparent; color: var(--text-muted); cursor: pointer;
    font-size: 1rem; line-height: 1; display: grid; place-items: center;
  }
  .uc-x:hover { background: var(--surface-overlay); color: var(--text-primary); }

  .uc-note { margin: 12px 2px; font-size: 0.75rem; color: var(--text-muted); }
  .uc-joined { margin: 10px 0 0; font-size: 0.75rem; color: var(--text-muted); }
  .uc-bio {
    margin: 7px 0 0; font-size: 0.78125rem; line-height: 1.5; color: var(--text-muted);
    display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3;
    -webkit-box-orient: vertical; overflow: hidden;
  }

  .uc-stats {
    display: grid; grid-template-columns: repeat(4, 1fr); gap: 4px;
    margin-top: 11px; padding: 9px 0;
    border-top: 1px solid var(--border-subtle); border-bottom: 1px solid var(--border-subtle);
  }
  .uc-stats div { display: flex; flex-direction: column; align-items: center; gap: 1px; }
  .uc-stats b { font-family: var(--font-mono); font-size: 0.875rem; color: var(--text-primary); }
  .uc-stats span { font-size: 0.625rem; color: var(--text-muted); }

  .uc-actions { display: flex; gap: 6px; margin-top: 11px; }
  .uc-btn {
    flex: 1; text-align: center; text-decoration: none;
    padding: 7px 4px; border-radius: 8px;
    border: 1px solid var(--border-subtle); background: var(--surface-overlay);
    color: var(--text-primary); cursor: pointer;
    font-family: inherit; font-size: 0.71875rem; font-weight: var(--fw-semibold);
  }
  .uc-btn:hover { border-color: var(--accent); color: var(--accent); }

  .uc-mod { margin-top: 11px; padding-top: 10px; border-top: 1px solid var(--border-subtle); }
  .uc-live {
    display: flex; align-items: center; gap: 8px; margin-bottom: 8px;
    padding: 6px 6px 6px 9px; border-radius: 8px;
    background: color-mix(in srgb, var(--danger) 12%, transparent);
    border-left: 2px solid var(--danger);
  }
  .uc-live-txt { flex: 1; min-width: 0; font-size: var(--fs-micro); color: var(--text-primary); }
  .uc-lift {
    flex: 0 0 auto; padding: 3px 8px; border-radius: 6px;
    border: 1px solid var(--border-subtle); background: var(--surface-raised);
    color: var(--text-primary); cursor: pointer; font-family: inherit; font-size: var(--fs-micro);
  }
  .uc-lift:hover:not(:disabled) { border-color: var(--success); color: var(--success); }
  .uc-reason {
    margin: -4px 0 8px; font-size: var(--fs-micro); color: var(--text-muted); font-style: italic;
  }

  .uc-modtoggle {
    width: 100%; padding: 7px; border-radius: 8px;
    border: 1px dashed var(--border-default); background: transparent;
    color: var(--text-muted); cursor: pointer;
    font-family: inherit; font-size: 0.71875rem; font-weight: var(--fw-semibold);
  }
  .uc-modtoggle:hover { border-color: var(--danger); color: var(--danger); }
  .uc-modnote { margin: 0 0 7px; font-size: var(--fs-micro); color: var(--text-muted); }
  .uc-reason-in {
    width: 100%; margin-bottom: 7px; padding: 6px 9px;
    background: var(--surface-overlay); border: 1px solid var(--border-subtle);
    border-radius: 8px; color: var(--text-primary); outline: none;
    font-family: inherit; font-size: 0.71875rem;
  }
  .uc-reason-in:focus { border-color: var(--accent); }
  .uc-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 5px; }
  .uc-to {
    padding: 6px 2px; border-radius: 7px;
    border: 1px solid var(--border-subtle); background: var(--surface-overlay);
    color: var(--text-primary); cursor: pointer;
    font-family: var(--font-mono); font-size: var(--fs-micro);
  }
  .uc-to:hover:not(:disabled) { border-color: var(--warning, var(--accent)); color: var(--warning, var(--accent)); }
  .uc-ban {
    width: 100%; margin-top: 6px; padding: 7px; border-radius: 8px;
    border: 1px solid color-mix(in srgb, var(--danger) 55%, transparent);
    background: color-mix(in srgb, var(--danger) 12%, transparent);
    color: var(--danger); cursor: pointer;
    font-family: inherit; font-size: 0.71875rem; font-weight: var(--fw-semibold);
  }
  .uc-ban:hover:not(:disabled) { background: var(--danger); color: #fff; }
  .uc-to:disabled, .uc-ban:disabled, .uc-lift:disabled { opacity: 0.5; cursor: default; }

  /* phone: the drawer owns the screen, so the card centres rather than
     trying to point at a name in a 100vw-wide list */
  @media (max-width: 640px) {
    .uc {
      left: 50% !important; top: auto !important; bottom: 12px;
      transform: translateX(-50%);
      width: min(320px, calc(100vw - 24px)) !important;
    }
    .uc-scrim { background: rgba(0, 0, 0, 0.45); }
  }
</style>
