# Anime-Kage Games — Architecture

Design notes for the arcade/games feature spanning the site and Discord.
Written 2026-08-29, against the platform as it stands that day.

---

## Decisions, up front

| Question | Decision | Because |
|---|---|---|
| New Discord bot on its own server? | **No.** Extend the existing bot. | It already runs `repo.New(pool)` — same database, same repo layer as the API. Sync is free. |
| One progression system? | **Yes.** One XP/level track, one currency (gold). | Five currencies is the single clearest lesson from PopVerse-Kingdoms. |
| Where does game state live? | **The main Postgres database**, new tables. | Anything else reintroduces the sync problem you are explicitly trying to avoid. |
| Discord RPG scope | **6 commands, 1 currency, 1 loop.** | PopVerse shipped 31 commands and 22 subsystems. That is the thing that killed it. |
| Seasonal 4x | **Keep it — as a timed event, not a standing system.** | The fun was the finale, not the daily upkeep. |

---

## 1. Where this runs

**Do not build a second bot, and do not put it on another server.**

The bot in `backend-go/cmd/discordbot/` is not a separate application. It is a
second binary in the same Go module, holding a pgx pool and calling the same
`internal/repo` package the HTTP API uses. It already reads and writes platform
data — `OutstandingInvite`, `LastInviteAt` — against the live database.

That means the hard part of "keep Discord and the site in sync" is already
solved, and solved in the strongest possible way: there is nothing to sync,
because there is one copy of the data. A user's gold is a column. Discord reads
it, the website reads it, and neither can drift from the other.

Splitting the bot onto its own host would mean inventing an API contract between
them, authenticating service-to-service, handling partial failure when one side
is up and the other is not, and reconciling two stores that will eventually
disagree. You would be building a distributed system to solve a problem you do
not currently have.

Your own constraint settles it: *"I don't mind downtime if the platform is
down."* That is the only argument for separation, and you have discarded it.

**What to build instead:** a new `internal/games` package holding the rules, and
two thin front-ends over it — HTTP handlers for the site, slash commands for the
bot. Game logic lives in neither front-end. That way a rule change lands in both
places at once, and the bot never grows its own copy of the economy.

```
internal/games/          rules, economy, state machine — no Discord, no HTTP
  economy.go             award/spend, the single ledger
  anidle.go              daily puzzle generation and scoring
  rpg.go                 adventure loop, encounters, seasons
internal/handler/games.go   HTTP  -> internal/games
cmd/discordbot/games.go     slash -> internal/games
```

---

## 2. Identity: mostly already solved

`invites` already carries the bridge:

```
invites(code, discord_user_id, discord_username, used_by_user_id, used_at)
```

Every member who joined through a Discord invite already has a durable
Discord-ID → platform-user-ID mapping. That covers the existing population.

It is not sufficient on its own, for two reasons: a user can exist on the site
without ever having used an invite, and an invite records who *issued* it as
well as who used it. So add an explicit link table rather than overloading
`invites`:

```sql
CREATE TABLE game_links (
  user_id         integer PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  discord_user_id text NOT NULL UNIQUE,
  linked_at       timestamptz NOT NULL DEFAULT now()
);
```

**`/adventure` flow.** On first use, look up `discord_user_id` in `game_links`.
If absent, look it up in `invites.discord_user_id` where `used_by_user_id IS NOT
NULL` — if found, that is a confident match, so write the link and continue with
no friction for the user. Only if both miss do you ask them to link, by showing a
short code they paste into their profile page on the site.

The point is that most of your members should never see the linking step at all.

---

## 3. One economy, and the mistake to avoid

One XP/level track and one currency is the right call. The risk is not the
number of currencies — it is the **exchange rate between activities**.

You are about to have two very different earning surfaces: a daily puzzle that
takes 30 seconds, and an RPG grind that can absorb an evening. If a daily puzzle
pays 50 gold and a boss drops 5,000, the puzzle is pointless. If the puzzle pays
500 and the boss pays 600, the RPG is pointless. Whichever way it tips, one half
of the feature dies, and it will not be obvious which until people play.

**Decide the daily ceiling before writing any reward code.** Something like: a
player doing everything available in a day earns ~1,000 gold, of which the
puzzles are ~30% and the RPG ~70%. Then price every reward as a share of that
budget rather than picking numbers per feature.

