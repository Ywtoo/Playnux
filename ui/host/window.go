package host

import "github.com/wailsapp/wails/v3/pkg/application"

// FitWindowToScreen reduz a janela ao espaço útil do monitor principal. As
// opções do Wails usam DIP, e o WorkArea já vem em DIP no Linux/GTK.
func FitWindowToScreen(app *application.App, width, height, minWidth, minHeight int) (int, int, int, int) {
	if app == nil || app.Screen == nil {
		return width, height, minWidth, minHeight
	}
	screen := app.Screen.GetPrimary()
	if screen == nil {
		return width, height, minWidth, minHeight
	}
	return fitWindowToWorkArea(screen.WorkArea, width, height, minWidth, minHeight)
}

func fitWindowToWorkArea(area application.Rect, width, height, minWidth, minHeight int) (int, int, int, int) {
	if area.Width <= 0 || area.Height <= 0 {
		return width, height, minWidth, minHeight
	}

	// Deixa uma margem para a decoração da janela e para não encostar nas bordas.
	const margin = 24
	maxWidth := area.Width - margin
	maxHeight := area.Height - margin
	if maxWidth < 320 {
		maxWidth = area.Width
	}
	if maxHeight < 240 {
		maxHeight = area.Height
	}

	if width > maxWidth {
		width = maxWidth
	}
	if height > maxHeight {
		height = maxHeight
	}
	if minWidth > width {
		minWidth = width
	}
	if minHeight > height {
		minHeight = height
	}
	return width, height, minWidth, minHeight
}
