package journal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
)

// Check duplicate names, depth, UTF-8 and surrogate escapes before typed decode.
// The input has already been bounded by file size and LimitReader.
func strictJSON(b []byte) error {
	if !utf8.Valid(b) {
		return ErrRepair
	}
	// encoding/json replaces isolated UTF-16 surrogates; disallow that ambiguity.
	for i := 0; i < len(b); i++ {
		if b[i] != '"' {
			continue
		}
		i++
		for i < len(b) && b[i] != '"' {
			if b[i] == '\\' {
				i++
				if i >= len(b) {
					return ErrRepair
				}
				if b[i] == 'u' {
					if i+4 >= len(b) {
						return ErrRepair
					}
					var v uint16
					for j := 1; j <= 4; j++ {
						n, ok := nibble(b[i+j])
						if !ok {
							return ErrRepair
						}
						v = v*16 + uint16(n)
					}
					i += 4
					if v >= 0xd800 && v <= 0xdbff {
						if i+6 >= len(b) || b[i+1] != '\\' || b[i+2] != 'u' {
							return ErrRepair
						}
						var w uint16
						for j := 3; j <= 6; j++ {
							n, ok := nibble(b[i+j])
							if !ok {
								return ErrRepair
							}
							w = w*16 + uint16(n)
						}
						if w < 0xdc00 || w > 0xdfff {
							return ErrRepair
						}
						i += 6
					} else if v >= 0xdc00 && v <= 0xdfff {
						return ErrRepair
					}
				}
			}
			i++
		}
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if e := jsonValue(d, 0); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrRepair
	}
	return nil
}
func nibble(b byte) (byte, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	}
	return 0, false
}
func jsonValue(d *json.Decoder, depth int) error {
	if depth > 16 {
		return ErrRepair
	}
	t, e := d.Token()
	if e != nil {
		return e
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, e := d.Token()
			if e != nil {
				return e
			}
			s, ok := k.(string)
			if !ok || seen[s] {
				return ErrRepair
			}
			if len(seen) >= 10000 {
				return ErrRepair
			}
			seen[s] = true
			if e = jsonValue(d, depth+1); e != nil {
				return e
			}
		}
	case '[':
		count := 0
		for d.More() {
			count++
			if count > 10000 {
				return ErrRepair
			}
			if e = jsonValue(d, depth+1); e != nil {
				return e
			}
		}
	default:
		return ErrRepair
	}
	end, e := d.Token()
	if e != nil {
		return e
	}
	if (delim == '{' && end != json.Delim('}')) || (delim == '[' && end != json.Delim(']')) {
		return fmt.Errorf("invalid JSON container")
	}
	return nil
}