**Record every movement in one ledger.** Not a balance column that gets
incremented from six places:

```sql
CREATE TABLE game_ledger (
  id         bigserial PRIMARY KEY,
  user_id    integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  delta      integer NOT NULL,              -- gold, signed
  xp_delta   integer NOT NULL DEFAULT 0,
  reason     text    NOT NULL,              -- 'anidle_solve', 'boss_drop', 'shop_buy'
  ref        text,                          -- puzzle id, encounter id, item id
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX game_ledger_user_idx ON game_ledger (user_id, created_at DESC);
```

Balance is `sum(delta)`, cached on `users` for reads. This costs one table and
buys you the ability to answer "where did this gold come from", to undo an
exploit without a database restore, and to see which activity actually pays —
which is the number you will need when the exchange rate turns out wrong.

Levels: keep the curve boring and published. `xp_for(level) = 100 * level^1.5`
is fine. Do not tune it in secret.

---

## 4. The site games (Anidle-style)

You already own most of the content. **2,149 anime, all with posters, and 38,256
episodes.** Four modes, in the order I would build them:

### 4.1 Guess the title — build first

Source: your own `anime` table. Zero dependencies, zero cost, no rights
question. Clues reveal progressively: year → format → studio → genres →
episode count → first letter.

This is the mode that proves the whole daily-puzzle pipeline — generation,
one-guess-per-day, streaks, sharing — with no external moving parts. Get it
working end to end before adding any other mode.

### 4.2 Guess the poster (silhouette reveal) — build second

Source: `anime.image_url`, which is populated for all 2,149 rows.

**Do not pre-generate silhouette assets.** The reveal is a CSS filter over the
one image you already have, so there is nothing to store, nothing to regenerate
when a poster changes, and the whole mode costs one column you already have:

```css
/* stage 0 */ filter: brightness(0) contrast(2);
/* stage 1 */ filter: brightness(0.15) blur(12px);
/* stage 2 */ filter: brightness(0.4) blur(6px);
/* stage 3 */ filter: blur(2px);
/* solved  */ filter: none;
```

Serve the same URL at every stage; only the class changes. Note the image is
fully present in the browser at stage 0 — anyone reading the DOM can see it.
That is acceptable for a fun daily; do not use this pattern if you ever make
puzzles worth cheating for.

### 4.3 Guess the opening/ending — build third

Source: **AnimeThemes.moe**, which is the answer to "is there a place I could
import things like that". Verified working on 2026-08-29:

```
GET https://api.animethemes.moe/anime
    ?filter[has]=resources
    &filter[site]=MyAnimeList
    &filter[external_id]=<mal_id>
    &include=animethemes.animethemeentries.videos,animethemes.song
```

For One Piece (`mal_id=21`) that returns 73 themes, each with direct links like
`https://v.animethemes.moe/OnePiece-OP1-NCDVD480.webm`. They are creditless
video with audio, keyed by MAL id — which every row in your `anime` table
already has.

Two architectural notes:

- **Link, do not mirror.** Point the player's browser at `v.animethemes.moe`
  rather than caching the media on your origin. It keeps that traffic and that
  content off your server, and it keeps you current when they replace a file.
- **Harvest the metadata, not the media.** Run a slow import that stores
  `(anime_id, theme_slug, video_url)` so puzzle generation is a local query and
  never depends on their API being up at midnight.

For the guessing itself, play audio only and start at a random offset — video
gives the answer away in one frame.

### 4.4 Guess the character — build last

Source: Jikan `/anime/{id}/characters`, which returns character images.

**This one has a live dependency you have already been burned by.** As of
2026-08-29 Jikan is returning 504 across most endpoints. Build this mode against
a harvested local table, never against a live call at puzzle-generation time:

```sql
CREATE TABLE game_characters (
  mal_char_id integer PRIMARY KEY,
  anime_id    integer NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
  name        text NOT NULL,
  image_url   text NOT NULL,
  favorites   integer NOT NULL DEFAULT 0   -- Jikan's popularity, for difficulty
);
```

`favorites` is worth storing: it is the only cheap signal for whether a character
is guessable. Sort by it and you get difficulty tiers for free.

The same silhouette CSS from 4.2 applies here.

### Daily puzzle mechanics — shared by all four

