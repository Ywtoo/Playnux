package launcher

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/playnux/core/models"
	"github.com/playnux/core/store"
	linuxstore "github.com/playnux/core/store/builtins/linux"
)

type fakeChecker struct {
	mu      sync.Mutex
	running bool
}

func (f *fakeChecker) Running(*models.Game) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.running
}

func (f *fakeChecker) set(value bool) {
	f.mu.Lock()
	f.running = value
	f.mu.Unlock()
}

type fakeSink struct {
	slept chan *models.Game
	woke  chan *models.Game
}

func (f *fakeSink) Sleep(game *models.Game) { f.slept <- game }
func (f *fakeSink) Wake(game *models.Game)  { f.woke <- game }

type fakeProvider struct{}

func (fakeProvider) ID() string   { return "fake" }
func (fakeProvider) Name() string { return "Fake" }
func (fakeProvider) Authenticate(context.Context, store.StoreAuth) error {
	return nil
}
func (fakeProvider) IsAuthenticated() bool { return true }
func (fakeProvider) FetchLibrary(context.Context) ([]*models.Game, error) {
	return nil, nil
}
func (fakeProvider) LaunchCommand(*models.Game) (string, []string, error) {
	return "true", nil, nil
}
func (fakeProvider) UninstallCommand(*models.Game) (string, []string, error) {
	return "true", nil, nil
}

func TestLaunchSleepsAndWakesWithProcess(t *testing.T) {
	checker := &fakeChecker{running: true}
	sink := &fakeSink{slept: make(chan *models.Game, 1), woke: make(chan *models.Game, 1)}
	instance := New(map[string]store.StoreProvider{"fake": fakeProvider{}}, checker, sink)
	instance.PollInterval = 5 * time.Millisecond
	instance.StartTimeout = 100 * time.Millisecond
	instance.ExitGrace = 10 * time.Millisecond
	defer instance.Close()

	game := &models.Game{ID: "1", GameID: "42", PluginID: "fake", Name: "Fake Game"}
	if err := instance.Launch(game); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-sink.slept:
		if got.ID != game.ID {
			t.Fatalf("sleep de %q, quer %q", got.ID, game.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("Deep Sleep não foi ativado")
	}

	checker.set(false)
	select {
	case got := <-sink.woke:
		if got.ID != game.ID {
			t.Fatalf("wake de %q, quer %q", got.ID, game.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("Deep Sleep não foi desativado")
	}
}

func TestLaunchRejectsDuplicateWhileMonitoring(t *testing.T) {
	checker := &fakeChecker{running: true}
	sink := &fakeSink{slept: make(chan *models.Game, 1), woke: make(chan *models.Game, 1)}
	instance := New(map[string]store.StoreProvider{"fake": fakeProvider{}}, checker, sink)
	instance.PollInterval = 5 * time.Millisecond
	instance.ExitGrace = 5 * time.Millisecond
	instance.StartTimeout = 100 * time.Millisecond
	defer instance.Close()

	game := &models.Game{ID: "1", GameID: "42", PluginID: "fake", Name: "Fake Game"}
	if err := instance.Launch(game); err != nil {
		t.Fatal(err)
	}
	if err := instance.Launch(game); err == nil {
		t.Fatal("esperava erro ao lançar o mesmo jogo duas vezes")
	}
	checker.set(false)
	<-time.After(50 * time.Millisecond)
}

func TestLaunchReportsUnknownStore(t *testing.T) {
	instance := New(nil, nil, nil)
	instance.Close()
	if err := instance.Launch(&models.Game{PluginID: "missing", Name: "X"}); err == nil {
		t.Fatal("esperava erro para loja desconhecida")
	}
}

func TestLaunchLocalGameUsesInstallDirectory(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "play.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncat asset.txt > result.txt\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "asset.txt"), []byte("game data"), 0o600); err != nil {
		t.Fatal(err)
	}
	instance := New(map[string]store.StoreProvider{"linux": linuxstore.NewProvider()}, &fakeChecker{}, nil)
	defer instance.Close()
	game := &models.Game{
		ID: "local-game", PluginID: "linux", Name: "Local Game",
		Executable: script, InstallDirectory: dir,
	}
	if err := instance.Launch(game); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		content, _ := os.ReadFile(filepath.Join(dir, "result.txt"))
		if string(content) == "game data" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("o jogo não encontrou o arquivo relativo ao diretório de instalação")
}
