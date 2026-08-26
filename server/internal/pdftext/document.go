package pdftext

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"
	"regexp"
	"strconv"
)

// document is a PDF opened far enough to find its pages.
//
// The cross-reference table is not read: a file that has been edited has
// several of them, a linearised one hides the first, and an object stream holds
// objects that no table points at directly. Scanning for every "N G obj" in the
// file finds all of them in one pass, and a later definition of the same number
// wins — which is what an incremental update means.
type document struct {
	data    []byte
	objects map[int]value
	streams map[int]stream
	// unpacked holds what has already been decoded, and spent is how much of
	// the document's budget that took. A PDF is an upload from whoever sends
	// one, and a few hundred kilobytes of it can ask for gigabytes back.
	unpacked map[int][]byte
	spent    int
	// charted holds the parsed form of a ToUnicode map, which every page using
	// the font would otherwise re-read: a long file names the same twelve
	// thousand entry CMap on all of its pages.
	charted map[int]map[uint32]string
}

var objectHeader = regexp.MustCompile(`(?m)(\d+)\s+(\d+)\s+obj\b`)

const (
	maximumBytes = 64 << 20
	// maximumDecoded is what one document may unpack in total, and
	// maximumStreamBytes what any one stream may come to. The corpus this was
	// measured against unpacks at most 6 MiB for a whole 127-page statement and
	// 1.9 MiB for its heaviest single page, so both bounds are far above any
	// file meant to be read and far below what a crafted one asks for.
	maximumDecoded     = 64 << 20
	maximumStreamBytes = 16 << 20
	// maximumPageBytes is what one page's content may come to. A page of text
	// is kilobytes; a page that asks for more than this is asking for something
	// other than to be read.
	maximumPageBytes = 8 << 20
)

func open(data []byte) (*document, error) {
	if len(data) == 0 {
		return nil, errors.New("this file is empty")
	}
	if len(data) > maximumBytes {
		return nil, errors.New("this PDF is larger than Ptium reads")
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) && !bytes.Contains(data[:min(len(data), 1024)], []byte("%PDF-")) {
		return nil, errors.New("this file is not a PDF")
	}
	doc := &document{data: data, objects: map[int]value{}, streams: map[int]stream{},
		unpacked: map[int][]byte{}, charted: map[int]map[uint32]string{}}
	doc.scan()
	doc.expandObjectStreams()
	return doc, nil
}

// scan reads every object the file defines, in file order.
func (d *document) scan() {
	for _, place := range objectHeader.FindAllSubmatchIndex(d.data, -1) {
		number, err := strconv.Atoi(string(d.data[place[2]:place[3]]))
		if err != nil {
			continue
		}
		reader := &lexer{data: d.data, at: place[1]}
		object, err := reader.object()
		if err != nil {
			continue
		}
		// A dictionary followed by "stream" owns the bytes after it.
		if entries, ok := object.(dict); ok {
			save := reader.at
			if word := reader.token(); word == "stream" {
				if raw, end, ok := d.streamBytes(entries, reader.at); ok {
					d.streams[number] = stream{dict: entries, raw: raw}
					d.objects[number] = entries
					_ = end
					continue
				}
			}
			reader.at = save
		}
		d.objects[number] = object
	}
}

// streamBytes takes the bytes of a stream, using its /Length when that can be
// resolved and the endstream marker when it cannot — a length that is itself a
// reference is common, and a wrong one is worse than looking for the marker.
func (d *document) streamBytes(entries dict, after int) ([]byte, int, bool) {
	start := after
	for start < len(d.data) && (d.data[start] == '\r' || d.data[start] == '\n') {
		start++
	}
	end := bytes.Index(d.data[start:], []byte("endstream"))
	if end < 0 {
		return nil, 0, false
	}
	raw := d.data[start : start+end]
	// The marker is preceded by the newline that ends the data.
	raw = bytes.TrimSuffix(raw, []byte("\n"))
	raw = bytes.TrimSuffix(raw, []byte("\r"))
	return raw, start + end, true
}

// resolve follows a reference to the object it names.
func (d *document) resolve(item value) value {
	for depth := 0; depth < 8; depth++ {
		pointer, ok := item.(ref)
		if !ok {
			return item
		}
		next, ok := d.objects[pointer.number]
		if !ok {
			return nil
		}
		item = next
	}
	return item
}

func (d *document) dict(item value) dict {
	entries, _ := d.resolve(item).(dict)
	return entries
}

func (d *document) array(item value) array {
	items, _ := d.resolve(item).(array)
	return items
}

func (d *document) name(item value) name {
	said, _ := d.resolve(item).(name)
	return said
}

func (d *document) number(item value) (float64, bool) {
	number, ok := d.resolve(item).(float64)
	return number, ok
}

