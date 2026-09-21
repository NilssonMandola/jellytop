// Command jellytop is a terminal admin console for a Jellyfin server: live
// sessions, the activity log, users, libraries and scheduled tasks.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/NilssonMandola/jellytop/internal/config"
	"github.com/NilssonMandola/jellytop/internal/jellyfin"
	"github.com/NilssonMandola/jellytop/internal/ui"
)

// version is overwritten at build time with -ldflags "-X main.version=…".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "jellytop:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		flagURL     = flag.String("url", "", "Jellyfin server URL (default "+config.DefaultURL+")")
		flagToken   = flag.String("token", "", "Jellyfin API key")
		flagVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Usage = usage
	flag.Parse()

	if *flagVersion {
		fmt.Println("jellytop", version)
		return nil
	}

	cfg, err := config.Load(*flagURL, *flagToken)
	if err != nil {
		if errors.Is(err, config.ErrNoToken) {
			return errors.New("no API token configured\n\n" + config.SetupHint())
		}
		return err
	}

	client := jellyfin.New(cfg.URL, cfg.Token)

	// Fail before entering the alt screen, so connection errors stay readable.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info, err := client.Ping(ctx)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", cfg.URL, err)
	}

	prog := tea.NewProgram(ui.New(client, info, cfg.URL), tea.WithAltScreen())
	_, err = prog.Run()
	return err
}

func usage() {
	fmt.Fprintf(os.Stderr, `jellytop — terminal admin console for Jellyfin

Usage:
  jellytop [flags]

Flags:
  -url string     Jellyfin server URL (default %s)
  -token string   Jellyfin API key
  -version        print version and exit

Configuration is read from flags, then $JELLYFIN_URL / $JELLYFIN_TOKEN,
then %s.
`, config.DefaultURL, config.Path())
}
