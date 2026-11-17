package credentials

import "github.com/diasYuri/opera/internal/privatefs"

func restrict(path string, directory bool) error     { return privatefs.Protect(path, directory) }
func checkPrivate(path string, directory bool) error { return privatefs.Check(path, directory) }
