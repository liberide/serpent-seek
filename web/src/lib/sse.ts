export type SSEHandler = (event: string, data: unknown) => void;

/**
 * Subscribes to a Server-Sent Events endpoint. Named engine events are exposed
 * to the handler; EventSource reconnects automatically (retry: 2000).
 */
export function subscribeSSE(path: string, handler: SSEHandler): () => void {
	const source = new EventSource(path, { withCredentials: true });
	const events = [
		'step_started',
		'step_finished',
		'provider_dead',
		'merge_progress',
		'merge_done',
		'request_done',
		'request_created'
	];
	for (const name of events) {
		source.addEventListener(name, (event) => {
			const message = event as MessageEvent;
			try {
				handler(name, JSON.parse(message.data));
			} catch {
				handler(name, message.data);
			}
		});
	}
	source.onerror = () => {
		// EventSource reconnects on its own; nothing to do here.
	};
	return () => source.close();
}
