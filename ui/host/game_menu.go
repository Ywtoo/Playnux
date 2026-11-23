package host

import (
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// RegisterGameMenu attaches GTK's context menu to game rows and covers.
func RegisterGameMenu(app *application.App) {
	registerGameMenu(app, "en")
}

func (h *Host) SetInterfaceLanguage(language string) {
	h.mu.RLock()
	app := h.app
	h.mu.RUnlock()
	if app != nil {
		application.InvokeSync(func() { registerGameMenu(app, language) })
	}
}

func registerGameMenu(app *application.App, language string) {
	menu := app.ContextMenu.New()
	add := func(english, portuguese, action string) {
		label := english
		if language == "pt-BR" {
			label = portuguese
		}
		menu.Add(label).OnClick(func(ctx *application.Context) {
			id := strings.TrimSpace(ctx.ContextMenuData())
			if id != "" {
				app.Event.Emit("library:game-menu", map[string]string{"id": id, "action": action})
			}
		})
	}
	add("Play", "Jogar", "play")
	add("View details", "Ver detalhes", "details")
	menu.AddSeparator()
	add("Download metadata…", "Baixar metadados…", "metadata")
	add("Edit cover…", "Editar capa…", "cover")
	add("Edit icon…", "Editar ícone…", "icon")
	menu.AddSeparator()
	add("Launch options…", "Propriedades de inicialização…", "launch")
	app.ContextMenu.Add("playnux-game", menu)
}
