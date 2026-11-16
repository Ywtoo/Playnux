// Package gameproc localiza o processo de um jogo no /proc do Linux.
//
// O Playnux não injeta nada no jogo: para saber quando ativar o Deep Sleep,
// comparamos os processos do usuário com o diretório de instalação e com as
// variáveis que a Steam/Heroic exportam ao iniciar o jogo.
package gameproc

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/playnux/core/models"
)

// Process é uma leitura pontual de um processo do sistema.
type Process struct {
	PID     int
	Cmdline []string
	Env     map[string]string
	Cwd     string
	Zombie  bool
}

// Scanner lê os processos de ProcRoot. Em produção ProcRoot é "/proc"; nos
// testes pode apontar para um diretório temporário.
type Scanner struct {
	ProcRoot string
	// Exclude permite ignorar PIDs conhecidos (por exemplo, o próprio Playnux).
	Exclude map[int]bool
}

func NewScanner() *Scanner {
	return &Scanner{ProcRoot: "/proc"}
}

// Find devolve o PID do primeiro processo que parece ser o jogo.
func (s *Scanner) Find(game *models.Game) (int, bool) {
	if game == nil {
		return 0, false
	}
	for _, process := range s.List() {
		if s.Exclude[process.PID] || process.Zombie {
			continue
		}
		if Match(game, process) {
			return process.PID, true
		}
	}
	return 0, false
}

// Running informa se alguma instância do jogo continua ativa.
func (s *Scanner) Running(game *models.Game) bool {
	_, ok := s.Find(game)
	return ok
}

// List lê todos os processos numéricos de ProcRoot.
func (s *Scanner) List() []Process {
	root := s.ProcRoot
	if root == "" {
		root = "/proc"
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	processes := make([]Process, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		process := readProcess(root, pid)
		if len(process.Cmdline) == 0 {
			continue
		}
		processes = append(processes, process)
	}
	return processes
}

func readProcess(root string, pid int) Process {
	dir := filepath.Join(root, strconv.Itoa(pid))
	process := Process{PID: pid, Env: map[string]string{}}

	if raw, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
		for _, part := range strings.Split(string(raw), "\x00") {
			if part != "" {
				process.Cmdline = append(process.Cmdline, part)
			}
		}
	}
	if len(process.Cmdline) == 0 {
		return process
	}

	if raw, err := os.ReadFile(filepath.Join(dir, "environ")); err == nil {
		for _, entry := range strings.Split(string(raw), "\x00") {
			key, value, ok := strings.Cut(entry, "=")
			if ok && key != "" {
				process.Env[key] = value
			}
		}
	}
	if cwd, err := os.Readlink(filepath.Join(dir, "cwd")); err == nil {
		process.Cwd = cwd
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "stat")); err == nil {
		// O terceiro campo é o estado; o nome do processo pode conter espaços e
		// parênteses, então usamos o último ")" como âncora.
		if idx := strings.LastIndexByte(string(raw), ')'); idx >= 0 {
			fields := strings.Fields(string(raw)[idx+1:])
			process.Zombie = len(fields) > 0 && fields[0] == "Z"
		}
	}
	return process
}

// Match aplica as regras de detecção de um jogo.
func Match(game *models.Game, process Process) bool {
	if game == nil {
		return false
	}

	// A Steam exporta o AppID no ambiente dos jogos, inclusive quando o jogo
	// roda sob Proton. É a forma mais confiável de identificação.
	if game.Source == "Steam" || game.PluginID == "steam" {
		for _, key := range []string{"SteamAppId", "SteamGameId"} {
			if strings.TrimSpace(process.Env[key]) == strings.TrimSpace(game.GameID) {
				return true
			}
		}
	}

	dir := strings.TrimSpace(game.InstallDirectory)
	if dir == "" {
		return false
	}
	dir = filepath.Clean(dir)
	for _, arg := range process.Cmdline {
		if pathWithin(arg, dir) {
			return true
		}
	}
	if process.Cwd != "" && pathWithin(process.Cwd, dir) {
		return true
	}
	return false
}

// pathWithin compara um caminho (inclusive argumentos do Wine com prefixos de
// unidade, como Z:\home\user\...) com o diretório de instalação. A comparação
// ignora maiúsculas/minúsculas porque prefixos Wine são case-insensitive.
func pathWithin(candidate, dir string) bool {
	candidate = strings.TrimSpace(strings.Trim(candidate, `"'`))
	if candidate == "" || dir == "" {
		return false
	}
	// Argumentos do Wine usam "\"; normalizamos para a comparação.
	normalized := strings.ReplaceAll(candidate, `\`, "/")
	dir = strings.ReplaceAll(dir, `\`, "/")
	if strings.HasPrefix(normalized, "Z:") {
		normalized = normalized[2:]
	}
	normalized = strings.TrimRight(filepath.Clean(normalized), "/")
	dir = strings.TrimRight(filepath.Clean(dir), "/")
	if normalized == "" || dir == "" {
		return false
	}
	lowerCandidate := strings.ToLower(normalized)
	lowerDir := strings.ToLower(dir)
	return lowerCandidate == lowerDir || strings.HasPrefix(lowerCandidate, lowerDir+"/")
}