```sql
CREATE TABLE game_puzzles (
  id        bigserial PRIMARY KEY,
  mode      text NOT NULL,          -- 'title' | 'poster' | 'theme' | 'character'
  play_date date NOT NULL,
  answer_id integer NOT NULL,
  payload   jsonb   NOT NULL,       -- clue order, chosen theme, audio offset
  UNIQUE (mode, play_date)
);

CREATE TABLE game_attempts (
  puzzle_id  bigint  NOT NULL REFERENCES game_puzzles(id) ON DELETE CASCADE,
  user_id    integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  guesses    jsonb   NOT NULL DEFAULT '[]',
  solved_at  timestamptz,
  PRIMARY KEY (puzzle_id, user_id)
);
```

Generate tomorrow's puzzles on a schedule, not on first request — otherwise the
first visitor after midnight pays the latency and a failed upstream call means
no puzzle at all. Store the answer server-side and validate guesses server-side;
never ship the answer to the client.

**Same puzzle in Discord.** Because the puzzle is a row, `/anidle` in Discord is
the same puzzle as the website, and an attempt made in either place is the same
row. No sync, no duplication. This is the payoff of decision 1.

---

## 5. The Discord RPG

### What PopVerse-Kingdoms taught

Reading the overview back: **22 subsystems, 31 slash commands, 5 currencies**,
with gear across 5 slots × 4 rarities × 8 stat types, a forge loop, a research
tree of 6 categories × 5 levels, land parcels, rallies, arena, prestige, and
90-day seasons. Its own documentation estimates 4–6 systems a player must learn
*before they can play at all*.

That is not a balance problem you can tune out. Nothing was individually wrong —
the total was. The correct response is not "the same but simpler"; it is a hard
budget, decided now and defended later.

### The budget

**Six commands. One currency. One loop.** If a seventh command is worth adding,
something has to go.

| Command | Does |
|---|---|
| `/adventure` | Join, link account, pick faction. Once. |
| `/me` | Level, gold, faction, badges. Read-only. |
| `/attack` | Hit the monster currently live in the spawn channel. |
| `/caut` | Timed treasure hunt on a cooldown. |
| `/guild` | Create, join, leave, list. Nothing else. |
| `/shop` | Spend gold. A short, flat list. |

### The loop

A monster spawns in a channel on a timer. People `/attack` it. Damage is rolled
from level plus a small faction modifier plus randomness. When it dies, everyone
who hit it splits gold and XP, weighted by contribution. `/caut` is the solo
filler between spawns, on its own cooldown so it cannot be spammed.

That is the entire game. It is shallow on purpose: shallow-and-alive beats
deep-and-abandoned, and you have already run the other experiment.

### Explicitly not building

Written down so it can be pointed at later: **no city building, no research
tree, no gear slots, no crafting or salvage, no hero collection or shards, no
map or territory, no troop tiers, no hospital, no PvP raids, no arena ladder, no
rally coordination, no premium currency, no land parcels, no prestige track.**

Every one of those was in PopVerse. Together they are why it stopped being fun.

### A population note

**You have 84 users.** That is the number the design has to work at, not the
number you hope for.

Guilds need enough people that a guild feels populated — with 84 accounts, of
whom some fraction play, three guilds of four people will feel empty and
competitive tables will be decided by who logged in. Consider launching with
factions only (which work at any size, because a faction is an identity rather
than a roster) and holding guilds until the population supports them.

The monster loop is the right shape for a small server: it is fun with three
people and still fun with fifty.

---

## 6. The seasonal 4x — as an event

Keep the finale. Drop the empire.

Make it a **scheduled event with a fixed end time**, not a standing system with
daily upkeep. Something like: on the last weekend of a season, factions fight
over a small board. Your accumulated level and gold set your starting strength.
The whole thing resolves in one evening, ranks are recorded, and the board is
gone until next season.

This preserves what was actually good about PopVerse — a competitive finish with
stakes — while removing what made it a chore, which was that the 4x layer
demanded attention every single day to stay competitive.

Design rule: **a player who shows up only for the event weekend should be able
to participate.** The moment the finale requires three months of daily
optimisation, you have rebuilt the thing you abandoned.

---

## 7. Placement on the site

A new top-level **Arcade** tab, with:

- **Today** — the four daily puzzles, their solved state, current streak.
- **Character sheet** — level, XP bar, gold, faction, badge case. Same data the
  bot's `/me` prints, because it is the same row.
- **Leaderboards** — daily and seasonal. Keep them short; long boards demotivate.

