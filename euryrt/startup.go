package euryrt

import (
	"fmt"

	"git.lunr.sh/luna/eurydice/euryrt/core"
)

// TODO: this is a bit janky, but oh well

type syncPluginConstructor func() core.SyncRuntime
type commonPluginConstructor func() core.AppCommonRuntime

var syncConstructorMap map[string]syncPluginConstructor = map[string]syncPluginConstructor{}
var commonConstructorMap map[string]commonPluginConstructor = map[string]commonPluginConstructor{}

func RegisterCommonRuntimeConstructor(pluginID string, pluginConstructor commonPluginConstructor) {
	commonConstructorMap[pluginID] = pluginConstructor
}

func RegisterSyncConstructor(pluginID string, pluginConstructor syncPluginConstructor) {
	syncConstructorMap[pluginID] = pluginConstructor
}

func ConstructCRT(pluginID string) (core.AppCommonRuntime, error) {
	constructorFunc, ok := commonConstructorMap[pluginID]

	if !ok {
		return nil, fmt.Errorf("plugin not registered")
	}

	return constructorFunc(), nil
}

func ConstructSync(pluginID string) (core.SyncRuntime, error) {
	constructorFunc, ok := syncConstructorMap[pluginID]

	if !ok {
		return nil, fmt.Errorf("plugin not registered")
	}

	return constructorFunc(), nil
}
