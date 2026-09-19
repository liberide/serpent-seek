<script lang="ts">
	import Modal from './Modal.svelte';
	import { t } from '$lib/i18n.svelte';

	let {
		open = false,
		title = '',
		message = '',
		confirmLabel = '',
		onconfirm,
		oncancel
	}: {
		open?: boolean;
		title?: string;
		message?: string;
		confirmLabel?: string;
		onconfirm?: () => void;
		oncancel?: () => void;
	} = $props();
</script>

<Modal {open} title={title || t('confirm.title')} onclose={oncancel}>
	<p class="mb-4 text-sm text-slate-300">{message}</p>
	<div class="flex justify-end gap-2">
		<button class="btn" onclick={() => oncancel?.()}>{t('common.cancel')}</button>
		<button class="btn btn-danger" onclick={() => onconfirm?.()}>{confirmLabel || t('confirm.confirm')}</button>
	</div>
</Modal>
