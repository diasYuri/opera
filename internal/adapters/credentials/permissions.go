package credentials

import "github.com/diasYuri/opera/internal/adapters/platform"

func restrict(path string, directory bool) error     { return platform.Protect(path, directory) }
func checkPrivate(path string, directory bool) error { return platform.Check(path, directory) }
