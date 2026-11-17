package tui

// actionHints spells out the verse actions instead of relying on the cryptic
// y/b/n shorthand. Narrow terminals keep the actions first and shorten only
// the bookmark label.
func actionHints(width int) string {
	if width >= 100 {
		return "y copy · b bookmark · n note"
	}
	return "y copy · b mark · n note"
}
