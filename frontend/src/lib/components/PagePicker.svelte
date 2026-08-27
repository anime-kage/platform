<script lang="ts">
  /**
   * The "pagina 3 din 32" line in a pager, turned into a jump-to-page control.
   *
   * Deliberately not styled as a button: in a pager it sits between two real
   * buttons (Anterior / Următor) and a third one there reads as a third
   * navigation step. It stays the quiet mono line it always was, and earns its
   * affordance from a dashed underline and a caret that flips when open — the
   * same cue a text link gets, not the same cue an action gets.
   *
   * Opens upward because a pager is the last thing on the page; a downward
   * panel would land off-screen.
   */
  interface Props {
    page: number;
    pages: number;
    /** Builds the href for a page number, so the picker stays link-based and
     *  every entry is middle-clickable and crawlable. Preferred where paging
     *  lives in the URL. */
    hrefFor?: (page: number) => string;
    /** For pagers whose page is component state rather than a URL param
     *  (the members grid, the lists grid). Entries render as buttons then,
     *  because a link that goes nowhere is a lie to anyone middle-clicking. */
    onselect?: (page: number) => void;
  }
  let { page, pages, hrefFor, onselect }: Props = $props();

  function choose(n: number) {
    open = false;
    onselect?.(n);
  }

  let open = $state(false);
  let root = $state<HTMLElement | null>(null);
  let jump = $state('');

  const list = $derived(Array.from({ length: pages }, (_, i) => i + 1));
  // Past this many, a wall of numbers stops being scannable and the input is
  // the faster route — so the grid scrolls and the field leads.
  const manyPages = $derived(pages > 40);

  function go(e: Event) {
    e.preventDefault();
    const n = Math.min(Math.max(1, Number(jump) || 1), pages);
    open = false;
    jump = '';
    if (hrefFor) window.location.href = hrefFor(n);
    else onselect?.(n);
  }
</script>

<svelte:window
  onpointerdown={(e) => {
    if (open && root && !root.contains(e.target as Node)) open = false;
  }}
  onkeydown={(e) => {
    if (e.key === 'Escape') open = false;
  }}
/>

<div class="pp" bind:this={root}>
  <button
    class="pp-at"
    aria-haspopup="dialog"
    aria-expanded={open}
    aria-label={`Pagina ${page} din ${pages}. Alege altă pagină`}
    onclick={() => (open = !open)}
  >
    pagina <span class="pp-n">{page}</span> din {pages}
    <span class="pp-caret" class:flip={open} aria-hidden="true">▾</span>
  </button>

  {#if open}
    <div class="pp-pop anim-fade" role="dialog" aria-label="Alege pagina">
      {#if manyPages}
        <form class="pp-jump" onsubmit={go}>
          <input
            type="number"
            min="1"
            max={pages}
            placeholder={`1 – ${pages}`}
            bind:value={jump}
            aria-label="Sari la pagina"
          />
          <button type="submit" class="pp-go">Sari</button>
        </form>
      {/if}
      <div class="pp-grid" class:scroll={manyPages}>
        {#each list as p (p)}
          {#if hrefFor}
            <a
              class="pp-p"
              class:on={p === page}
              href={hrefFor(p)}
              aria-current={p === page ? 'page' : undefined}
              onclick={() => (open = false)}
            >
              {p}
            </a>
          {:else}
            <button
              class="pp-p"
              class:on={p === page}
              aria-current={p === page ? 'page' : undefined}
              onclick={() => choose(p)}
            >
              {p}
            </button>
          {/if}
        {/each}
      </div>
    </div>
  {/if}
</div>

<style>
  .pp { position: relative; display: inline-block; }

  /* Reads as the old static line, plus a hint that it opens. */
  .pp-at {
    font-family: var(--font-mono); font-size: var(--fs-caption);
    color: var(--text-muted); background: none; border: 0; cursor: pointer;
    padding: 2px 0; display: inline-flex; align-items: center; gap: 6px;
    border-bottom: 1px dashed var(--border-default);
    transition: color 120ms ease, border-color 120ms ease;
  }
  .pp-at:hover { color: var(--text-primary); border-bottom-color: var(--accent); }
  .pp-at:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 3px; border-radius: 2px; }
  .pp-n { color: var(--text-primary); font-weight: var(--fw-semibold); }
  .pp-caret { font-size: 0.7em; transition: transform 140ms ease; }
  .pp-caret.flip { transform: rotate(180deg); }

  .pp-pop {
    position: absolute; bottom: calc(100% + 10px); left: 50%; transform: translateX(-50%);
    z-index: 40; width: max-content; max-width: min(420px, 90vw);
    background: var(--surface-raised);
    border: 1px solid var(--border-default); border-radius: var(--radius-md);
    box-shadow: 0 18px 40px rgba(0, 0, 0, 0.45);
    padding: 10px;
  }
  /* little pointer down to the trigger */
  .pp-pop::after {
    content: ''; position: absolute; top: 100%; left: 50%; transform: translateX(-50%);
    border: 6px solid transparent; border-top-color: var(--border-default);
  }

  .pp-grid {
    display: grid; gap: 4px;
    grid-template-columns: repeat(auto-fill, minmax(38px, 1fr));
    min-width: 232px;
  }
  .pp-grid.scroll { max-height: 232px; overflow-y: auto; padding-right: 2px; }

  .pp-p {
    display: grid; place-items: center; min-height: 34px;
    appearance: none; cursor: pointer; font: inherit;
    font-family: var(--font-mono); font-size: var(--fs-micro);
    color: var(--text-muted); text-decoration: none;
    background: var(--surface-inset);
    border: 1px solid transparent; border-radius: var(--radius-sm);
    transition: color 120ms ease, background 120ms ease, border-color 120ms ease;
  }
  .pp-p:hover { color: var(--text-primary); background: var(--surface-overlay); border-color: var(--border-default); }
  .pp-p:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 1px; }
  .pp-p.on {
    background: var(--accent); border-color: var(--accent);
    color: var(--on-accent); font-weight: var(--fw-semibold);
  }

  .pp-jump { display: flex; gap: 6px; margin-bottom: 8px; }
  .pp-jump input {
    flex: 1; min-width: 0;
    background: var(--surface-inset); color: var(--text-primary);
    border: 1px solid var(--border-default); border-radius: var(--radius-sm);
    padding: 6px 10px; font-family: var(--font-mono); font-size: var(--fs-micro);
  }
  .pp-jump input:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 1px; }
  .pp-go {
    background: var(--surface-overlay); color: var(--text-primary);
    border: 1px solid var(--border-default); border-radius: var(--radius-sm);
    padding: 6px 12px; font-size: var(--fs-micro); cursor: pointer;
  }
  .pp-go:hover { border-color: var(--accent); color: var(--accent); }

  @media (pointer: coarse) {
    .pp-at { min-height: var(--tap-min); }
    .pp-p { min-height: var(--tap-min); }
  }
</style>
