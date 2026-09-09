#!/usr/bin/env python3
"""Fetch episodes listed in a CSV and file them on the Hetzner Storage Box.

The mirror image of the upload scripts: a CSV supplies one source URL per
episode, yt-dlp pulls it, and the result lands as
hetzner:anime/<slug>/<slug>_ep###.mp4 -- the naming already on the box.

Only what is missing is fetched. The box is the source of truth, so an episode
already there is skipped rather than re-downloaded, which makes the run
resumable simply by re-running it.

Every download is checked before it is filed: a file under --min-mb or shorter
than --min-minutes is discarded rather than uploaded. That guard is not
theoretical -- mp4upload answers some dead links with a 16-byte stub, and the
earlier catalogue run filed 18,946 of them before anyone noticed.

    csv-download.py --csv naruto_shippuuden_mp4upload.csv --slug naruto-shippuuden
    csv-download.py --csv ... --slug ... --limit 3        # proba
"""
import argparse, csv, json, os, random, re, subprocess, sys, time
from pathlib import Path

def ytdlp():
    for c in (Path('/home/alexlv/bot/venv/bin/yt-dlp'), Path(sys.executable).with_name('yt-dlp')):
        if c.exists():
            return str(c)
    import shutil
    found = shutil.which('yt-dlp')
    if not found:
        sys.exit("yt-dlp negasit (nici in venv, nici in PATH)")
    return found


def on_box(slug):
    r = subprocess.run(['rclone', 'lsf', f'hetzner:anime/{slug}', '--include', '*.mp4'],
                       capture_output=True, text=True)
    eps = set()
    for line in r.stdout.splitlines():
        m = re.search(r'(\d{3,4})\.mp4$', line.strip())
        if m:
            eps.add(int(m.group(1)))
    return eps


