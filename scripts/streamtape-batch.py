#!/usr/bin/env python3
"""Run the Streamtape upload across many series, one after another.

Reads the series list produced from the download progress (serii_90.json) and
invokes streamtape-upload.py per series, pausing between them. Each series keeps
its own state file, so this is resumable at both levels: an interrupted batch
re-runs only the series it had not finished, and an interrupted series re-uploads
only the episodes it had not sent.

    streamtape-batch.py --list /home/alexlv/bot/serii_90.json --staging
    streamtape-batch.py --list ... --staging --apply --limit-series 5
"""
import argparse, json, os, random, subprocess, sys, time
from datetime import datetime

HERE = os.path.dirname(os.path.abspath(__file__))
# Which per-series uploader to drive. One runner for both hosts, so pacing,
# resume and failure handling live in a single place rather than being copied
# and drifting apart.
UPLOADERS = {
    'streamtape': os.path.join(HERE, 'streamtape-upload.py'),
    'mp4upload':  os.path.join(HERE, 'mp4upload-upload.py'),
}


def log(msg):
    # Flushed, because a batch this long is watched through its log file and a
    # buffered one stays empty for hours.
    print(f"[{datetime.now():%H:%M:%S}] {msg}", flush=True)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--list', required=True, help='JSON list of series to process')
    ap.add_argument('--host', choices=sorted(UPLOADERS), default='streamtape',
                    help='which uploader to run')
    ap.add_argument('--state', default=None,
                    help='progress file; defaults to one per host')
    ap.add_argument('--series-pause', type=float, default=10.0,
                    help='base seconds between series')
    ap.add_argument('--series-jitter', type=float, default=5.0)
    ap.add_argument('--limit-series', type=int, default=0, help='stop after N series')
    # Zero by default. Waiting inline for Streamtape's encoder costs up to ten
    # minutes per series, which across 521 series dwarfs the uploads, and it
    # does not even make the check trustworthy: a file that has not started
    # converting reports its raw size and scores ~100%, so it passes whether or
    # not it later truncates. Upload fast, then verify everything in one sweep
    # once the queue has drained (see --sweep).
    ap.add_argument('--convert-wait', type=int, default=0,
                    help='seconds each series waits for conversions (0 = do not wait)')
    ap.add_argument('--retry-suspect', action='store_true',
                    help='remux and re-upload files the verify pass flags')
    ap.add_argument('--sweep', action='store_true',
                    help='skip uploading; re-verify and register every series already done')
    ap.add_argument('--apply', action='store_true')
    ap.add_argument('--staging', action='store_true')
    a = ap.parse_args()

    uploader = UPLOADERS[a.host]
    if not a.state:
        a.state = f'/home/alexlv/bot/state/{a.host}_batch.json'
    series = [s['slug'] for s in json.load(open(a.list))]
    state = json.load(open(a.state)) if os.path.exists(a.state) else {'done': [], 'failed': {}}
    todo = series if a.sweep else [s for s in series if s not in state['done']]
    if a.limit_series:
        todo = todo[:a.limit_series]

    log(f"host: {a.host} | serii in lista: {len(series)} | ramase: {len(todo)} | "
        f"tinta: {'STAGING' if a.staging else 'PRODUCTIE'} | scriere DB: {'DA' if a.apply else 'NU'}")

    for n, slug in enumerate(todo, 1):
        cmd = [sys.executable, '-u', uploader, '--slug', slug]
        # Only Streamtape has an encoder queue to wait on; mp4upload's uploader
        # has no such flag and argparse rejects the whole invocation if it is
        # passed anyway.
        if a.host == 'streamtape':
            cmd += ['--convert-wait', str(a.convert_wait)]
        if a.sweep:   cmd.append('--register-only')
        if a.retry_suspect: cmd.append('--retry-suspect')
        if a.staging: cmd.append('--staging')
        if a.apply:   cmd.append('--apply')

        log(f"[{n}/{len(todo)}] {slug} ...")
        r = subprocess.run(cmd, capture_output=True, text=True)
        out = (r.stdout or r.stderr).strip().splitlines()
        # Always surface the findings, whatever else the series printed. The
        # last four lines are the SQL tags, and the SUSPECTE block prints
        # before them, so a tail-only log reported "0 suspects" across a sweep
        # that had actually flagged 188 files.
        flagged = [l for l in out if 'SUSPECT' in l or 'din marimea sursei' in l
                   or 'reincarcat' in l or 'remux esuat' in l]
        for line in flagged + out[-4:]:
            log(f"    {line.strip()}")

        if r.returncode == 0:
            state['done'].append(slug)
            state['failed'].pop(slug, None)
        else:
            # Record and carry on. One series with no files on the box, or a
            # transient API error, must not end a run of hundreds.
            state['failed'][slug] = (r.stderr or r.stdout).strip()[-300:]
            log(f"    ESUAT (cod {r.returncode}), continui")
        json.dump(state, open(a.state, 'w'), indent=1)

        if n < len(todo):
            time.sleep(a.series_pause + random.uniform(0, a.series_jitter))

    log(f"gata. reusite: {len(state['done'])} | esuate: {len(state['failed'])}")
    if state['failed']:
        for slug in list(state['failed'])[:10]:
            log(f"  esuat: {slug}")


if __name__ == '__main__':
    main()
