<script lang="ts">
  import api, { type GiphyGif } from '$lib/api';

  /**
   * GIF picker. Calls our own proxy, never Giphy directly — the API key stays
   * server-side and one shared cache keeps the free tier's 100 calls/hour from
   * being spent by a handful of people typing.
   *
   * `onPick` receives the GIF's URL; every composer inserts it as a bare link,
   * which the renderers turn back into the image.
   */
  let {
    onPick,
    compact = false
  }: { onPick: (url: string) => void; compact?: boolean } = $props();

  let open = $state(false);
  let q = $state('');
  let gifs = $state<GiphyGif[]>([]);
  let loading = $state(false);
  let note = $state('');
  /** Set when the endpoint says 503 — no key configured, so hide the button. */
  let unavailable = $state(false);

  let timer: ReturnType<typeof setTimeout> | undefined;

  async function load(query: string) {
    loading = true;
    note = '';
    try {
      gifs = (await api.searchGifs(query)).data;
      if (gifs.length === 0) note = 'Niciun GIF pentru căutarea asta.';
    } catch (e) {
      const err = e as { statusCode?: number; error?: string };
      if (err.statusCode === 503) {
        unavailable = true;
        open = false;
      } else {
        gifs = [];
        note = err.error ?? 'Căutarea a eșuat.';
      }
    } finally {
      loading = false;
    }
  }

  function toggle() {
    open = !open;
    if (open && gifs.length === 0) load('');
  }

  /* Debounced: one call per pause, not one per keystroke. With a 100/hour
     upstream budget the difference between 1 and 6 calls for "naruto" is the
     whole feature's viability. */
  function onInput() {
    clearTimeout(timer);
    timer = setTimeout(() => load(q), 400);
  }

  function choose(g: GiphyGif) {
    onPick(g.url);
    open = false;
    q = '';
  }
</script>

{#if !unavailable}
  <div class="wrap">
    <button type="button" class="btn" class:compact onclick={toggle} title="Adaugă un GIF">
      GIF
    </button>

    {#if open}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div class="pop" class:right={compact}>
        <input
          class="q"
          bind:value={q}
          oninput={onInput}
          placeholder="Caută un GIF…"
          aria-label="Caută un GIF"
        />
        <div class="grid">
          {#if loading}
            <p class="note">Se caută…</p>
          {:else if note}
            <p class="note">{note}</p>
          {:else}
            {#each gifs as g (g.id)}
              <button type="button" class="cell" onclick={() => choose(g)} title={g.title || 'GIF'}>
                <img src={g.preview} alt={g.title || 'GIF'} loading="lazy" referrerpolicy="no-referrer" />
              </button>
            {/each}
          {/if}
        </div>
        <p class="credit">Powered by GIPHY</p>
      </div>
    {/if}
  </div>
{/if}

<style>
  .wrap { position: relative; display: inline-flex; }
  .btn {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.06em;
    padding: 4px 8px; cursor: pointer; border-radius: var(--radius-sm);
    background: none; border: 1px solid var(--border-default); color: var(--text-muted);
    line-height: 1;
  }
  .btn:hover { color: var(--text-primary); border-color: var(--border-strong); }
  .btn.compact { padding: 4px 7px; }

  .pop {
    position: absolute; bottom: calc(100% + 6px); left: 0; z-index: var(--z-dropdown, 50);
    width: min(320px, 80vw);
    padding: 8px;
    border: 1px solid var(--border-default); border-radius: var(--radius-md);
    background: var(--surface-raised); box-shadow: var(--shadow-lg, 0 12px 32px rgba(0,0,0,.45));
  }
  /* In the chat the picker sits against the right edge of the viewport, so a
     panel growing rightwards runs off the page. Anchor its right edge to the
     button instead and let it open leftwards, into the space that exists. */
  .pop.right { left: auto; right: 0; width: min(300px, calc(100vw - 2rem)); }

  .q {
    width: 100%; padding: 8px 10px; margin-bottom: 8px;
    font: inherit; font-size: var(--fs-small);
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-sm); color: var(--text-primary); outline: none;
  }
  .q:focus { border-color: var(--accent); }

  .grid {
    display: grid; grid-template-columns: repeat(3, 1fr); gap: 5px;
    max-height: 260px; overflow-y: auto;
  }
  .cell {
    padding: 0; border: 1px solid transparent; border-radius: var(--radius-sm);
    background: var(--surface-overlay); cursor: pointer; overflow: hidden; aspect-ratio: 1;
  }
  .cell:hover { border-color: var(--accent); }
  .cell img { width: 100%; height: 100%; object-fit: cover; display: block; }

  .note { grid-column: 1 / -1; padding: 18px 6px; text-align: center; font-size: var(--fs-caption); color: var(--text-muted); }
  /* Giphy's terms require visible attribution wherever their content appears. */
  .credit {
    margin-top: 6px; text-align: right;
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-faint);
    letter-spacing: 0.08em;
  }
</style>
