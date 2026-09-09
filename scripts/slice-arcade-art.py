#!/usr/bin/env python3
"""Cut the arcade sprites out of the contact sheet and key out their backdrop.

    venv/bin/python3 slice-arcade-art.py SHEET.png OUTDIR

The sheet is a flattened screenshot: RGB, no alpha, and each sprite sits on a
rounded chip that is a slightly different dark from the card behind it, which is
a different dark again from the page. So the backdrop is sampled per tile from
its own border rather than assumed to be one colour for the whole sheet.

Alpha is a ramp, not a threshold: pixel art has anti-aliased edges even when the
art itself is hard-edged, and a hard cut leaves a dark fringe that shows up on
the light and sakura themes.
"""
import sys, pathlib
import numpy as np
from PIL import Image

# Boxes are in the sheet's own pixels, taken from the 3054x1408 original.
# Every box is pulled in by this much before keying. The boxes were traced from
# the sheet by eye and land on the seam between the card and the chip the icon
# sits on -- two different darks. The border sample picks the chip, so the strip
# of card colour survives as a hard vertical line down the left of the sprite.
# Trimming the seam away is more reliable than trying to key two backdrops.
# Boxes sit INSIDE the chip so the border ring samples the chip colour. Widening
# them past the chip was tried and is worse: the ring then samples the card and
# the page, the chip is no longer a backdrop candidate, and every sprite comes
# out as an opaque square.
INSET = 7

# Art that reaches the chip edge, so the standard inset clips it. Values are the
# inset for that badge specifically -- smaller means more art kept, at the cost
# of a little chip seam that the multi-backdrop keying then removes.
INSET_OVERRIDE = {
    "week_poster": 0,   # the three cards fan out to the chip edge
    "level_50": 3,      # wings
    "level_100": 2,     # wings
    # Full-bleed night scene: the sky is uniform, so trimming further costs
    # nothing visually and removes the chip seam a smaller inset left behind.
    "no_hint_week": 9,
}

S = 1.5273  # displayed 2000px wide -> original 3054px
def box(x0, y0, x1, y1, name: str | None = None):
    a, b, c, d = (int(v * S) for v in (x0, y0, x1, y1))
    n = INSET_OVERRIDE.get(name or "", INSET)
    return (a + n, b + n, c - n, d - n)

TOP = {
    "coin":          box(698, 138, 918, 298),
    "chest-closed":  box(1100, 120, 1325, 298),
    "chest-open":    box(1465, 110, 1760, 298),
}
COLS = [(68, 182), (553, 667), (1038, 1152), (1523, 1637)]
ROWS = [(412, 528), (592, 708), (770, 886)]
BADGES = [
    ["one_guess", "first_try_5", "clutch", "perfect_day"],
    ["week_title", "week_poster", "perfect_week", "no_hint_week"],
    ["gold_5000", "level_25", "level_50", "level_100"],
]

TOL_NEAR = 26.0   # within this of the backdrop -> fully transparent
TOL_FAR = 62.0    # beyond this -> fully opaque; between the two, a ramp


def backdrops_of(a: np.ndarray, k: int = 3) -> np.ndarray:
    """The k most common colours in the tile's border ring.

    There is never just one backdrop. Each icon sits on a rounded chip, the chip
    sits on a card, and the card sits on the page -- three different darks. A
    single median lands on whichever dominates the ring and leaves the others
    opaque, which is what produced the hard line down the left of several
    badges. Keying against all of them at once removes the seam without having
    to hand-place a box around it.
    """
    ring = np.concatenate([
        a[0:4, :, :].reshape(-1, 3), a[-4:, :, :].reshape(-1, 3),
        a[:, 0:4, :].reshape(-1, 3), a[:, -4:, :].reshape(-1, 3),
    ])
    q = (ring // 6).astype(np.int32)                     # bucket near-identical shades
    key = q[:, 0] * 10000 + q[:, 1] * 100 + q[:, 2]
    vals, counts = np.unique(key, return_counts=True)
    top = vals[counts.argsort()[::-1][:k]]
    out = []
    for t in top:
        sel = ring[key == t]
        out.append(np.median(sel, axis=0))
    return np.array(out)


def round_corners(im: Image.Image, radius_frac: float = 0.16) -> Image.Image:
    """Soften the square on full-bleed art.

    Four of the badges are scene illustrations rather than isolated sprites --
    the climber, the eye, the wolf, the cards -- so they legitimately fill their
    tile and key out to a hard rectangle. Rounding makes that read as a designed
    frame instead of a bad cut. It is a no-op on the free-floating sprites,
    whose corners are already transparent.
    """
    from PIL import ImageDraw
    r = int(min(im.size) * radius_frac)
    mask = Image.new("L", im.size, 0)
    ImageDraw.Draw(mask).rounded_rectangle([0, 0, im.width - 1, im.height - 1], r, fill=255)
    out = im.copy()
    a = out.getchannel("A")
    out.putalpha(Image.composite(a, Image.new("L", im.size, 0), mask))
    return out


def cut(sheet: Image.Image, bbox, name: str, outdir: pathlib.Path, rounded: bool = False) -> str:
    a = np.asarray(sheet.crop(bbox).convert("RGB")).astype(np.float32)
    # Distance to the NEAREST backdrop, so a pixel matching any of them goes.
    dist = np.min(np.stack([
        np.sqrt(((a - bg) ** 2).sum(axis=2)) for bg in backdrops_of(a)
    ]), axis=0)
    alpha = np.clip((dist - TOL_NEAR) / (TOL_FAR - TOL_NEAR), 0.0, 1.0)

    rgba = np.dstack([a, alpha * 255.0]).astype(np.uint8)
    im = Image.fromarray(rgba, "RGBA")
    if bb := im.getbbox():           # trim to what actually survived
        im = im.crop(bb)
    # Square it so every sprite sits identically in a square slot.
    side = max(im.size)
    sq = Image.new("RGBA", (side, side), (0, 0, 0, 0))
    sq.paste(im, ((side - im.width) // 2, (side - im.height) // 2))
    if side < 128:
        sq = sq.resize((128, 128), Image.NEAREST)   # never smooth pixel art
    if rounded:
        sq = round_corners(sq)

    out = outdir / f"{name}.png"
    out.parent.mkdir(parents=True, exist_ok=True)
    sq.save(out)
    opaque = (np.asarray(sq)[:, :, 3] > 8).mean() * 100
    return f"{name:14s} {sq.size[0]}x{sq.size[1]}  {opaque:4.1f}% opaque"


def main() -> int:
    sheet = Image.open(sys.argv[1])
    outdir = pathlib.Path(sys.argv[2])
    for name, bbox in TOP.items():
        print("  " + cut(sheet, bbox, name, outdir))
    for r, row in enumerate(BADGES):
        for c, name in enumerate(row):
            bbox = box(COLS[c][0], ROWS[r][0], COLS[c][1], ROWS[r][1], name)
            print("  " + cut(sheet, bbox, name, outdir / "icons", rounded=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
