<script lang="ts">
  // Custom "Marchează" dropdown for the title pages — replaces the native
  // <select> popup. Status is independent of the watchlist/readlist button;
  // clearing the mark removes the list entry entirely (via onclear).
  let {
    value = '',
    options,
    busy = false,
    placeholder = '+ Marchează',
    clearLabel = 'Elimină din listă',
    onselect,
    onclear = null
  }: {
    value?: string;
    options: [string, string][];
    busy?: boolean;
    placeholder?: string;
    clearLabel?: string;
    onselect: (v: string) => void;
    onclear?: (() => void) | null;
  } = $props();

  let open = $state(false);
  let root = $state<HTMLElement | null>(null);
  let btn = $state<HTMLButtonElement | null>(null);
  // The title-page hero clips its overflow (for the blurred backdrop), so an
  // absolutely-positioned menu gets cut off — only the first row shows. Render
  // the menu fixed, anchored to the button rect, so it escapes any clipper.
  let pos = $state({ top: 0, left: 0, width: 0 });

  const label = $derived(options.find(([s]) => s === value)?.[1] ?? null);

  function place() {
    if (!btn) return;
    const r = btn.getBoundingClientRect();
    pos = { top: r.bottom + 6, left: r.left, width: r.width };
  }

  function toggle() {
    open = !open;
    if (open) place();
  }

  function pick(v: string) {
    open = false;
    if (v !== value) onselect(v);
  }
</script>

<svelte:window
  onpointerdown={(e) => {
    if (open && root && !root.contains(e.target as Node) && !(e.target as HTMLElement)?.closest?.('.sp-menu'))
      open = false;
  }}
  onkeydown={(e) => {
    if (e.key === 'Escape') open = false;
  }}
  onscroll={() => open && place()}
  onresize={() => open && place()}
/>

<div class="sp" bind:this={root}>
  <button
    bind:this={btn}
    class="sp-btn"
    class:on={!!label}
    disabled={busy}
    aria-haspopup="listbox"
    aria-expanded={open}
    onclick={toggle}
  >
    {label ? `✓ ${label}` : placeholder}
    <span class="caret" class:flip={open} aria-hidden="true">▾</span>
  </button>

  {#if open}
    <div
      class="menu sp-menu anim-fade"
      role="listbox"
      aria-label="Marchează"
      style={`top:${pos.top}px; left:${pos.left}px; min-width:${Math.max(pos.width, 200)}px;`}
    >
      {#each options as [s, l] (s)}
        <button class="opt" class:sel={s === value} role="option" aria-selected={s === value} onclick={() => pick(s)}>
          <span class="tick" aria-hidden="true">{s === value ? '✓' : ''}</span>{l}
        </button>
      {/each}
      {#if value && onclear}
        <div class="sep" role="separator"></div>
        <button
          class="opt clear"
          onclick={() => {
            open = false;
            onclear?.();
          }}
        >
          <span class="tick" aria-hidden="true"></span>{clearLabel}
        </button>
      {/if}
    </div>
  {/if}
</div>

<style>
  .sp { position: relative; display: inline-flex; }

  /* mirrors the title pages' .btn.ghost */
  .sp-btn {
    display: inline-flex; align-items: center; gap: 8px;
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 13px 24px; border-radius: var(--radius-md); cursor: pointer; white-space: nowrap;
    border: 1px solid var(--border-default); background: transparent; color: var(--text-primary);
  }
  .sp-btn:hover { background: var(--surface-raised); }
  .sp-btn.on {
    color: var(--accent);
    border-color: color-mix(in srgb, var(--accent) 55%, transparent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }
  .sp-btn:disabled { opacity: 0.6; cursor: wait; }

  .caret { font-size: 0.625rem; color: var(--text-faint); transition: transform var(--motion-fast) var(--ease); }
  .sp-btn.on .caret { color: var(--accent); }
  .caret.flip { transform: rotate(180deg); }

  .menu {
    position: fixed; z-index: 200;
    min-width: 200px; padding: 6px;
    background: var(--surface-raised);
    border: 1px solid var(--border-default); border-radius: var(--radius-md);
    box-shadow: 0 14px 38px rgba(0, 0, 0, 0.4);
  }
  .opt {
    display: flex; align-items: center; gap: 8px; width: 100%;
    padding: 9px 12px; border: none; border-radius: var(--radius-sm);
    background: none; cursor: pointer; text-align: left;
    font-size: var(--fs-small); font-weight: var(--fw-medium); color: var(--text-primary);
  }
  .opt:hover { background: var(--surface-overlay); }
  .opt.sel { color: var(--accent); font-weight: var(--fw-semibold); }
  .tick { width: 14px; flex: 0 0 auto; font-size: 0.75rem; color: var(--accent); }
  .sep { height: 1px; margin: 6px 4px; background: var(--border-subtle); }
  .opt.clear { color: var(--text-muted); }
  .opt.clear:hover { color: var(--danger); }
</style>
