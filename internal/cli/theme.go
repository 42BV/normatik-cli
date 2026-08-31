package cli

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/fang"
)

// normatikColorScheme is het fang-thema in de Normatik-huisstijl: kompas-oranje
// als accent, terracotta voor commando's, muted plum-grey voor help-body en
// descriptions. De LightDarkFunc kiest per terminal-achtergrond een leesbare
// ink/muted-tint; de brand-accenten (oranje/terracotta) lezen op zowel licht
// als donker.
func normatikColorScheme(c lipgloss.LightDarkFunc) fang.ColorScheme {
	orange := lipgloss.Color("#D97A54")    // kompas-naald
	terra := lipgloss.Color("#A2543F")     // terracotta (knoppen / "NIS2")
	terraDeep := lipgloss.Color("#9A4E3B") // diepe terracotta voor error-balk
	ink := c(lipgloss.Color("#3D3341"), lipgloss.Color("#EDE8EC"))
	muted := c(lipgloss.Color("#6B6470"), lipgloss.Color("#A79FAC"))
	codeblock := c(lipgloss.Color("#F2E9E3"), lipgloss.Color("#2A2530")) // subtiele warme box-bg

	return fang.ColorScheme{
		// Base kleurt de Long-paragraaf boven usage (fang styles.Text).
		// Zelfde muted als command-/flag-descriptions, niet de witte ink.
		Base:           muted,
		Title:          orange,
		Description:    muted,
		Codeblock:      codeblock,
		Program:        terra,
		DimmedArgument: muted,
		Comment:        muted,
		Flag:           orange,
		FlagDefault:    muted,
		Command:        terra,
		QuotedString:   orange,
		Argument:       ink,
		Help:           muted,
		Dash:           orange,
		ErrorHeader:    [2]color.Color{lipgloss.Color("#FFFFFF"), terraDeep},
		ErrorDetails:   terra,
	}
}
