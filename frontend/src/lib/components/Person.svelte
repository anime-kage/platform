<script lang="ts">
  // Inline name link for the team pages. Meta lines stay mono-faint around
  // it, so the person pops. Plain text on the baseline — no avatar, it broke
  // the line rhythm in compact meta rows.
  let { name = null, self = false }: { name?: string | null; self?: boolean } = $props();
</script>

{#if name}
  <a class="person" href="/user/{encodeURIComponent(name)}">{name}{self ? ' (tu)' : ''}</a>
{:else}
  <span class="p-none">—</span>
{/if}

<style>
  .person {
    font-family: var(--font-body); font-size: var(--fs-caption);
    font-weight: var(--fw-semibold); color: var(--text-primary);
    letter-spacing: 0; text-transform: none; white-space: nowrap;
    text-decoration: none;
  }
  .person:hover { color: var(--accent); }
  .p-none { color: var(--text-muted); }
</style>
