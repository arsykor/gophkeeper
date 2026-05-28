// Command client is the GophKeeper TUI client.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	clientconfig "github.com/arsykor/gophkeeper/internal/client/config"
	grpcclient "github.com/arsykor/gophkeeper/internal/client/grpc"
	"github.com/arsykor/gophkeeper/internal/client/tui"
)

// Version and BuildDate are set by -ldflags at build time.
var (
	Version   = "dev"
	BuildDate = "unknown"
)

func main() {
	cfg, err := clientconfig.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	client, err := grpcclient.New(cfg.ServerAddress, cfg.Insecure)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to server: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	app := tui.NewApp(client, Version, BuildDate)
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
}
