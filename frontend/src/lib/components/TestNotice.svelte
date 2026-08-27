<script lang="ts">
  import { fade, scale } from 'svelte/transition';

  // "The site is still in testing" — shown once, centred, on the first home page
  // a member lands on after signing in.
  //
  // It exists to answer one question before it gets asked: where is the content.
  // The catalog is populated but almost nothing is published yet, and a member
  // who joined expecting episodes has no way to tell "empty" from "broken".
  //
  // Once per browser, not once per visit: a modal that reappears on every
  // navigation stops being information and becomes an obstacle people learn to
  // click through without reading.
  //
  // `scope` gives each surface its own "seen" flag, so the landing page and the
  // home page each get one showing. That is deliberate rather than sloppy: the
  // two audiences barely overlap in time. Someone reads the landing copy before
  // they have an account, then signs up, and by the time they are inside — often
  // days later, after waiting for an invite — they are looking for episodes and
  // have forgotten. One shared flag would silently skip the showing that
  // actually prevents the question.
  let { scope = 'home' }: { scope?: 'home' | 'landing' } = $props();
  const SEEN_KEY = `ak-test-notice-seen:${scope}`;
  const DWELL = 9000; // long enough to read three sentences without rushing

  let open = $state(false);
  let closeBtn = $state<HTMLButtonElement | null>(null);
  let timer: ReturnType<typeof setTimeout>;

  $effect(() => {
    let seen = false;
    try {
      seen = localStorage.getItem(SEEN_KEY) === '1';
    } catch {
      // private mode: show it, better than suppressing it on a storage error
    }
    if (seen) return;
    open = true;
    // Mark it seen on *show*, not on dismiss — otherwise navigating away
    // without closing it means it comes back on the next page.
    try {
      localStorage.setItem(SEEN_KEY, '1');
    } catch {
      /* private mode */
    }
    closeBtn?.focus();
    timer = setTimeout(() => (open = false), DWELL);
    return () => clearTimeout(timer);
  });

  function dismiss() {
    clearTimeout(timer);
    open = false;
  }
</script>

<svelte:window onkeydown={(e) => open && e.key === 'Escape' && dismiss()} />

{#if open}
  <!-- The scrim is a button so a click anywhere outside dismisses; it is
       aria-hidden because the dialog below already offers a labelled close. -->
  <button class="scrim" transition:fade={{ duration: 180 }} onclick={dismiss} aria-hidden="true" tabindex="-1"
  ></button>

  <div
    class="wrap"
    role="dialog"
    aria-modal="true"
    aria-labelledby="test-notice-title"
    transition:scale={{ duration: 200, start: 0.96 }}
  >
    <div class="card">
      <h2 id="test-notice-title">Site-ul este în test.</h2>
      <p class="body">
        Încă nu există <strong>anime-uri sau manga publicate</strong>. Momentan îți poți
        face cont și explora; conținutul apare pe măsură ce echipa îl termină.
      </p>
      <button class="ok" bind:this={closeBtn} onclick={dismiss}>Am înțeles</button>
      <span class="timer" style="animation-duration:{DWELL}ms"></span>
    </div>
  </div>
{/if}

<style>
  .scrim {
    position: fixed;
    inset: 0;
    z-index: calc(var(--z-overlay) + 10);
    border: none;
    padding: 0;
    background: rgba(0, 0, 0, 0.62);
    -webkit-backdrop-filter: blur(3px);
    backdrop-filter: blur(3px);
    cursor: default;
  }

  .wrap {
    position: fixed;
    inset: 0;
    z-index: calc(var(--z-overlay) + 11);
    display: grid;
    place-items: center;
    padding: var(--space-4);
    pointer-events: none;
  }

  .card {
    position: relative;
    overflow: hidden;
    pointer-events: auto;
    width: min(30rem, 100%);
    padding: var(--space-5) var(--space-5) var(--space-4);
    text-align: center;
    background: var(--surface-raised);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-3);
  }

  h2 {
    margin: 0 0 var(--space-3);
    font-family: var(--font-display);
    font-size: var(--fs-h2);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-snug);
    color: var(--text-primary);
  }

  .body {
    margin: 0 0 var(--space-3);
    font-size: var(--fs-caption);
    line-height: var(--lh-normal);
    color: var(--text-muted);
  }
  .body strong { color: var(--text-primary); font-weight: var(--fw-semibold); }

  .ok {
    margin-top: var(--space-1);
    padding: 9px 20px;
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--accent);
    color: var(--on-accent);
    font-size: var(--fs-small);
    font-weight: var(--fw-semibold);
    cursor: pointer;
  }
  .ok:hover { background: var(--accent-hover); }
  .ok:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 2px; }

  /* the dwell timer, so the auto-close is visible rather than sudden */
  .timer {
    position: absolute;
    left: 0;
    bottom: 0;
    width: 100%;
    height: 2px;
    background: var(--warning);
    transform-origin: left center;
    animation: drain linear forwards;
  }
  @keyframes drain {
    from { transform: scaleX(1); }
    to { transform: scaleX(0); }
  }

  @media (prefers-reduced-motion: reduce) {
    .timer { animation: none; opacity: 0.4; }
  }
</style>
