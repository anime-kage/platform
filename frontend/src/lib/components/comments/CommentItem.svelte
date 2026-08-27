<script lang="ts">
  import RichText from '$lib/components/RichText.svelte';
  import type { Snippet } from 'svelte';
  import api from '$lib/api';
  import { nameHue } from '$lib/avatar';
  import VoteButtons from './VoteButtons.svelte';
  import type { Comment as CommentType } from '$shared/types';

  interface Props {
    comment: CommentType;
    user: { id: number; username: string } | null;
    /** reply row: smaller avatar, tighter gap */
    small?: boolean;
    compact?: boolean;
    /** hide the reply button (flat threads, e.g. review pages) */
    flat?: boolean;
    /** show the "↪ @user quoted text" context line (nested replies) */
    showQuote?: boolean;
    editing?: boolean;
    editContent?: string;
    replying?: boolean;
    onVote: (type: 'like' | 'dislike') => void;
    onToggleReply?: () => void;
    onStartEdit: () => void;
    onCancelEdit: () => void;
    onSaveEdit: () => void;
    onDelete: () => void;
    onReport: () => void;
    /** the reply composer, rendered when `replying` */
    replyForm?: Snippet;
    /** thread content (show-replies button, nested replies) */
    children?: Snippet;
  }

  let {
    comment,
    user,
    small = false,
    compact = false,
    flat = false,
    showQuote = false,
    editing = false,
    editContent = $bindable(''),
    replying = false,
    onVote,
    onToggleReply,
    onStartEdit,
    onCancelEdit,
    onSaveEdit,
    onDelete,
    onReport,
    replyForm,
    children
  }: Props = $props();

  function formatDate(date: Date | string) {
    const d = new Date(date);
    const now = new Date();
    const diff = now.getTime() - d.getTime();
    const minutes = Math.floor(diff / 60000);
    const hours = Math.floor(diff / 3600000);
    const days = Math.floor(diff / 86400000);

    if (minutes < 1) return 'acum';
    if (minutes < 60) return `${minutes}m`;
    if (hours < 24) return `${hours}h`;
    if (days < 7) return `${days}z`;
    return d.toLocaleDateString('ro-RO', { day: 'numeric', month: 'short' });
  }
</script>

