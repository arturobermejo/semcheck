package semcheck

import "go/types"

// isNamed reports whether t is the type pkgPath.name, however the source code
// spells it: through a renamed import, a dot import or a type alias.
func isNamed(t types.Type, pkgPath, name string) bool {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}

	obj := named.Obj()

	return obj.Name() == name && obj.Pkg() != nil && obj.Pkg().Path() == pkgPath
}

func isPointerTo(t types.Type, pkgPath, name string) bool {
	ptr, ok := types.Unalias(t).(*types.Pointer)

	return ok && isNamed(ptr.Elem(), pkgPath, name)
}
