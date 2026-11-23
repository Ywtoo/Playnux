package host

import (
	"fmt"
	"os/exec"

	"github.com/playnux/ui/internal/release"
)

func (h *Host) GetVersion() string { return release.Version() }

func (h *Host) CheckForUpdates() (release.Update, error) { return release.Check(h.appContext()) }

func (h *Host) InstallUpdate(version string) error {
	binary, err := release.Install(h.appContext(), version)
	if err != nil {
		return err
	}
	h.mu.Lock()
	h.updateBinary = binary
	h.mu.Unlock()
	return nil
}

func (h *Host) RestartAfterUpdate() error {
	h.mu.Lock()
	if h.updateBinary == "" || h.app == nil {
		h.mu.Unlock()
		return fmt.Errorf("there is no installed update to restart into")
	}
	h.restartBinary = h.updateBinary
	app := h.app
	h.mu.Unlock()
	app.Quit()
	return nil
}

// RelaunchAfterShutdown runs only after the app has released the old library.
func RelaunchAfterShutdown(h *Host) error {
	h.mu.RLock()
	binary := h.restartBinary
	h.mu.RUnlock()
	if binary == "" {
		return nil
	}
	cmd := exec.Command(binary)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
