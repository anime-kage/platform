#!/usr/bin/env python3
"""Harvest characters from AniList into game_characters.

    DATABASE_URL=... python3 import-characters-anilist.py --top 100

The companion to import-characters.py, which reads Jikan. Jikan is a scraper of
MyAnimeList, and MAL currently refuses the sub-pages it needs: /anime/{id} and
/seasons/now answer fine while /anime/{id}/characters and /anime/{id}/episodes
fail on every series tried, cached ones excepted. AniList is a real API, so it
is unaffected. Series already harvested from Jikan are left alone; this fills in
the rest of the top N.

Two hazards make mixing the sources non-obvious, and both are handled here.

IDs only *partly* agree. AniList seeded its character database from MAL long
ago, so legacy characters share an id -- Ichigo is 5 in both, Aizen 1086 -- but
the two have drifted since: Frieren is 184947 on MAL and 176754 on AniList,
Fern 188176 against 183965. Deduplicating on the id would therefore work for
old characters and fail silently for new ones, which is worse than not trying,
because the pending list is mostly sequels of series already imported (Frieren
S2, the Bleach arcs, Vinland S2). We deduplicate on the NAME instead.

Name order differs. MAL writes "Kurosaki, Ichigo"; AniList writes "Ichigo
Kurosaki". Left alone the pool would read half one way and half the other, so
existing rows are rewritten to given-name-first on every run (idempotent: a
name without a comma is already in that form). Safe for gameplay because a
character round is answered by picking from autocomplete and submits ids, never
typed text -- renaming changes only what is displayed.
"""
import json, os, re, sys, time, urllib.error, urllib.request
import psycopg2

API = "https://graphql.anilist.co"
BUFFER = float(os.getenv("ANILIST_BUFFER", "2.0"))   # AniList allows 90/min
SUPPORT_MIN_FAVS = 200            # same bar as the Jikan import; measured to
                                  # keep 9-25 per series there, 17-51 here
PER_PAGE = 25
MAX_PAGES = 5                     # 125 characters is far past the useful tail

QUERY = """
query ($mal: Int, $page: Int, $per: Int) {
  Media(idMal: $mal, type: ANIME) {
    characters(sort: FAVOURITES_DESC, page: $page, perPage: $per) {
      pageInfo { hasNextPage }
      edges { role node { id favourites name { full } image { large } } }
    }
  }
}
"""


def norm_key(name: str) -> str:
    """Order-insensitive identity for a person.

    "Kurosaki, Ichigo" and "Ichigo Kurosaki" must collide, so the tokens are
    lowercased, stripped of punctuation and sorted.
    """
    toks = [t for t in re.split(r"[^0-9a-z]+", name.lower()) if t]
    return " ".join(sorted(toks))


def given_first(name: str) -> str:
    """"Kurosaki, Ichigo" -> "Ichigo Kurosaki". Anything else is returned as is.

    One exception: a family name carrying an initial, as the One Piece cast
    does, keeps its order. "Monkey D., Luffy" is "Monkey D. Luffy" to everyone
    who watches it, never "Luffy Monkey D.".
    """
    if "," not in name:
        return name.strip()
    surname, _, rest = name.partition(",")
    rest, surname = rest.strip(), surname.strip()
    if not rest:
        return surname
    if re.search(r"\b[A-Z]\.$", surname):
        return f"{surname} {rest}"
    return f"{rest} {surname}"


def gql(mal_id: int, page: int, tries: int = 4):
    payload = json.dumps({"query": QUERY,
                          "variables": {"mal": mal_id, "page": page, "per": PER_PAGE}}).encode()
    last = None
    for n in range(tries):
        req = urllib.request.Request(API, data=payload, headers={
            "Content-Type": "application/json", "Accept": "application/json",
            "User-Agent": "Anime-Kage/1.0 (character import)"})
        try:
            with urllib.request.urlopen(req, timeout=30) as r:
                return json.loads(r.read())
        except urllib.error.HTTPError as e:
            # 429 carries Retry-After; respect it rather than guessing.
            if e.code == 429:
                wait = int(e.headers.get("Retry-After", "60"))
                print(f"    rate limited, sleeping {wait}s")
                time.sleep(wait + 1)
                last = e
                continue
            last = e
        except Exception as e:  # noqa: BLE001
            last = e
        if n < tries - 1:
            time.sleep(BUFFER * (n + 2))
    raise last if last else RuntimeError("unreachable")


