import { t } from './i18n.svelte';

/**
 * Stable engine reason codes for skipped/dead provider nodes. The engine
 * persists and emits these codes instead of prose so the UI can localize them;
 * any value that is not a known code (e.g. a real upstream error) is returned
 * unchanged.
 */
const REASON_KEYS: Record<string, string> = {
	provider_disabled: 'deadReason.providerDisabled',
	searxng_url_missing: 'deadReason.searxngUrlMissing',
	service_account_missing: 'deadReason.missingServiceAccount',
	api_key_missing: 'deadReason.missingApiKey',
	provider_marked_dead: 'deadReason.markedDead'
};

const UNKNOWN_DRIVER_PREFIX = 'unknown_driver:';

/** Localizes an engine provider-skip reason, passing through unknown strings. */
export function providerReason(reason: string | null | undefined): string {
	if (!reason) return '';
	if (reason.startsWith(UNKNOWN_DRIVER_PREFIX)) {
		return t('deadReason.unknownDriver', { driver: reason.slice(UNKNOWN_DRIVER_PREFIX.length) });
	}
	const key = REASON_KEYS[reason];
	return key ? t(key) : reason;
}
