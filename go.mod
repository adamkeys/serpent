module github.com/adamkeys/serpent

go 1.23.4

require github.com/ebitengine/purego v0.9.1

// Support for returning structs on Linux. Can be turned back to mainline once
// https://github.com/ebitengine/purego/pull/361 is merged.
replace github.com/ebitengine/purego => github.com/tmc/purego v0.0.0-20251108172255-a82375ff6944
