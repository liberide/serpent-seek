# Grafana dashboard

`dashboard.json` is a ready-to-import dashboard for the SerpentSeek `/metrics`
endpoint.

**Import:** Grafana → Dashboards → New → Import → Upload JSON. Select the
Prometheus data source when prompted.

The dashboard covers every exported metric:
`serpentseek_search_requests_total`, `serpentseek_provider_requests_total`,
`serpentseek_provider_duration_seconds`, `serpentseek_active_requests`,
`serpentseek_sse_clients`, `serpentseek_sse_dropped_events_total`,
`serpentseek_quota_rejected_total` and `serpentseek_build_info`.

If `/metrics` is protected by a token, add it to the Prometheus scrape config:

```yaml
scrape_configs:
  - job_name: serpentseek
    bearer_token: <METRICS_AUTH_TOKEN>
    static_configs:
      - targets: ["serpentseek:8080"]
```
