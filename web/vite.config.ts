import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			preprocess: vitePreprocess(),
			// Resolve $lib imports to src/lib.
			alias: { $lib: 'src/lib' },
			adapter: adapter({
				pages: 'build',
				assets: 'build',
				fallback: 'index.html',
				precompress: false,
				strict: false
			})
		})
	],
	server: {
		port: 5173,
		proxy: {
			'/api': 'http://localhost:8080',
			'/search': 'http://localhost:8080',
			'/healthz': 'http://localhost:8080',
			'/version': 'http://localhost:8080',
			'/openapi.yaml': 'http://localhost:8080'
		}
	}
});
