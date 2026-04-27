// Package server — EntityServer is provided by
// CodeValdSharedLib/entitygraph/server. This file re-exports NewEntityServer
// so callers import only this package.
package server

import (
	egserver "github.com/aosanya/CodeValdSharedLib/entitygraph/server"
)

// NewEntityServer constructs an EntityServer backed by the given DataManager.
// It is a thin re-export of the shared implementation in
// CodeValdSharedLib/entitygraph/server.
var NewEntityServer = egserver.NewEntityServer
