package rockbox

import (
	"git.lunr.sh/luna/eurydice/euryrt"
	"git.lunr.sh/luna/eurydice/euryrt/core"
)

func init() {
	constructor := func() core.SyncRuntime {
		return &RockboxRT{}
	}

	euryrt.RegisterSyncConstructor("rockbox", constructor)
}
