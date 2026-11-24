package tui

import "github.com/charmbracelet/lipgloss"

var (
	colAccent = lipgloss.AdaptiveColor{Light: "#0f766e", Dark: "#5eead4"}
	colMuted  = lipgloss.AdaptiveColor{Light: "#6b7280", Dark: "#94a3b8"}
	colText   = lipgloss.AdaptiveColor{Light: "#111827", Dark: "#e2e8f0"}
	colRed    = lipgloss.AdaptiveColor{Light: "#b91c1c", Dark: "#f87171"}
	colGold   = lipgloss.AdaptiveColor{Light: "#a16207", Dark: "#facc15"}
	colErr    = lipgloss.AdaptiveColor{Light: "#b91c1c", Dark: "#fb7185"}
	colPanel  = lipgloss.AdaptiveColor{Light: "#e5e7eb", Dark: "#334155"}
	colHeat1  = lipgloss.AdaptiveColor{Light: "#99f6e4", Dark: "#115e59"}
	colHeat2  = lipgloss.AdaptiveColor{Light: "#2dd4bf", Dark: "#0f766e"}
	colHeat3  = lipgloss.AdaptiveColor{Light: "#f59e0b", Dark: "#facc15"}
	colHeat4  = lipgloss.AdaptiveColor{Light: "#dc2626", Dark: "#fb7185"}
)

var (
	appTitle    = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	appSubtitle = lipgloss.NewStyle().Foreground(colMuted)
	headerRule  = lipgloss.NewStyle().Foreground(colPanel)
	promptStyle = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	chipStyle   = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	chipDim     = lipgloss.NewStyle().Foreground(colMuted)

	sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	mutedStyle   = lipgloss.NewStyle().Foreground(colMuted)
	textStyle    = lipgloss.NewStyle().Foreground(colText)
	labelStyle   = lipgloss.NewStyle().Bold(true).Foreground(colText)
	selStyle     = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	snipStyle    = lipgloss.NewStyle().Foreground(colText)
	hitStyle     = lipgloss.NewStyle().Bold(true).Foreground(colGold)

	italicStyle          = lipgloss.NewStyle().Italic(true).Foreground(colText)
	redStyle             = lipgloss.NewStyle().Foreground(colRed)
	redItalic            = lipgloss.NewStyle().Italic(true).Foreground(colRed)
	titleStyle           = lipgloss.NewStyle().Bold(true).Foreground(colText)
	sectionHead          = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	noteStyle            = lipgloss.NewStyle().Foreground(colMuted).Italic(true)
	userNoteStyle        = lipgloss.NewStyle().Bold(true).Foreground(colGold)
	strongTagStyle       = lipgloss.NewStyle().Bold(true).Foreground(colGold)
	strongActiveTagStyle = lipgloss.NewStyle().Bold(true).Reverse(true).Foreground(colGold)
	parallelLabelStyle   = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	calNoteStyle         = lipgloss.NewStyle().Foreground(colAccent)
	calBookmarkStyle     = lipgloss.NewStyle().Foreground(colGold)
	calHistoryStyle      = lipgloss.NewStyle().Foreground(colRed)
	statusOK             = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	statusErr            = lipgloss.NewStyle().Bold(true).Foreground(colErr)
	footerStyle          = lipgloss.NewStyle().Foreground(colMuted)
	readerRule           = lipgloss.NewStyle().Foreground(colPanel)
	cursorMarker         = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	verseNumStyle        = lipgloss.NewStyle().Foreground(colMuted)
	cursorNum            = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	panelStyle           = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colPanel).
				Padding(0, 1)

	heatStyles = []lipgloss.Style{
		lipgloss.NewStyle().Foreground(colMuted),
		lipgloss.NewStyle().Foreground(colHeat1),
		lipgloss.NewStyle().Foreground(colHeat2),
		lipgloss.NewStyle().Foreground(colHeat3),
		lipgloss.NewStyle().Foreground(colHeat4).Bold(true),
	}
)

// heatLevel maps a normalised score to one of the five heatmap glyph levels.
func heatLevel(score float64) int {
	switch {
	case score <= 0:
		return 0
	case score <= 0.25:
		return 1
	case score <= 0.5:
		return 2
	case score <= 0.75:
		return 3
	default:
		return 4
	}
}

// heatStyle returns the style for a heatmap cell. Out-of-scope cells are
// dimmed but keep their shape so the overall distribution remains visible.
func heatStyle(level int, dim bool) lipgloss.Style {
	level = min(max(level, 0), len(heatStyles)-1)
	if dim {
		return mutedStyle
	}
	return heatStyles[level]
}
