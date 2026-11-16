// Package launcher inicia jogos pelas URIs oficiais das lojas e monitora o
// processo resultante para ativar e desativar o Deep Sleep.
package launcher

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/playnux/core/models"
	"github.com/playnux/core/store"
)

// ProcessChecker informa se o jogo está em execução. Em produção é o Scanner
// de /proc; nos testes pode ser um fake determinístico.
type ProcessChecker interface {
	Running(game *models.Game) bool
}

// Sink recebe as transições de estado do gameplay. A camada Wails implementa o
// congelamento visual; o launcher não conhece detalhes da interface.
type Sink interface {
	Sleep(game *models.Game)
	Wake(game *models.Game)
}

// Launcher mantém no máximo um monitor por jogo.
type Launcher struct {
	providers map[string]store.StoreProvider
	checker   ProcessChecker
	sink      Sink

	PollInterval time.Duration
	StartTimeout time.Duration
	ExitGrace    time.Duration

	mu      sync.Mutex
	running map[string]bool
	closed  bool
}

// New cria o lançador. Um sink nil desabilita apenas as notificações de Deep
// Sleep, útil em testes da inicialização do comando.
func New(providers map[string]store.StoreProvider, checker ProcessChecker, sink Sink) *Launcher {
	if checker == nil {
		checker = noopChecker{}
	}
	return &Launcher{
		providers:    providers,
		checker:      checker,
		sink:         sink,
		PollInterval: time.Second,
		StartTimeout: 2 * time.Minute,
		ExitGrace:    3 * time.Second,
		running:      map[string]bool{},
	}
}

// Launch inicia o comando da loja e retorna assim que o processo é criado. O
// monitor de gameplay continua em background.
type Options struct {
	Directory   string            `json:"directory"`
	Arguments   []string          `json:"arguments"`
	Runner      string            `json:"runner,omitempty"`
	ProtonPath  string            `json:"proton_path,omitempty"`
	Command     string            `json:"-"`
	CommandArgs []string          `json:"-"`
	Environment map[string]string `json:"-"`
}

func (l *Launcher) Launch(game *models.Game) error {
	return l.LaunchWithOptions(game, Options{})
}

func (l *Launcher) LaunchWithOptions(game *models.Game, options Options) error {
	if game == nil {
		return fmt.Errorf("launcher: jogo inválido")
	}
	provider, ok := l.providers[game.PluginID]
	if !ok {
		return fmt.Errorf("launcher: loja %q não registrada", game.PluginID)
	}
	if !l.markRunning(game.ID) {
		return fmt.Errorf("launcher: %s já está em execução", game.Name)
	}

	command, args, err := provider.LaunchCommand(game)
	if err != nil {
		l.unmarkRunning(game.ID)
		return err
	}
	if command == "" {
		l.unmarkRunning(game.ID)
		return fmt.Errorf("launcher: a loja não informou um comando para %s", game.Name)
	}

	if options.Command != "" {
		command = options.Command
		args = append([]string(nil), options.CommandArgs...)
	}
	cmd := exec.Command(command, append(args, options.Arguments...)...)
	if game.PluginID == "linux" && filepath.IsAbs(game.InstallDirectory) {
		cmd.Dir = game.InstallDirectory
	}
	if options.Directory != "" {
		cmd.Dir = options.Directory
	}
	if len(options.Environment) > 0 {
		cmd.Env = append(cmd.Env, cmd.Environ()...)
		for key, value := range options.Environment {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	if err := cmd.Start(); err != nil {
		l.unmarkRunning(game.ID)
		return fmt.Errorf("launcher: falha ao iniciar %s: %w", game.Name, err)
	}
	// Evita processos zumbis quando o comando é o próprio jogo (build nativo).
	go func() { _ = cmd.Wait() }()

	go l.monitor(game)
	return nil
}

// Close interrompe os monitores. Os processos dos jogos não são encerrados.
func (l *Launcher) Close() {
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
}

func (l *Launcher) monitor(game *models.Game) {
	defer l.unmarkRunning(game.ID)

	deadline := time.Now().Add(l.StartTimeout)
	for time.Now().Before(deadline) && !l.isClosed() {
		if l.checker.Running(game) {
			l.sleep(game)
			l.waitUntilStopped(game)
			l.wake(game)
			return
		}
		l.pause()
	}
}

// waitUntilStopped exige algumas leituras negativas seguidas para não acordar
// a UI em uma troca de processo do Proton ou do launcher da loja.
func (l *Launcher) waitUntilStopped(game *models.Game) {
	missing := 0
	for !l.isClosed() {
		l.pause()
		if l.checker.Running(game) {
			missing = 0
			continue
		}
		missing++
		if time.Duration(missing)*l.PollInterval >= l.ExitGrace {
			return
		}
	}
}

func (l *Launcher) sleep(game *models.Game) {
	if l.sink != nil {
		l.sink.Sleep(game)
	}
}

func (l *Launcher) wake(game *models.Game) {
	if l.sink != nil {
		l.sink.Wake(game)
	}
}

func (l *Launcher) pause() {
	interval := l.PollInterval
	if interval <= 0 {
		interval = time.Second
	}
	time.Sleep(interval)
}

func (l *Launcher) isClosed() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closed
}

func (l *Launcher) markRunning(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.running[id] {
		return false
	}
	l.running[id] = true
	return true
}

func (l *Launcher) unmarkRunning(id string) {
	l.mu.Lock()
	delete(l.running, id)
	l.mu.Unlock()
}

type noopChecker struct{}

func (noopChecker) Running(*models.Game) bool { return false }