def probe_seconds(path):
    r = subprocess.run(['ffprobe', '-v', 'error', '-show_entries', 'format=duration',
                        '-of', 'default=nw=1:nk=1', path], capture_output=True, text=True)
    try:
        return float(r.stdout.strip())
    except ValueError:
        return 0.0


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--csv', required=True)
    # One of the two: --slug for a single-series CSV, --slug-col when the file
    # carries its own slug per row (a missing-episodes export spanning hundreds
    # of series). Without the latter, every download would be filed under a
    # single series' name.
    ap.add_argument('--slug', help='series slug, when the CSV covers one series')
    ap.add_argument('--slug-col', help='CSV column holding the slug per row')
    ap.add_argument('--url-col', default='Source URL')
    ap.add_argument('--ep-col', default='Episode')
    ap.add_argument('--limit', type=int, default=0)
    ap.add_argument('--min-mb', type=float, default=20.0,
                    help='discard anything smaller; catches dead-link stubs')
    ap.add_argument('--min-minutes', type=float, default=5.0,
                    help='discard anything shorter; catches truncated pulls')
    ap.add_argument('--pause', type=float, default=20.0)
    ap.add_argument('--jitter', type=float, default=10.0)
    ap.add_argument('--force', action='store_true', help='re-fetch even if the box has it')
    ap.add_argument('--work', default='/tmp/csv-download')
    ap.add_argument('--bot', default='/home/alexlv/bot')
    # Kept apart from --bot on purpose: --bot is where the .env with the
    # host credentials lives, while the resume state is bulk and belongs
    # in its own folder. Pointing --bot at the state folder would leave
    # the scripts unable to find their API keys.
    ap.add_argument('--state', default='/home/alexlv/bot/state')
    a = ap.parse_args()

    if not (a.slug or a.slug_col):
        sys.exit("da fie --slug, fie --slug-col")
    tag = a.slug or os.path.splitext(os.path.basename(a.csv))[0]
    state_path = os.path.join(a.state, f'csvdl_{tag}.json')
    state = json.load(open(state_path)) if os.path.exists(state_path) else {'done': {}, 'failed': {}}

    rows = {}
    # utf-8-sig: the exported file carries a BOM, which otherwise becomes part
    # of the first column's name and hides it from DictReader.
    with open(a.csv, newline='', encoding='utf-8-sig') as f:
        for r in csv.DictReader(f):
            ep, url = (r.get(a.ep_col) or '').strip(), (r.get(a.url_col) or '').strip()
            slug = (r.get(a.slug_col) or '').strip() if a.slug_col else a.slug
            if slug and ep.isdigit() and url.startswith('http'):
                rows[(slug, int(ep))] = url
    if not rows:
        sys.exit(f"nicio linie utila in {a.csv} (coloane {a.ep_col!r} / {a.url_col!r})")

    # --force means "ignore what the box already has", because a replacement
    # run is deliberately overwriting lower-quality files. It does NOT mean
    # "fetch everything again": episodes this script already pulled stay done,
    # or a replacement pass would re-download its own earlier work.
    # ONE recursive listing, not one per series. Listing 251 series separately
    # took over ten minutes before a single download began, and opened an SFTP
    # connection per series against a box that limits them.
    present = set()
    if not a.force:
        print("  citesc ce e deja pe box...", flush=True)
        r = subprocess.run(['rclone', 'lsf', 'hetzner:anime', '--recursive',
                            '--include', '*.mp4'], capture_output=True, text=True)
        if r.returncode != 0:
            sys.exit(f"rclone lsf a esuat: {r.stderr.strip()[:200]}")
        for line in r.stdout.splitlines():
            m = re.match(r'([^/]+)/.*?ep(\d{3,4})\.mp4$', line.strip())
            if m:
                present.add((m.group(1), int(m.group(2))))
        print(f"  pe box: {len(present)} episoade in total", flush=True)

    todo = sorted(k for k in rows
                  if k not in present and f"{k[0]}:{k[1]}" not in state['done'])
    if a.limit:
        todo = todo[:a.limit]
    print(f"  csv: {len(rows)} episoade in {len({s for s, _ in rows})} serii | "
          f"de descarcat: {len(todo)}", flush=True)

    os.makedirs(a.work, exist_ok=True)
    yt = ytdlp()
    ok = bad = 0

    for n, (slug, ep) in enumerate(todo, 1):
        key = f"{slug}:{ep}"
        name = f"{slug}_ep{ep:03d}.mp4"
        out = os.path.join(a.work, name)
        for stale in Path(a.work).glob(f"{slug}_ep{ep:03d}*"):
            stale.unlink(missing_ok=True)

        r = subprocess.run([yt, '--no-warnings', '--no-playlist', '-f', 'best',
                            '-o', out, rows[(slug, ep)]], capture_output=True, text=True)
        if r.returncode != 0 or not os.path.exists(out):
            err = (r.stderr or r.stdout).strip().splitlines()
            state['failed'][key] = (err[-1] if err else 'necunoscut')[:200]
            print(f"  [{n}/{len(todo)}] {slug[:22]:<22} ep{ep:>4}: descarcare esuata", flush=True)
            bad += 1
        else:
            mb = os.path.getsize(out) / 1048576
            mins = probe_seconds(out) / 60
            if mb < a.min_mb or mins < a.min_minutes:
                state['failed'][key] = f"prea mic/scurt: {mb:.1f} MB, {mins:.1f} min"
                print(f"  [{n}/{len(todo)}] {slug[:22]:<22} ep{ep:>4}: RESPINS ({mb:.1f} MB, {mins:.1f} min)", flush=True)
                os.remove(out)
                bad += 1
            else:
                # move, not copy: the local copy has no reason to survive, and
                # rclone deletes it only once the upload verifiably succeeded.
                m = subprocess.run(['rclone', 'move', out, f'hetzner:anime/{slug}/'],
                                   capture_output=True, text=True)
                if m.returncode != 0:
                    state['failed'][key] = f"urcare pe box esuata: {m.stderr.strip()[:150]}"
                    print(f"  [{n}/{len(todo)}] {slug[:22]:<22} ep{ep:>4}: urcare esuata", flush=True)
                    bad += 1
                else:
                    state['done'][key] = {'mb': round(mb, 1), 'min': round(mins, 1)}
                    state['failed'].pop(key, None)
                    print(f"  [{n}/{len(todo)}] {slug[:22]:<22} ep{ep:>4} -> {mb:.0f} MB, {mins:.0f} min", flush=True)
                    ok += 1

        json.dump(state, open(state_path, 'w'), indent=1)
        if n < len(todo):
            time.sleep(a.pause + random.uniform(0, a.jitter))

    print(f"  gata. reusite: {ok} | esuate: {bad}", flush=True)


if __name__ == '__main__':
    main()
