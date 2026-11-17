// Package activity groups timestamped user events into calendar days. It is
// intentionally independent of the store and TUI so date grouping can be
// tested without a terminal or an on-disk state file.
package activity

import "time"

// Kind identifies one source of dated activity. Values are bit flags so a
// single value can also represent a set of sources.
type Kind uint8

const (
	// Note is a user note attached to a verse.
	Note Kind = 1 << iota
	// Bookmark is a saved verse location.
	Bookmark
	// History is a recently read verse.
	History

	// NoKind means there is no activity.
	NoKind Kind = 0
	// SourceAll includes every dated source.
	SourceAll = Note | Bookmark | History
)

// Label returns a user-facing name for a single source.
func (k Kind) Label() string {
	switch k {
	case Note:
		return "Note"
	case Bookmark:
		return "Bookmark"
	case History:
		return "History"
	default:
		return "Activity"
	}
}

// Short returns a compact source marker.
func (k Kind) Short() string {
	switch k {
	case Note:
		return "N"
	case Bookmark:
		return "B"
	case History:
		return "H"
	default:
		return "·"
	}
}

// Event is one timestamped item from the user's library.
type Event struct {
	Kind Kind
	At   time.Time
	Key  string
	Ref  string
	Text string
}

// Day is one cell in a Sunday-first month grid.
type Day struct {
	Date      time.Time
	InMonth   bool
	Total     int
	Notes     int
	Bookmarks int
	History   int
	Level     int
	Dominant  Kind
	Events    []Event
}

// Month is a complete Sunday-first calendar grid for one month.
type Month struct {
	Year  int
	Month time.Month
	Weeks [][]Day
	Days  []Day
}

// Options controls which events are counted and which month is built.
type Options struct {
	Year   int
	Month  time.Month
	Source Kind
}

// DaysInMonth returns the number of days in the given month.
func DaysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.Local).Day()
}

type dateKey struct {
	year  int
	month time.Month
	day   int
}

func keyOf(t time.Time) dateKey {
	t = t.Local()
	return dateKey{t.Year(), t.Month(), t.Day()}
}

// Build groups events into a Sunday-first grid. Events outside the displayed
// month are ignored, and events with a zero timestamp are skipped because old
// state files may predate timestamps.
func Build(events []Event, opts Options) Month {
	now := time.Now().Local()
	if opts.Year == 0 {
		opts.Year = now.Year()
	}
	if opts.Month == 0 {
		opts.Month = now.Month()
	}
	source := opts.Source
	if source == 0 {
		source = SourceAll
	}

	first := time.Date(opts.Year, opts.Month, 1, 0, 0, 0, 0, time.Local)
	offset := int(first.Weekday())
	cells := ((offset + DaysInMonth(opts.Year, opts.Month) + 6) / 7) * 7
	days := make([]Day, 0, cells)
	byDate := make(map[dateKey]*Day, cells)
	for i := 0; i < cells; i++ {
		d := first.AddDate(0, 0, i-offset)
		day := Day{
			Date:    d,
			InMonth: d.Year() == opts.Year && d.Month() == opts.Month,
		}
		days = append(days, day)
		byDate[keyOf(d)] = &days[len(days)-1]
	}

	for _, event := range events {
		if event.At.IsZero() || event.Kind&source == 0 {
			continue
		}
		day := byDate[keyOf(event.At)]
		if day == nil || !day.InMonth {
			continue
		}
		switch event.Kind {
		case Note:
			day.Notes++
		case Bookmark:
			day.Bookmarks++
		case History:
			day.History++
		}
		day.Events = append(day.Events, event)
	}

	for i := range days {
		day := &days[i]
		day.Total = day.Notes + day.Bookmarks + day.History
		day.Dominant = dominant(day)
		day.Level = level(day.Total)
	}

	month := Month{Year: opts.Year, Month: opts.Month, Days: days}
	for i := 0; i < len(days); i += 7 {
		end := min(i+7, len(days))
		month.Weeks = append(month.Weeks, days[i:end])
	}
	return month
}

func dominant(day *Day) Kind {
	best, bestCount := NoKind, 0
	for _, kind := range []Kind{Note, Bookmark, History} {
		count := 0
		switch kind {
		case Note:
			count = day.Notes
		case Bookmark:
			count = day.Bookmarks
		case History:
			count = day.History
		}
		if count > bestCount {
			best, bestCount = kind, count
		}
	}
	return best
}

func level(total int) int {
	switch {
	case total <= 0:
		return 0
	case total == 1:
		return 1
	case total <= 3:
		return 2
	case total <= 6:
		return 3
	default:
		return 4
	}
}

// Day returns the cell for a local date, or an empty cell when the date is
// outside the displayed month.
func (m Month) Day(date time.Time) Day {
	key := keyOf(date)
	for _, day := range m.Days {
		if day.InMonth && keyOf(day.Date) == key {
			return day
		}
	}
	return Day{Date: date}
}

// Totals returns the month's counts by source.
func (m Month) Totals() (notes, bookmarks, history int) {
	for _, day := range m.Days {
		if !day.InMonth {
			continue
		}
		notes += day.Notes
		bookmarks += day.Bookmarks
		history += day.History
	}
	return notes, bookmarks, history
}
