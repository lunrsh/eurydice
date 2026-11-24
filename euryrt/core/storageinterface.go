package core

type PluginStorage interface {
	// Gets all keys in the KV store
	KVGetAllKeys() []string
	// Gets a value, given the specified key
	KVGet(key string) (any, error)
	// Sets a value, given the specified key
	KVSet(key string, value any) error

	// Returns the base file path that you should use for any big files/block storage.
	GetFSBasePath() string
}
