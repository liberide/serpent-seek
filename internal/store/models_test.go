package store

import "testing"

func TestProviderCostPerRequest(t *testing.T) {
	cases := []struct {
		name     string
		provider *Provider
		want     float64
	}{
		{"nil", nil, 0},
		{"no params", &Provider{}, 0},
		{"empty", &Provider{Params: map[string]string{"cost_per_request": ""}}, 0},
		{"spaces", &Provider{Params: map[string]string{"cost_per_request": "   "}}, 0},
		{"valid", &Provider{Params: map[string]string{"cost_per_request": "0.002"}}, 0.002},
		{"spaces around", &Provider{Params: map[string]string{"cost_per_request": " 1.5 "}}, 1.5},
		{"invalid", &Provider{Params: map[string]string{"cost_per_request": "free"}}, 0},
		{"zero", &Provider{Params: map[string]string{"cost_per_request": "0"}}, 0},
		{"negative", &Provider{Params: map[string]string{"cost_per_request": "-1"}}, 0},
	}
	for _, tc := range cases {
		if got := tc.provider.CostPerRequest(); got != tc.want {
			t.Errorf("%s: CostPerRequest = %v, want %v", tc.name, got, tc.want)
		}
	}
}
