//go:build windows && 386

package keyboard

// input matches Windows INPUT on 32-bit systems, where MOUSEINPUT is the
// 24-byte largest union member and needs no padding after Type.
type input struct {
	Type uint32
	Data [24]byte
}
