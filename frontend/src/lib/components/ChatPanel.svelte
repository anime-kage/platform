<script lang="ts">
  import GifPicker from '$lib/components/GifPicker.svelte';
  import { onDestroy } from 'svelte';
  import { authStore } from '$lib/stores/auth';
  import { chatOpen, initChatOpen, toggleChat } from '$lib/stores/chat';
  import { ROLE_BADGES, EMOTES } from '$lib/data/community';
  import { emotes as emoteStore, loadEmotes, emoteMap } from '$lib/stores/emotes';
  import { nameHue } from '$lib/avatar';
  import { api } from '$lib/api';
  import { toast } from '$lib/stores/toast';
  import type { ChatMessage } from '$shared/types';
  import EmojiPicker from '$lib/components/EmojiPicker.svelte';
  import Spoiler from '$lib/components/Spoiler.svelte';
  import { parseText } from '$lib/markdown';
  import ChatUserCard from '$lib/components/ChatUserCard.svelte';

  // Live chat. Backlog over HTTP, then an EventSource for
  // everything after — SSE because the server only ever pushes, and
  // EventSource reconnects on its own when the connection drops.
  const auth = $derived($authStore);

  const open = $derived($chatOpen);

  // --chat-rail: how much of the right edge the chat is currently occupying.
  //
  // Published on :root so anything else pinned to that edge can step aside —
  // today the toast cues, which used to land straight on top of the launcher.
  // The chat is the only component that can know this figure: open it is
  // --chat-w wide, closed it is a launcher whose width depends on its label. So
  // it is measured, not hardcoded — a guessed constant goes silently wrong the
  // first time that label changes.
  const FAB_INSET = 14; // .chat-fab's own right offset
  const FAB_GAP = 10; // breathing room beside it
  let fabW = $state(0);

  $effect(() => {
    const rail = !auth.isAuthenticated
      ? '0px'
      : open
        ? 'var(--chat-w)'
        : `${fabW + FAB_INSET + FAB_GAP}px`;
    document.documentElement.style.setProperty('--chat-rail', rail);
  });

  /* On a phone the panel covers the screen, but the page behind it kept its
     own scroll: two scrollable surfaces under one finger, and the body could
     be moved while the chat was open. Locked only on phones -- on desktop the
     chat is a side rail and the page is meant to stay usable beside it.
     Tracked through a media query listener so rotating the device is handled
     rather than only the width at open time. */
  $effect(() => {
    if (typeof window === 'undefined') return;
    const mq = window.matchMedia('(max-width: 640px)');
    const apply = () => {
      document.documentElement.classList.toggle('chat-locked', open && mq.matches);
    };
    apply();
    mq.addEventListener('change', apply);
    return () => {
      mq.removeEventListener('change', apply);
      document.documentElement.classList.remove('chat-locked');
    };
  });

  // Guarded because onDestroy also fires on the server, once SSR has finished
  // rendering the component — and there is no document there.
  onDestroy(() => {
    if (typeof document !== 'undefined') {
      document.documentElement.style.removeProperty('--chat-rail');
      document.documentElement.classList.remove('chat-locked');
    }
  });

  let input = $state('');
  let messages = $state<ChatMessage[]>([]);
  let viewers = $state(0);
  let loading = $state(true);
  let connected = $state(false);
  let sending = $state(false);
  let scroller = $state<HTMLDivElement | null>(null);
  let inputEl = $state<HTMLInputElement | null>(null);
  // Twitch-style reply: quote the answered message inline, no threads
  let replyTarget = $state<{ u: string; text: string; id: number } | null>(null);

  const myName = $derived(auth.user?.username ?? '');
  // deleting a message and timing someone out are the same job, so the same
  // roles get both (the server is the authority — chatmod.go)
  const CHAT_STAFF = ['translator', 'verifier', 'coordinator', 'moderator', 'admin'];
  const canModerate = $derived(CHAT_STAFF.includes(auth.user?.role ?? ''));
  const MAX = 500;

  // mirrors chatRank in the backend — a card must not offer a button the
  // server will refuse. The server still re-checks; this only hides it.
  const RANK: Record<string, number> = {
    user: 0, translator: 1, verifier: 1, coordinator: 2, moderator: 2, admin: 3
  };
  const canModerateUser = (targetRole: string, targetName: string) =>
    canModerate &&
    targetName !== myName &&
    (RANK[auth.user?.role ?? 'user'] ?? 0) > (RANK[targetRole] ?? 0);

  const excerpt = (t: string) => (t.length > 60 ? `${t.slice(0, 60)}…` : t);

  /* Uploaded emotes (code → image URL), loaded once and shared. EMOTES — the
     original hardcoded emoji map — stays as a fallback so the codes people
     already use keep working; an uploaded emote with the same code wins. */
  $effect(() => {
    if (auth.isAuthenticated) loadEmotes();
  });
  const customEmotes = $derived(emoteMap($emoteStore));

  // the user card: which message's author, and where to put it
  let card = $state<{ msg: ChatMessage; x: number; y: number } | null>(null);

  function openCard(e: MouseEvent, m: ChatMessage) {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    card = { msg: m, x: r.left, y: r.bottom + 6 };
  }

  // Jump to the message a reply is answering.
  //
  // The target may not be loaded: the panel keeps a recent window, and the
  // original can be older than that. Saying so is better than doing nothing,
  // which reads as a broken button.
  let flashId = $state<number | null>(null);
  let flashTimer: ReturnType<typeof setTimeout> | null = null;
  // Read at click time rather than cached: someone can change the OS setting
  // while the page is open, and this is cheap.
  const reduceMotion = () =>
    typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;

  function jumpToMessage(id: number) {
    const el = scroller?.querySelector<HTMLElement>(`[data-msg-id="${id}"]`);
    if (!el) {
      toast.info('Mesajul original nu mai e încărcat în chat.');
      return;
    }
    // Leaving the bottom means new messages must stop yanking the view away.
    pinned = false;
    el.scrollIntoView({
      behavior: reduceMotion() ? 'auto' : 'smooth',
      block: 'center'
    });
    flashId = id;
    if (flashTimer) clearTimeout(flashTimer);
    flashTimer = setTimeout(() => (flashId = null), 1600);
  }

  function startReply(u: string, text: string, id: number) {
    replyTarget = { u, text: excerpt(text), id };
    inputEl?.focus();
  }

  function mention(u: string) {
    const sep = input && !input.endsWith(' ') ? ' ' : '';
    input = `${input}${sep}@${u} `;
    inputEl?.focus();
  }

  const mentionsMe = (text: string) =>
    !!myName && text.toLowerCase().includes(`@${myName.toLowerCase()}`);

  $effect(() => {
    initChatOpen();
  });

  const toggle = toggleChat;

  // Stay pinned to the bottom only when the reader already is: yanking the
  // view down while someone is reading scrollback is the classic chat sin.
  let pinned = true;
  function onScroll() {
    if (!scroller) return;
    pinned = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 60;
  }
  $effect(() => {
    messages.length;
    if (scroller && pinned) scroller.scrollTop = scroller.scrollHeight;
  });

  // ── live connection ───────────────────────────────────────────────────────
  // Only while the panel is open: a closed panel has nothing to render, and an
  // idle stream still costs the server a goroutine.
  $effect(() => {
    if (!auth.isAuthenticated || !open) return;

    let es: EventSource | null = null;
    let stopped = false;

    (async () => {
      try {
        const r = await api.getChatMessages();
        if (stopped) return;
        messages = r.data;
        viewers = r.viewers;
      } catch {
        if (!stopped) toast.error('Chatul nu a putut fi încărcat.');
      } finally {
        if (!stopped) loading = false;
      }
      if (stopped) return;

      es = new EventSource(api.chatStreamUrl());
      es.onopen = () => (connected = true);
      es.onerror = () => (connected = false); // EventSource retries by itself
      es.addEventListener('message', (e) => {
        const m: ChatMessage = JSON.parse((e as MessageEvent).data);
        // the sender already appended it optimistically from the POST reply
        if (messages.some((x) => x.id === m.id)) return;
        messages = [...messages, m];
      });
      es.addEventListener('delete', (e) => {
        const { id } = JSON.parse((e as MessageEvent).data);
        messages = messages.filter((m) => m.id !== id);
      });
      es.addEventListener('viewers', (e) => {
        viewers = JSON.parse((e as MessageEvent).data).viewers;
      });
    })();

    return () => {
      stopped = true;
      es?.close();
      connected = false;
    };
  });

  async function send() {
    const t = input.trim();
    if (!t || sending) return;
    if (t.length > MAX) {
      toast.error(`Mesajul e prea lung (max ${MAX} caractere).`);
      return;
    }
    sending = true;
    const reply = replyTarget;
    try {
      const r = await api.sendChatMessage({
        body: t,
        ...(reply ? { replyToUser: reply.u, replyToExcerpt: reply.text, replyToId: reply.id } : {})
      });
      // append straight away so your own message never waits on the round trip
      if (!messages.some((m) => m.id === r.data.id)) messages = [...messages, r.data];
      pinned = true;
      input = '';
      replyTarget = null;
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Mesajul nu a fost trimis.');
    } finally {
      sending = false;
    }
  }

  async function remove(m: ChatMessage) {
    try {
      await api.deleteChatMessage(m.id);
      messages = messages.filter((x) => x.id !== m.id); // the SSE echo is idempotent
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Ștergerea a eșuat.');
    }
  }

  // emote codes need whitespace around them so tokenize can match them;
  // plain emoji just get appended
  function insertPick(t: string) {
    if (EMOTES[t] || customEmotes[t]) {
      const sep = input && !input.endsWith(' ') ? ' ' : '';
      input = `${input}${sep}${t} `;
    } else {
      input += t;
    }
    inputEl?.focus();
  }

  const tokenize = (text: string) =>
    text
      .split(/(\s+)/)
      .filter((p) => p !== '')
      .map((p, i) => ({
        key: i,
        emote: EMOTES[p] ?? null,
        emoteUrl: customEmotes[p] ?? null,
        mention: /^@[\p{L}\d_]+$/u.test(p),
        raw: p
      }));

  const hhmm = (iso: string) =>
    new Date(iso).toLocaleTimeString('ro-RO', { hour: '2-digit', minute: '2-digit' });

  /* ── When a message was sent ───────────────────────────────────────────────
     The date goes on a separator between days, not on every row. This is what
     Discord, Slack and WhatsApp all do, and the reason is the panel: it is one
     narrow column, so a full "17-08-2026 21:34" in front of every name eats the
     width the message itself needs — and repeats the same eleven characters on
     every consecutive line, where only the minutes actually differ. A separator
     states the day once and every row under it inherits it.

     The full date is still on each row, in the timestamp's tooltip, for when
     you want it on one specific message. */

  /** Local calendar day, as YYYY-MM-DD — the key the separator breaks on.
      Built from the local getters rather than toISOString(), which is UTC and
      would move the boundary by a couple of hours. */
  const dayKey = (iso: string) => {
    const d = new Date(iso);
    return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
  };

  /** "Azi" / "Ieri" / "15 aug" / "15 aug 2025" — the year only when it isn't
      this one, so the common case stays short. */
  const dayLabel = (iso: string) => {
    const d = new Date(iso);
    const now = new Date();
    if (dayKey(iso) === dayKey(now.toISOString())) return 'Azi';
    const yesterday = new Date(now);
    yesterday.setDate(now.getDate() - 1);
    if (dayKey(iso) === dayKey(yesterday.toISOString())) return 'Ieri';
    return d.toLocaleDateString('ro-RO', {
      day: 'numeric',
      month: 'short',
      ...(d.getFullYear() === now.getFullYear() ? {} : { year: 'numeric' })
    });
  };

  /** The tooltip on a row's timestamp: "duminică, 17 august 2026, 21:34". */
  const fullStamp = (iso: string) =>
    new Date(iso).toLocaleString('ro-RO', {
      weekday: 'long',
      day: 'numeric',
      month: 'long',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit'
    });

  // a stable per-name colour, same helper the avatars use
  const nameColor = (u: string) => `hsl(${nameHue(u)} 62% 62%)`;
