<script lang="ts">
  import '$lib/styles/tokens.css';
  import '$lib/styles/base.css';
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { authStore } from '$lib/stores/auth';
  import { notifications } from '$lib/stores/notifications';
  import { chatOpen } from '$lib/stores/chat';
  import Header from '$lib/components/Header.svelte';
  import Footer from '$lib/components/Footer.svelte';
  import Toast from '$lib/components/Toast.svelte';
  import ChatPanel from '$lib/components/ChatPanel.svelte';

  let { children } = $props();

  // an open chat docks: the page makes room for it instead of being covered —
  // matters most on the team pages (admin/translate/verify/publish), where the
  // right edge is a working column, but nothing should sit under the drawer
  const auth = $derived($authStore);
  const chatDocked = $derived(auth.isAuthenticated && $chatOpen);

  onMount(async () => {
    await authStore.init();
  });

  // The site is invite-only, so a guest gets the landing page and the auth flow
  // and nothing else. Listed as exact paths rather than prefixes: a new public
  // route then has to be added here on purpose instead of inheriting access
  // because it happened to sit under a permitted prefix.
  //
  // Note what this is and is not. It gates the UI; it is not the data boundary.
  // The catalog and community endpoints answer unauthenticated requests (see
  // router.go's optionalAuth routes), so this stops people browsing the site,
  // not a script reading /api/anime directly. Closing that is a backend change.
  const GUEST_PATHS = new Set([
    '/',
    '/login',
    '/register',
    '/parola-uitata',
    '/reseteaza-parola'
  ]);

  $effect(() => {
    // isLoading covers both SSR and the gap before authStore.init() resolves —
    // redirecting during it would bounce a signed-in member on every cold load.
    if (auth.isLoading || auth.isAuthenticated) return;
    if (GUEST_PATHS.has(page.url.pathname)) return;
    goto('/', { replaceState: true });
  });

  // Poll the unread badge while signed in; tear it down on logout so a guest
  // (or the next account) doesn't inherit stale counts.
  $effect(() => {
    if (auth.isAuthenticated) notifications.start();
    else notifications.stop();
  });
</script>

<div class="app-shell" class:chat-docked={chatDocked}>
  <Header />
  <main>
    {@render children()}
  </main>
  <Footer />
  <Toast />
  <ChatPanel />
</div>

<style>
  .app-shell {
    min-height: 100dvh;
    display: flex;
    flex-direction: column;
  }
  main {
    flex: 1;
  }
  /* The chat docks down to 1100px: `main` gives up 340px and the page keeps
     a real column beside it (1026px on a 1366 laptop). That squeeze used to
     break the grids — not because 1026px is too narrow, but because their
     `1fr` tracks had min-width:auto and refused to shrink below their
     content, so they overflowed instead of reflowing. The tracks are
     minmax(0, 1fr) now and the squeeze is harmless. Below 1100px the
     leftover column would be under ~760px, so the panel overlays instead
     (see ChatPanel). The header keeps full width above the drawer. */
  @media (min-width: 1100px) {
    .app-shell.chat-docked main {
      padding-right: var(--chat-w);
    }
  }
</style>