Badges belong on the existing profile page rather than a new one, and the chat
badge should be an opt-in showcase of one or two, not everything a user owns —
a wall of icons beside every message will make chat unreadable. There is already
precedent for this in the emote work: chat is dense, and things added to every
message need to be small and few.

---

## 8. Build order

Each phase should be shippable and enjoyable on its own. Do not start a phase
before the previous one has real users.

1. **Economy + identity.** `game_links`, `game_ledger`, level curve, `/me` and
   the character sheet. No games yet — just a number that exists in both places.
   This proves the architecture with almost no game design risk.
2. **Guess the title.** The full daily pipeline against your own data.
3. **Poster silhouette.** Same pipeline, second mode, no new dependencies.
4. **Discord monster loop.** `/adventure`, `/attack`, `/caut`, `/shop`.
5. **Themes (AnimeThemes) and characters (Jikan harvest).** Both need an
   importer first; neither should block phases 1–4.
6. **Factions and badges** once there is something to be a faction about.
7. **Guilds** — only if the population supports it.
8. **Seasonal 4x event**, at the end of the first real season.

---

## 9. Risks worth naming

**The exchange rate between puzzle and RPG earnings.** Discussed in §3. This is
the most likely thing to quietly ruin the feature, and the ledger is what lets
you detect and correct it.

**Scope creep back toward PopVerse.** The six-command budget only works if it is
treated as a constraint rather than a starting point. The "explicitly not
building" list exists to be re-read.

**External content dependencies.** AnimeThemes and Jikan are both third parties.
Jikan is 504ing broadly as of today. Harvest to local tables and generate
puzzles from local data; a puzzle mode should degrade to "not available today"
and never take the page down.

**Anti-cheat, mildly.** Answers stay server-side, guesses are validated
server-side, and rate-limit the guess endpoint. The poster silhouette is
inspectable in the DOM by design — fine for a daily, not fine for anything with
real stakes.

**Population.** 84 users. Design for the server you have; the monster loop and
factions work at that size, and guilds and ladders mostly do not.

---

# Part II — The seasonal finale, and other Discord games

Added 2026-08-29, after working through the "no troops" question.

## 10. The finale: nodes, not armies

### The insight

In PopVerse the thing you committed to a fight was **troops** — a stack you had
spent days building. Remove troops and it looks like the game loses its
substance. It does not. What it loses is the *upkeep*. The thing players commit
instead is **presence**: an action, on a cooldown, at a moment when it matters.

That is a better fit for a finale, because presence cannot be stockpiled by
whoever grinded hardest in week three. Everyone who turns up on the night has
roughly the same ammunition, and the contest becomes *who organised, who showed
up, and where they chose to push*. It also means a player who missed the whole
season can still take part — the design rule from §6.

### The rules, in full

Seven nodes. One evening. Three hours.

**Node state** is two fields: who owns it, and how firmly.

```
owner_faction   text or null      -- null = neutral
control         integer 0..100    -- 0 means it falls
```

**One action.** `/siege <node>`. What it does depends on who owns it:

| Node is | Your `/siege` does |
|---|---|
| Neutral | You claim it. `owner = you`, `control = 20`. |
| Someone else's | `control -= 20`. At 0 it goes neutral — it does not flip straight to you. |
| Yours | `control += 20`, capped at 100. This is "repair". |

Requiring a neutral step before a capture is what stops a node ping-ponging
between two factions on alternating actions, and it gives the losing side a
window to contest.

**Cooldown: 15 minutes per player.** Over a three-hour event that is 12 actions
each. This is the entire resource economy of the finale.

**Scoring: every 5 minutes, each node pays its holder.** Thirty-six ticks. Points
go to the faction, not the player. Highest total when the clock stops wins — so
*holding early* is worth as much as holding at the end, and there is no
last-minute snipe that erases three hours of work.

**The anti-steamroll rule: a node absorbs at most 5 actions per 5-minute tick.**
Beyond that, `/siege` on that node is refused with "the front is full — try
elsewhere".

This one line is what makes faction size matter less than faction *coordination*.
A faction with thirty players cannot dogpile a single node; it has to split
across the map, and splitting requires someone to organise it. A faction of six
that all push the same node at the same time can take it from a faction of
twenty that spread themselves thin. Given you have 84 users and factions will
not be evenly sized, you need something like this or the biggest faction simply
wins.

