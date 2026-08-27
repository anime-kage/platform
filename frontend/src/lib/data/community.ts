/* ============================================================
   Community seed content (matches the design sketch 1-1).
   The backend has no social features yet — members, reviews,
   forum threads, chat and requests are demo data so the UI is
   complete; anime/manga referenced by the pages come from the
   real API. Replace with API calls once endpoints exist.
   ============================================================ */

export const hueGrad = (h: number) =>
  `linear-gradient(158deg, oklch(0.52 0.082 ${h}) 0%, oklch(0.3 0.062 ${h}) 46%, oklch(0.155 0.032 ${h}) 100%)`;

/* Deterministic pseudo-randomness so the seeded social layer looks the
   same on every visit (network sizes, member subsets, activity bars). */
export const seedHash = (s: string) => {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) h = ((h ^ s.charCodeAt(i)) * 16777619) >>> 0;
  return h;
};

/** Follower/following counts for accounts without seed stats (real users). */
export const seedCounts = (key: string) => ({
  followers: 24 + (seedHash(key + ':f') % 380),
  following: 12 + (seedHash(key + ':g') % 160)
});

/** Deterministic subset of seed members shown as someone's network. */
export const seedNetwork = (key: string, kind: 'followers' | 'following') => {
  const h = seedHash(key + ':' + kind);
  const picked = MEMBERS.filter((m, i) => m.id !== key && ((h >> i) & 1) === 1);
  return picked.length ? picked : MEMBERS.filter((m) => m.id !== key).slice(0, 3);
};

/** 14 days of pseudo activity (episodes/day) for seed member profiles. */
export const seedActivity = (key: string) =>
  Array.from({ length: 14 }, (_, i) => {
    const v = seedHash(`${key}:day${i}`) % 8;
    return v <= 1 ? 0 : v - 1; // some rest days, peaks up to 6
  });

export interface Member {
  id: string;
  name: string;
  hue: number;
  bio: string;
  followers: number;
  following: number;
  reviewCount: number;
  listCount: number;
}

export const MEMBERS: Member[] = [
  { id: 'andrei', name: 'Andrei', hue: 18, bio: 'Shounen & battle anime. Maraton de weekend, mereu.', followers: 312, following: 148, reviewCount: 48, listCount: 6 },
  { id: 'maria', name: 'Maria', hue: 165, bio: 'Ghibli pe repeat. Traducător la Anime·Kage.', followers: 489, following: 96, reviewCount: 62, listCount: 4 },
  { id: 'vlad', name: 'Vlad', hue: 265, bio: 'Thrillere psihologice & sci-fi cerebral.', followers: 204, following: 130, reviewCount: 37, listCount: 5 },
  { id: 'ioana', name: 'Ioana', hue: 330, bio: 'Slice of life & romance. Plâng la orice final.', followers: 178, following: 210, reviewCount: 54, listCount: 7 },
  { id: 'dragos', name: 'Dragoș', hue: 95, bio: 'Clasice atemporale. Listele mele = teme de casă.', followers: 256, following: 74, reviewCount: 29, listCount: 8 }
];

export const memberById = (id: string) => MEMBERS.find((m) => m.id === id) ?? MEMBERS[0];

/* ---- Home page strips ----
   The ACTIVITY / NEWS / TOPICS demo arrays are gone. All three strips on /home
   read real data now: activity from /api/community/activity?scope=site, the
   forum column from /api/community/forum, and "Știri & anunțuri" from the
   announcements table (written in /admin/anunturi). Each renders an empty state
   rather than filler when there is nothing yet. */

/* ---- Reviews feed (texts from the sketch; anime filled from API) ---- */
export const REVIEW_SEEDS = [
  { memberId: 'andrei', rating: 5, date: 'acum 2 zile', likes: 142, text: 'O continuare care respectă tonul melancolic al primului sezon. Animație impecabilă și un ritm care îți dă timp să respiri. Rămâne o meditație despre timp și pierdere.', replies: [{ memberId: 'maria', text: 'Exact ce simțeam și eu despre ritm. Nu se grăbește niciodată.', date: '1 zi' }, { memberId: 'vlad', text: 'Episodul 4 m-a distrus complet.', date: '22 h' }] },
  { memberId: 'maria', rating: 5, date: 'acum 4 zile', likes: 98, text: 'Lent la început, devastator la final. Fiecare detaliu din primele episoade contează.', replies: [{ memberId: 'dragos', text: 'El Psy Congroo.', date: '3 zile' }] },
  { memberId: 'vlad', rating: 4, date: 'acum 5 zile', likes: 64, text: 'Haotic și violent, exact cum trebuie. Protagonistul e incomod și îmi place enorm că nu încearcă să fie eroul clasic.', replies: [] },
  { memberId: 'ioana', rating: 5, date: 'acum 1 săptămână', likes: 211, text: 'Îl revezi la fiecare anotimp și descoperi ceva nou. Pur și simplu perfect, de la coloana sonoră până la ultimul cadru.', replies: [{ memberId: 'andrei', text: 'Un clasic absolut.', date: '5 zile' }] },
  { memberId: 'dragos', rating: 3, date: 'acum 1 săptămână', likes: 33, text: 'Lupte spectaculoase, dar pacing-ul din partea a doua m-a pierdut puțin. Rămâne solid, totuși.', replies: [] }
];

