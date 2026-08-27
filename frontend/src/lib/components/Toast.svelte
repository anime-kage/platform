<script lang="ts">
  import { toast as toastStore, type Toast } from '$lib/stores/toast';
  import { fly } from 'svelte/transition';

  // Notifications rendered as subtitle cues.
  //
  // This is a fansub platform: the timed, outlined, box-less subtitle line is the
  // one object the whole product is built around. So a confirmation is not a card
  // in a corner — it is a cue at the bottom of the frame, in the same treatment
  // the editor preview already gives real cues (.preview::cue in ReleaseEditor).
  //
  // Two things fall out of this for free:
  //  - the layered outline exists precisely so text stays readable over content
  //    nobody controls, which is the exact problem a toast has;
  //  - white-on-outline needs no theme variant, so light mode is not a special
  //    case the way a filled panel would have been.
  //
  // The mono timing tag above the line does the work the panel used to do: it
  // names the state, and it counts the cue out the way a subtitle has a duration.

  let toasts = $state<Toast[]>([]);
  $effect(() => toastStore.subscribe((value) => (toasts = value)));

  // Per-toast dismissal deadline. The store does not record when a toast was
  // created, so the first render of each id is what we time from.
  const deadlines = new Map<string, number>();
  let now = $state(Date.now());

  $effect(() => {
    for (const t of toasts) {
      if (!deadlines.has(t.id)) deadlines.set(t.id, Date.now() + (t.duration ?? 0));
    }
    for (const id of [...deadlines.keys()]) {
      if (!toasts.some((t) => t.id === id)) deadlines.delete(id);
    }
  });

  // Only tick while something is on screen — an interval left running for an
  // empty container is pure wakeup cost.
  $effect(() => {
    if (toasts.length === 0) return;
    const h = setInterval(() => (now = Date.now()), 250);
    return () => clearInterval(h);
  });

  function countdown(t: Toast): string {
    const end = deadlines.get(t.id);
    if (!end || !t.duration) return '';
    const secs = Math.max(0, Math.ceil((end - now) / 1000));
    return `00:${String(secs).padStart(2, '0')}`;
  }

  const still =
    typeof window !== 'undefined' &&
    window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
</script>

<div class="cues" aria-live="polite" aria-relevant="additions">
  {#each toasts as t (t.id)}
    <div
      class="cue c-{t.type}"
      role={t.type === 'error' ? 'alert' : 'status'}
      in:fly={{ x: still ? 0 : 14, duration: still ? 0 : 160 }}
      out:fly={{ x: still ? 0 : 10, duration: still ? 0 : 120 }}
    >
      <div class="meta">
        {#if countdown(t)}<span class="clock">{countdown(t)}</span>{/if}
        <button class="x" onclick={() => toastStore.remove(t.id)} aria-label="Închide">
          <svg viewBox="0 0 12 12" aria-hidden="true">
            <path d="M3 3l6 6M9 3l-6 6" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
          </svg>
        </button>
      </div>
      <p class="line">{t.message}</p>
    </div>
  {/each}
</div>

<style>
  .cues {
    position: fixed;
    /* clears the fixed header rather than sitting under it */
    top: calc(var(--header-h) + var(--space-3));
    /* …and steps left of whatever the chat is occupying on that edge: the
       launcher button when closed, the full drawer when open. ChatPanel
       publishes --chat-rail; 0px is the fallback for pages without a chat. */
    right: calc(var(--space-5) + var(--chat-rail, 0px));
    transition: right var(--motion-base) var(--ease);
    z-index: var(--z-toast);
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: var(--space-3);
    pointer-events: none;
  }

  .cue {
    pointer-events: auto;
    /* Narrower than the bottom-band version was: a corner cue is read in a
       glance, and a long line pulled towards the middle of the page would stop
       reading as a corner at all. */
    max-width: min(26rem, calc(100vw - 2 * var(--space-5) - var(--chat-rail, 0px)));
    text-align: right;
    --tone: var(--accent);
    /* One shadow definition, inherited by every layer below. Heavier than the
       player's cue style on purpose: that one sits over video that is usually
       dark, this one can land on anything including a white light-mode page. */
    --outline:
      0 0 2px rgba(0, 0, 0, 0.95),
      0 0 4px rgba(0, 0, 0, 0.9),
      0 1px 3px rgba(0, 0, 0, 0.9),
      0 2px 8px rgba(0, 0, 0, 0.7);
  }

  .c-success { --tone: #6cd07d; }
  .c-error   { --tone: #ff7a7e; }
  .c-warning { --tone: #ffc65c; }
  .c-info    { --tone: var(--cyan-bright); }

  /* The state colours above are lifted from the token palette rather than used
     raw: --success/--danger/--warning are tuned for fills and borders on a dark
     surface, and at text size they read muddy. These are the same hues,
     brightened enough to carry a whole line of text.

     Colour is now the only *visual* carrier of the state, since the word is
     gone. Non-visually it still travels: an error cue is role="alert" and every
     other kind is role="status", so a screen reader announces the urgent ones
     differently regardless of the palette. */

  .meta {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.5rem;
    margin-bottom: 0.3rem;
    font-family: var(--font-mono);
    font-size: var(--fs-micro);
    font-weight: var(--fw-semibold);
    letter-spacing: 0.16em;
    text-transform: uppercase;
    text-shadow: var(--outline);
  }

  .clock {
    color: rgba(255, 255, 255, 0.72);
    font-variant-numeric: tabular-nums;
  }

  .x {
    display: grid;
    place-items: center;
    width: 16px;
    height: 16px;
    margin-left: 0.15rem;
    padding: 0;
    background: none;
    border: none;
    border-radius: var(--radius-sm);
    color: rgba(255, 255, 255, 0.55);
    cursor: pointer;
    transition: color var(--motion-fast) var(--ease);
  }
  .x:hover { color: #fff; }
  .x:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 2px; }
  .x svg { width: 10px; height: 10px; fill: none; }

  /* the cue itself */
  .line {
    margin: 0;
    font-family: var(--font-body);
    /* A step down from full cue size. At the bottom of the frame the line was
       standing in for a real subtitle; in the corner that scale would shout. */
    font-size: clamp(1rem, 0.96rem + 0.28vw, 1.1875rem);
    font-weight: var(--fw-semibold);
    line-height: 1.4;
    /* The line itself is the signal now — it used to be white under a mono
       "REUȘIT" / "EROARE" label, which said in a word what the colour already
       says at a glance. The heavy outline below is what lets a coloured line
       stay legible over arbitrary page content. */
    color: var(--tone);
    text-shadow: var(--outline);
    text-wrap: balance;
  }

  @media (max-width: 640px) {
    .cues {
      top: calc(var(--header-h) + var(--space-2));
      left: var(--space-4);
      /* No rail here on purpose. On a phone the open chat is a full-bleed sheet,
         so there is no "beside it" to move to — stepping aside would push the
         cue off screen. It overlays instead, which is the only option left. */
      right: var(--space-4);
      gap: var(--space-2);
    }
    /* On a phone the corner is most of the width anyway — let the line use it
       rather than wrapping early against an arbitrary cap. */
    .cue { max-width: 100%; }
  }
</style>
