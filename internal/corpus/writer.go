package corpus

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// Binary format (version 2), all integers little-endian.
//
//	header (80 bytes)
//	string blob
//	book records       (22 bytes each)
//	chapter records    (12 bytes each)
//	verse records      (48 bytes each)
//	span records       (8 bytes each)
//	note records       (12 bytes each)
//	heading records    (12 bytes each)
//	Strong's records   (8 bytes each)
//
// Version 2 adds Strong's-tagged word spans. The verse record grew from 40 to
// 48 bytes to carry a 32-bit Strong's offset and a 16-bit count; the header
// grew from 72 to 80 bytes to address the new section.
const (
	Magic         = "TBC3"
	FormatVersion = 2
	headerSize    = 80

	bookRecordSize    = 22
	chapterRecordSize = 12
	verseRecordSize   = 48
	spanRecordSize    = 8
	noteRecordSize    = 12
	headingRecordSize = 12
	strongRecordSize  = 8
)

// Write serialises b in the compact corpus format.
func Write(w io.Writer, b *Bible) (int64, error) {
	if err := b.Validate(); err != nil {
		return 0, err
	}

	str := newStringTable()
	books := make([]byte, 0, len(b.Books)*bookRecordSize)
	var buf [12]byte
	for i := range b.Books {
		bk := &b.Books[i]
		nameOff, nameLen := str.add(bk.Name)
		abbrOff, abbrLen := str.add(bk.Abbrev)
		books = append(books,
			byte(bk.ID), byte(bk.ID>>8),
			bk.Testament, 0,
			byte(bk.ChapterCount), byte(bk.ChapterCount>>8),
			byte(bk.FirstChapter), byte(bk.FirstChapter>>8),
		)
		binary.LittleEndian.PutUint32(buf[:4], nameOff)
		binary.LittleEndian.PutUint32(buf[4:8], nameLen)
		binary.LittleEndian.PutUint32(buf[8:12], abbrOff)
		books = append(books, buf[:12]...)
		binary.LittleEndian.PutUint16(buf[:2], uint16(abbrLen))
		books = append(books, buf[:2]...)
	}

	chapters := make([]byte, 0, len(b.Chapters)*chapterRecordSize)
	for _, c := range b.Chapters {
		var r [12]byte
		binary.LittleEndian.PutUint16(r[0:2], c.BookID)
		binary.LittleEndian.PutUint16(r[2:4], c.Number)
		binary.LittleEndian.PutUint32(r[4:8], c.FirstVerse)
		binary.LittleEndian.PutUint16(r[8:10], c.VerseCount)
		chapters = append(chapters, r[:]...)
	}

	verses := make([]byte, 0, len(b.Verses)*verseRecordSize)
	spans := make([]byte, 0, 64*1024)
	notes := make([]byte, 0, 32*1024)
	headings := make([]byte, 0, 16*1024)
	strongs := make([]byte, 0, 320*1024)

	writeSpans := func(list []Span) (uint32, uint16) {
		off := uint32(len(spans))
		for _, s := range list {
			var r [8]byte
			binary.LittleEndian.PutUint32(r[0:4], uint32(s.Start))
			binary.LittleEndian.PutUint32(r[4:8], uint32(s.End))
			spans = append(spans, r[:]...)
		}
		if len(list) > math.MaxUint16 {
			panic("too many spans")
		}
		return off, uint16(len(list))
	}

	writeNotes := func(list []Note) (uint32, uint16) {
		off := uint32(len(notes))
		for _, n := range list {
			var r [12]byte
			binary.LittleEndian.PutUint32(r[0:4], uint32(n.Anchor))
			to, tl := str.add(n.Text)
			binary.LittleEndian.PutUint32(r[4:8], to)
			binary.LittleEndian.PutUint32(r[8:12], tl)
			notes = append(notes, r[:]...)
		}
		if len(list) > math.MaxUint16 {
			panic("too many notes")
		}
		return off, uint16(len(list))
	}

	writeHeadings := func(list []Heading) (uint32, uint16) {
		off := uint32(len(headings))
		for _, h := range list {
			var r [12]byte
			if h.Kind == "title" {
				r[0] = 1
			}
			to, tl := str.add(h.Text)
			binary.LittleEndian.PutUint32(r[4:8], to)
			binary.LittleEndian.PutUint32(r[8:12], tl)
			headings = append(headings, r[:]...)
		}
		if len(list) > math.MaxUint16 {
			panic("too many headings")
		}
		return off, uint16(len(list))
	}

	writeStrongs := func(list []StrongSpan) (uint32, uint16) {
		off := uint32(len(strongs))
		for _, s := range list {
			var r [strongRecordSize]byte
			binary.LittleEndian.PutUint16(r[0:2], uint16(s.Start))
			binary.LittleEndian.PutUint16(r[2:4], uint16(s.End))
			if s.Greek {
				r[4] = 1
			}
			binary.LittleEndian.PutUint16(r[6:8], s.Number)
			strongs = append(strongs, r[:]...)
		}
		if len(list) > math.MaxUint16 {
			panic("too many Strong's spans")
		}
		return off, uint16(len(list))
	}

	for i := range b.Verses {
		v := &b.Verses[i]
		to, tl := str.add(v.Text)
		io_, ic := writeSpans(v.Italics())
		ro, rc := writeSpans(v.RedLetter())
		no, nc := writeNotes(v.Notes())
		ho, hc := writeHeadings(v.Headings())
		so, sc := writeStrongs(v.Strongs())

		var r [verseRecordSize]byte
		binary.LittleEndian.PutUint16(r[0:2], v.BookID)
		binary.LittleEndian.PutUint16(r[2:4], v.Chapter)
		binary.LittleEndian.PutUint16(r[4:6], v.Number)
		r[6] = v.Flags
		binary.LittleEndian.PutUint32(r[8:12], to)
		binary.LittleEndian.PutUint32(r[12:16], tl)
		binary.LittleEndian.PutUint32(r[16:20], io_)
		binary.LittleEndian.PutUint16(r[20:22], ic)
		binary.LittleEndian.PutUint32(r[22:26], ro)
		binary.LittleEndian.PutUint16(r[26:28], rc)
		binary.LittleEndian.PutUint32(r[28:32], no)
		binary.LittleEndian.PutUint16(r[32:34], nc)
		binary.LittleEndian.PutUint32(r[34:38], ho)
		binary.LittleEndian.PutUint16(r[38:40], hc)
		binary.LittleEndian.PutUint32(r[40:44], so)
		binary.LittleEndian.PutUint16(r[44:46], sc)
		verses = append(verses, r[:]...)
	}

	blob := str.bytes()
	off := uint32(headerSize)
	next := func(n int) uint32 {
		o := off
		off += uint32(n)
		off = (off + 7) &^ 7
		return o
	}
	stringOff := next(len(blob))
	booksOff := next(len(books))
	chaptersOff := next(len(chapters))
	versesOff := next(len(verses))
	spansOff := next(len(spans))
	notesOff := next(len(notes))
	headingsOff := next(len(headings))
	strongsOff := next(len(strongs))
	total := off

	hdr := make([]byte, headerSize)
	copy(hdr[0:4], Magic)
	binary.LittleEndian.PutUint16(hdr[4:6], FormatVersion)
	binary.LittleEndian.PutUint16(hdr[6:8], headerSize)
	binary.LittleEndian.PutUint16(hdr[8:10], uint16(len(b.Books)))
	binary.LittleEndian.PutUint16(hdr[10:12], uint16(len(b.Chapters)))
	binary.LittleEndian.PutUint32(hdr[12:16], uint32(len(b.Verses)))
	binary.LittleEndian.PutUint32(hdr[16:20], uint32(len(blob)))
	binary.LittleEndian.PutUint32(hdr[20:24], booksOff)
	binary.LittleEndian.PutUint32(hdr[24:28], uint32(len(books)))
	binary.LittleEndian.PutUint32(hdr[28:32], chaptersOff)
	binary.LittleEndian.PutUint32(hdr[32:36], uint32(len(chapters)))
	binary.LittleEndian.PutUint32(hdr[36:40], versesOff)
	binary.LittleEndian.PutUint32(hdr[40:44], uint32(len(verses)))
	binary.LittleEndian.PutUint32(hdr[44:48], spansOff)
	binary.LittleEndian.PutUint32(hdr[48:52], uint32(len(spans)))
	binary.LittleEndian.PutUint32(hdr[52:56], notesOff)
	binary.LittleEndian.PutUint32(hdr[56:60], uint32(len(notes)))
	binary.LittleEndian.PutUint32(hdr[60:64], headingsOff)
	binary.LittleEndian.PutUint32(hdr[64:68], uint32(len(headings)))
	binary.LittleEndian.PutUint32(hdr[68:72], strongsOff)
	binary.LittleEndian.PutUint32(hdr[72:76], uint32(len(strongs)))
	// hdr[76:80] reserved

	wb := bufio.NewWriterSize(w, 256*1024)
	sections := [][]byte{hdr, blob, books, chapters, verses, spans, notes, headings, strongs}
	// Pad between sections to match the offsets above.
	offsets := []uint32{0, stringOff, booksOff, chaptersOff, versesOff, spansOff, notesOff, headingsOff, strongsOff}
	written := uint32(0)
	for i, sec := range sections {
		if offsets[i] < written {
			return 0, fmt.Errorf("internal: overlapping section %d", i)
		}
		if pad := offsets[i] - written; pad > 0 {
			if _, err := wb.Write(make([]byte, pad)); err != nil {
				return 0, err
			}
			written += pad
		}
		if _, err := wb.Write(sec); err != nil {
			return 0, err
		}
		written += uint32(len(sec))
	}
	if pad := total - written; pad > 0 {
		if _, err := wb.Write(make([]byte, pad)); err != nil {
			return 0, err
		}
	}
	if err := wb.Flush(); err != nil {
		return 0, err
	}
	return int64(total), nil
}

type stringTable struct {
	blob []byte
	seen map[string]uint32
}

func newStringTable() *stringTable {
	return &stringTable{seen: make(map[string]uint32, 40000)}
}

func (t *stringTable) add(s string) (uint32, uint32) {
	if s == "" {
		return 0, 0
	}
	if off, ok := t.seen[s]; ok {
		return off, uint32(len(s))
	}
	off := uint32(len(t.blob))
	t.blob = append(t.blob, s...)
	t.seen[s] = off
	return off, uint32(len(s))
}

func (t *stringTable) bytes() []byte { return t.blob }
