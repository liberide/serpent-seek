/** Shared payload types for the analytics endpoints. */

export type DailyStat = {
	date: string;
	requests: number;
	ok: number;
	empty: number;
	fail: number;
	avg_ms: number;
};

export type HourStat = { hour: number; requests: number; ok: number; empty: number; fail: number };

export type TopQuery = { query: string; count: number; ok: number; fail: number };

export type AnalyticsProviderStat = {
	provider: string;
	code: string;
	enabled: boolean;
	configured: boolean;
	total: number;
	ok: number;
	empty: number;
	fail: number;
	skip: number;
	avg_ms: number;
	success_rate: number;
	cost?: number;
	cost_configured?: boolean;
};

export type AnalyticsReport = {
	total_requests: number;
	ok: number;
	empty: number;
	fail: number;
	success_rate: number;
	avg_ms: number;
	p95_ms: number;
	active_days: number;
	busiest_date: string;
	busiest_count: number;
	daily: DailyStat[];
	hourly: HourStat[];
	top_queries: TopQuery[];
	cost_total?: number;
	cost_configured?: boolean;
};

export type AnalyticsRange = { from: string; to: string };

export type AnalyticsResponse = {
	report: AnalyticsReport;
	providers: AnalyticsProviderStat[];
	range: AnalyticsRange;
};

/** One row of the per-user analytics leaderboard (GET /api/stats/users). */
export type UserStatRow = {
	user_id: string;
	name: string;
	role?: string;
	disabled?: boolean;
	created_at?: string;
	exists: boolean;
	requests: number;
	ok: number;
	empty: number;
	fail: number;
	avg_ms: number;
	last_active?: string;
	cost?: number;
	keys_count: number;
	quota?: QuotaStatus | null;
};

export type UsersAnalyticsResponse = {
	range: AnalyticsRange;
	rows: UserStatRow[];
	totals: {
		requests: number;
		ok: number;
		empty: number;
		fail: number;
		active_users: number;
		cost: number;
		cost_configured: boolean;
	};
};

/** Runtime per-user quota view (mirrors store.UserQuotaStatus). */
export type QuotaStatus = {
	requests: number;
	period: string;
	window_hours: number;
	tz_offset: number;
	used: number;
	remaining: number;
	reset_at?: string;
	updated_at?: string;
};

/** The pseudo owner id for requests made without a user. */
export const ANONYMOUS_USER_ID = 'anonymous';