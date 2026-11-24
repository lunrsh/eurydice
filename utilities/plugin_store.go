package utilities

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"git.lunr.sh/luna/eurydice/state"
)

type PluginStore struct {
	appState *state.ApplicationState
	pluginID string

	hasHydrated bool
}

// Gets all keys in the KV store
func (store *PluginStore) KVGetAllKeys() []string {
	return slices.Collect(maps.Keys(store.appState.Config.JSONConfig.PluginKVStore[store.pluginID]))
}

// Gets a value, given the specified key
func (store *PluginStore) KVGet(key string) (any, error) {
	value, ok := store.appState.Config.JSONConfig.PluginKVStore[store.pluginID][key]

	if !ok {
		return nil, fmt.Errorf("could not index '%s'", key)
	}

	return value, nil
}

// Sets a value, given the specified key
func (store *PluginStore) KVSet(key string, value any) error {
	// TODO: check value before it gets added
	store.appState.Config.JSONConfig.PluginKVStore[store.pluginID][key] = value

	// Write the updated configuration to text
	updatedConfiguration, err := json.Marshal(store.appState.Config.JSONConfig)

	if err != nil {
		return fmt.Errorf("Failed to marshal configuration: %w", err)
	}

	// Save it to disk
	if err := os.WriteFile(store.appState.Config.JSONConfigPath, updatedConfiguration, 0644); err != nil {
		return fmt.Errorf("Failed to write configuration: %w", err)
	}

	// We're done!
	return nil
}

// Returns the base file path that you should use for any big files/block storage.
func (store *PluginStore) GetFSBasePath() string {
	return store.appState.Config.AppStatePath
}

// Initialize the PluginStore
func (store *PluginStore) Hydrate(appState *state.ApplicationState, pluginID string) {
	if store.hasHydrated {
		return
	}

	store.appState = appState
	store.pluginID = filepath.Clean(pluginID)

	// Create the KV store if it does not currently exist
	if _, ok := store.appState.Config.JSONConfig.PluginKVStore[store.pluginID]; !ok {
		store.appState.Config.JSONConfig.PluginKVStore[store.pluginID] = map[string]any{}
	}

	// Create the base path if it does not currently exist
	if err := os.MkdirAll(filepath.Join(appState.Config.AppStatePath, "plugins", store.pluginID), 0755); err != nil {
		panic(fmt.Sprintf("failed to initialize plugin large file store for plugin '%s': %s", pluginID, err.Error()))
	}
}
