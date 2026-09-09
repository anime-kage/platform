#!/usr/bin/env python3
"""Attach a series' own vtbe uploads as both an AK-player and an iframe source.

Reads file codes one per line, in episode order (line 1 = episode 1). For each
it writes two content_links rows, matching the shape already in the table:

    extract  provider=vtbe  provider_ref=https://vtbe.to/<code>.html   priority 1000000
    embed                   hosting_url =https://vtbe.to/embed-<code>.html  priority 10001

Any existing active vtbe row for that episode is deactivated first, so a re-run
replaces rather than accumulates. Everything happens in one transaction.

Usage:
    add-vtbe-sources.py --slug naruto --codes codes.txt [--apply]

Without --apply it only reports what it would do. Run it that way first.
"""
import argparse, os, re, subprocess, sys

CODE_RE = re.compile(r'^[A-Za-z0-9]{8,24}$')

def psql(sql, env_path, compose, service):
    """Run SQL in the prod postgres container and return stdout."""
    user = db = None
    for line in open(env_path, encoding='utf-8'):
        if line.startswith('DB_USER='): user = line.split('=',1)[1].strip()
        if line.startswith('DB_NAME='): db   = line.split('=',1)[1].strip()
    cmd = ['docker','compose','--env-file',env_path,'-f',compose,'exec','-T',
           service,'psql','-U',user,'-d',db,'-v','ON_ERROR_STOP=1','-t','-A','-F','|']
    r = subprocess.run(cmd, input=sql, capture_output=True, text=True)
    if r.returncode != 0:
        sys.exit(f"psql failed:\n{r.stderr}")
    return r.stdout.strip()

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--slug', required=True)
    ap.add_argument('--codes', required=True)
    # The codes in a batch are positional, so a file covering ep501-1000 needs
    # --start 501. Without it every code lands on ep1 upward and silently
    # overwrites an earlier batch.
    ap.add_argument('--start', type=int, default=1,
                    help='episode number that the first code belongs to')
    ap.add_argument('--apply', action='store_true')
    ap.add_argument('--root', default='/home/alexlv/anime-kage/anime-kage')
    # Staging first is the point of having a staging environment. Same code
    # path, same SQL, different container -- so a rehearsal actually rehearses.
    ap.add_argument('--staging', action='store_true',
                    help='run against the staging stack instead of prod')
    a = ap.parse_args()

    if a.staging:
        env = os.path.join(a.root, '.env.staging')
        compose = os.path.join(a.root, 'docker-compose.staging.yml')
        service = 'postgres-staging'
    else:
        env = os.path.join(a.root, '.env')
        compose = os.path.join(a.root, 'docker-compose.prod.yml')
        service = 'postgres'
    print(f"  tinta         : {'STAGING' if a.staging else 'PRODUCTIE'}")

    # Validate every code before touching the database. A malformed code becomes
    # a link that looks fine in the table and fails silently for a viewer, which
    # is exactly the failure this whole script exists to avoid.
    codes, bad = [], []
    for i, line in enumerate(open(a.codes, encoding='utf-8'), 1):
        c = line.strip()
        if not c or c.startswith('#'):
            continue
        # tolerate a pasted full URL as well as a bare code
        m = re.search(r'vtbe\.to/(?:embed-)?([A-Za-z0-9]+)(?:\.html)?', c)
        if m:
            c = m.group(1)
        (codes if CODE_RE.fullmatch(c) else bad).append((i, c))
    if bad:
        for ln, c in bad[:10]:
            print(f"  linia {ln}: cod invalid -> {c!r}")
        sys.exit(f"{len(bad)} coduri invalide, nu continui")
    codes = [c for _, c in codes]

    dupes = {c for c in codes if codes.count(c) > 1}
    if dupes:
        sys.exit(f"coduri duplicate ({len(dupes)}): {sorted(dupes)[:5]} ...")

    lo, hi = a.start, a.start + len(codes) - 1
    # Check the exact episodes this run targets exist, not just that the series
    # has enough episodes in total: the codes are positional, and a gap in the
    # range would silently drop links on the floor.
    present = psql(f"SELECT count(*) FROM anime a JOIN episodes e ON e.anime_id=a.id "
                   f"WHERE a.slug='{a.slug}' AND e.episode_number BETWEEN {lo} AND {hi};",
                   env, compose, service)
    print(f"  coduri citite : {len(codes)}")
    print(f"  interval tinta: ep{lo} - ep{hi}")
    print(f"  episoade gasite in acel interval: {present} din {len(codes)}")
    if int(present) < len(codes):
        sys.exit("lipsesc episoade din intervalul tinta, opresc")

    values = ",\n".join(
        f"({n+a.start},'{c}')" for n, c in enumerate(codes))

    sql = f"""
BEGIN;
CREATE TEMP TABLE incoming(ep int, code text) ON COMMIT DROP;
INSERT INTO incoming(ep, code) VALUES
{values};

-- Retire only the pair this script itself writes (extract at 1000000, embed at
-- 10001), and only when the code changed. Matching every vtbe row for the
-- episode also switched off the catalogue's own third-party vtbe links.
UPDATE content_links cl SET is_active = false
  FROM episodes e JOIN anime a ON a.id = e.anime_id
  JOIN incoming i ON i.ep = e.episode_number
 WHERE cl.episode_id = e.id AND a.slug = '{a.slug}' AND cl.is_active
   AND cl.priority IN (1000000, 10001)
   AND cl.hosting_url LIKE 'https://vtbe.to/%'
   AND cl.hosting_url NOT IN ('https://vtbe.to/'||i.code||'.html',
                              'https://vtbe.to/embed-'||i.code||'.html');

INSERT INTO content_links (episode_id, hosting_url, language, is_active, kind, provider, provider_ref, priority)
SELECT e.id, 'https://vtbe.to/'||i.code||'.html', 'ro', true, 'extract', 'vtbe',
       'https://vtbe.to/'||i.code||'.html', 1000000
  FROM incoming i JOIN anime a ON a.slug = '{a.slug}'
  JOIN episodes e ON e.anime_id = a.id AND e.episode_number = i.ep;

INSERT INTO content_links (episode_id, hosting_url, language, is_active, kind, priority)
SELECT e.id, 'https://vtbe.to/embed-'||i.code||'.html', 'ro', true, 'embed', 10001
  FROM incoming i JOIN anime a ON a.slug = '{a.slug}'
  JOIN episodes e ON e.anime_id = a.id AND e.episode_number = i.ep;

SELECT 'extract active: '||count(*) FROM content_links cl
  JOIN episodes e ON e.id=cl.episode_id JOIN anime a ON a.id=e.anime_id
 WHERE a.slug='{a.slug}' AND cl.kind='extract' AND cl.provider='vtbe' AND cl.is_active;
{"COMMIT;" if a.apply else "ROLLBACK;"}
"""
    out = psql(sql, env, compose, service)
    print("  " + "\n  ".join(l for l in out.splitlines() if l.strip()))
    print("  APLICAT" if a.apply else "  DRY RUN (ruleaza cu --apply ca sa scrie)")

if __name__ == '__main__':
    main()