def chars_for(mal_id: int):
    """Every Main, plus Supporting the audience actually knows."""
    out, page = [], 1
    while page <= MAX_PAGES:
        d = gql(mal_id, page)
        media = (d.get("data") or {}).get("Media")
        if not media:
            errs = d.get("errors") or []
            raise RuntimeError(errs[0]["message"] if errs else "no Media in response")
        block = media["characters"]
        kept_here = 0
        for e in block["edges"]:
            node, role = e["node"], (e.get("role") or "").upper()
            favs = node.get("favourites") or 0
            img = (node.get("image") or {}).get("large")
            name = ((node.get("name") or {}).get("full") or "").strip()
            if not (node.get("id") and name and img):
                continue
            if role != "MAIN" and favs < SUPPORT_MIN_FAVS:
                continue
            out.append((node["id"], given_first(name), img, favs))
            kept_here += 1
        # Sorted by favourites descending, so once a whole page yields nothing
        # the tail is all obscure supports.
        if not block["pageInfo"]["hasNextPage"] or kept_here == 0:
            break
        page += 1
        time.sleep(BUFFER)
    return out


def main() -> int:
    dsn = os.getenv("DATABASE_URL")
    if not dsn:
        print("DATABASE_URL is required", file=sys.stderr)
        return 2
    top = 100
    args = sys.argv[1:]
    if "--top" in args:
        top = int(args[args.index("--top") + 1])

    conn = psycopg2.connect(dsn)
    conn.autocommit = True

    with conn.cursor() as c:
        # One pass to make the existing MAL-format names given-name-first, so
        # the pool reads consistently once the two sources are mixed. Done in
        # Python rather than SQL so given_first() is the only implementation --
        # the initial-surname rule would otherwise have to exist twice and the
        # two would drift.
        c.execute("SELECT mal_char_id, name FROM game_characters")
        rows = c.fetchall()
        renames = [(given_first(n), cid) for cid, n in rows if given_first(n) != n]
        if renames:
            c.executemany("UPDATE game_characters SET name = %s WHERE mal_char_id = %s", renames)
            print(f"normalised {len(renames)} existing name(s) to given-name-first")
            rows = [(cid, given_first(n)) for cid, n in rows]
        seen_key = {norm_key(n): cid for cid, n in rows}
        seen_id = {cid: norm_key(n) for cid, n in rows}

        # The top N by score, minus whatever Jikan already covered.
        c.execute("""SELECT t.id, t.mal_id, t.title FROM (
                       SELECT id, mal_id, title FROM anime
                        WHERE mal_id IS NOT NULL AND score IS NOT NULL
                        ORDER BY score DESC NULLS LAST LIMIT %s) t
                      WHERE NOT EXISTS (
                        SELECT 1 FROM game_characters g WHERE g.anime_id = t.id)""", (top,))
        targets = c.fetchall()

    print(f"{len(targets)} series to fill in (top {top}), {BUFFER}s apart, "
          f"{len(seen_key)} characters already held")
    added = skipped_dupe = skipped_id = failed = 0

    for anime_id, mal_id, title in targets:
        try:
            found = chars_for(mal_id)
        except Exception as e:  # noqa: BLE001 - one bad series must not stop the run
            print(f"  !! {title[:38]}: {type(e).__name__}: {e}")
            failed += 1
            time.sleep(BUFFER)
            continue

        rows_to_write = []
        for cid, name, img, favs in found:
            key = norm_key(name)
            if key in seen_key:
                skipped_dupe += 1          # already here, from either source
                continue
            if cid in seen_id:
                # An AniList id that happens to equal a different character's
                # MAL id. Writing it would overwrite that row's picture.
                print(f"     id {cid} already belongs to someone else, skipping {name}")
                skipped_id += 1
                continue
            rows_to_write.append((cid, anime_id, name, img, favs))
            seen_key[key] = cid
            seen_id[cid] = key

        if rows_to_write:
            with conn.cursor() as c:
                c.executemany("""
                    INSERT INTO game_characters (mal_char_id, anime_id, name, image_url, favorites)
                    VALUES (%s, %s, %s, %s, %s)
                    ON CONFLICT (mal_char_id) DO NOTHING""", rows_to_write)
            added += len(rows_to_write)
        print(f"  ok {title[:38]:40s} +{len(rows_to_write)}")
        time.sleep(BUFFER)

    print(f"done — {added} added, {skipped_dupe} already known, "
          f"{skipped_id} id clashes, {failed} errored")
    return 0


sys.exit(main())
