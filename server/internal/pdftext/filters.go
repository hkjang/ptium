package pdftext

import (
	"bytes"
	"errors"
)

// The filters a PDF wraps a stream in, besides Flate.
//
// A stream is often wrapped twice — ASCII85 over Flate is what several
// generators write by default, so that the file stays printable characters all
// the way through. Undoing only the Flate half left the page empty, and an
// empty page is reported to the person as a file with no text in it: "this
// looks like a scan". The text was there the whole time, behind one more
// wrapper.

// decodeASCII85 undoes the base-85 encoding a stream can be printed in.
//
// Five characters carry four bytes. "z" is four zero bytes written short, and a
// final group shorter than five is padded with "u" and then trimmed back.
func decodeASCII85(data []byte) ([]byte, error) {
	if start := bytes.Index(data, []byte("<~")); start >= 0 {
		data = data[start+2:]
	}
	if end := bytes.Index(data, []byte("~>")); end >= 0 {
		data = data[:end]
	}
	out := make([]byte, 0, len(data)*4/5+4)
	var group [5]byte
	held := 0
	for _, symbol := range data {
		switch {
		case symbol == 'z' && held == 0:
			out = append(out, 0, 0, 0, 0)
			continue
		case symbol <= ' ' || symbol == '\n' || symbol == '\r':
			continue
		case symbol < '!' || symbol > 'u':
			return nil, errors.New("ascii85: a character outside the alphabet")
		}
		group[held] = symbol - '!'
		held++
		if held < 5 {
			continue
		}
		out = append(out, unpack85(group, 5)...)
		held = 0
	}
	if held > 0 {
		if held == 1 {
			return nil, errors.New("ascii85: a final group of one character")
		}
		for fill := held; fill < 5; fill++ {
			group[fill] = 'u' - '!'
		}
		out = append(out, unpack85(group, held)...)
	}
	return out, nil
}

// unpack85 turns one group of base-85 digits into the bytes it stands for,
// keeping only as many as the group was long enough to carry.
func unpack85(group [5]byte, held int) []byte {
	value := uint32(0)
	for _, digit := range group {
		value = value*85 + uint32(digit)
	}
	whole := []byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)}
	return whole[:held-1]
}

// decodeASCIIHex undoes the hexadecimal encoding a stream can be printed in.
// An odd final digit is paired with a zero, which is what the format says.
func decodeASCIIHex(data []byte) ([]byte, error) {
	out := make([]byte, 0, len(data)/2+1)
	value, held := 0, 0
	for _, symbol := range data {
		if symbol == '>' {
			break
		}
		var digit int
		switch {
		case symbol >= '0' && symbol <= '9':
			digit = int(symbol - '0')
		case symbol >= 'a' && symbol <= 'f':
			digit = int(symbol-'a') + 10
		case symbol >= 'A' && symbol <= 'F':
			digit = int(symbol-'A') + 10
		case symbol <= ' ':
			continue
		default:
			return nil, errors.New("asciihex: a character that is not a hex digit")
		}
		value = value<<4 | digit
		held++
		if held == 2 {
			out = append(out, byte(value))
			value, held = 0, 0
		}
	}
	if held == 1 {
		out = append(out, byte(value<<4))
	}
	return out, nil
}

// decodeRunLength undoes the run-length encoding a stream can be packed in.
//
// A length byte under 128 means that many bytes plus one follow as they are;
// over 128 means the next byte repeats 257 minus the length times; 128 ends it.
func decodeRunLength(data []byte) ([]byte, error) {
	out := make([]byte, 0, len(data)*2)
	for at := 0; at < len(data); {
		length := int(data[at])
		at++
		switch {
		case length == 128:
			return out, nil
		case length < 128:
			end := at + length + 1
			if end > len(data) {
				return nil, errors.New("runlength: a run past the end of the stream")
			}
			out = append(out, data[at:end]...)
			at = end
		default:
			if at >= len(data) {
				return nil, errors.New("runlength: a repeat with nothing to repeat")
			}
			for count := 0; count < 257-length; count++ {
				out = append(out, data[at])
			}
			at++
		}
		if len(out) > maximumStreamBytes {
			return nil, errors.New("runlength: the stream expands past what is allowed")
		}
	}
	return out, nil
}

// decodeLZW undoes the LZW packing older files are written with.
//
// The one thing a reader has to get right here is the early change: by default
// a PDF widens its codes one code sooner than the plain algorithm would, and
// the general-purpose decoders do not, so the output runs correct for a few
// hundred bytes and then turns to noise. Whether it applies is in the stream's
// own DecodeParms.
func decodeLZW(data []byte, earlyChange bool) ([]byte, error) {
	const (
		clearTable = 256
		endOfData  = 257
		firstEntry = 258
	)
	early := 0
	if earlyChange {
		early = 1
	}
	table := make([][]byte, firstEntry, 4096)
	for at := 0; at < 256; at++ {
		table[at] = []byte{byte(at)}
	}
	out := make([]byte, 0, len(data)*4)
	width, held, bits := 9, uint32(0), 0
	var previous []byte
	for _, symbol := range data {
		held = held<<8 | uint32(symbol)
		bits += 8
		for bits >= width {
			code := int(held>>(uint(bits-width))) & ((1 << uint(width)) - 1)
			bits -= width
			switch {
			case code == endOfData:
				return out, nil
			case code == clearTable:
				table = table[:firstEntry]
				width, previous = 9, nil
				continue
			}
			var entry []byte
			switch {
			case code < len(table) && table[code] != nil:
				entry = table[code]
			case previous != nil:
				// The one code a writer may use before it is in the table.
				entry = append(append([]byte{}, previous...), previous[0])
			default:
				return nil, errors.New("lzw: a code that was never defined")
			}
			out = append(out, entry...)
			if len(out) > maximumStreamBytes {
				return nil, errors.New("lzw: the stream expands past what is allowed")
			}
			if previous != nil && len(table) < 4096 {
				table = append(table, append(append([]byte{}, previous...), entry[0]))
			}
			previous = entry
			switch len(table) + early {
			case 512:
				width = 10
			case 1024:
				width = 11
			case 2048:
				width = 12
			}
		}
	}
	return out, nil
}
