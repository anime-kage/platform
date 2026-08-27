<script lang="ts">
  import { fade } from 'svelte/transition';

  // Mascot speech bubble. Fixed to one side of the viewport and hidden on
  // small screens, where it would sit on top of the content it comments on.
  let {
    show = false,
    type = 'tip',
    message = '',
    side = 'right'
  }: {
    show?: boolean;
    type?: 'tip' | 'success' | 'error';
    message?: string;
    side?: 'right' | 'left';
  } = $props();
</script>

{#if show && message}
  <div class="punpun {type} {side}" transition:fade={{ duration: 200 }} aria-live="polite">
    <div class="bubble">{message}</div>
    <img src="/punpun.png" alt="" width="120" height="123" />
  </div>
{/if}

<style>
  .punpun {
    position: fixed; top: 50%; transform: translateY(-50%);
    display: flex; flex-direction: column; z-index: 60;
    pointer-events: none;
  }
  .punpun.right { right: 30px; align-items: flex-end; }
  .punpun.left { left: 30px; align-items: flex-start; }

  .punpun img {
    max-width: 120px; height: auto; margin-top: 10px;
    filter: drop-shadow(2px 2px 4px rgba(0, 0, 0, 0.5));
  }
  .punpun.success img { filter: drop-shadow(2px 2px 4px rgba(0, 0, 0, 0.5)) hue-rotate(90deg); }
  .punpun.error img { filter: drop-shadow(2px 2px 4px rgba(0, 0, 0, 0.5)) hue-rotate(-30deg); }

  .bubble {
    position: relative; max-width: 220px; padding: 14px 18px;
    background: var(--surface-raised); color: var(--text-primary);
    border: 1px solid var(--border-default); border-radius: 12px;
    font-size: var(--fs-small); line-height: 1.5;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
  }
  .punpun.right .bubble { margin-right: 20px; }
  .punpun.left .bubble { margin-left: 20px; }

  /* speech tail pointing down toward Punpun */
  .bubble::after {
    content: ''; position: absolute; bottom: 20px; width: 0; height: 0;
    border-top: 10px solid transparent; border-bottom: 10px solid transparent;
  }
  .punpun.right .bubble::after { right: -10px; border-left: 10px solid var(--surface-raised); }
  .punpun.left .bubble::after { left: -10px; border-right: 10px solid var(--surface-raised); }

  .punpun.tip .bubble { border-color: color-mix(in srgb, var(--accent) 60%, transparent); }
  .punpun.success .bubble {
    border-color: color-mix(in srgb, var(--success) 60%, transparent);
    background: color-mix(in srgb, var(--success) 12%, var(--surface-raised));
  }
  .punpun.error .bubble {
    border-color: color-mix(in srgb, var(--danger) 60%, transparent);
    background: color-mix(in srgb, var(--danger) 12%, var(--surface-raised));
  }
  .punpun.right.success .bubble::after { border-left-color: color-mix(in srgb, var(--success) 12%, var(--surface-raised)); }
  .punpun.left.success .bubble::after { border-right-color: color-mix(in srgb, var(--success) 12%, var(--surface-raised)); }
  .punpun.right.error .bubble::after { border-left-color: color-mix(in srgb, var(--danger) 12%, var(--surface-raised)); }
  .punpun.left.error .bubble::after { border-right-color: color-mix(in srgb, var(--danger) 12%, var(--surface-raised)); }

  @media (max-width: 900px) {
    .punpun { display: none; }
  }
</style>
