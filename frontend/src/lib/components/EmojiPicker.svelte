<script lang="ts">
  import { EMOJI_CATEGORIES, type EmojiItem } from '$lib/data/emoji';

  interface PickItem extends EmojiItem {
    /* what gets inserted on pick — emote codes insert the code, not the glyph */
    insert?: string;
  }

  let {
    onPick,
    emotes = undefined,
    customEmotes = undefined
  }: {
    onPick: (text: string) => void;
    /** built-in shortcuts: code → a single emoji character */
    emotes?: Record<string, string>;
    /** uploaded image emotes: code → /uploads/ path. Their own section, because
        they are the platform's actual emotes and the emoji ones are shortcuts. */
    customEmotes?: Record<string, string>;
  } = $props();

  let open = $state(false);
  let query = $state('');
  let btn = $state<HTMLElement | null>(null);
  let pop = $state<HTMLElement | null>(null);
  let searchEl = $state<HTMLInputElement | null>(null);
  let pos = $state({ top: 0, left: 0 });

  const POP_W = 312;
  const POP_H = 324;

  /* An uploaded emote's "glyph" is an image path, not a character. Without this
     the grid printed the URL as text. `loading="lazy"` on the <img> matters:
     with 100–200 emotes only the ones scrolled into view are ever fetched. */
  const isImageEmote = (v: string) => v.startsWith('/uploads/') || v.startsWith('http');

  const allCategories = $derived.by(() => {
    const cats: { label: string; items: PickItem[] }[] = [];
    if (customEmotes && Object.keys(customEmotes).length) {
      cats.push({
        label: 'Emote Anime·Kage',
        items: Object.entries(customEmotes).map(([code, url]) => ({
          ch: url,
          name: code,
          kw: `emote ${code.toLowerCase()}`,
          insert: code
        }))
      });
    }
    if (emotes) {
      cats.push({
        label: 'Emoji rapide',
        items: Object.entries(emotes).map(([code, glyph]) => ({
          ch: glyph,
          name: code,
          kw: `emote ${code.toLowerCase()}`,
          insert: code
        }))
      });
    }
    cats.push(...EMOJI_CATEGORIES);
    return cats;
  });

  const filtered = $derived.by(() => {
    const q = query.trim().toLowerCase();
    if (!q) return allCategories;
    return allCategories
      .map((c) => ({
        label: c.label,
        items: c.items.filter(
          (it) => it.name.toLowerCase().includes(q) || it.kw.includes(q) || it.ch === q
        )
      }))
      .filter((c) => c.items.length > 0);
  });

  /* the popover renders in a body portal with position: fixed, so no ancestor
     overflow/transform can clip or reposition it */
  function portal(node: HTMLElement) {
    document.body.appendChild(node);
    return { destroy: () => node.remove() };
  }

  function place() {
    if (!btn) return;
    const r = btn.getBoundingClientRect();
    // Clamp against the space actually available: on a phone the popover is
    // capped by CSS to the viewport, so positioning it by its preferred size
    // would push it off-screen.
    const w = Math.min(POP_W, window.innerWidth - 16);
    const h = Math.min(POP_H, window.innerHeight - 16);
    const left = Math.max(8, Math.min(r.left, window.innerWidth - w - 8));
    let top = r.top - h - 8;
    if (top < 8) top = Math.max(8, Math.min(r.bottom + 8, window.innerHeight - h - 8));
    pos = { top, left };
  }

  function toggle() {
    if (!open) {
      place();
      query = '';
    }
    open = !open;
  }

  function onWindowPointerDown(e: PointerEvent) {
    if (!open) return;
    const t = e.target as Node;
    if (btn?.contains(t) || pop?.contains(t)) return;
    open = false;
  }

  function onWindowKeydown(e: KeyboardEvent) {
    if (open && e.key === 'Escape') open = false;
  }

  $effect(() => {
    if (!open) return;
    // preventScroll matters: the popover is a position:fixed body portal, and a
    // bare focus() makes the browser scroll the PAGE trying to bring it into
    // view -- on a phone that jumped to the top of the document every time the
    // emote button was tapped.
    //
    // And on a phone the search box is not focused at all: it would raise the
    // on-screen keyboard over the emote grid the user is trying to tap. Typing
    // to filter is a desktop affordance; tapping is the mobile one.
    if (window.matchMedia('(max-width: 640px)').matches) return;
    searchEl?.focus({ preventScroll: true });
  });
</script>

<svelte:window
  onpointerdowncapture={onWindowPointerDown}
  onkeydown={onWindowKeydown}
  onresize={() => open && place()}
  onscrollcapture={(e) => {
    if (open && !(pop && e.target instanceof Node && pop.contains(e.target))) place();
  }}
/>

