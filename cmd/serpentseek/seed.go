package main

import (
	"context"

	"github.com/google/uuid"

	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/logging"
	"github.com/liberide/serpent-seek/internal/store"
)

// providerSeed describes the built-in provider defaults. Each seed becomes one
// provider *instance*; users may add more instances of the same driver later.
type providerSeed struct {
	code        string
	name        string
	enabled     bool
	baseURL     string
	credentials map[string]string
	params      map[string]string
}

// providerSeeds returns the providers seeded exactly once on a fresh install.
// Only Google (Vertex/Grounding) is created, and it is enabled by default; all
// other drivers stay available in the "Add provider" catalog but are not
// instantiated automatically.
func providerSeeds(cfg *config.Config) []providerSeed {
	return []providerSeed{
		{
			code: "google_vertex", name: "Google (Vertex/Grounding)",
			enabled:     true,
			baseURL:     "https://generativelanguage.googleapis.com",
			credentials: map[string]string{"api_key": cfg.DefaultGoogleAPIKey},
			params:      map[string]string{"model": cfg.DefaultGoogleModel, "driver_mode": cfg.DefaultGoogleDriver},
		},
	}
}

// seededSetting marks that the bootstrap seed has run for this install.
const seededSetting = "core.seeded"

// seed inserts default provider instances and an initial active chain exactly
// once per installation (persisted markers). After that, user deletions are
// permanent: restarting must not resurrect deleted providers or chains.
func seed(ctx context.Context, st store.Storage, cfg *config.Config, log *logging.Logger) error {
	if v, err := st.GetSetting(ctx, seededSetting); err == nil && v == "1" {
		return nil
	}
	existing, err := st.ListProviders(ctx)
	if err != nil {
		return err
	}
	ids := map[string]string{}
	for _, p := range existing {
		if _, ok := ids[p.Code]; !ok {
			ids[p.Code] = p.ID
		}
	}
	for _, sd := range providerSeeds(cfg) {
		if _, ok := ids[sd.code]; ok {
			continue
		}
		inst := &store.Provider{
			ID: uuid.NewString(), Code: sd.code, Name: sd.name, Enabled: sd.enabled,
			BaseURL: sd.baseURL, Credentials: sd.credentials, Params: sd.params,
		}
		if err := st.UpsertProvider(ctx, inst); err != nil {
			return err
		}
		ids[sd.code] = inst.ID
		log.Info("", "", "seeded provider instance "+sd.code+" ("+sd.name+")")
	}

	chains, err := st.ListChains(ctx)
	if err != nil {
		return err
	}
	if len(chains) == 0 {
		chain := defaultChain(ids)
		if err := st.SaveChain(ctx, chain); err != nil {
			return err
		}
		if err := st.ActivateChain(ctx, chain.ID); err != nil {
			return err
		}
		log.Info("", "", "seeded default active chain "+chain.Name)
	}
	return st.SetSetting(ctx, seededSetting, "1")
}

// defaultChain builds the initial active chain: a single Google
// (Vertex/Grounding) block. ids maps a driver code to its provider instance id.
func defaultChain(ids map[string]string) *store.Chain {
	order := []string{"google_vertex"}
	chain := &store.Chain{ID: uuid.NewString(), Name: "Default chain", Active: false, Mode: store.ChainModeFirstSuccess}
	var keys []string
	for i, code := range order {
		id := ids[code]
		if id == "" {
			continue
		}
		key := code
		keys = append(keys, key)
		chain.Nodes = append(chain.Nodes, store.ChainNode{
			Key:          key,
			ProviderID:   id,
			Label:        key,
			TimeoutMS:    20000,
			Retries:      0,
			RetryDelayMS: 700,
			DelayPolicy:  "linear",
			OnSuccess:    "stop",
			OnEmpty:      "next",
			OnFail:       "next",
			IsStart:      i == 0,
			PosX:         float64(i * 260),
			PosY:         80,
			Params:       map[string]string{},
		})
	}
	for i := 0; i+1 < len(keys); i++ {
		chain.Edges = append(chain.Edges, store.ChainEdge{
			FromKey: keys[i], ToKey: keys[i+1], Condition: "next",
		})
	}
	return chain
}
