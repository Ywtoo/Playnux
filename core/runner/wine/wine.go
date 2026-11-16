package wine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// RunnerType define o tipo de motor de compatibilidade (Proton ou Wine puro).
type RunnerType string

const (
	RunnerTypeProton RunnerType = "proton"
	RunnerTypeWine   RunnerType = "wine"
	RunnerTypeSystem RunnerType = "system"
)

// Environment encapsula os caminhos e variáveis de execução para jogos Windows no Linux.
type Environment struct {
	Type       RunnerType
	BinaryPath string            // Caminho para o binário do proton ou wine (ex: proton, wine64)
	PrefixPath string            // WINEPREFIX (pasta onde fica o C: simulado)
	CustomEnv  map[string]string // Variáveis como DXVK_ASYNC, MANGOHUD, GAMEMODERUN, etc.
}

// FindDefaultProton tenta descobrir versões instaladas do Proton da Valve ou GE-Proton no host.
func FindDefaultProton() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// Caminhos padrão onde o Steam e a comunidade instalam o Proton no Linux
	candidates := []string{
		// GE-Proton / Custom
		filepath.Join(home, ".steam/root/compatibilitytools.d"),
		filepath.Join(home, ".local/share/Steam/compatibilitytools.d"),
		// Steam Oficial (Proton Experimental, 9.0, 8.0)
		filepath.Join(home, ".steam/steam/steamapps/common"),
		filepath.Join(home, ".local/share/Steam/steamapps/common"),
		filepath.Join(home, ".var/app/com.valvesoftware.Steam/.local/share/Steam/compatibilitytools.d"),
	}

	for _, dir := range candidates {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				protonBin := filepath.Join(dir, e.Name(), "proton")
				if info, err := os.Stat(protonBin); err == nil && !info.IsDir() {
					return protonBin, nil
				}
			}
		}
	}

	// Fallback para wine do sistema se não achar proton
	if sysWine, err := exec.LookPath("wine"); err == nil {
		return sysWine, nil
	}

	return "", fmt.Errorf("nenhum runner Proton ou Wine detectado no sistema")
}

// BuildCommand monta o exec.Cmd pronto para rodar um .exe Windows isolado no seu Wineprefix.
func (env *Environment) BuildCommand(gameExe string, args []string) *exec.Cmd {
	var cmd *exec.Cmd

	if env.Type == RunnerTypeProton {
		// Formato do Proton da Valve: proton run <jogo.exe> [args...]
		cmdArgs := append([]string{"run", gameExe}, args...)
		cmd = exec.Command(env.BinaryPath, cmdArgs...)
	} else {
		// Formato do Wine clássico: wine <jogo.exe> [args...]
		cmdArgs := append([]string{gameExe}, args...)
		cmd = exec.Command(env.BinaryPath, cmdArgs...)
	}

	// Configura as variáveis de ambiente essenciais do Wine
	cmd.Env = os.Environ()
	if env.PrefixPath != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("WINEPREFIX=%s", env.PrefixPath))
	}
	for k, v := range env.CustomEnv {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	return cmd
}
