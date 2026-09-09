# Arcade art

Note the folder is `icons/`, not `badges/`. Ad blockers and Brave Shields match
on path fragments, and "badges" is close enough to the patterns they block that
it is not worth the risk for a word nobody sees.

Drop the pixel-art assets here with these exact names. The page renders each one
as an `<img>`, and falls back to the built-in glyph if a file is missing — so a
half-finished set degrades one badge at a time instead of breaking the page.

PNG with transparency, square, 128×128 or larger (they are displayed at 30–56px
but retina doubles that, and the badge tab shows them bigger than the strip).

## Badges — filename must match the badge code

| File | Badge |
|---|---|
| `icons/one_guess.png`    | Din prima |
| `icons/first_try_5.png`  | Ochi format |
| `icons/clutch.png`       | La limită |
| `icons/perfect_day.png`  | Zi completă |
| `icons/week_title.png`   | Săptămână de titluri |
| `icons/week_poster.png`  | Săptămână de postere |
| `icons/perfect_week.png` | Săptămână perfectă |
| `icons/no_hint_week.png` | Fără ajutor |
| `icons/gold_5000.png`    | Cinci mii |
| `icons/level_25.png`     | Nivel 25 |
| `icons/level_50.png`     | Nivel 50 |
| `icons/level_100.png`    | Nivel 100 |

The name is the badge's code in `internal/games/games.go`, not its Romanian
label — so adding a badge later means adding a file with the new code's name.

## Economy

| File | Used for |
|---|---|
| `aur.png`         | the gold figure in the identity strip and the chest reveal |
| `chest-closed.png` | the chest button once opened for the day |
| `chest-open.png`   | the chest button while it is still claimable |
