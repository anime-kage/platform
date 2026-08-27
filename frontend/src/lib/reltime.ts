/** Romanian relative time for team pages: "acum", "acum 20 min", "ieri", "acum 5 zile". */
export function reltime(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return '';
  const mins = Math.floor((Date.now() - then) / 60_000);
  if (mins < 1) return 'acum';
  if (mins < 60) return `acum ${mins} min`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `acum ${hours} h`;
  const days = Math.floor(hours / 24);
  if (days === 1) return 'ieri';
  if (days < 30) return `acum ${days} zile`;
  return new Date(iso).toLocaleDateString('ro-RO', { day: 'numeric', month: 'short' });
}

/** How long something has been waiting: "3 h", "2 zile". */
export function waitedFor(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return '';
  const hours = Math.floor((Date.now() - then) / 3_600_000);
  if (hours < 1) return 'sub o oră';
  if (hours < 24) return `${hours} h`;
  const days = Math.floor(hours / 24);
  return days === 1 ? 'o zi' : `${days} zile`;
}