### Making structures feel different

This is where the "structures" character from PopVerse comes back, at a cost of
one column. Node type changes what holding it *does*:

| Type | Count | Effect while held |
|---|---|---|
| **Shrine** | 3 | 10 points per tick. Plain, and the bulk of the score. |
| **Forge** | 1 | Your faction's `/siege` becomes 30 instead of 20, everywhere. |
| **Beacon** | 1 | 20 points per tick, but caps at `control = 60` — valuable and fragile. |
| **Vault** | 1 | 0 points per tick. Pays a large one-off gold prize to whoever holds it *at the final whistle*. |
| **Gate** | 1 | 5 points per tick, and while you hold it your cooldown is 10 minutes instead of 15. |

Now there are real decisions with no new systems: the Forge is a force
multiplier worth taking first, the Vault is worthless until the last ten minutes
and then it is the only thing that matters, the Beacon pays double but can never
be made safe, and the Gate buys you tempo rather than points.

Five node types, one command, two state fields.

### Why this stays simple

Everything a player must understand fits in a Discord embed: seven nodes, one
command, fifteen-minute cooldown, hold things to score. Compare that with
PopVerse's 4–6 systems before you could play at all.

The bot posts a live board that edits itself every tick — node, holder, control
bar, current score. That single message is the entire UI.

```
⛩ Shrine A   ▓▓▓▓▓▓░░░░  60   Konoha
⛩ Shrine B   ▓▓▓▓▓▓▓▓▓▓ 100   Konoha
⛩ Shrine C   ░░░░░░░░░░   —   neutral
🔨 Forge      ▓▓▓▓░░░░░░  40   Mugiwara
📡 Beacon     ▓▓▓▓▓▓░░░░  60   Mugiwara   (max 60)
🏦 Vault      ▓▓░░░░░░░░  20   Konoha     (pays at the end)
🚪 Gate       ░░░░░░░░░░   —   neutral

Konoha 1,240  ·  Mugiwara 1,115  ·  Soul Society 890
next tick 2m14s
```

### Tables

```sql
CREATE TABLE game_seasons (
  id         serial PRIMARY KEY,
  name       text NOT NULL,
  starts_at  timestamptz NOT NULL,
  ends_at    timestamptz NOT NULL
);

CREATE TABLE game_nodes (
  id            serial PRIMARY KEY,
  season_id     integer NOT NULL REFERENCES game_seasons(id) ON DELETE CASCADE,
  name          text NOT NULL,
  kind          text NOT NULL,        -- shrine | forge | beacon | vault | gate
  owner_faction text,
  control       integer NOT NULL DEFAULT 0
);

CREATE TABLE game_sieges (               -- also the cooldown source of truth
  id         bigserial PRIMARY KEY,
  node_id    integer NOT NULL REFERENCES game_nodes(id) ON DELETE CASCADE,
  user_id    integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  faction    text NOT NULL,
  amount     integer NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX game_sieges_cooldown_idx ON game_sieges (user_id, created_at DESC);
CREATE INDEX game_sieges_tickcap_idx  ON game_sieges (node_id, created_at DESC);

CREATE TABLE game_faction_scores (
  season_id integer NOT NULL REFERENCES game_seasons(id) ON DELETE CASCADE,
  faction   text NOT NULL,
  points    bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (season_id, faction)
);
```

Both game rules — the player cooldown and the per-node tick cap — are queries
over `game_sieges`, so there is no separate cooldown state to get out of sync,
and the whole event is replayable from the log if something goes wrong.

---

## 11. Other Discord games

### The principle that decides what belongs where

**The site is asynchronous, solo and permanent. Discord is live, social and
fast.** A game belongs on Discord if it is *better because other people are
there right now*. If it works alone at 3am, it belongs on the site.

That single test keeps the two halves from becoming duplicates of each other,
and it stops the bot turning back into PopVerse.

### 11.1 Theme quiz — fastest finger

The bot drops a short audio clip in a channel; first correct answer in chat
wins. Runs a few times a day on a schedule, or on `/quiz`.

**Build this first.** It reuses the AnimeThemes harvest from §4.3 — the same
table, the same clips, zero new content pipeline — but it is a completely
different experience from the site's daily version: live, competitive, and over
in fifteen seconds. It is the clearest example of the principle above.

