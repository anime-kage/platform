/// <reference types="vitest/config" />
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [sveltekit()],

	// Smart TVs run old Chromium: LG webOS 4/5/6 is Chromium 53/68/79, Samsung
	// Tizen 5.5/6/6.5 is Chrome 69/76/85. Vite 7 defaults to
	// "baseline-widely-available" (Chrome 107+), which emitted `?.`, `??` and
	// `||=` directly into the bundle -- syntax those engines cannot PARSE, so
	// the whole file throws before a single line runs and the viewer gets a
	// blank page rather than a degraded one.
	//
	// es2017 covers Chromium 55+, which reaches essentially every smart TV
	// still in use, at the cost of a slightly larger bundle.
	build: {
		target: ['es2017', 'chrome63', 'safari11.1', 'firefox67'],
		// CSS is minified against the same floor so Vite does not collapse
		// colours into syntax (like #rgba) that old parsers reject.
		cssTarget: ['chrome63', 'safari11.1']
	},

	// Dependency pre-bundling in dev uses its own target; keep it aligned so
	// what you see locally matches what a TV gets.
	optimizeDeps: {
		esbuildOptions: { target: 'es2017' }
	},

	// Tests. Two projects because they need different environments and the
	// split keeps the fast ones fast:
	//
	//   unit       pure functions (markdown, providers, reltime...) in plain
	//              node. No DOM, no component compile -- milliseconds.
	//   component  .svelte files mounted in happy-dom via @testing-library.
	//
	// `npm test` runs both. `npm test -- --project unit` runs just the quick
	// half, which is what you want while writing a parser test.
	test: {
		projects: [
			{
				extends: true,
				test: {
					name: 'unit',
					environment: 'node',
					include: ['src/**/*.test.ts'],
					exclude: ['src/**/*.svelte.test.ts']
				}
			},
			{
				extends: true,
				// Without browser conditions, `import Foo from './Foo.svelte'`
				// resolves to Svelte's SSR build, whose mount() throws
				// "not available on the server". Client build only here.
				resolve: { conditions: ['browser'] },
				test: {
					name: 'component',
					environment: 'happy-dom',
					include: ['src/**/*.svelte.test.ts'],
					setupFiles: ['./src/tests/setup.ts']
				}
			}
		]
	}
});
