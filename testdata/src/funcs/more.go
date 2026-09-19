package funcs

// A package is all of its files: this one is analyzed in the same pass.

func a() {} // want "^found function a$"

func ab() {} // want "^found function ab$"
