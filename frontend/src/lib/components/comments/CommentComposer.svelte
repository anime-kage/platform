<script lang="ts">
  import GifPicker from '$lib/components/GifPicker.svelte';
  import api from '$lib/api';
  import { nameHue } from '$lib/avatar';
  import EmojiPicker from '$lib/components/EmojiPicker.svelte';
  import SpoilerButton from '$lib/components/SpoilerButton.svelte';

  interface Props {
    user: { username: string; avatarUrl?: string | null };
    /** bound textarea content */
    value: string;
    placeholder?: string;
    rows?: number;
    /** reply variant: smaller avatar, tighter chrome */
    small?: boolean;
    /** embedded (compact) section: smaller avatar only */
    compact?: boolean;
    posting?: boolean;
    submitLabel?: string;
    postingLabel?: string;
    onSubmit: () => void;
    /** renders an "Anulează" button when provided */
    onCancel?: () => void;
    /** Ctrl/Cmd+Enter submits (main composer behavior) */
    submitOnCtrlEnter?: boolean;
  }

  let {
    user,
    value = $bindable(),
    placeholder = 'Scrie un comentariu...',
    rows = 3,
    small = false,
    compact = false,
    posting = false,
    submitLabel = 'Trimite',
    postingLabel = 'Se trimite...',
    onSubmit,
    onCancel,
    submitOnCtrlEnter = false
  }: Props = $props();

  let inputEl = $state<HTMLTextAreaElement | null>(null);


  function onKeydown(e: KeyboardEvent) {
    if (submitOnCtrlEnter && e.key === 'Enter' && (e.ctrlKey || e.metaKey)) onSubmit();
  }
</script>

<div class="composer" class:small>
  <div class="avatar" class:avatar-sm={small || compact} class:monogram={!user.avatarUrl} style="--mg-hue: {nameHue(user.username)}">
    {#if user.avatarUrl}
      <img src={api.resolveUrl(user.avatarUrl)} alt={user.username} />
    {:else}
      <span class="avatar-text">{user.username.charAt(0).toUpperCase()}</span>
    {/if}
  </div>
  <div class="form-body">
    <textarea
      bind:this={inputEl}
      bind:value
      onkeydown={onKeydown}
      class="comment-input"
      {placeholder}
      {rows}
      maxlength="2000"
    ></textarea>
    <div class="form-footer">
      <div class="foot-left">
        <EmojiPicker onPick={(e) => (value += e)} />
        <SpoilerButton bind:value input={inputEl} />
        <GifPicker onPick={(url) => (value = value ? `${value} ${url}` : url)} />
        <span class="char-count">{value.length}/2000</span>
      </div>
      <div class="actions">
        {#if onCancel}
          <button onclick={onCancel} class="btn-text">Anulează</button>
        {/if}
        <button
          onclick={onSubmit}
          class="btn-submit"
          class:btn-sm={small}
          disabled={posting || !value.trim()}
        >
          {posting ? postingLabel : submitLabel}
        </button>
      </div>
    </div>
  </div>
</div>

<style>
  .composer {
    display: flex;
    gap: 12px;
    margin-bottom: 8px;
  }

  .composer.small {
    margin-top: 14px;
  }

  .avatar {
    flex-shrink: 0;
    width: 36px;
    height: 36px;
    border-radius: 50%;
    overflow: hidden;
    background: var(--surface-overlay);
    border: 1px solid var(--border-subtle);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .avatar-sm {
    width: 28px;
    height: 28px;
  }

  .avatar-text { font-size: var(--fs-caption); }

  .form-body {
    flex: 1;
    min-width: 0;
  }

  .comment-input {
    width: 100%;
    padding: 10px 12px;
    background: var(--surface-inset);
    border: 1px solid var(--border-default);
    border-radius: 6px;
    color: var(--text-primary);
    font-family: var(--font-body);
    font-size: var(--fs-body);
    line-height: 1.55;
    resize: vertical;
    transition: border-color 0.15s;
  }

  .comment-input::placeholder {
    color: var(--text-faint);
  }

  .comment-input:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--focus-ring);
  }

  .form-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-top: 8px;
  }

  .foot-left {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .char-count {
    font-family: var(--font-mono);
    font-size: var(--fs-micro);
    color: var(--text-muted);
  }

  .actions {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .btn-submit {
    padding: 7px 18px;
    background: var(--accent);
    color: var(--on-accent);
    border: none;
    border-radius: 6px;
    font-family: var(--font-mono);
    font-size: var(--fs-caption);
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    cursor: pointer;
    transition: background 0.15s;
  }

  .btn-submit:hover:not(:disabled) {
    background: var(--accent-hover);
  }

  .btn-submit:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  .btn-sm {
    padding: 5px 14px;
  }

  .btn-text {
    background: none;
    border: none;
    padding: 5px 8px;
    font-family: var(--font-mono);
    font-size: var(--fs-caption);
    color: var(--text-muted);
    cursor: pointer;
    transition: color 0.15s;
  }

  .btn-text:hover {
    color: var(--text-primary);
  }
</style>