</script>

{#if auth.isAuthenticated}
  {#if open}
    <aside class="chat" aria-label="Chat live">
      <div class="chat-head">
        <div class="chat-title">
          <span class="label">Chat live</span>
          <span class="online" class:off={!connected} title={connected ? 'Conectat' : 'Se reconectează…'}>
            <span class="dot" class:anim-pulse={connected}></span>{viewers}
          </span>
        </div>
        <button class="collapse" onclick={toggle} title="Închide chatul">»</button>
      </div>

      <div class="msgs" bind:this={scroller} onscroll={onScroll}>
        {#if loading}
          <p class="chat-note">Se încarcă…</p>
        {:else if messages.length === 0}
          <p class="chat-note">Niciun mesaj încă. Zi ceva — chatul e al vostru.</p>
        {/if}
        {#each messages as m, i (m.id)}
          {@const badge = ROLE_BADGES[m.role]}
          <!-- first message of a new calendar day gets the date above it -->
          {#if i === 0 || dayKey(messages[i - 1].createdAt) !== dayKey(m.createdAt)}
            <div class="daymark"><span>{dayLabel(m.createdAt)}</span></div>
          {/if}
          <div
            class="msg"
            data-msg-id={m.id}
            class:mention-me={mentionsMe(m.body) && m.username !== myName}
            class:flash={flashId === m.id}
          >
            {#if m.replyToUser}
              {#if m.replyToId}
                <button
                  class="chat-quote chat-quote-btn"
                  title="Mergi la mesajul original"
                  onclick={() => jumpToMessage(m.replyToId!)}
                >↪ @{m.replyToUser}: {m.replyToExcerpt}</button>
              {:else}
                <div class="chat-quote">↪ @{m.replyToUser}: {m.replyToExcerpt}</div>
              {/if}
            {/if}
            <span class="time" title={fullStamp(m.createdAt)}>{hhmm(m.createdAt)}</span>
            {#if badge}
              <span class="badge" title={badge.title} style={`background:${badge.bg}`}>{badge.glyph}</span>
            {/if}
            <button
              class="uname"
              style={`color:${nameColor(m.username)}`}
              title={`Vezi profilul lui ${m.username}`}
              onclick={(e) => openCard(e, m)}>{m.username}</button
            ><span class="colon">: </span>
            <!-- Spoilers are split off FIRST: tokenize() splits on whitespace,
                 so a `||two words||` spoiler would otherwise be torn into
                 separate tokens and never match. Emotes and mentions still
                 resolve inside a revealed spoiler. -->
            {#each parseText(m.body) as part}
              {#if part.kind === 'spoiler'}
                <Spoiler>
                  {#each tokenize(part.text) as tk (tk.key)}
                    {#if tk.emoteUrl}<img class="emote-img" src={tk.emoteUrl} alt={tk.raw} title={tk.raw} loading="lazy" />
                      {:else if tk.emote}<span class="emote" title={tk.raw}>{tk.emote}</span
                      >{:else if tk.mention}<span class="chat-mention">{tk.raw}</span
                      >{:else}{tk.raw}{/if}
                  {/each}
                </Spoiler>
              {:else if part.kind === 'gif'}
                <img class="chat-gif" src={part.url} alt="GIF" loading="lazy" referrerpolicy="no-referrer" />
              {:else}
                {#each tokenize(part.text) as tk (tk.key)}
                  {#if tk.emoteUrl}<img class="emote-img" src={tk.emoteUrl} alt={tk.raw} title={tk.raw} loading="lazy" />
                    {:else if tk.emote}<span class="emote" title={tk.raw}>{tk.emote}</span
                    >{:else if tk.mention}<span class="chat-mention">{tk.raw}</span
                    >{:else}{tk.raw}{/if}
                {/each}
              {/if}
            {/each}
            <button class="msg-reply" title="Răspunde" onclick={() => startReply(m.username, m.body, m.id)}>↩</button>
            {#if canModerate || m.userId === auth.user?.id}
              <button class="msg-del" title="Șterge mesajul" onclick={() => remove(m)}>×</button>
            {/if}
          </div>
        {/each}
      </div>

      <div class="composer">
        {#if replyTarget}
          <div class="replying">
            <span class="replying-txt">↪ Răspunzi lui <strong>@{replyTarget.u}</strong> · {replyTarget.text}</span>
            <button class="replying-x" title="Anulează răspunsul" onclick={() => (replyTarget = null)}>×</button>
          </div>
        {/if}
        <div class="input-row">
          <input
            bind:this={inputEl}
            bind:value={input}
            maxlength={MAX}
            disabled={sending}
            placeholder={replyTarget ? `Răspunde-i lui @${replyTarget.u}...` : 'Scrie un mesaj...'}
            onkeydown={(e) => {
              if (e.key === 'Enter') send();
              if (e.key === 'Escape') replyTarget = null;
            }}
          />
          {#if input.length > 0}
            <span
              class="counter"
              class:near={input.length > MAX - 60}
              class:over={input.length >= MAX}>{input.length}/{MAX}</span
            >
          {/if}
          <EmojiPicker emotes={EMOTES} customEmotes={customEmotes} onPick={insertPick} />
          <GifPicker compact onPick={(url) => (input = input ? `${input} ${url}` : url)} />
          <button class="ib send" title="Trimite" disabled={sending} onclick={send}>➤</button>
        </div>
      </div>
    </aside>
    {#if card}
      <ChatUserCard
        username={card.msg.username}
        role={card.msg.role}
        anchor={{ x: card.x, y: card.y }}
        canModerate={canModerateUser(card.msg.role, card.msg.username)}
        onMention={mention}
        onReply={() => card && startReply(card.msg.username, card.msg.body, card.msg.id)}
        onClose={() => (card = null)}
      />
    {/if}

    <!-- overlay mode only (<1400px): the panel can't share the row with the
         page there, so it floats over it and this closes it on tap-outside -->
    <button class="chat-scrim" aria-label="Închide chatul" onclick={toggle}></button>
  {:else}
    <button class="chat-fab" bind:clientWidth={fabW} onclick={toggle} title="Deschide chatul">
      <span>Chat live</span>
    </button>
  {/if}
{/if}

<style>
  /* fixed drawer; the layout pads `main` while it's open (chat-docked), so it
     never covers content on wide screens; nav dropdowns sit above (z 100) */
  .chat {
    position: fixed; top: 62px; right: 0; bottom: 0;
    width: var(--chat-w); max-width: 100vw; z-index: 80;
    display: flex; flex-direction: column;
    background: var(--surface-raised);
    border-left: 1px solid var(--border-subtle);
    box-shadow: -14px 0 34px rgba(0, 0, 0, 0.22);
  }

  .chat-head {
    flex: 0 0 auto; display: flex; align-items: center; justify-content: space-between;
    padding: 0 8px 0 14px; height: 48px;
    border-bottom: 1px solid var(--border-subtle);
  }
  .chat-title { display: flex; align-items: center; gap: 9px; }
  .label { font-weight: var(--fw-bold); font-size: 0.8125rem; letter-spacing: 0.02em; }
  .online {
    display: inline-flex; align-items: center; gap: 5px;
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--success);
  }
  .dot { width: 6px; height: 6px; border-radius: 50%; background: var(--success); }
  .anim-pulse { animation: ak-pulse 2.2s ease-in-out infinite; }
  .collapse {
    width: 30px; height: 30px; border-radius: 7px; border: none;
    background: transparent; color: var(--text-muted);
    cursor: pointer; font-size: 1rem; line-height: 1;
  }
  .collapse:hover { background: var(--surface-overlay); color: var(--text-primary); }

  .msgs { flex: 1; overflow-y: auto; padding: 8px 0; min-height: 0; }
  /* The body sets the row's type scale: the name and the badge are sized in
     `em`, so they track it and the three stay in proportion. The timestamp is
     the one thing pinned in `rem` — it's a label, not content, and it should
     stay quiet as the message text grows. */
  .msg {
    position: relative;
    padding: 4px 34px 4px 14px; font-size: 0.9375rem; line-height: 1.55;
    color: var(--text-primary); word-wrap: break-word;
  }
  .msg:hover { background: var(--surface-overlay); }
  .msg.mention-me {
    background: color-mix(in srgb, var(--accent) 9%, transparent);
    box-shadow: inset 2px 0 0 var(--accent);
  }
  /* The three pieces of the header share one line. The time and the name sit
     on the text baseline, which is what makes them read as one run; the badge
     is a box, not text, so it centres on the line instead — `middle` puts it
     on the same optical axis rather than hanging below the baseline. */
  .time {
    font-family: var(--font-mono); font-size: 0.72rem; color: var(--text-faint);
    margin-right: 7px; cursor: default;
  }

  /* The day separator: a centred label with a rule running through it, so it
     reads as a break in the stream rather than as another message. */
  .daymark {
    display: flex; align-items: center; gap: 10px;
    padding: 10px 14px 6px; user-select: none;
  }
  .daymark::before,
  .daymark::after {
    content: ''; flex: 1; height: 1px; background: var(--border-subtle);
  }
  .daymark span {
    font-family: var(--font-mono); font-size: 0.66rem; font-weight: var(--fw-semibold);
    letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-faint);
    white-space: nowrap;
  }
  .badge {
    display: inline-grid; place-items: center;
    width: 1.4em; height: 1.4em; border-radius: 5px;
    font-size: 0.78em; color: #fff; margin-right: 5px;
    vertical-align: middle;
  }
  .uname {
    font-weight: var(--fw-bold);
    background: none; border: none; padding: 0; cursor: pointer;
    font-family: inherit; font-size: 1.02em; line-height: inherit;
  }
  .uname:hover { text-decoration: underline; }
  .colon { color: var(--text-muted); font-weight: var(--fw-bold); }

  /* The quote doubles as a link to the message being answered. Styled as the
     quote it already was, not as a button: it sits inside a dense message list
     where a real button would compete with the message itself. */
  /* A full reset, so the clickable quote is pixel-identical to the plain one.
     A <button> arrives with its own border, background, padding, centred text
     and font — leaving any of that in place makes replies that happen to have
     a jump target look different from replies that do not, which is a worse
     inconsistency than having no jump at all.
     Declared BEFORE .chat-quote on purpose: equal specificity, so the rules
     below win and re-apply the quote's own border-left and padding-left. */
  .chat-quote-btn {
    display: block;
    width: 100%;
    margin: 0;
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    line-height: inherit;
    text-align: left;
    appearance: none;
    cursor: pointer;
  }
  /* The only visible difference, and only while pointing at it. */
  .chat-quote-btn:hover { color: var(--text-primary); }
  .chat-quote-btn:focus-visible {
    outline: 2px solid var(--focus-ring);
    outline-offset: 2px;
  }

  /* Where the jump lands. Long enough to find with your eyes after the scroll
     settles, and it fades rather than snapping off. */
  .msg.flash {
    background: color-mix(in srgb, var(--accent) 18%, transparent);
    border-radius: var(--radius-sm);
    animation: msg-flash 1.6s ease-out;
  }
  @keyframes msg-flash {
    0%   { background: color-mix(in srgb, var(--accent) 34%, transparent); }
    100% { background: transparent; }
  }
  @media (prefers-reduced-motion: reduce) {
    .msg.flash { animation: none; }
  }

  /* Twitch-style reply context above a message */
  .chat-quote {
    font-size: var(--fs-micro); color: var(--text-muted);
    border-left: 2px solid color-mix(in srgb, var(--accent) 55%, transparent);
    padding-left: 7px; margin-bottom: 1px;
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }

  /* The rule that makes mixed-size uploads look like one set, and the same one
     Twitch uses: a fixed height, width:auto so the aspect is kept, and a
     max-width so an unusually wide emote cannot stretch the line. Nothing is
     resized on upload — this is what normalises them. */
  .emote-img {
    /* inline-block is load-bearing: base.css resets `img` to display:block,
       so without this an emote breaks out of the sentence onto its own line. */
    display: inline-block;
    height: 28px;
    width: auto;
    max-width: 112px;
    vertical-align: middle;
    margin: -2px 1px;
    object-fit: contain;
  }

  /* Small: the chat column is narrow and a GIF is an aside, not the message. */
  .chat-gif {
    display: block; max-width: min(170px, 100%); height: auto;
    margin: 4px 0 2px; border-radius: var(--radius-sm);
    border: 1px solid var(--border-subtle);
  }

  .chat-mention {
    color: var(--accent); font-weight: var(--fw-semibold);
    background: color-mix(in srgb, var(--accent) 13%, transparent);
    border-radius: 4px; padding: 0 3px;
  }

  .msg-reply {
    position: absolute; top: 3px; right: 8px;
    width: 22px; height: 22px; border-radius: 6px; border: none;
    background: var(--surface-raised); color: var(--text-muted);
    cursor: pointer; font-size: 0.75rem; line-height: 1;
    display: grid; place-items: center;
    opacity: 0; transition: opacity 0.12s;
  }
  .msg:hover .msg-reply, .msg:hover .msg-del { opacity: 1; }
  .msg-reply:hover { color: var(--accent); }
  .msg-del {
    float: right; margin-left: 2px; padding: 0 4px;
    background: none; border: 0; cursor: pointer; line-height: 1;
    font-size: 0.85rem; color: var(--text-muted); opacity: 0;
    transition: opacity var(--motion-fast) var(--ease);
  }
  .msg-del:hover { color: var(--danger); }

  /* the dot stops pulsing and greys out while the stream is reconnecting */
  .online.off { color: var(--text-muted); }
  .online.off .dot { background: var(--text-muted); }

  /* A flex item, not absolutely positioned: the reply bar changes the
     composer's height, and a hard-coded `bottom` drifts off the input the
     moment it appears. */
  .counter {
    flex: 0 0 auto;
    font-family: var(--font-mono); font-size: 0.625rem; color: var(--text-faint);
    pointer-events: none; font-variant-numeric: tabular-nums;
  }
  .counter.near { color: var(--warning); }
  .counter.over { color: var(--danger); }
  .emote { display: inline-block; font-size: 1.25rem; line-height: 1; vertical-align: -4px; margin: 0 2px; }

  .chat-note {
    margin: 10px 12px; font-size: 0.75rem; color: var(--text-muted); line-height: 1.5;
  }

  .composer {
    flex: 0 0 auto; position: relative;
    border-top: 1px solid var(--border-subtle); padding: 10px 12px;
  }
  .replying {
    display: flex; align-items: center; gap: 8px;
    margin-bottom: 8px; padding: 6px 8px 6px 10px;
    background: var(--surface-overlay); border-radius: 8px;
    border-left: 2px solid var(--accent);
  }
  .replying-txt {
    flex: 1; min-width: 0;
    font-size: 0.71875rem; color: var(--text-muted);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .replying-txt strong { color: var(--accent); }
  .replying-x {
    flex: 0 0 auto; width: 20px; height: 20px; border-radius: 6px; border: none;
    background: transparent; color: var(--text-muted); cursor: pointer;
    font-size: 0.875rem; line-height: 1; display: grid; place-items: center;
  }
  .replying-x:hover { background: var(--surface-raised); color: var(--text-primary); }

  .input-row {
    display: flex; align-items: center; gap: 8px;
    background: var(--surface-overlay); border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md); padding: 4px 4px 4px 12px;
  }
  .input-row:focus-within { border-color: var(--accent); }
  .input-row input {
    flex: 1; min-width: 0; background: transparent; border: none;
    padding: 8px 0; color: var(--text-primary); outline: none; font-size: 0.8125rem;
  }
  .input-row input::placeholder { color: var(--text-faint); }
  .ib {
    flex: 0 0 auto; width: 32px; height: 32px; border-radius: 8px; border: none;
    background: transparent; color: var(--text-muted);
    cursor: pointer; font-size: 1rem; display: grid; place-items: center;
  }
  .ib:hover { background: var(--surface-raised); color: var(--text-primary); }
  .ib.send { background: var(--accent); color: var(--on-accent); font-size: 0.875rem; }
  .ib.send:hover { background: var(--accent-hover); }

  .chat-fab {
    position: fixed; top: 74px; right: 14px; z-index: 80;
    display: inline-flex; align-items: center; gap: 9px;
    height: 40px; padding: 0 15px; border-radius: var(--radius-md);
    border: 1px solid var(--border-default); background: var(--surface-raised);
    color: var(--text-primary); cursor: pointer;
    font-weight: var(--fw-semibold); font-size: 0.8125rem;
    box-shadow: 0 6px 18px rgba(0, 0, 0, 0.14);
  }
  .chat-fab:hover { border-color: var(--accent); }

  /* Docked (≥1100px): the layout pads `main`, so the page keeps a usable
     column beside the panel (1026px on a 1366 laptop) and both stay live —
     no scrim, because making the chat modal on a laptop is worse than a
     narrower page. Below 1100px the remainder would be under ~760px, so
     the panel floats over the page and a scrim closes it on tap-outside. */
  .chat-scrim {
    display: none;
    position: fixed; inset: 62px 0 0 0; z-index: 79;
    border: none; background: rgba(0, 0, 0, 0.5); cursor: default;
    -webkit-backdrop-filter: blur(2px); backdrop-filter: blur(2px);
  }
  @media (max-width: 1099px) {
    .chat-scrim { display: block; }
    .chat { box-shadow: -20px 0 60px rgba(0, 0, 0, 0.5); }
  }
  /* Set on <html> while the phone sheet is open, so the page underneath cannot
     be scrolled at the same time. :global because the class is on an element
     outside this component. */
  :global(html.chat-locked),
  :global(html.chat-locked body) {
    overflow: hidden;
    overscroll-behavior: none;
  }

  /* phone: full-bleed sheet, and it owns the screen while open */
  @media (max-width: 640px) {
    /* dvh, not the default bottom:0 alone: with mobile browser chrome showing
       and hiding, a fixed panel measured against the large viewport leaves a
       strip of page visible at the bottom -- the "95% of the screen" effect. */
    .chat { width: 100%; top: 0; bottom: 0; height: 100dvh; z-index: 210; }
    .chat-scrim { inset: 0; z-index: 209; }
  }

  .chat-fab {
    /* keep the pill clear of the viewport edge and above the fold line */
    right: max(14px, env(safe-area-inset-right));
  }
  @media (max-width: 640px) {
    .chat-fab {
      top: auto; bottom: max(16px, env(safe-area-inset-bottom));
      height: var(--tap-min); border-radius: var(--radius-pill);
      box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
    }
  }
</style>
