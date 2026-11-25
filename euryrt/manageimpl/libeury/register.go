package libeury

import (
	"git.lunr.sh/luna/eurydice/euryrt"
	"git.lunr.sh/luna/eurydice/euryrt/core"
)

func init() {
	constructor := func() core.AppCommonRuntime {
		return &LibEuryRT{}
	}

	euryrt.RegisterCommonRuntimeConstructor("libeury", constructor)
}
