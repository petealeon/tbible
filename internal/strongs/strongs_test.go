package strongs

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseTag(t *testing.T) {
	cases := []struct {
		tag   string
		greek bool
		num   uint16
		ok    bool
	}{
		{"H7225", false, 7225, true},
		{"g3056", true, 3056, true},
		{`H7225G`, false, 7225, true},
		{" H0012 ", false, 12, true},
		{"H0", false, 0, false},
		{"X123", false, 0, false},
		{"", false, 0, false},
	}
	for _, tc := range cases {
		greek, num, ok := ParseTag(tc.tag)
		if greek != tc.greek || num != tc.num || ok != tc.ok {
			t.Errorf("ParseTag(%q) = %v, %d, %v", tc.tag, greek, num, ok)
		}
	}
}

func TestDictionaryRoundTrip(t *testing.T) {
	d := &Dictionary{m: map[uint32]Entry{
		key(false, 7225): {Number: 7225, Lemma: "bərēʾšîṯ", Gloss: "beginning"},
		key(true, 3056):  {Number: 3056, Greek: true, Gloss: "word"},
	}}
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Len() != 2 {
		t.Fatalf("Len = %d", parsed.Len())
	}
	if e, ok := parsed.LookupTag("H7225G"); !ok || e.Gloss != "beginning" || e.Tag() != "H7225" {
		t.Fatalf("LookupTag(H7225G) = %+v, %v", e, ok)
	}
	if e, ok := parsed.Lookup(true, 3056); !ok || e.Tag() != "G3056" {
		t.Fatalf("Lookup(G3056) = %+v, %v", e, ok)
	}
	if _, ok := parsed.Lookup(true, 7225); ok {
		t.Fatal("Hebrew number resolved as Greek")
	}
}

func TestCleanDefinition(t *testing.T) {
	got := cleanDefinition("  <p>A <b>word</b></p>  &amp; more   space")
	for _, want := range []string{"A word", "& more space"} {
		if !strings.Contains(got, want) {
			t.Fatalf("cleanDefinition = %q, want to contain %q", got, want)
		}
	}
	if strings.Contains(got, "<b>") {
		t.Fatalf("HTML tag survived: %q", got)
	}
}
