package pdf

import (
	"bytes"
	"fmt"
	"strings"
)

// Bytes writes the document out.
//
// Objects are numbered as they are written and the cross-reference table is
// built from where each one landed, which is the whole of what a PDF reader
// needs to find them again.
func (d *Document) Bytes() []byte {
	var out bytes.Buffer
	offsets := []int{0}
	object := func(body string) int {
		number := len(offsets)
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", number, body)
		return number
	}
	stream := func(dictionary string, data []byte) int {
		number := len(offsets)
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n<< %s /Length %d >>\nstream\n", number, dictionary, len(data))
		out.Write(data)
		out.WriteString("\nendstream\nendobj\n")
		return number
	}
	out.WriteString("%PDF-1.7\n%\xE2\xE3\xCF\xD3\n")

	fontObject := d.writeFont(object, stream)
	imageObjects := make(map[string]int, len(d.images))
	for _, image := range d.images {
		dictionary := fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /%s /BitsPerComponent %d",
			image.width, image.height, image.colorSpace, image.bits)
		if image.filter != "" {
			dictionary += " /Filter /" + image.filter
		}
		if len(image.softMask) > 0 {
			mask := stream(fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /FlateDecode",
				image.width, image.height), deflate(image.softMask))
			dictionary += fmt.Sprintf(" /SMask %d 0 R", mask)
		}
		imageObjects[image.name] = stream(dictionary, image.data)
	}

	shadingObjects := make(map[string]int, len(d.shadings))
	for _, wash := range d.shadings {
		fromRed, fromGreen, fromBlue := rgb(wash.from)
		toRed, toGreen, toBlue := rgb(wash.to)
		shadingObjects[wash.name] = object(fmt.Sprintf(
			"<< /ShadingType 2 /ColorSpace /DeviceRGB /Coords [%s %s %s %s] /Extend [true true] "+
				"/Function << /FunctionType 2 /Domain [0 1] /C0 [%s %s %s] /C1 [%s %s %s] /N 1 >> >>",
			number(wash.x1), number(wash.y1), number(wash.x2), number(wash.y2),
			number(fromRed), number(fromGreen), number(fromBlue),
			number(toRed), number(toGreen), number(toBlue)))
	}

	// The page tree is written after its pages, so each page can name it: the
	// number is known before anything is written because it is allocated here.
	pagesObject := len(offsets) + len(d.pages)*2 + d.annotationCount()
	pageObjects := make([]int, 0, len(d.pages))
	for _, page := range d.pages {
		content := stream("/Filter /FlateDecode", deflate(page.content.Bytes()))
		annotations := ""
		if len(page.links) > 0 {
			var ids []string
			for _, one := range page.links {
				// A link that names neither a page in this document nor an
				// address is not written at all: it is nothing to click.
				written := d.annotation(page, one)
				if written == "" {
					continue
				}
				ids = append(ids, fmt.Sprintf("%d 0 R", object(written)))
			}
			if len(ids) > 0 {
				annotations = " /Annots [" + strings.Join(ids, " ") + "]"
			}
		}
		// A page names what it draws with. Listing every picture in the deck on
		// every page is not wrong, and it is not what the page is.
		resources := fmt.Sprintf("<< /Font << /%s %d 0 R >>", d.fontName, fontObject)
		if len(page.images) > 0 {
			var xobjects []string
			for _, name := range page.images {
				xobjects = append(xobjects, fmt.Sprintf("/%s %d 0 R", name, imageObjects[name]))
			}
			resources += " /XObject << " + strings.Join(xobjects, " ") + " >>"
		}
		if len(page.shadings) > 0 {
			var washes []string
			for _, name := range page.shadings {
				washes = append(washes, fmt.Sprintf("/%s %d 0 R", name, shadingObjects[name]))
			}
			resources += " /Shading << " + strings.Join(washes, " ") + " >>"
		}
		resources += " >>"
		pageObjects = append(pageObjects, object(fmt.Sprintf(
			"<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %s %s] /Resources %s /Contents %d 0 R%s >>",
			pagesObject, number(d.Width), number(d.Height), resources, content, annotations)))
	}
	var kids []string
	for _, id := range pageObjects {
		kids = append(kids, fmt.Sprintf("%d 0 R", id))
	}
	pages := object(fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", len(pageObjects), strings.Join(kids, " ")))
	catalog := object(fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pages))
	info := object(fmt.Sprintf("<< /Title %s /Producer (Ptium) >>", literal(d.Title)))

	start := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root %d 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(offsets), catalog, info, start)
	return out.Bytes()
}

// annotationCount is how many objects the links will take, so the page tree's
// own number can be worked out before any of them are written.
func (d *Document) annotationCount() int {
	count := 0
	for _, page := range d.pages {
		count += len(page.links)
	}
	return count
}

func (d *Document) annotation(page *Page, one link) string {
	rectangle := fmt.Sprintf("[%s %s %s %s]", number(one.x), number(page.flip(one.y+one.height)),
		number(one.x+one.width), number(page.flip(one.y)))
	// A link inside the document names a page; one that leaves it names a URI.
	if one.page > 0 && one.page <= len(d.pages) {
		return fmt.Sprintf("<< /Type /Annot /Subtype /Link /Rect %s /Border [0 0 0] /Dest [%d /Fit] >>",
			rectangle, one.page-1)
	}
	if strings.TrimSpace(one.target) == "" {
		// A link that names neither a page in this document nor an address is
		// not a link. Written anyway it became /URI (), an annotation a reader
		// will happily let somebody click on its way to nowhere.
		return ""
	}
	return fmt.Sprintf("<< /Type /Annot /Subtype /Link /Rect %s /Border [0 0 0] /A << /Type /Action /S /URI /URI %s >> >>",
		rectangle, uriLiteral(one.target))
}

// uriLiteral writes an address. A URI in a PDF is an ASCII string — the format
// says so, and a reader that has to guess at UTF-16 for a link is a reader that
// may open the wrong thing. A character outside ASCII is percent-encoded, which
// is what a URL does with them anyway.
func uriLiteral(value string) string {
	var out bytes.Buffer
	out.WriteByte('(')
	for _, part := range []byte(value) {
		switch {
		case part == '(' || part == ')' || part == '\\':
			out.WriteByte('\\')
			out.WriteByte(part)
		case part < 0x20 || part > 0x7E:
			fmt.Fprintf(&out, "%%%02X", part)
		default:
			out.WriteByte(part)
		}
	}
	out.WriteByte(')')
	return out.String()
}

// literal writes a PDF string. Text goes out as UTF-16 so a Korean title
// survives; the escapes are what the format asks for.
func literal(value string) string {
	var out bytes.Buffer
	out.WriteString("(\xFE\xFF")
	for _, character := range value {
		units := []uint16{uint16(character)}
		if character > 0xFFFF {
			character -= 0x10000
			units = []uint16{uint16(0xD800 + (character >> 10)), uint16(0xDC00 + (character & 0x3FF))}
		}
		for _, unit := range units {
			for _, part := range []byte{byte(unit >> 8), byte(unit)} {
				switch part {
				case '(', ')', '\\':
					out.WriteByte('\\')
				}
				out.WriteByte(part)
			}
		}
	}
	out.WriteString(")")
	return out.String()
}
