package tui

// singleByteKeys maps a single command byte to its Key. Both the lower- and
// upper-case forms of each command key map to the same Key (Req 3.2, 3.3, 5.1,
// 5.3, 5.4, 7.1).
var singleByteKeys = map[byte]Key{
	'd': KeyDelete,
	'D': KeyDelete,
	'y': KeyConfirmYes,
	'Y': KeyConfirmYes,
	'n': KeyConfirmNo,
	'N': KeyConfirmNo,
	'q': KeyQuit,
	'Q': KeyQuit,
}

// escapeSeqKeys maps the final byte of a three-byte CSI arrow sequence
// (ESC '[' X) to its Key (Req 3.2, 3.3).
var escapeSeqKeys = map[byte]Key{
	'A': KeyUp,   // ESC [ A
	'B': KeyDown, // ESC [ B
}

// decodeKey maps a raw byte buffer to a Key. Single bytes decode command keys;
// the three-byte sequence ESC '[' A/B decodes an arrow key. It is pure and
// table-driven so it can be unit tested directly. An empty buffer yields
// KeyNone and anything unrecognized yields KeyOther.
func decodeKey(b []byte) Key {
	switch len(b) {
	case 0:
		return KeyNone
	case 1:
		if k, ok := singleByteKeys[b[0]]; ok {
			return k
		}
	case 3:
		if b[0] == 0x1b && b[1] == '[' {
			if k, ok := escapeSeqKeys[b[2]]; ok {
				return k
			}
		}
	}
	return KeyOther
}
