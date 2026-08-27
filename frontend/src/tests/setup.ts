// Component-test setup. Runs before every *.svelte.test.ts file.
import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/svelte';
import { afterEach } from 'vitest';

// Unmount anything a test rendered. Without this, the second test in a file
// queries a document that still holds the first test's markup and matches the
// wrong element -- which fails in a way that looks like a component bug.
afterEach(() => cleanup());
