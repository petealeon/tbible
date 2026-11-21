package corpus

import (
	"encoding/binary"
	"fmt"
	"unsafe"
)

// Open parses a corpus blob. The returned Bible owns copies of the text, so
// data may be reused or modified after Open returns.
func Open(data []byte) (*Bible, error) {
	return open(data, false)
}

// OpenBorrowed parses a corpus blob without copying its strings. The returned
// Bible borrows from data, which must remain alive and unmodified for as long
// as the Bible is used. It is intended for compile-time embedded blobs.
func OpenBorrowed(data []byte) (*Bible, error) {
	return open(data, true)
}

func open(data []byte, borrow bool) (*Bible, error) {
	if len(data) < headerSize {
		return nil, fmt.Errorf("corpus: truncated header")
	}
	if string(data[0:4]) != Magic {
		return nil, fmt.Errorf("corpus: bad magic %q", data[0:4])
	}
	version := binary.LittleEndian.Uint16(data[4:6])
	if version != FormatVersion {
		return nil, fmt.Errorf("corpus: unsupported version %d", version)
	}
	if hs := binary.LittleEndian.Uint16(data[6:8]); hs != headerSize {
		return nil, fmt.Errorf("corpus: unsupported header size %d", hs)
	}
	bookCount := int(binary.LittleEndian.Uint16(data[8:10]))
	chapterCount := int(binary.LittleEndian.Uint16(data[10:12]))
	verseCount64 := int64(binary.LittleEndian.Uint32(data[12:16]))
	stringLen64 := int64(binary.LittleEndian.Uint32(data[16:20]))
	if bookCount <= 0 || chapterCount <= 0 || verseCount64 <= 0 {
		return nil, fmt.Errorf("corpus: empty corpus")
	}
	if verseCount64 > int64(len(data))/verseRecordSize {
		return nil, fmt.Errorf("corpus: verse count out of range")
	}
	if stringLen64 < 0 || stringLen64 > int64(len(data))-headerSize {
		return nil, fmt.Errorf("corpus: string blob out of range")
	}
	verseCount := int(verseCount64)
	stringLen := int(stringLen64)

	sections := []struct {
		name  string
		off   int
		size  int
		rec   int
		count int
	}{
		{"books", int(binary.LittleEndian.Uint32(data[20:24])), int(binary.LittleEndian.Uint32(data[24:28])), bookRecordSize, bookCount},
		{"chapters", int(binary.LittleEndian.Uint32(data[28:32])), int(binary.LittleEndian.Uint32(data[32:36])), chapterRecordSize, chapterCount},
		{"verses", int(binary.LittleEndian.Uint32(data[36:40])), int(binary.LittleEndian.Uint32(data[40:44])), verseRecordSize, verseCount},
		{"spans", int(binary.LittleEndian.Uint32(data[44:48])), int(binary.LittleEndian.Uint32(data[48:52])), spanRecordSize, 0},
		{"notes", int(binary.LittleEndian.Uint32(data[52:56])), int(binary.LittleEndian.Uint32(data[56:60])), noteRecordSize, 0},
		{"headings", int(binary.LittleEndian.Uint32(data[60:64])), int(binary.LittleEndian.Uint32(data[64:68])), headingRecordSize, 0},
		{"strongs", int(binary.LittleEndian.Uint32(data[68:72])), int(binary.LittleEndian.Uint32(data[72:76])), strongRecordSize, 0},
	}
	// The string blob follows the header and is not addressed by a separate
	// offset field: every section offset is absolute.
	stringEnd := headerSize + stringLen
	for i, s := range sections {
		if s.off < stringEnd || s.off%8 != 0 || s.size < 0 || s.off > len(data) || s.size > len(data)-s.off {
			return nil, fmt.Errorf("corpus: %s section out of range", s.name)
		}
		if s.rec > 0 && s.size%s.rec != 0 {
			return nil, fmt.Errorf("corpus: %s section has partial record", s.name)
		}
		if i < 3 && int64(s.size) != int64(s.count)*int64(s.rec) {
			return nil, fmt.Errorf("corpus: %s section size mismatch", s.name)
		}
	}
	blob := data[headerSize : headerSize+stringLen]
	str := func(off, size int) (string, error) {
		if off < 0 || size < 0 || off > len(blob) || size > len(blob)-off {
			return "", fmt.Errorf("corpus: string out of range")
		}
		if size == 0 {
			return "", nil
		}
		raw := blob[off : off+size]
		if borrow {
			return unsafe.String(unsafe.SliceData(raw), len(raw)), nil
		}
		return string(raw), nil
	}

	b := &Bible{
		Books:    make([]Book, bookCount),
		Chapters: make([]Chapter, chapterCount),
		Verses:   make([]Verse, verseCount),
	}

	bookSec := data[sections[0].off : sections[0].off+sections[0].size]
	for i := 0; i < bookCount; i++ {
		r := bookSec[i*bookRecordSize:]
		bk := &b.Books[i]
		bk.ID = binary.LittleEndian.Uint16(r[0:2])
		bk.Testament = r[2]
		bk.ChapterCount = binary.LittleEndian.Uint16(r[4:6])
		bk.FirstChapter = binary.LittleEndian.Uint16(r[6:8])
		var err error
		bk.Name, err = str(int(binary.LittleEndian.Uint32(r[8:12])), int(binary.LittleEndian.Uint32(r[12:16])))
		if err != nil {
			return nil, fmt.Errorf("corpus: book %d name: %w", i, err)
		}
		bk.Abbrev, err = str(int(binary.LittleEndian.Uint32(r[16:20])), int(binary.LittleEndian.Uint16(r[20:22])))
		if err != nil {
			return nil, fmt.Errorf("corpus: book %d abbrev: %w", i, err)
		}
	}

	chapterSec := data[sections[1].off : sections[1].off+sections[1].size]
	for i := 0; i < chapterCount; i++ {
		r := chapterSec[i*chapterRecordSize:]
		b.Chapters[i] = Chapter{
			BookID:     binary.LittleEndian.Uint16(r[0:2]),
			Number:     binary.LittleEndian.Uint16(r[2:4]),
			FirstVerse: binary.LittleEndian.Uint32(r[4:8]),
			VerseCount: binary.LittleEndian.Uint16(r[8:10]),
		}
	}

	verseSec := data[sections[2].off : sections[2].off+sections[2].size]
	spanSec := data[sections[3].off : sections[3].off+sections[3].size]
	noteSec := data[sections[4].off : sections[4].off+sections[4].size]
	headSec := data[sections[5].off : sections[5].off+sections[5].size]
	strongSec := data[sections[6].off : sections[6].off+sections[6].size]

	// First pass: validate record ranges and size the flat decoration arrays.
	type decorCounts struct {
		italics, red, notes, headings, strongs int
	}
	counts := make([]decorCounts, verseCount)
	var total decorCounts
	for i := 0; i < verseCount; i++ {
		r := verseSec[i*verseRecordSize:]
		c := decorCounts{
			italics:  int(binary.LittleEndian.Uint16(r[20:22])),
			red:      int(binary.LittleEndian.Uint16(r[26:28])),
			notes:    int(binary.LittleEndian.Uint16(r[32:34])),
			headings: int(binary.LittleEndian.Uint16(r[38:40])),
			strongs:  int(binary.LittleEndian.Uint16(r[44:46])),
		}
		io := int(binary.LittleEndian.Uint32(r[16:20]))
		ro := int(binary.LittleEndian.Uint32(r[22:26]))
		no := int(binary.LittleEndian.Uint32(r[28:32]))
		ho := int(binary.LittleEndian.Uint32(r[34:38]))
		so := int(binary.LittleEndian.Uint32(r[40:44]))
		if !withinRecords(io, c.italics, spanRecordSize, len(spanSec)) ||
			!withinRecords(ro, c.red, spanRecordSize, len(spanSec)) ||
			!withinRecords(no, c.notes, noteRecordSize, len(noteSec)) ||
			!withinRecords(ho, c.headings, headingRecordSize, len(headSec)) ||
			!withinRecords(so, c.strongs, strongRecordSize, len(strongSec)) {
			return nil, fmt.Errorf("corpus: verse %d decorations out of range", i)
		}
		counts[i] = c
		total.italics += c.italics
		total.red += c.red
		total.notes += c.notes
		total.headings += c.headings
		total.strongs += c.strongs
	}
	if total.italics > 0xffff || total.red > 0xffff || total.notes > 0xffff || total.headings > 0xffff {
		return nil, fmt.Errorf("corpus: too many decoration records")
	}
	b.decor = decorations{
		italics:   make([]Span, total.italics),
		redLetter: make([]Span, total.red),
		notes:     make([]Note, total.notes),
		headings:  make([]Heading, total.headings),
		strongs:   make([]StrongSpan, total.strongs),
	}
	b.decorBuilt = true

	// Second pass: decode into the flat arrays and point each verse at its slice.
	var oi, or, on, oh, os int
	for i := 0; i < verseCount; i++ {
		r := verseSec[i*verseRecordSize:]
		v := &b.Verses[i]
		v.BookID = binary.LittleEndian.Uint16(r[0:2])
		v.Chapter = binary.LittleEndian.Uint16(r[2:4])
		v.Number = binary.LittleEndian.Uint16(r[4:6])
		v.Flags = r[6]
		var err error
		v.Text, err = str(int(binary.LittleEndian.Uint32(r[8:12])), int(binary.LittleEndian.Uint32(r[12:16])))
		if err != nil {
			return nil, fmt.Errorf("corpus: verse %d text: %w", i, err)
		}
		c := counts[i]
		v.decor = &b.decor
		v.italOff, v.italLen = uint16(oi), uint16(c.italics)
		v.redOff, v.redLen = uint16(or), uint16(c.red)
		v.noteOff, v.noteLen = uint16(on), uint16(c.notes)
		v.headOff, v.headLen = uint16(oh), uint16(c.headings)
		v.strongOff, v.strongLen = uint32(os), uint16(c.strongs)

		if err := readSpansInto(spanSec, int(binary.LittleEndian.Uint32(r[16:20])), c.italics, len(v.Text), b.decor.italics[oi:oi+c.italics]); err != nil {
			return nil, fmt.Errorf("corpus: verse %d italics: %w", i, err)
		}
		if err := readSpansInto(spanSec, int(binary.LittleEndian.Uint32(r[22:26])), c.red, len(v.Text), b.decor.redLetter[or:or+c.red]); err != nil {
			return nil, fmt.Errorf("corpus: verse %d red letter: %w", i, err)
		}
		noff := int(binary.LittleEndian.Uint32(r[28:32]))
		for j := 0; j < c.notes; j++ {
			nr := noteSec[noff+j*noteRecordSize:]
			text, err := str(int(binary.LittleEndian.Uint32(nr[4:8])), int(binary.LittleEndian.Uint32(nr[8:12])))
			if err != nil {
				return nil, fmt.Errorf("corpus: verse %d note text: %w", i, err)
			}
			b.decor.notes[on+j] = Note{
				Anchor: int(binary.LittleEndian.Uint32(nr[0:4])),
				Text:   text,
			}
		}
		hoff := int(binary.LittleEndian.Uint32(r[34:38]))
		for j := 0; j < c.headings; j++ {
			hr := headSec[hoff+j*headingRecordSize:]
			text, err := str(int(binary.LittleEndian.Uint32(hr[4:8])), int(binary.LittleEndian.Uint32(hr[8:12])))
			if err != nil {
				return nil, fmt.Errorf("corpus: verse %d heading text: %w", i, err)
			}
			kind := "section"
			if hr[0] == 1 {
				kind = "title"
			}
			b.decor.headings[oh+j] = Heading{Text: text, Kind: kind}
		}
		soff := int(binary.LittleEndian.Uint32(r[40:44]))
		for j := 0; j < c.strongs; j++ {
			sr := strongSec[soff+j*strongRecordSize:]
			start := int(binary.LittleEndian.Uint16(sr[0:2]))
			end := int(binary.LittleEndian.Uint16(sr[2:4]))
			if start < 0 || end <= start || end > len(v.Text) {
				return nil, fmt.Errorf("corpus: verse %d Strong's span out of bounds", i)
			}
			b.decor.strongs[os+j] = StrongSpan{
				Start:  start,
				End:    end,
				Number: binary.LittleEndian.Uint16(sr[6:8]),
				Greek:  sr[4] == 1,
			}
		}
		oi += c.italics
		or += c.red
		on += c.notes
		oh += c.headings
		os += c.strongs
	}

	if err := b.Validate(); err != nil {
		return nil, err
	}
	return b, nil
}

func withinRecords(off, count, rec, total int) bool {
	if off < 0 || count < 0 || rec <= 0 || off > total {
		return false
	}
	return count <= (total-off)/rec
}

func readSpansInto(sec []byte, off, count, textLen int, out []Span) error {
	if count == 0 {
		return nil
	}
	if !withinRecords(off, count, spanRecordSize, len(sec)) {
		return fmt.Errorf("span range out of bounds")
	}
	for i := 0; i < count; i++ {
		r := sec[off+i*spanRecordSize:]
		start := int(binary.LittleEndian.Uint32(r[0:4]))
		end := int(binary.LittleEndian.Uint32(r[4:8]))
		if start < 0 || end < start || end > textLen {
			return fmt.Errorf("span %d out of text bounds", i)
		}
		out[i] = Span{Start: start, End: end}
	}
	return nil
}
