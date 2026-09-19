import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		port: 5173,
		proxy: {
			'/api': 'http://localhost:8080',
			'/search': 'http://localhost:8080',
			'/healthz': 'http://localhost:8080',
			'/version': 'http://localhost:8080'
		}
	}
});
