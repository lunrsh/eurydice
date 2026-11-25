package state

import "strings"

var Version = ""
var IsDevelBuild = strings.Contains(Version, "-")
