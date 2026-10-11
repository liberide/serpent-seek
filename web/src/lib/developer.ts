// Shared types for the Developer section.

/** A developer token as returned by GET /api/developer (secret-free view). */
export type DeveloperToken = {
	id: string;
	name: string;
	prefix: string;
	created_at: string;
	last_used_at: string;
	revoked_at: string;
	/** Granted endpoint ids; empty when all_endpoints is true. */
	permissions: string[];
	/** True when the token may call every endpoint (no restriction). */
	all_endpoints: boolean;
};

/** Payload of GET /api/developer and PUT /api/developer/mode. */
export type DeveloperStatus = {
	enabled: boolean;
	env_locked: boolean;
	tokens: DeveloperToken[];
};

/** One grantable API operation from GET /api/developer/endpoints. */
export type DeveloperEndpoint = {
	id: string;
	method: string;
	path: string;
	tag?: string;
	summary?: string;
	public: boolean;
};
