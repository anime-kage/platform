/**
 * Fails if the built client bundle uses syntax an old browser cannot PARSE.
 *
 * This exists because a parse error is invisible in normal testing and fatal in
 * production: the engine rejects the whole file before running a line, so the
 * viewer gets a blank page rather than a degraded one. Smart TVs are where this
 * bites -- LG webOS 4/5/6 is Chromium 53/68/79, Samsung Tizen 5.5/6/6.5 is
 * Chrome 69/76/85 -- and none of them are in anyone's test matrix.
 *
 * Spoofing a user-agent does NOT reproduce this. The engine's capabilities are
 * what matter, so the check parses the real output against a pinned ES version.
 *
 *   node scripts/check-browser-target.mjs [dir] [ecmaVersion]
 *
 * Dynamic import() is permitted even though the spec calls it ES2020: browsers
 * shipped it in Chrome 63, and SvelteKit needs it for code splitting.
 */
import { parse } from 'acorn';
import fs from 'fs';
import path from 'path';

const DIR = process.argv[2] ?? '.svelte-kit/output/client';
const ECMA = Number(process.argv[3] ?? 2020);

function walk(dir, out = []) {
	if (!fs.existsSync(dir)) return out;
	for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
		const p = path.join(dir, e.name);
		if (e.isDirectory()) walk(p, out);
		else if (e.name.endsWith('.js')) out.push(p);
	}
	return out;
}

const files = walk(DIR);
if (!files.length) {
	console.error(`No .js found under ${DIR} -- run \`npm run build\` first.`);
	process.exit(2);
}

const failures = [];
for (const f of files) {
	try {
		parse(fs.readFileSync(f, 'utf8'), { ecmaVersion: ECMA, sourceType: 'module' });
	} catch (err) {
		failures.push({ file: path.relative(DIR, f), message: err.message });
	}
}

if (failures.length) {
	console.error(`\nES${ECMA} parse check FAILED: ${failures.length}/${files.length} files\n`);
	for (const { file, message } of failures.slice(0, 10)) {
		console.error(`  ${file}\n     ${message}`);
	}
	if (failures.length > 10) console.error(`  ... and ${failures.length - 10} more`);
	console.error(`\nThe bundle needs a newer engine than ES${ECMA}. Check`);
	console.error(`build.target in vite.config.ts.\n`);
	process.exit(1);
}

console.log(`ES${ECMA} parse check passed: ${files.length}/${files.length} files.`);