<div class="item" class:small class:compact>
  <div class="avatar" class:monogram={!comment.user.avatarUrl} style="--mg-hue: {nameHue(comment.user.username)}">
    {#if comment.user.avatarUrl}
      <img src={api.resolveUrl(comment.user.avatarUrl)} alt={comment.user.username} />
    {:else}
      <span class="avatar-text">{comment.user.username.charAt(0).toUpperCase()}</span>
    {/if}
  </div>
  <div class="content">
    <div class="header">
      <a href="/user/{comment.user.username}" class="author">{comment.user.username}</a>
      <span class="date">{formatDate(comment.createdAt)}</span>
    </div>

    {#if showQuote && comment.replyToUsername}
      <a href="/user/{comment.replyToUsername}" class="quote">
        <span class="quote-icon">↪</span>
        <span class="quote-user">@{comment.replyToUsername}</span>
        {#if comment.replyToExcerpt}
          <span class="quote-text">{comment.replyToExcerpt}</span>
        {/if}
      </a>
    {/if}

    {#if editing}
      <div class="edit-form">
        <textarea
          bind:value={editContent}
          class="edit-input"
          rows={small ? 2 : 3}
          maxlength="2000"
        ></textarea>
        <div class="edit-actions">
          <button onclick={onCancelEdit} class="btn-text">Anulează</button>
          <button onclick={onSaveEdit} class="btn-text accent">Salvează</button>
        </div>
      </div>
    {:else}
      <p class="text"><RichText text={comment.content} /></p>
    {/if}

    <div class="actions">
      <VoteButtons
        likes={comment.likesCount}
        dislikes={comment.dislikesCount}
        userVote={comment.userVote ?? null}
        {onVote}
      />

      {#if user && !flat && onToggleReply}
        <button onclick={onToggleReply} class="action-btn" class:active={replying}>
          <svg class="action-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 10h10a8 8 0 018 8v2M3 10l6 6m-6-6l6-6" />
          </svg>
          <span>Răspunde</span>
        </button>
      {/if}

      {#if user && user.id === comment.userId}
        <button onclick={onStartEdit} class="action-btn">
          <svg class="action-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
          </svg>
          <span>Editează</span>
        </button>
        <button onclick={onDelete} class="action-btn danger">
          <svg class="action-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
          </svg>
          <span>Șterge</span>
        </button>
      {:else if user}
        <button onclick={onReport} class="action-btn muted">
          <svg class="action-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 21v-4m0 0V5a2 2 0 012-2h6.5l1 1H21l-3 6 3 6h-8.5l-1-1H5a2 2 0 00-2 2zm9-13.5V9" />
          </svg>
          <span>Raportează</span>
        </button>
      {/if}
    </div>

    {#if replying && replyForm}
      {@render replyForm()}
    {/if}

    {#if children}
      {@render children()}
    {/if}
  </div>
</div>

<style>
  .item {
    display: flex;
    gap: 14px;
  }

  .item.small {
    gap: 10px;
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

  .item.small .avatar,
  .item.compact .avatar {
    width: 28px;
    height: 28px;
  }

  .avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .avatar-text { font-size: var(--fs-caption); }

  .content {
    flex: 1;
    min-width: 0;
  }

  .header {
    display: flex;
    align-items: baseline;
    gap: 10px;
    margin-bottom: 4px;
  }

  .author {
    font-size: var(--fs-body);
    font-weight: 700;
    color: var(--text-primary);
  }

  .author:hover {
    color: var(--accent);
  }

  .date {
    font-family: var(--font-mono);
    font-size: var(--fs-micro);
    color: var(--text-muted);
  }

  /* quoted context: who + what a nested reply answers */
  .quote {
    display: flex;
    align-items: baseline;
    gap: 6px;
    max-width: 100%;
    margin: 2px 0 6px;
    padding: 6px 10px;
    background: var(--surface-raised);
    border-left: 2px solid var(--accent);
    border-radius: 0 6px 6px 0;
    font-size: var(--fs-caption);
    line-height: 1.4;
    overflow: hidden;
  }

  .quote-icon {
    color: var(--accent);
    flex-shrink: 0;
  }

  .quote-user {
    font-family: var(--font-mono);
    font-weight: 600;
    color: var(--accent);
    flex-shrink: 0;
  }

  .quote-text {
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }

  .quote:hover .quote-user {
    color: var(--accent-hover);
  }

  .text {
    font-size: var(--fs-body);
    line-height: 1.65;
    color: var(--text-muted);
    white-space: pre-wrap;
    word-break: break-word;
    margin: 0;
  }

  /* ---- Edit in place ---- */
  .edit-input {
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

  .edit-input:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--focus-ring);
  }

  .edit-actions {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 6px;
    margin-top: 6px;
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

  .btn-text.accent {
    color: var(--accent);
  }

  .btn-text.accent:hover {
    color: var(--accent-hover);
  }

  /* ---- Action row: quiet mono buttons ---- */
  .actions {
    display: flex;
    align-items: center;
    gap: 4px;
    margin-top: 8px;
    flex-wrap: wrap;
  }

  .action-btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 4px 8px;
    background: none;
    border: none;
    border-radius: 4px;
    font-family: var(--font-mono);
    font-size: var(--fs-caption);
    color: var(--text-muted);
    cursor: pointer;
    transition: color 0.15s, background 0.15s;
  }

  .action-btn:hover {
    color: var(--text-primary);
    background: var(--surface-overlay);
  }

  .action-btn.active {
    color: var(--accent);
  }

  .action-btn.danger:hover {
    color: var(--danger);
  }

  .action-btn.muted:hover {
    color: var(--text-muted);
  }

  .action-icon {
    width: 15px;
    height: 15px;
  }

  @media (max-width: 640px) {
    .item {
      gap: 10px;
    }
  }
</style>
