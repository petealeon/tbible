package corpus

// BookInfo describes one canonical book. Chapters and Verses are the
// expected counts from the 1769 KJV; tbgen validates the imported text
// against them.
type BookInfo struct {
	ID        uint16
	Name      string
	Abbrev    string
	USFM      string
	Testament uint8 // 0 = Old, 1 = New
	Chapters  uint16
	Verses    uint32
}

// Canonical is the 66-book Protestant canon in order.
var Canonical = []BookInfo{
	{1, "Genesis", "Gen", "GEN", 0, 50, 1533},
	{2, "Exodus", "Exod", "EXO", 0, 40, 1213},
	{3, "Leviticus", "Lev", "LEV", 0, 27, 859},
	{4, "Numbers", "Num", "NUM", 0, 36, 1288},
	{5, "Deuteronomy", "Deut", "DEU", 0, 34, 959},
	{6, "Joshua", "Josh", "JOS", 0, 24, 658},
	{7, "Judges", "Judg", "JDG", 0, 21, 618},
	{8, "Ruth", "Ruth", "RUT", 0, 4, 85},
	{9, "1 Samuel", "1 Sam", "1SA", 0, 31, 810},
	{10, "2 Samuel", "2 Sam", "2SA", 0, 24, 695},
	{11, "1 Kings", "1 Kgs", "1KI", 0, 22, 816},
	{12, "2 Kings", "2 Kgs", "2KI", 0, 25, 719},
	{13, "1 Chronicles", "1 Chr", "1CH", 0, 29, 942},
	{14, "2 Chronicles", "2 Chr", "2CH", 0, 36, 822},
	{15, "Ezra", "Ezra", "EZR", 0, 10, 280},
	{16, "Nehemiah", "Neh", "NEH", 0, 13, 406},
	{17, "Esther", "Esth", "EST", 0, 10, 167},
	{18, "Job", "Job", "JOB", 0, 42, 1070},
	{19, "Psalms", "Ps", "PSA", 0, 150, 2461},
	{20, "Proverbs", "Prov", "PRO", 0, 31, 915},
	{21, "Ecclesiastes", "Eccl", "ECC", 0, 12, 222},
	{22, "Song of Solomon", "Song", "SNG", 0, 8, 117},
	{23, "Isaiah", "Isa", "ISA", 0, 66, 1292},
	{24, "Jeremiah", "Jer", "JER", 0, 52, 1364},
	{25, "Lamentations", "Lam", "LAM", 0, 5, 154},
	{26, "Ezekiel", "Ezek", "EZK", 0, 48, 1273},
	{27, "Daniel", "Dan", "DAN", 0, 12, 357},
	{28, "Hosea", "Hos", "HOS", 0, 14, 197},
	{29, "Joel", "Joel", "JOL", 0, 3, 73},
	{30, "Amos", "Amos", "AMO", 0, 9, 146},
	{31, "Obadiah", "Obad", "OBA", 0, 1, 21},
	{32, "Jonah", "Jonah", "JON", 0, 4, 48},
	{33, "Micah", "Mic", "MIC", 0, 7, 105},
	{34, "Nahum", "Nah", "NAM", 0, 3, 47},
	{35, "Habakkuk", "Hab", "HAB", 0, 3, 56},
	{36, "Zephaniah", "Zeph", "ZEP", 0, 3, 53},
	{37, "Haggai", "Hag", "HAG", 0, 2, 38},
	{38, "Zechariah", "Zech", "ZEC", 0, 14, 211},
	{39, "Malachi", "Mal", "MAL", 0, 4, 55},
	{40, "Matthew", "Matt", "MAT", 1, 28, 1071},
	{41, "Mark", "Mark", "MRK", 1, 16, 678},
	{42, "Luke", "Luke", "LUK", 1, 24, 1151},
	{43, "John", "John", "JHN", 1, 21, 879},
	{44, "Acts", "Acts", "ACT", 1, 28, 1007},
	{45, "Romans", "Rom", "ROM", 1, 16, 433},
	{46, "1 Corinthians", "1 Cor", "1CO", 1, 16, 437},
	{47, "2 Corinthians", "2 Cor", "2CO", 1, 13, 257},
	{48, "Galatians", "Gal", "GAL", 1, 6, 149},
	{49, "Ephesians", "Eph", "EPH", 1, 6, 155},
	{50, "Philippians", "Phil", "PHP", 1, 4, 104},
	{51, "Colossians", "Col", "COL", 1, 4, 95},
	{52, "1 Thessalonians", "1 Thess", "1TH", 1, 5, 89},
	{53, "2 Thessalonians", "2 Thess", "2TH", 1, 3, 47},
	{54, "1 Timothy", "1 Tim", "1TI", 1, 6, 113},
	{55, "2 Timothy", "2 Tim", "2TI", 1, 4, 83},
	{56, "Titus", "Titus", "TIT", 1, 3, 46},
	{57, "Philemon", "Phlm", "PHM", 1, 1, 25},
	{58, "Hebrews", "Heb", "HEB", 1, 13, 303},
	{59, "James", "Jas", "JAS", 1, 5, 108},
	{60, "1 Peter", "1 Pet", "1PE", 1, 5, 105},
	{61, "2 Peter", "2 Pet", "2PE", 1, 3, 61},
	{62, "1 John", "1 John", "1JN", 1, 5, 105},
	{63, "2 John", "2 John", "2JN", 1, 1, 13},
	{64, "3 John", "3 John", "3JN", 1, 1, 14},
	{65, "Jude", "Jude", "JUD", 1, 1, 25},
	{66, "Revelation", "Rev", "REV", 1, 22, 404},
}

// BookByUSFM returns the canonical book for a USFM book code.
func BookByUSFM(code string) *BookInfo {
	for i := range Canonical {
		if Canonical[i].USFM == code {
			return &Canonical[i]
		}
	}
	return nil
}

// TotalChapters and TotalVerses are the canonical KJV totals.
const (
	TotalChapters = 1189
	TotalVerses   = 31102
)
