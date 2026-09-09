#!/usr/bin/env python3
"""Fill in missing season/franchise edges from AniList.

    DATABASE_URL=... python3 import-relations-anilist.py [--all] [--limit N]

The relation graph is what the arcade's character round uses to decide that
naming any season of a franchise counts. Where a series has no edges at all the
walk cannot leave it, so every other season reads as a different show -- ONE
PIECE FAN LETTER sat unconnected to One Piece for exactly that reason.

AniList rather than Jikan because MyAnimeList has been returning 504 for the
sub-resources for days, and because AniList answers this in one query per
series.

By default only series with NO relations are fetched, which makes the run
resumable: stop it and start it again and it carries on where it left off.
--all re-reads everything, for when a franchise has grown.
"""
import json, os, sys, time, urllib.error, urllib.request
import psycopg2

API = "https://graphql.anilist.co"
BUFFER = float(os.getenv("ANILIST_BUFFER", "1.3"))   # AniList allows 90/min

# AniList's vocabulary is wider than ours. Anything not here is deliberately
# dropped: ADAPTATION and SOURCE point at the manga, CHARACTER and OTHER are not
# watch-order relationships, and storing them would widen the "same series"
# walk into things nobody would call the same show.
KEEP = {"SEQUEL", "PREQUEL", "SIDE_STORY", "PARENT", "ALTERNATIVE", "SUMMARY", "SPIN_OFF"}

QUERY = """
query ($mal: Int) {
  Media(idMal: $mal, type: ANIME) {
    relations { edges { relationType node { idMal type } } }
  }
}
"""


def fetch(mal_id, tries=4):
    payload = json.dumps({"query": QUERY, "variables": {"mal": mal_id}}).encode()
    last = None
    for n in range(tries):
        req = urllib.request.Request(API, data=payload, headers={
            "Content-Type": "application/json", "Accept": "application/json",
            "User-Agent": "Anime-Kage/1.0 (relations import)"})
        try:
            with urllib.request.urlopen(req, timeout=30) as r:
                return json.load(r)
        except urllib.error.HTTPError as e:
            if e.code == 429:
                wait = int(e.headers.get("Retry-After", "60"))
                print(f"    rate limited, sleeping {wait}s", flush=True)
                time.sleep(wait + 1)
                last = e
                continue
            if e.code == 404:
                return None
            last = e
        except Exception as e:  # noqa: BLE001
            last = e
        if n < tries - 1:
            time.sleep(BUFFER * (n + 2))
    raise last if last else RuntimeError("unreachable")


def main() -> int:
    dsn = os.getenv("DATABASE_URL")
    if not dsn:
        print("DATABASE_URL is required", file=sys.stderr)
        return 2
    args = sys.argv[1:]
    every = "--all" in args
    limit = 100000
    if "--limit" in args:
        limit = int(args[args.index("--limit") + 1])

    conn = psycopg2.connect(dsn)
    conn.autocommit = True
    with conn.cursor() as c:
        if every:
            c.execute("""SELECT id, mal_id, title FROM anime
                          WHERE mal_id IS NOT NULL ORDER BY id LIMIT %s""", (limit,))
        else:
            c.execute("""SELECT id, mal_id, title FROM anime a
                          WHERE a.mal_id IS NOT NULL
                            AND NOT EXISTS (SELECT 1 FROM anime_relations r
                                             WHERE r.anime_id = a.id)
                          ORDER BY a.id LIMIT %s""", (limit,))
        targets = c.fetchall()

    print(f"{len(targets)} series to check, {BUFFER}s apart", flush=True)
    added = touched = failed = 0
    for anime_id, mal_id, title in targets:
        try:
            payload = fetch(mal_id)
        except Exception as e:  # noqa: BLE001 - one bad series must not stop the run
            print(f"  !! {title[:40]}: {type(e).__name__}: {e}", flush=True)
            failed += 1
            time.sleep(BUFFER)
            continue
        media = (payload or {}).get("data", {}).get("Media")
        if not media:
            time.sleep(BUFFER)
            continue

        rows = []
        for edge in media["relations"]["edges"]:
            node, kind = edge["node"], edge["relationType"]
            if kind not in KEEP or node.get("type") != "ANIME" or not node.get("idMal"):
                continue
            rows.append((anime_id, kind, node["idMal"]))
        if rows:
            with conn.cursor() as c:
                c.executemany("""
                    INSERT INTO anime_relations (anime_id, relation, related_mal_id, synced_at)
                    VALUES (%s, %s, %s, now())
                    ON CONFLICT (anime_id, related_mal_id)
                    DO UPDATE SET relation = EXCLUDED.relation, synced_at = now()""", rows)
            added += len(rows)
            touched += 1
            print(f"  ok {title[:42]:44s} {len(rows)} relații", flush=True)
        time.sleep(BUFFER)

    print(f"done — {added} relations on {touched} series, {failed} errored", flush=True)
    return 0


sys.exit(main())
