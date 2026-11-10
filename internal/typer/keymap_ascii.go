package typer

// MacANSIKey maps a printable US-ANSI ASCII rune to a macOS virtual keycode (kVK_ANSI_*)
// and reports whether the Shift modifier is required.
func MacANSIKey(r rune) (keyCode uint16, shift bool, ok bool) {
	switch r {
	case 'a', 'A':
		return 0x00, r == 'A', true
	case 's', 'S':
		return 0x01, r == 'S', true
	case 'd', 'D':
		return 0x02, r == 'D', true
	case 'f', 'F':
		return 0x03, r == 'F', true
	case 'h', 'H':
		return 0x04, r == 'H', true
	case 'g', 'G':
		return 0x05, r == 'G', true
	case 'z', 'Z':
		return 0x06, r == 'Z', true
	case 'x', 'X':
		return 0x07, r == 'X', true
	case 'c', 'C':
		return 0x08, r == 'C', true
	case 'v', 'V':
		return 0x09, r == 'V', true
	case 'b', 'B':
		return 0x0B, r == 'B', true
	case 'q', 'Q':
		return 0x0C, r == 'Q', true
	case 'w', 'W':
		return 0x0D, r == 'W', true
	case 'e', 'E':
		return 0x0E, r == 'E', true
	case 'r', 'R':
		return 0x0F, r == 'R', true
	case 'y', 'Y':
		return 0x10, r == 'Y', true
	case 't', 'T':
		return 0x11, r == 'T', true
	case '1', '!':
		return 0x12, r == '!', true
	case '2', '@':
		return 0x13, r == '@', true
	case '3', '#':
		return 0x14, r == '#', true
	case '4', '$':
		return 0x15, r == '$', true
	case '6', '^':
		return 0x16, r == '^', true
	case '5', '%':
		return 0x17, r == '%', true
	case '=', '+':
		return 0x18, r == '+', true
	case '9', '(':
		return 0x19, r == '(', true
	case '7', '&':
		return 0x1A, r == '&', true
	case '-', '_':
		return 0x1B, r == '_', true
	case '8', '*':
		return 0x1C, r == '*', true
	case '0', ')':
		return 0x1D, r == ')', true
	case ']', '}':
		return 0x1E, r == '}', true
	case 'o', 'O':
		return 0x1F, r == 'O', true
	case 'u', 'U':
		return 0x20, r == 'U', true
	case '[', '{':
		return 0x21, r == '{', true
	case 'i', 'I':
		return 0x22, r == 'I', true
	case 'p', 'P':
		return 0x23, r == 'P', true
	case 'l', 'L':
		return 0x25, r == 'L', true
	case 'j', 'J':
		return 0x26, r == 'J', true
	case '\'', '"':
		return 0x27, r == '"', true
	case 'k', 'K':
		return 0x28, r == 'K', true
	case ';', ':':
		return 0x29, r == ':', true
	case '\\', '|':
		return 0x2A, r == '|', true
	case ',', '<':
		return 0x2B, r == '<', true
	case '/', '?':
		return 0x2C, r == '?', true
	case 'n', 'N':
		return 0x2D, r == 'N', true
	case 'm', 'M':
		return 0x2E, r == 'M', true
	case '.', '>':
		return 0x2F, r == '>', true
	case '`', '~':
		return 0x32, r == '~', true
	case ' ':
		return 0x31, false, true
	default:
		return 0, false, false
	}
}
