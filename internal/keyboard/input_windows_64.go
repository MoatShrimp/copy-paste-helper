//go:build windows && (amd64 || arm64)

package keyboard

// input matches Windows INPUT on 64-bit systems. The explicit padding aligns
// its 32-byte union to a pointer boundary.
type input struct {
	Type uint32
	_    uint32
	Data [32]byte
}
