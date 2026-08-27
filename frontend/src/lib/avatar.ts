/**
 * The default-avatar hue: a stable per-username hue for the `.monogram`
 * tile (base.css). Same hash everywhere so a user's tile matches across
 * comments, team pages and profiles.
 */
export function nameHue(name: string): number {
  let h = 0;
  for (const c of name) h = (h * 31 + c.charCodeAt(0)) % 360;
  return h;
}
