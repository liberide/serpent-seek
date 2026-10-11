import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';

// Focused unit tests for pure UI helpers. The svelte() plugin is required to
// compile the `.svelte.ts` runes module that several helpers import.
export default defineConfig({
	plugins: [svelte()],
	test: {
		environment: 'node',
		include: ['src/**/*.test.ts']
	}
});