Answer matching wants to be forgiving: strip case, punctuation and articles, and
accept the romaji and English titles you already store in `anime.title` and
`anime.title_english`.

### 11.2 Prediction market

`/bet <event> <option> <amount>`. Payout split proportionally among winners.

Worth building because **the economy needs a sink**. Almost everything else here
creates gold; without somewhere for it to go, gold inflates until it is
meaningless and every reward stops mattering. Betting destroys the losers' stake
and recycles it, which drains the pool without you having to price anything.

The things to bet on are already lying around: which faction takes the Vault,
whether anyone solves tomorrow's puzzle in under three guesses, next season's
winner. It costs one table and no content.

### 11.3 Daily check-in with a streak

`/daily`. Small gold, larger on a streak, streak resets when you miss.

Trivially simple and the single most reliable retention mechanic in this whole
document. One command, one row per user, no content pipeline. Cap the streak
bonus so a two-year streak does not out-earn actually playing.

### 11.4 Character quick-draw

Same as 11.1 but with a silhouette from the `game_characters` table, revealed in
stages while people guess in chat. Reuses the §4.4 harvest.

Build it only after the theme quiz proves the format — it is the same mechanic
with a different medium, so if the first one is not fun, this will not be either.

### 11.5 Gifting

`/gift @user <amount>`. Pure transfer, no fee.

Not a game, but it is what makes the currency feel like a currency rather than a
score, and it lets veterans hand newcomers a leg up without you designing a
tutorial reward. Every movement already goes through `game_ledger`, so this is
two inserts and a balance check. Rate-limit it, and never let the balance go
negative.

### Deliberately rejected

- **Gacha / character collection.** It is the single biggest complexity magnet in
  this space and it was one of PopVerse's 22 systems. If it ever happens, it is
  its own project.
- **Voice-presence rewards.** Rewards being idle rather than playing, and the
  bot needs new gateway intents to see it.
- **Anything with a second currency.** §3.

### Suggested order

Theme quiz → `/daily` → gifting → prediction market → character quick-draw.

Quiz first because it reuses content you will already have harvested and proves
the live-channel format. `/daily` next because it is an afternoon of work and
does more for retention than anything else on the list. The market comes after
there is enough gold in circulation for a sink to be worth building.

---

# Part III — Classes and character cards

Added 2026-08-29.

## 12. Character cards

### Why this one earns its place

Part I rejected gacha outright. This proposal is different in the way that
matters: **it reuses content the platform already needs.** The
`game_characters` table exists for the §4.4 guess-the-character puzzle. Cards
are a second feature over the same import, and `favorites` — verified present on
all 1,477 One Piece entries, ranging 0 to 149,217 — is already the rarity signal.

One import, two features, no new content pipeline. That is the opposite of the
PopVerse pattern, where each system dragged in its own assets and rules.

### The flaw to fix first

> *"it will give you some xp and gold based on the rarity"*

**A card that pays gold is not a gold sink — it is a refund.** Follow the loop:
spend gold on a pull, receive gold from the card. Only the difference matters.

- If a card returns more than the pull costs, it is a money printer and the
  economy inflates until gold is meaningless.
- If it returns less, it is a sink with extra steps — and one that reads badly,
  because players feel the loss twice: once paying, once being handed a
  consolation payout smaller than what they spent.

Either way the gold leg does nothing except make the arithmetic opaque. If you
want a pull to cost 400 net, price it at 400 and pay no gold back.

**Cards should pay XP and collection value, never gold.** XP cannot be spent, so
it cannot loop. That makes pulls a clean sink — which is exactly the job you
wanted them to do.

### The design

**One free pull a day, extra pulls cost gold.** The free pull is the retention
hook you were after; the paid pulls are the sink.

| Rarity | Share of pool | XP on first copy | Duplicate |
|---|---|---|---|
| Legendary | top 1% by favorites | 500 | 100 XP |
| Epic | next 4% | 200 | 40 XP |
| Rare | next 15% | 60 | 15 XP |
| Common | remaining 80% | 20 | 5 XP |

Duplicates convert to XP automatically. No shards, no crafting currency, no
second economy — Part I §3 still applies.

### Rarity must be percentile, not threshold

The `favorites` range is 0 to 149,217 and the distribution is violently skewed:
a handful of shounen leads hold enormous counts and the long tail sits near
zero. A fixed cutoff ("over 10,000 is Legendary") would make the same forty
characters legendary forever and everything else Common.

