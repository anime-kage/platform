<script lang="ts">
  import { onMount } from 'svelte';
  import { authStore } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import api from '$lib/api';
  import CommentItem from '$lib/components/comments/CommentItem.svelte';
  import CommentComposer from '$lib/components/comments/CommentComposer.svelte';
  import type { Comment as CommentType } from '$shared/types';

  interface Props {
    animeId?: number;
    mangaId?: number;
    /** scope the thread to one episode/chapter; omit for series-wide discussion */
    episodeId?: number;
    chapterId?: number;
    /** scope to one review's reply thread (watchlist/readlist entry id) */
    reviewId?: number;
    /** a news post's thread — a target in its own right, no parent title */
    announcementId?: number;
    heading?: string;
    /** slimmer chrome for embedding (e.g. under a review card) */
    compact?: boolean;
    /** single-level comments, no reply threads (Letterboxd review pages) */
    flat?: boolean;
  }

  let {
    animeId,
    mangaId,
    episodeId,
    chapterId,
    reviewId,
    announcementId,
    heading = 'Comentarii',
    compact = false,
    flat = false
  }: Props = $props();

  let comments = $state<CommentType[]>([]);
  let loading = $state(true);
  let posting = $state(false);
  let newComment = $state('');
  let page = $state(1);
  let totalPages = $state(1);
  let editingId = $state<number | null>(null);
  let editContent = $state('');
  let replyingTo = $state<number | null>(null);
  let replyContent = $state('');
  let expandedReplies = $state<Set<number>>(new Set());
  let loadingReplies = $state<Set<number>>(new Set());
  let expandedBranches = $state<Set<number>>(new Set());

  // Reddit-style grouping: direct replies to a comment stay visible; deeper
  // exchanges collapse into a sub-thread under the direct reply they grew from.
  type Branch = { head: CommentType; children: CommentType[] };

  function buildBranches(comment: CommentType): Branch[] {
    const branches: Branch[] = [];
    const branchOf = new Map<number, Branch>();
    for (const r of comment.replies ?? []) {
      const parentBranch =
        r.parentId && r.parentId !== comment.id ? branchOf.get(r.parentId) : undefined;
      if (parentBranch) {
        parentBranch.children.push(r);
        branchOf.set(r.id, parentBranch);
      } else {
        const b: Branch = { head: r, children: [] };
        branches.push(b);
        branchOf.set(r.id, b);
      }
    }
    return branches;
  }

  function toggleBranch(headId: number) {
    if (expandedBranches.has(headId)) expandedBranches.delete(headId);
    else expandedBranches.add(headId);
    expandedBranches = new Set(expandedBranches);
  }

  const authState = $derived($authStore);
  const user = $derived(authState.user);

  onMount(() => loadComments());

  async function loadComments(p = 1) {
    loading = true;
    try {
      const res = animeId
        ? await api.getAnimeComments(animeId, p, 20, episodeId, reviewId)
        : announcementId
          ? await api.getAnnouncementComments(announcementId, p, 20)
          : await api.getMangaComments(mangaId!, p, 20, chapterId, reviewId);
      comments = res.data;
      totalPages = res.pagination.totalPages;
      page = p;
    } catch {
      toast.error('Nu s-au putut încărca comentariile');
    } finally {
      loading = false;
    }
  }

  async function submitComment() {
    const content = newComment.trim();
    if (!content || !user) return;
    posting = true;
    try {
      const res = animeId
        ? await api.postAnimeComment(animeId, content, episodeId, reviewId)
        : announcementId
          ? await api.postAnnouncementComment(announcementId, content)
          : await api.postMangaComment(mangaId!, content, chapterId, reviewId);
      comments = [res.data, ...comments];
      newComment = '';
      toast.success('Comentariu adăugat!');
    } catch {
      toast.error('Eroare la postarea comentariului');
    } finally {
      posting = false;
    }
  }

  async function toggleReplies(commentId: number) {
    if (expandedReplies.has(commentId)) {
      expandedReplies.delete(commentId);
      expandedReplies = new Set(expandedReplies);
    } else {
      await loadReplies(commentId);
    }
  }

  async function loadReplies(commentId: number) {
    loadingReplies.add(commentId);
    loadingReplies = new Set(loadingReplies);
    try {
      const res = await api.getReplies(commentId);
      comments = comments.map(c =>
        c.id === commentId ? { ...c, replies: res.data } : c
      );
      expandedReplies.add(commentId);
      expandedReplies = new Set(expandedReplies);
    } catch {
      toast.error('Nu s-au putut încărca răspunsurile');
    } finally {
      loadingReplies.delete(commentId);
      loadingReplies = new Set(loadingReplies);
    }
  }

  // targetId = the comment/reply being answered, rootId = its thread's
  // top-level comment (the reply lands in that thread)
  async function submitReply(targetId: number, rootId: number) {
    const content = replyContent.trim();
    if (!content || !user) return;
    try {
      const res = await api.postReply(targetId, content);
      const root = comments.find(c => c.id === rootId);
      if (root && root.repliesCount > 0 && !root.replies) {
        // thread never loaded — fetch it whole (includes the new reply)
        comments = comments.map(c =>
          c.id === rootId ? { ...c, repliesCount: c.repliesCount + 1 } : c
        );
        await loadReplies(rootId);
      } else {
        comments = comments.map(c => {
          if (c.id === rootId) {
            return {
              ...c,
              replies: [...(c.replies || []), res.data],
              repliesCount: c.repliesCount + 1
            };
          }
          return c;
        });
        if (!expandedReplies.has(rootId)) {
          expandedReplies.add(rootId);
          expandedReplies = new Set(expandedReplies);
        }
      }
      // make sure the sub-thread holding the new reply is open
      if (targetId !== rootId) {
        const root = comments.find(c => c.id === rootId);
        if (root?.replies) {
          const byId = new Map(root.replies.map(r => [r.id, r]));
          let head = byId.get(targetId);
          while (head && head.parentId && head.parentId !== rootId) head = byId.get(head.parentId);
          if (head) {
            expandedBranches.add(head.id);
            expandedBranches = new Set(expandedBranches);
          }
        }
      }
      replyContent = '';
      replyingTo = null;
      toast.success('Răspuns adăugat!');
    } catch {
      toast.error('Eroare la postarea răspunsului');
    }
  }

  async function handleVote(comment: CommentType, voteType: 'like' | 'dislike', isReply = false, parentId?: number) {
    if (!user) {
      toast.info('Trebuie să fii autentificat pentru a vota');
      return;
    }
    try {
      const res = await api.voteComment(comment.id, voteType);
      const prevVote = comment.userVote;

      const updateComment = (c: CommentType) => {
        if (c.id !== comment.id) return c;
        let likes = c.likesCount;
        let dislikes = c.dislikesCount;

        if (res.voteType === null) {
          if (prevVote === 'like') likes--;
          else if (prevVote === 'dislike') dislikes--;
        } else if (prevVote === null) {
          if (res.voteType === 'like') likes++;
          else dislikes++;
        } else {
          if (res.voteType === 'like') { likes++; dislikes--; }
          else { dislikes++; likes--; }
        }
        return { ...c, likesCount: likes, dislikesCount: dislikes, userVote: res.voteType };
      };

      if (isReply && parentId) {
        comments = comments.map(c => {
          if (c.id === parentId && c.replies) {
            return { ...c, replies: c.replies.map(updateComment) };
          }
          return c;
        });
      } else {
        comments = comments.map(updateComment);
      }
    } catch {
      toast.error('Eroare la votare');
    }
  }

  function startEdit(comment: CommentType) {
    editingId = comment.id;
    editContent = comment.content;
  }

  async function saveEdit(commentId: number, isReply = false, parentId?: number) {
    if (!editContent.trim()) return;
    try {
      const res = await api.editComment(commentId, editContent.trim());

      if (isReply && parentId) {
        comments = comments.map(c => {
          if (c.id === parentId && c.replies) {
            return {
              ...c,
              replies: c.replies.map(r =>
                r.id === commentId ? { ...r, content: editContent.trim(), updatedAt: res.data.updatedAt } : r
              )
            };
          }
          return c;
        });
      } else {
        comments = comments.map(c =>
          c.id === commentId ? { ...c, content: editContent.trim(), updatedAt: res.data.updatedAt } : c
        );
      }
      editingId = null;
      toast.success('Comentariu editat');
    } catch {
      toast.error('Eroare la editare');
    }
  }

  async function handleDelete(commentId: number, isReply = false, parentId?: number) {
    try {
      await api.deleteComment(commentId);

      if (isReply && parentId) {
        comments = comments.map(c => {
          if (c.id === parentId && c.replies) {
            return {
              ...c,
              replies: c.replies.filter(r => r.id !== commentId),
              repliesCount: Math.max(0, c.repliesCount - 1)
            };
          }
          return c;
        });
      } else {
        comments = comments.filter(c => c.id !== commentId);
      }
      toast.success('Comentariu șters');
    } catch {
      toast.error('Eroare la ștergere');
    }
  }

  async function handleReport(commentId: number) {
    try {
      await api.reportComment(commentId);
      toast.success('Comentariu raportat');
    } catch {
      toast.error('Eroare la raportare');
    }
  }
