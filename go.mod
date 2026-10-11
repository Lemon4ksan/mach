module github.com/lemon4ksan/mach

go 1.27.0

require (
	github.com/lemon4ksan/foundation v0.0.0-20260924154538-d834ce78e81a
	github.com/lemon4ksan/seal v0.0.0
	golang.org/x/crypto v0.58.0
	golang.org/x/net v0.61.0
	golang.org/x/sys v0.49.0
)

require golang.org/x/text v0.43.0 // indirect

replace (
	github.com/lemon4ksan/foundation => ../foundation
	github.com/lemon4ksan/seal => ../seal
)
