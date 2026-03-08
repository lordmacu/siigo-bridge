package parsers

import (
	"fmt"
	"math"
)

// DecodePacked decodes a COBOL packed decimal (COMP-3) field.
// Each byte contains two BCD digits (4 bits each), except the last byte
// where the lower nibble is the sign:
//   0xC = positive, 0xD = negative, 0xF = unsigned (positive)
//
// Example: bytes [0x12, 0x34, 0x5C] = +12345
// With decimals=2: 123.45
func DecodePacked(data []byte, decimals int) float64 {
	if len(data) == 0 {
		return 0
	}

	var digits []byte
	negative := false

	for i, b := range data {
		hi := (b >> 4) & 0x0F
		lo := b & 0x0F

		if i == len(data)-1 {
			// Last byte: hi is digit, lo is sign
			digits = append(digits, hi)
			if lo == 0x0D {
				negative = true
			}
		} else {
			digits = append(digits, hi, lo)
		}
	}

	// Convert digits to integer
	var value float64
	for _, d := range digits {
		if d > 9 {
			return 0 // invalid BCD
		}
		value = value*10 + float64(d)
	}

	// Apply decimal places
	if decimals > 0 {
		value = value / math.Pow(10, float64(decimals))
	}

	if negative {
		value = -value
	}

	return value
}

// DecodePackedString decodes a packed decimal and returns a formatted string.
func DecodePackedString(data []byte, decimals int) string {
	val := DecodePacked(data, decimals)
	if decimals > 0 {
		return fmt.Sprintf("%.*f", decimals, val)
	}
	return fmt.Sprintf("%.0f", val)
}

// ExtractPacked extracts a packed decimal field from a record at given offset and length.
func ExtractPacked(rec []byte, offset, length, decimals int) string {
	if offset+length > len(rec) || offset < 0 {
		return "0"
	}
	return DecodePackedString(rec[offset:offset+length], decimals)
}