<button
  type="button"
  class="emoji-toggle"
  class:on={open}
  bind:this={btn}
  onclick={toggle}
  aria-label="Adaugă emoji"
  title="Emoji"
>☺</button>

{#if open}
  <div
    class="emoji-pop"
    style={`top:${pos.top}px;left:${pos.left}px;width:${POP_W}px;height:${POP_H}px`}
    role="dialog"
    aria-label="Alege un emoji"
    use:portal
    bind:this={pop}
  >
    <div class="search-row">
      <span class="search-ico">⌕</span>
      <input
        bind:this={searchEl}
        bind:value={query}
        placeholder="Caută emoji..."
        aria-label="Caută emoji"
      />
      {#if query}
        <button type="button" class="search-x" onclick={() => (query = '')} title="Șterge">×</button>
      {/if}
    </div>
    <div class="scroll">
      {#each filtered as cat (cat.label)}
        <p class="cat-head">{cat.label}</p>
        <div class="grid">
          {#each cat.items as it (it.insert ?? it.ch)}
            <button
              type="button"
              class="emoji"
              title={it.name}
              onclick={() => onPick(it.insert ?? it.ch)}
            >{#if isImageEmote(it.ch)}<img
                  class="emote-img"
                  src={it.ch}
                  alt={it.name}
                  loading="lazy"
                />{:else}{it.ch}{/if}</button>
          {/each}
        </div>
      {:else}
        <p class="empty">Niciun rezultat pentru „{query}”</p>
      {/each}
    </div>
  </div>
{/if}

<style>
  .emoji-toggle {
    background: none;
    border: 1px solid var(--border-default);
    border-radius: 6px;
    width: 30px;
    height: 30px;
    display: grid;
    place-items: center;
    font-size: 1rem;
    line-height: 1;
    color: var(--text-faint);
    cursor: pointer;
    transition: color 0.15s, border-color 0.15s;
  }

  .emoji-toggle:hover,
  .emoji-toggle.on {
    color: var(--accent);
    border-color: color-mix(in srgb, var(--accent) 55%, transparent);
  }

  .emoji-pop {
    position: fixed;
    /* Above the chat panel, which is z-index 210 on phones. At 200 the popover
       opened correctly and rendered *behind* the panel, so on mobile the emote
       button looked dead. */
    z-index: 260;
    /* Never wider or taller than the screen it has to fit on. The inline style
       sets the preferred size; these caps win on a narrow phone. */
    max-width: calc(100vw - 16px);
    max-height: calc(100dvh - 16px);
    display: flex;
    flex-direction: column;
    background: var(--surface-overlay);
    border: 1px solid var(--border-default);
    border-radius: 12px;
    box-shadow: 0 12px 32px rgba(0, 0, 0, 0.35);
    overflow: hidden;
  }

  .search-row {
    flex: 0 0 auto;
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 8px 10px;
    border-bottom: 1px solid var(--border-subtle);
  }

  .search-ico {
    color: var(--text-faint);
    font-size: 1rem;
    line-height: 1;
  }

  .search-row input {
    flex: 1;
    min-width: 0;
    background: none;
    border: none;
    outline: none;
    color: var(--text-primary);
    font-size: var(--fs-small, 0.875rem);
  }

  .search-row input::placeholder {
    color: var(--text-faint);
  }

  .search-x {
    background: none;
    border: none;
    color: var(--text-muted);
    font-size: 1rem;
    line-height: 1;
    cursor: pointer;
    padding: 0 2px;
  }

  .search-x:hover {
    color: var(--text-primary);
  }

  .scroll {
    flex: 1;
    overflow-y: auto;
    padding: 0 8px 8px;
    scrollbar-width: thin;
  }

  .cat-head {
    position: sticky;
    top: 0;
    z-index: 1;
    margin: 0 -8px;
    padding: 7px 12px 5px;
    background: var(--surface-overlay);
    font-family: var(--font-mono);
    font-size: var(--fs-micro);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--text-muted);
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(8, minmax(0, 1fr));
    gap: 2px;
  }

  /* Same fixed height as chat, so the picker previews the real thing. */
  .emote-img {
    height: 24px; width: auto; max-width: 100%;
    object-fit: contain; display: block; margin: 0 auto;
  }

  .emoji {
    background: none;
    border: none;
    border-radius: 6px;
    aspect-ratio: 1;
    display: grid;
    place-items: center;
    font-size: 1.125rem;
    line-height: 1;
    cursor: pointer;
    transition: background 0.12s, transform 0.12s;
  }

  .emoji:hover {
    background: var(--surface-raised);
    transform: scale(1.15);
  }

  .empty {
    padding: 18px 8px;
    text-align: center;
    color: var(--text-muted);
    font-size: var(--fs-small, 0.875rem);
  }
</style>
