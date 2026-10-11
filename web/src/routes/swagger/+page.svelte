<script lang="ts">
	import { onMount } from 'svelte';
	// Swagger UI is bundled locally (no CDN / no external resources). The UMD
	// bundle is loaded as a same-origin asset; it attaches SwaggerUIBundle to
	// window, which the existing CSP (`script-src 'self'`) allows.
	import swaggerBundleUrl from 'swagger-ui-dist/swagger-ui-bundle.js?url';
	import 'swagger-ui-dist/swagger-ui.css';
	import '$lib/swagger.css';
	import { t } from '$lib/i18n.svelte';

	let container: HTMLDivElement;

	// window.SwaggerUIBundle is provided by the injected UMD script.
	type SwaggerWindow = Window & { SwaggerUIBundle?: (opts: Record<string, unknown>) => unknown };

	onMount(() => {
		const win = window as SwaggerWindow;
		const start = () => {
			if (!win.SwaggerUIBundle || !container) return;
			win.SwaggerUIBundle({
				// Cache-bust: the spec is embedded at build time and must reflect
				// the running binary, not a browser-cached copy.
				url: `/openapi.yaml?t=${Date.now()}`,
				domNode: container,
				deepLinking: true,
				docExpansion: 'list',
				defaultModelsExpandDepth: 0,
				persistAuthorization: true,
				tryItOutEnabled: true,
				// Never contact the public validator: the page must not use any
				// third-party resource (and the CSP forbids it anyway).
				validatorUrl: null
			});
		};
		if (win.SwaggerUIBundle) {
			start();
			return;
		}
		const script = document.createElement('script');
		script.id = 'swagger-ui-bundle';
		script.src = swaggerBundleUrl;
		script.async = true;
		script.onload = start;
		document.body.appendChild(script);
	});
</script>

<div class="space-y-3">
	<div class="flex justify-end">
		<a class="btn text-xs" href="/openapi.yaml" download="serpentseek-openapi.yaml">
			{t('developer.download')}
		</a>
	</div>
	<div bind:this={container} class="swagger-host rounded-xl border border-slate-800 p-2"></div>
</div>