// decoded is a stream's bytes with its filters undone. Only Flate is supported,
// which is what every producer of the files this was measured against uses.
// decoded is a stream's bytes, unpacked once and remembered.
//
// The same map from codes to characters is asked for by every page that uses
// the font, and unpacking a 170 KiB CMap a hundred and twenty-seven times is
// most of the time a long file takes. Remembering it also bounds what a
// document can be made to unpack: a hundred kilobytes of well-chosen zlib
// expands to gigabytes, and one page is free to name the same stream sixty
// times.
func (d *document) decoded(number int) ([]byte, bool) {
	if said, ok := d.unpacked[number]; ok {
		return said, said != nil
	}
	held, ok := d.streams[number]
	if !ok {
		return nil, false
	}
	if d.spent >= maximumDecoded {
		d.unpacked[number] = nil
		return nil, false
	}
	said, ok := d.decodeStream(held)
	if !ok {
		d.unpacked[number] = nil
		return nil, false
	}
	d.spent += len(said)
	d.unpacked[number] = said
	return said, true
}

func (d *document) decodeStream(held stream) ([]byte, bool) {
	filters := []name{}
	switch filter := d.resolve(held.dict["Filter"]).(type) {
	case name:
		filters = append(filters, filter)
	case array:
		for _, item := range filter {
			if said, ok := d.resolve(item).(name); ok {
				filters = append(filters, said)
			}
		}
	}
	data := held.raw
	for _, filter := range filters {
		switch filter {
		case "FlateDecode", "Fl":
			reader, err := zlib.NewReader(bytes.NewReader(data))
			if err != nil {
				return nil, false
			}
			expanded, err := io.ReadAll(io.LimitReader(reader, maximumStreamBytes))
			reader.Close()
			if err != nil && len(expanded) == 0 {
				return nil, false
			}
			data = expanded
		default:
			// An image or a font in a filter this does not undo is not text.
			return nil, false
		}
	}
	if predictor := d.dict(held.dict["DecodeParms"]); predictor != nil {
		if columns, ok := d.number(predictor["Columns"]); ok {
			if kind, ok := d.number(predictor["Predictor"]); ok && kind >= 10 {
				data = undoPNGPredictor(data, int(columns))
			}
		}
	}
	return data, true
}

// undoPNGPredictor reverses the row filtering an xref stream is written with.
func undoPNGPredictor(data []byte, columns int) []byte {
	if columns <= 0 {
		return data
	}
	stride := columns + 1
	rows := len(data) / stride
	out := make([]byte, 0, rows*columns)
	previous := make([]byte, columns)
	for row := 0; row < rows; row++ {
		line := data[row*stride : (row+1)*stride]
		filter, current := line[0], append([]byte{}, line[1:]...)
		for index := range current {
			left := byte(0)
			if index >= 1 {
				left = current[index-1]
			}
			up := previous[index]
			switch filter {
			case 1:
				current[index] += left
			case 2:
				current[index] += up
			case 3:
				current[index] += byte((int(left) + int(up)) / 2)
			case 4:
				current[index] += paeth(left, up, upperLeft(previous, index))
			}
		}
		out = append(out, current...)
		previous = current
	}
	return out
}

func upperLeft(previous []byte, index int) byte {
	if index == 0 {
		return 0
	}
	return previous[index-1]
}

func paeth(left, up, upLeft byte) byte {
	estimate := int(left) + int(up) - int(upLeft)
	dl, du, dul := abs(estimate-int(left)), abs(estimate-int(up)), abs(estimate-int(upLeft))
	switch {
	case dl <= du && dl <= dul:
		return left
	case du <= dul:
		return up
	default:
		return upLeft
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

// expandObjectStreams reads the objects that live compressed inside another
// object, which is where most of a modern PDF's dictionaries are.
func (d *document) expandObjectStreams() {
	for number, held := range d.streams {
		if d.name(held.dict["Type"]) != "ObjStm" {
			continue
		}
		data, ok := d.decoded(number)
		if !ok {
			continue
		}
		count, _ := d.number(held.dict["N"])
		first, _ := d.number(held.dict["First"])
		if count <= 0 || int(first) > len(data) {
			continue
		}
		header := &lexer{data: data[:int(first)]}
		type entry struct{ number, offset int }
		entries := make([]entry, 0, int(count))
		for index := 0; index < int(count); index++ {
			id, err := strconv.Atoi(header.token())
			if err != nil {
				break
			}
			offset, err := strconv.Atoi(header.token())
			if err != nil {
				break
			}
			entries = append(entries, entry{number: id, offset: offset})
		}
		for _, item := range entries {
			at := int(first) + item.offset
			if at < 0 || at >= len(data) {
				continue
			}
			reader := &lexer{data: data, at: at}
			object, err := reader.object()
			if err != nil {
				continue
			}
			// A definition in the file itself is a later edit and wins.
			if _, taken := d.objects[item.number]; !taken {
				d.objects[item.number] = object
			}
		}
	}
}