</script>

<section class="comments-section" class:compact>
  <div class="comments-header">
    <h2 class="section-title">
      {heading}
      <span class="comment-count">{comments.length}</span>
    </h2>
  </div>

  <!-- one reply/comment row; rootId scopes which thread a reply lands in -->
  {#snippet replyItem(reply: CommentType, root: CommentType, showQuote: boolean)}
    <div class="reply">
      <CommentItem
        comment={reply}
        {user}
        small
        {compact}
        {flat}
        {showQuote}
        editing={editingId === reply.id}
        bind:editContent
        replying={replyingTo === reply.id}
        onVote={(t) => handleVote(reply, t, true, root.id)}
        onToggleReply={() => (replyingTo = replyingTo === reply.id ? null : reply.id)}
        onStartEdit={() => startEdit(reply)}
        onCancelEdit={() => (editingId = null)}
        onSaveEdit={() => saveEdit(reply.id, true, root.id)}
        onDelete={() => handleDelete(reply.id, true, root.id)}
        onReport={() => handleReport(reply.id)}
      >
        {#snippet replyForm()}
          {#if user}
            <CommentComposer
              {user}
              small
              bind:value={replyContent}
              rows={2}
              placeholder={`Răspunde-i lui @${reply.user.username}...`}
              submitLabel="Răspunde"
              onSubmit={() => submitReply(reply.id, root.id)}
              onCancel={() => (replyingTo = null)}
            />
          {/if}
        {/snippet}
      </CommentItem>
    </div>
  {/snippet}

  <!-- New comment form (hidden while a reply box is open, to keep one
       composer on screen at a time) -->
  {#if user}
    {#if replyingTo === null}
      <CommentComposer
        {user}
        {compact}
        bind:value={newComment}
        placeholder="Scrie un comentariu... (Ctrl+Enter pentru a trimite)"
        {posting}
        onSubmit={submitComment}
        submitOnCtrlEnter
      />
    {/if}
  {:else}
    <div class="login-prompt">
      <svg class="prompt-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
      </svg>
      <a href="/login" class="login-link">Conectează-te</a> pentru a lăsa un comentariu.
    </div>
  {/if}

  <!-- Comments list -->
  {#if loading && comments.length === 0}
    <div class="loading">
      <div class="spinner"></div>
      <p>Se încarcă comentariile...</p>
    </div>
  {:else if comments.length === 0}
    <div class="empty">
      <svg class="empty-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
      </svg>
      <p>Niciun comentariu deocamdată.</p>
      <p class="empty-subtitle">Fii primul care lasă un comentariu!</p>
    </div>
  {:else}
    <div class="comments-list">
      {#each comments as comment (comment.id)}
        <div class="comment">
          <CommentItem
            {comment}
            {user}
            {compact}
            {flat}
            editing={editingId === comment.id}
            bind:editContent
            replying={replyingTo === comment.id}
            onVote={(t) => handleVote(comment, t)}
            onToggleReply={() => (replyingTo = replyingTo === comment.id ? null : comment.id)}
            onStartEdit={() => startEdit(comment)}
            onCancelEdit={() => (editingId = null)}
            onSaveEdit={() => saveEdit(comment.id)}
            onDelete={() => handleDelete(comment.id)}
            onReport={() => handleReport(comment.id)}
          >
            {#snippet replyForm()}
              {#if user}
                <CommentComposer
                  {user}
                  small
                  bind:value={replyContent}
                  rows={2}
                  placeholder={`Răspunde-i lui @${comment.user.username}...`}
                  submitLabel="Răspunde"
                  onSubmit={() => submitReply(comment.id, comment.id)}
                  onCancel={() => (replyingTo = null)}
                />
              {/if}
            {/snippet}

            <!-- Replies -->
            {#if comment.repliesCount > 0}
              <button
                onclick={() => toggleReplies(comment.id)}
                class="show-replies-btn"
                disabled={loadingReplies.has(comment.id)}
              >
                <svg class="replies-icon" class:expanded={expandedReplies.has(comment.id)} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                </svg>
                {#if loadingReplies.has(comment.id)}
                  Se încarcă...
                {:else if expandedReplies.has(comment.id)}
                  Ascunde răspunsurile ({comment.repliesCount})
                {:else}
                  Arată răspunsurile ({comment.repliesCount})
                {/if}
              </button>

              {#if expandedReplies.has(comment.id) && comment.replies}
                <div class="replies-list">
                  {#each buildBranches(comment) as branch (branch.head.id)}
                    {@render replyItem(branch.head, comment, false)}

                    {#if branch.children.length}
                      <button class="sub-toggle" onclick={() => toggleBranch(branch.head.id)}>
                        {#if expandedBranches.has(branch.head.id)}
                          − Ascunde firul
                        {:else}
                          ＋ Continuă firul ({branch.children.length})
                        {/if}
                      </button>
                      {#if expandedBranches.has(branch.head.id)}
                        <div class="sub-replies">
                          {#each branch.children as child (child.id)}
                            {@render replyItem(child, comment, child.parentId !== branch.head.id)}
                          {/each}
                        </div>
                      {/if}
                    {/if}
                  {/each}
                </div>
              {/if}
            {/if}
          </CommentItem>
        </div>
      {/each}
    </div>

    <!-- Pagination -->
    {#if totalPages > 1}
      <div class="pagination">
        <button
          onclick={() => loadComments(page - 1)}
          disabled={page === 1 || loading}
          class="pagination-btn"
        >
          <svg class="pagination-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
          </svg>
          Anterior
        </button>

        <div class="pagination-info">
          <span>Pagina {page} din {totalPages}</span>
        </div>

        <button
          onclick={() => loadComments(page + 1)}
          disabled={page >= totalPages || loading}
          class="pagination-btn"
        >
          Următor
          <svg class="pagination-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
          </svg>
        </button>
      </div>
    {/if}
  {/if}
</section>

<style>
  .comments-section {
    margin-top: 8px;
  }

  .comments-header {
    margin-bottom: 18px;
  }

  .section-title {
    display: flex;
    align-items: baseline;
    gap: 10px;
    font-family: var(--font-mono);
    font-size: var(--fs-micro);
    font-weight: 600;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: var(--text-muted);
    margin: 0;
  }

  .comment-count {
    color: var(--accent);
  }

  /* ---- Login prompt ---- */
  .login-prompt {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 14px 16px;
    border: 1px dashed var(--border-default);
    border-radius: 6px;
    font-size: var(--fs-small);
    color: var(--text-muted);
  }

  .prompt-icon {
    width: 18px;
    height: 18px;
    color: var(--text-muted);
    flex-shrink: 0;
  }

  .login-link {
    color: var(--accent);
    font-weight: 600;
  }

  .login-link:hover {
    color: var(--accent-hover);
  }

  /* ---- Loading / empty ---- */
  .loading {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    padding: 36px 0;
    color: var(--text-muted);
    font-size: var(--fs-small);
  }

  .spinner {
    width: 22px;
    height: 22px;
    border: 2px solid var(--border-default);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .empty {
    padding: 36px 0;
    text-align: center;
    color: var(--text-muted);
    font-size: var(--fs-body);
  }

  .empty-icon {
    width: 28px;
    height: 28px;
    margin: 0 auto 10px;
    color: var(--text-muted);
  }

  .empty-subtitle {
    color: var(--text-muted);
    font-size: var(--fs-caption);
    margin-top: 4px;
  }

  /* ---- Comments list: flat, hairline-separated (Letterboxd) ---- */
  .comments-list {
    margin-top: 10px;
  }

  .comment {
    padding: 20px 0;
    border-top: 1px solid var(--border-subtle);
  }

  /* ---- Replies thread ---- */
  .show-replies-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    margin-top: 10px;
    padding: 4px 0;
    background: none;
    border: none;
    font-family: var(--font-mono);
    font-size: var(--fs-caption);
    color: var(--accent);
    cursor: pointer;
    transition: color 0.15s;
  }

  .show-replies-btn:hover:not(:disabled) {
    color: var(--accent-hover);
  }

  .show-replies-btn:disabled {
    opacity: 0.6;
    cursor: wait;
  }

  .replies-icon {
    width: 13px;
    height: 13px;
    transition: transform 0.15s;
  }

  .replies-icon.expanded {
    transform: rotate(180deg);
  }

  .replies-list {
    margin-top: 12px;
    padding-left: 16px;
    border-left: 2px solid var(--border-subtle);
  }

  .reply {
    padding: 12px 0;
  }

  .reply + .reply {
    border-top: 1px solid var(--border-subtle);
  }

  /* collapsed deeper exchange under a direct reply (Reddit-style) */
  .sub-toggle {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    margin: 2px 0 8px;
    padding: 3px 0;
    background: none;
    border: none;
    font-family: var(--font-mono);
    font-size: var(--fs-caption);
    color: var(--text-muted);
    cursor: pointer;
    transition: color 0.15s;
  }

  .sub-toggle:hover {
    color: var(--accent);
  }

  .sub-replies {
    margin: 0 0 10px;
    padding-left: 14px;
    border-left: 2px solid var(--border-subtle);
  }

  .sub-replies .reply {
    padding: 10px 0;
  }

  /* ---- Pagination ---- */
  .pagination {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 16px;
    margin-top: 20px;
    padding-top: 16px;
    border-top: 1px solid var(--border-subtle);
  }

  .pagination-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 6px 14px;
    background: none;
    border: 1px solid var(--border-default);
    border-radius: 6px;
    font-family: var(--font-mono);
    font-size: var(--fs-caption);
    color: var(--text-muted);
    cursor: pointer;
    transition: color 0.15s, border-color 0.15s;
  }

  .pagination-btn:hover:not(:disabled) {
    color: var(--text-primary);
    border-color: var(--border-strong);
  }

  .pagination-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .pagination-icon {
    width: 13px;
    height: 13px;
  }

  .pagination-info {
    font-family: var(--font-mono);
    font-size: var(--fs-micro);
    color: var(--text-muted);
  }

  /* ---- compact variant (embedded threads, e.g. under a review) ---- */
  .compact {
    margin-top: 0;
  }

  .compact .comments-header {
    margin-bottom: 10px;
  }

  .compact .comment {
    padding: 12px 0;
  }

  .compact .loading,
  .compact .empty {
    padding: 16px 0;
  }

  @media (max-width: 640px) {
    .replies-list {
      padding-left: 10px;
    }
  }
</style>