**Bucket by percentile across the whole harvested set**, recomputed whenever you
re-import. That guarantees the tiers stay populated regardless of how the raw
numbers move.

### Cap the pool, or it caps you

One Piece alone returns **1,477 characters**. Across 2,149 anime a naive import
is hundreds of thousands of rows, most of them one-scene extras nobody could
name — which makes both the cards and the guessing puzzle worse, not better.

Filter at import time using the `role` field Jikan already returns:

- take every `Main` character
- take `Supporting` only above a favorites floor
- ignore the rest

A pool in the low tens of thousands is plenty, and every card in it is someone a
player might actually recognise.

Store the image URL rather than mirroring the files, exactly as `anime.image_url`
already does for posters.

### Trading, at your size

Player-to-player trading is the part to be careful with. **You have 84 users.**
A market that thin has almost no liquidity: few trades, erratic prices, and a
handful of active traders setting the value of everything. It also adds a
dispute and scam surface that you will personally end up moderating.

Trading also moves gold rather than destroying it — so it is economy-neutral,
and does not help the sink problem it is being proposed to solve.

**Start with a bot-run shop instead.** It buys and sells at prices you set,
which gives you a working price floor and ceiling, a real sink, and no
counterparty risk:

- **Sell to the shop**: any card, small fixed gold by rarity. A floor under
  every card.
- **Buy from the shop**: a rotating handful of specific cards at a high price.
  Guaranteed acquisition for someone chasing one character, and a large sink.

Add player-to-player trading later, when there are enough active players for
prices to mean something. When you do, make the bot hold both sides and swap
atomically — never "you go first".

### Tables

```sql
ALTER TABLE game_characters ADD COLUMN rarity text;   -- recomputed on import

CREATE TABLE game_cards (
  user_id     integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  mal_char_id integer NOT NULL REFERENCES game_characters(mal_char_id) ON DELETE CASCADE,
  copies      integer NOT NULL DEFAULT 1,
  first_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, mal_char_id)
);
```

Every pull and every shop transaction goes through `game_ledger` like everything
else, so the card economy is visible in the same place as the rest.

---

## 13. Classes

### Cheap if they touch one number each

Classes are affordable in a way most of PopVerse's systems were not, on one
condition: **a class is exactly one modifier, and it never interacts with
another class.** The moment classes have skills, or counter each other, or
combine with faction bonuses into a matrix, you are balancing a game instead of
running one.

One number each, tied to a mechanic that already exists:

| Class | Effect |
|---|---|
| A | `/siege` cooldown 12 minutes instead of 15 |
| B | +1 guess on the daily puzzles |
| C | +15% XP from all sources |
| D | Free daily card pull becomes two |
| E | +5 seconds of clip before the theme quiz opens to everyone |

Five classes, five numbers, no interactions. A player can understand the whole
system from one embed, and you can rebalance any of it by changing a constant.

### On elf / warrior / mage

Worth a second thought — not mechanically, tonally.

Your factions are anime series: Naruto, One Piece. Those are on-theme, instantly
legible, and something people already have an opinion about. Grafting generic
high-fantasy classes on top gives the game two unrelated identity systems, one
of which has nothing to do with anime. "I am Konoha, and also an elf" does not
mean anything.

Anime-native archetypes cost nothing extra and fit the rest of the platform —
something along the lines of Protagonist, Strategist, Support, Rival, Collector.
Those also map naturally onto the effects above: the Strategist gets the extra
puzzle guess, the Collector gets the extra pull, the Rival gets the siege
cooldown.

Mechanically identical, and it stops the game feeling like two games wearing one
coat.

### Faction and class must stay orthogonal

Faction should mean **who you fight for** — it drives the finale and nothing
else. Class should mean **how you play** — it tweaks your own numbers and never
touches the finale.

Keep that line and the combined system stays understandable at five factions ×
five classes. Cross them — a faction that boosts a class, a class that only works
for one faction — and you have twenty-five cases to balance and explain, which
is precisely how PopVerse got to 22 systems.

### Where this goes in the build order

Classes are a Phase 6 item, alongside factions — after the economy, the daily
puzzles and the monster loop are all live. They are a modifier on existing
mechanics, so there has to be something to modify first.

Cards are Phase 5b, immediately after the character harvest lands for the
guessing puzzle, since the import is shared.
