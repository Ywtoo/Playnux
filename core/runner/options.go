// Package runner concentra validação e armazenamento das opções de
// inicialização de jogos locais, sem depender da interface.
package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/playnux/core/models"
	"github.com/playnux/core/runner/launcher"
)

// AllowLocal informs whether a game accepts its own launch configuration.
// PC store games may opt into an isolated Proton/Wine launch when a Windows
// executable is available; emulation keeps its own emulator arguments.
func AllowLocal(game *models.Game) bool {
	switch game.PluginID {
	case "linux", "emulation", "steam", "gog", "epic":
		return true
	}
	return false
}

// Validate normaliza as opções antes de gravar. Devolve os valores corrigidos.
func Validate(game *models.Game, directory string, arguments []string, runner, protonPath string) (launcher.Options, error) {
	directory = strings.TrimSpace(directory)
	if directory != "" {
		info, err := os.Stat(directory)
		if !filepath.IsAbs(directory) || err != nil || !info.IsDir() {
			return launcher.Options{}, fmt.Errorf("informe um diretório de trabalho absoluto que exista")
		}
	}
	size := 0
	for _, arg := range arguments {
		size += len(arg)
		if size > 8192 || strings.ContainsRune(arg, 0) {
			return launcher.Options{}, fmt.Errorf("argumentos de inicialização inválidos ou longos demais")
		}
	}
	runner = strings.TrimSpace(runner)
	protonPath = strings.TrimSpace(protonPath)
	switch runner {
	case "", "auto":
		runner, protonPath = "auto", ""
	case "wine":
		if _, err := exec.LookPath("wine"); err != nil {
			return launcher.Options{}, fmt.Errorf("o Wine do sistema não foi encontrado")
		}
		protonPath = ""
	case "proton":
		if protonPath == "" {
			return launcher.Options{}, fmt.Errorf("selecione uma versão do Proton")
		}
		if info, statErr := os.Stat(protonPath); statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 || !filepath.IsAbs(protonPath) {
			return launcher.Options{}, fmt.Errorf("o executável do Proton selecionado não existe")
		}
	default:
		return launcher.Options{}, fmt.Errorf("runner de compatibilidade desconhecido")
	}
	return launcher.Options{Directory: directory, Arguments: arguments, Runner: runner, ProtonPath: protonPath}, nil
}
