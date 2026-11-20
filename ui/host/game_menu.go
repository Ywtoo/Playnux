package host

import (
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// RegisterGameMenu attaches GTK's context menu to game rows and covers.
func RegisterGameMenu(app *application.App) {
	menu := app.ContextMenu.New()
	add := func(label, action string) {
		menu.Add(label).OnClick(func(ctx *application.Context) {
			id := strings.TrimSpace(ctx.ContextMenuData())
			if id != "" {
				app.Event.Emit("library:game-menu", map[string]string{"id": id, "action": action})
			}
		})
	}
	add("Jogar", "play")
	add("Ver detalhes", "details")
	menu.AddSeparator()
	add("Baixar metadados…", "metadata")
	add("Editar capa…", "cover")
	add("Editar ícone…", "icon")
	menu.AddSeparator()
	add("Propriedades de inicialização…", "launch")
	app.ContextMenu.Add("playnux-game", menu)
}
