<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { notify } from '$lib/stores';
	import { t } from '$lib/i18n.svelte';

	let endpoint = $state('');
	let copied = $state('');

	const apiKeyPlaceholder = 'seek_ak_…';

	const claudeConfig = $derived(JSON.stringify(
		{
			mcpServers: {
				serpentseek: {
					url: endpoint,
					headers: { Authorization: `Bearer ${apiKeyPlaceholder}` }
				}
			}
		},
		null,
		2
	));

	const cursorConfig = $derived(
		`npx mcp-remote ${endpoint} --header "Authorization=Bearer ${apiKeyPlaceholder}"`
	);

	const stdioHint = $derived(t('mcp.stdioHint') || 'Use mcp-remote if the client only supports local stdio servers:');

	const curlExample = $derived(
		`curl -s ${endpoint} \\\n  -H 'Authorization: Bearer ${apiKeyPlaceholder}' \\\n  -H 'Content-Type: application/json' \\\n  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search","arguments":{"query":"hello world","count":3}}}'`
	);

	onMount(() => {
		endpoint = `${window.location.origin}/mcp`;
	});

	async function copyText(key: string, text: string) {
		try {
			await navigator.clipboard.writeText(text);
			copied = key;
			setTimeout(() => (copied = ''), 1500);
		} catch {
			notify(t('mcp.copyError') || 'Copy failed', 'error');
		}
	}

	// Quick test: run a real JSON-RPC tools/call against /mcp.
	let testQuery = $state('');
	let testCount = $state(5);
	let testing = $state(false);
	let testResult = $state<string | null>(null);
	let testError = $state<string | null>(null);

	async function runTest() {
		if (!testQuery.trim()) return;
		testing = true;
		testResult = null;
		testError = null;
		try {
			const res = await api<any>('/mcp', {
				method: 'POST',
				body: {
					jsonrpc: '2.0',
					id: 1,
					method: 'tools/call',
					params: {
						name: 'search',
						arguments: { query: testQuery.trim(), count: testCount }
					}
				}
			});
			if (res.error) {
				testError = res.error.message ?? JSON.stringify(res.error);
			} else {
				const content = res.result?.content ?? [];
				testResult = content.map((c: any) => c.text).join('\n') || t('mcp.noResults');
			}
		} catch (error) {
			testError = (error as Error).message;
		} finally {
			testing = false;
		}
	}
</script>

<div class="space-y-4">
	<h1 class="text-xl font-semibold">{t('mcp.title')}</h1>
	<p class="text-sm text-slate-400">{t('mcp.subtitle')}</p>

	<div class="card">
		<span class="label">{t('mcp.endpoint')}</span>
		<div class="flex items-center gap-2">
			<code class="min-w-0 flex-1 truncate rounded-lg border border-slate-800 bg-slate-950 px-3 py-2 font-mono text-sm text-emerald-300">
				{endpoint || '…'}
			</code>
			<button class="btn" onclick={() => copyText('endpoint', endpoint)}>
				{copied === 'endpoint' ? t('mcp.copied') : t('mcp.copy')}
			</button>
		</div>
		<p class="mt-2 text-xs text-slate-500">{t('mcp.endpointHint')}</p>
		<p class="mt-1 text-xs text-slate-500">{t('mcp.authNote')}</p>
	</div>

	<div class="card">
		<span class="label">{t('mcp.tool')}</span>
		<p class="text-sm">
			<code class="rounded bg-slate-800 px-1.5 py-0.5 font-mono text-emerald-300">{t('mcp.toolSearchName')}</code>
			<span class="text-slate-300"> — {t('mcp.toolSearchDesc')}</span>
		</p>
		<div class="mt-2 text-xs text-slate-500">
			<p>{t('mcp.arguments')}:</p>
			<ul class="mt-1 list-inside list-disc space-y-0.5">
				<li>{t('mcp.argQuery')}</li>
				<li>{t('mcp.argCount')}</li>
			</ul>
		</div>
	</div>

	<div class="card">
		<h2 class="mb-2 text-sm font-semibold">{t('mcp.clientsTitle')}</h2>

		<div class="mb-3">
			<div class="mb-1 flex items-center gap-2">
				<span class="text-sm font-medium text-slate-200">{t('mcp.claudeTitle')}</span>
				<button class="btn px-2 py-0.5 text-xs" onclick={() => copyText('claude', claudeConfig)}>
					{copied === 'claude' ? t('mcp.copied') : t('mcp.copy')}
				</button>
			</div>
			<pre class="overflow-x-auto rounded-lg border border-slate-800 bg-slate-950 p-3 font-mono text-xs text-slate-300">{claudeConfig}</pre>
		</div>

		<div class="mb-3">
			<div class="mb-1 flex items-center gap-2">
				<span class="text-sm font-medium text-slate-200">{t('mcp.cursorTitle')}</span>
				<button class="btn px-2 py-0.5 text-xs" onclick={() => copyText('cursor', cursorConfig)}>
					{copied === 'cursor' ? t('mcp.copied') : t('mcp.copy')}
				</button>
			</div>
			<p class="mb-1 text-xs text-slate-500">{stdioHint}</p>
			<pre class="overflow-x-auto rounded-lg border border-slate-800 bg-slate-950 p-3 font-mono text-xs text-slate-300">{cursorConfig}</pre>
		</div>

		<div>
			<div class="mb-1 flex items-center gap-2">
				<span class="text-sm font-medium text-slate-200">{t('mcp.curlTitle')}</span>
				<button class="btn px-2 py-0.5 text-xs" onclick={() => copyText('curl', curlExample)}>
					{copied === 'curl' ? t('mcp.copied') : t('mcp.copy')}
				</button>
			</div>
			<pre class="overflow-x-auto rounded-lg border border-slate-800 bg-slate-950 p-3 font-mono text-xs text-slate-300">{curlExample}</pre>
		</div>
	</div>

	<div class="card">
		<h2 class="mb-2 text-sm font-semibold">{t('mcp.testTitle')}</h2>
		<form
			class="flex flex-wrap items-end gap-2"
			onsubmit={(event) => {
				event.preventDefault();
				runTest();
			}}
		>
			<div class="min-w-[220px] flex-1">
				<span class="label">{t('mcp.testQuery')}</span>
				<input class="input" bind:value={testQuery} placeholder="hello world" />
			</div>
			<div class="w-24">
				<span class="label">{t('playground.count')}</span>
				<input type="number" min="0" max="20" class="input font-mono" bind:value={testCount} />
			</div>
			<button class="btn btn-primary" type="submit" disabled={testing}>
				{testing ? t('common.loading') : t('mcp.testRun')}
			</button>
		</form>

		{#if testResult !== null}
			<div class="mt-3">
				<span class="label">{t('mcp.testResult')}</span>
				<pre class="whitespace-pre-wrap rounded-lg border border-slate-800 bg-slate-950 p-3 font-mono text-xs text-slate-300">{testResult}</pre>
			</div>
		{/if}
		{#if testError}
			<div class="mt-3">
				<span class="label">{t('mcp.testError')}</span>
				<pre class="whitespace-pre-wrap rounded-lg border border-rose-800 bg-rose-950/40 p-3 font-mono text-xs text-rose-300">{testError}</pre>
			</div>
		{/if}
	</div>
</div>
