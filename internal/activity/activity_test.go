package activity

import (
	"testing"
	"time"
)

func localTime(year int, month time.Month, day, hour int) time.Time {
	return time.Date(year, month, day, hour, 0, 0, 0, time.Local)
}

func TestBuildUsesLocalDatesAndSundayFirstGrid(t *testing.T) {
	year, month := 2026, time.November
	events := []Event{
		{Kind: Note, At: localTime(year, month, 17, 23), Key: "43:3:16", Ref: "John 3:16", Text: "late note"},
	}
	report := Build(events, Options{Year: year, Month: month})

	first := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	wantFirst := first.AddDate(0, 0, -int(first.Weekday()))
	if got := report.Days[0].Date; !sameDate(got, wantFirst) {
		t.Fatalf("first cell = %s, want %s", got, wantFirst)
	}
	if len(report.Weeks)*7 != len(report.Days) {
		t.Fatalf("weeks=%d days=%d", len(report.Weeks), len(report.Days))
	}
	for i, week := range report.Weeks {
		if len(week) != 7 {
			t.Fatalf("week %d has %d days", i, len(week))
		}
	}
	if got := report.Day(localTime(year, month, 17, 8)); got.Total != 1 || !got.InMonth {
		t.Fatalf("day 17 = %+v", got)
	}
}

func TestBuildCountsAndDominantType(t *testing.T) {
	year, month := 2026, time.November
	events := []Event{
		{Kind: Note, At: localTime(year, month, 10, 1)},
		{Kind: Bookmark, At: localTime(year, month, 10, 2)},
		{Kind: Bookmark, At: localTime(year, month, 10, 3)},
		{Kind: History, At: localTime(year, month, 11, 4)},
	}
	report := Build(events, Options{Year: year, Month: month})

	day := report.Day(localTime(year, month, 10, 12))
	if day.Total != 3 || day.Notes != 1 || day.Bookmarks != 2 || day.History != 0 {
		t.Fatalf("day 10 counts = %+v", day)
	}
	if day.Dominant != Bookmark {
		t.Fatalf("dominant = %v, want Bookmark", day.Dominant)
	}
	if day.Level != 2 {
		t.Fatalf("level = %d, want 2", day.Level)
	}
	other := report.Day(localTime(year, month, 11, 12))
	if other.Dominant != History || other.Level != 1 {
		t.Fatalf("day 11 = %+v", other)
	}
	notes, bookmarks, history := report.Totals()
	if notes != 1 || bookmarks != 2 || history != 1 {
		t.Fatalf("totals = %d %d %d", notes, bookmarks, history)
	}
}

func TestBuildSourceFilterAndAdjacentMonths(t *testing.T) {
	year, month := 2026, time.November
	events := []Event{
		{Kind: Note, At: localTime(year, month, 5, 1)},
		{Kind: Bookmark, At: localTime(year, month, 5, 2)},
		{Kind: History, At: localTime(year, time.October, 31, 3)},
	}
	report := Build(events, Options{Year: year, Month: month, Source: Note})

	day := report.Day(localTime(year, month, 5, 12))
	if day.Total != 1 || day.Notes != 1 || day.Bookmarks != 0 {
		t.Fatalf("filtered day = %+v", day)
	}
	outside := report.Day(localTime(year, time.October, 31, 12))
	if outside.Total != 0 {
		t.Fatalf("adjacent-month event leaked into grid: %+v", outside)
	}
}

func TestBuildSkipsZeroTimestamps(t *testing.T) {
	report := Build([]Event{{Kind: Note, Text: "undated"}}, Options{Year: 2026, Month: time.November})
	for _, day := range report.Days {
		if day.Total != 0 {
			t.Fatalf("zero timestamp was counted: %+v", day)
		}
	}
}

func sameDate(a, b time.Time) bool {
	a, b = a.Local(), b.Local()
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}
