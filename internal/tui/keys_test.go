package tui

import "testing"

// TestDecodeKey verifies that decodeKey maps raw byte buffers to the expected
// Key values: both cases of each command key, the two arrow escape sequences,
// an unrecognized byte, and the empty buffer (Req 3.2, 3.3, 5.1, 5.3, 5.4,
// 7.1).
func TestDecodeKey(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want Key
	}{
		{"empty buffer -> KeyNone", []byte{}, KeyNone},
		{"nil buffer -> KeyNone", nil, KeyNone},

		{"lowercase d -> KeyDelete", []byte{'d'}, KeyDelete},
		{"uppercase D -> KeyDelete", []byte{'D'}, KeyDelete},
		{"lowercase y -> KeyConfirmYes", []byte{'y'}, KeyConfirmYes},
		{"uppercase Y -> KeyConfirmYes", []byte{'Y'}, KeyConfirmYes},
		{"lowercase n -> KeyConfirmNo", []byte{'n'}, KeyConfirmNo},
		{"uppercase N -> KeyConfirmNo", []byte{'N'}, KeyConfirmNo},
		{"lowercase q -> KeyQuit", []byte{'q'}, KeyQuit},
		{"uppercase Q -> KeyQuit", []byte{'Q'}, KeyQuit},

		{"ESC [ A -> KeyUp", []byte{0x1b, '[', 'A'}, KeyUp},
		{"ESC [ B -> KeyDown", []byte{0x1b, '[', 'B'}, KeyDown},

		{"unrecognized single byte -> KeyOther", []byte{'z'}, KeyOther},
		{"unrecognized escape final byte -> KeyOther", []byte{0x1b, '[', 'C'}, KeyOther},
		{"malformed escape sequence -> KeyOther", []byte{0x1b, 'X', 'A'}, KeyOther},
		{"two-byte buffer -> KeyOther", []byte{0x1b, '['}, KeyOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decodeKey(tt.in); got != tt.want {
				t.Errorf("decodeKey(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
