<script lang="ts">
  /**
   * "||spoiler||" insert button, sat beside the emoji picker in every composer
   * (comments, reviews, chat, news editor). One component so the affordance is
   * identical everywhere — the syntax is only learnable if it is always in the
   * same place with the same label.
   */
  let {
    value = $bindable(''),
    input = null,
    compact = false
  }: {
    value?: string;
    /** the field being edited, so the selection can be wrapped */
    input?: HTMLTextAreaElement | HTMLInputElement | null;
    compact?: boolean;
  } = $props();

  function apply() {
    const el = input;
    const start = el?.selectionStart ?? value.length;
    const end = el?.selectionEnd ?? start;
    const picked = value.slice(start, end) || 'spoiler';
    value = value.slice(0, start) + '||' + picked + '||' + value.slice(end);
    queueMicrotask(() => {
      el?.focus();
      el?.setSelectionRange(start + 2, start + 2 + picked.length);
    });
  }
</script>

<button type="button" class="sp" class:compact title="Ascunde ca spoiler — ||text||" onclick={apply}>
  {compact ? '||' : '||spoiler||'}
</button>

<style>
  .sp {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    padding: 4px 8px; cursor: pointer; border-radius: var(--radius-sm);
    background: none; border: 1px solid var(--border-default); color: var(--text-muted);
    white-space: nowrap; line-height: 1;
  }
  .sp:hover { color: var(--text-primary); border-color: var(--border-strong); }
  .sp.compact { padding: 4px 7px; }
</style>