/* ---- Forum ----
   The /comunitate Forum tab is now real (persisted threads/replies via
   /api/community/forum). The old THREADS/ONLINE_MEMBERS/FORUM_STATS demo
   arrays were removed with that rewrite (Phase 8.15). FORUM_CATS stays as the
   shared category list. */
export const FORUM_CATS = ['Toate', 'Discuții', 'Recomandări', 'Teorii', 'Meta', 'Ajutor'];

/* ---- Requests (Cereri) ---- */
export type RequestStatus = 'in-lucru' | 'aprobat' | 'in-asteptare' | 'respins';

export interface RequestSeed {
  id: number;
  title: string;
  type: 'Anime' | 'Manga';
  note: string;
  votes: number;
  status: RequestStatus;
}

export const REQUEST_SEEDS: RequestSeed[] = [
  { id: 1, title: 'Vinland Saga: Sezonul 3', type: 'Anime', note: 'Continuarea poveștii lui Thorfinn.', votes: 342, status: 'in-lucru' },
  { id: 2, title: 'Vagabond', type: 'Manga', note: 'Capodopera lui Takehiko Inoue, fără sub RO complet.', votes: 257, status: 'aprobat' },
  { id: 3, title: 'Monster', type: 'Anime', note: 'Thriller psihologic clasic de Naoki Urasawa.', votes: 188, status: 'in-asteptare' },
  { id: 4, title: 'Berserk (2016) — recolorat', type: 'Manga', note: 'Capitolele color, HD.', votes: 141, status: 'in-asteptare' },
  { id: 5, title: 'Kaiji: Ultimate Survivor', type: 'Anime', note: '', votes: 97, status: 'in-asteptare' },
  { id: 6, title: 'Made in Abyss: sezoanele noi', type: 'Anime', note: '', votes: 64, status: 'respins' }
];

export const REQUEST_STATUS_META: Record<RequestStatus, { label: string; color: string }> = {
  'in-lucru': { label: 'în lucru', color: 'var(--accent)' },
  aprobat: { label: 'aprobat', color: 'var(--success)' },
  'in-asteptare': { label: 'în așteptare', color: 'var(--text-faint)' },
  respins: { label: 'respins', color: 'var(--danger)' }
};

/* ---- Chat live ---- */
export const EMOTES: Record<string, string> = {
  Kagege: '🥷', Peko: '🐰', Copium: '😮‍💨', Sadge: '😔',
  KEKW: '😹', Based: '😎', Pain: '😩', Pog: '😲',
  Clap: '👏', Hype: '🔥', RIPBozo: '💀', Love: '💜',
  Bonk: '🔨', Weeb: '🤓', Nyaa: '😺', GG: '🤝'
};

/** Role badges shown next to a name in chat and on profiles (PLAN 8.6).
    Roles only — earned badges are a separate future system (PLAN 8.8). */
export const ROLE_BADGES: Record<string, { bg: string; glyph: string; title: string }> = {
  admin: { bg: '#C0563F', glyph: '★', title: 'Administrator' },
  moderator: { bg: '#2BA55B', glyph: '⚔', title: 'Moderator' },
  coordinator: { bg: '#E0952E', glyph: '⚑', title: 'Coordonator' },
  verifier: { bg: '#8A6DFF', glyph: '✓', title: 'Verificator' },
  translator: { bg: '#0E8C9B', glyph: '字', title: 'Traducător' }
};

/* ---- Notification type → glyph/colour (the header bell + /notificari) ----
   Real notifications come from /api/notifications; this only maps a type to
   its icon and accent for the rows that have no actor avatar. */
export const NOTIF_TYPE_META: Record<string, { icon: string; color: string }> = {
  reply: { icon: '❝', color: '#3E8FD0' },
  follow: { icon: '＋', color: 'var(--success)' },
  release: { icon: '▶', color: 'var(--accent)' },
  system: { icon: 'ℹ', color: 'var(--text-muted)' }
};

/* ---- Landing features ---- */
export const FEATURES = [
  { k: '01', title: 'Liste & colecții', text: 'Creează liste curate, urmărește-ți jurnalul și descoperă ce iubește comunitatea.' },
  { k: '02', title: 'Recenzii & scoruri', text: 'Notează, scrie recenzii și vezi opiniile sincere ale altor fani.' },
  { k: '03', title: 'Forum & chat live', text: 'Discută teorii, întreabă și socializează în timp real cu alți membri.' },
  { k: '04', title: 'Calendar de difuzare', text: 'Nu rata niciun episod — programul săptămânal, mereu la zi.' }
];
