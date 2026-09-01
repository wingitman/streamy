// Command streamy is a Delbysoft terminal application.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/wingitman/streamy/internal/app"
	"github.com/wingitman/streamy/internal/chat"
	"github.com/wingitman/streamy/internal/config"
	"github.com/wingitman/streamy/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "show version")
	loginPlatform := flag.String("login", "", "authorize a platform (twitch or youtube)")
	loginConnection := flag.String("connection", "", "connection ID used with --login")
	flag.Parse()
	if *showVersion {
		fmt.Printf("streamy %s (%s)\n", version.Version, version.Commit)
		return
	}
	if *loginPlatform != "" {
		if *loginConnection == "" {
			fmt.Fprintln(os.Stderr, "streamy: --connection is required with --login")
			os.Exit(2)
		}
		if err := login(chat.Platform(*loginPlatform), chat.ConnectionID(*loginConnection)); err != nil {
			fmt.Fprintln(os.Stderr, "streamy: login failed:", err)
			os.Exit(1)
		}
		fmt.Println("streamy: login complete")
		return
	}
	cfg, err := config.Load("streamy")
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	adapters, warnings := buildAdapters(cfg)
	configPath, err := config.Path("streamy")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	histories, err := openHistories(cfg, configPath, adapters)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		disconnectAdapters(adapters)
		os.Exit(1)
	}
	defer closeHistories(histories)
	defer disconnectAdapters(adapters)
	model, err := app.New("streamy", cfg, adapters, histories)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	model.SetWarnings(warnings)
	model.SetIntegrationConnector(integrationConnector(configPath))
	for _, history := range histories {
		for _, message := range history.Page(0, history.Len()) {
			model.AddHistoryMessage(message)
		}
	}
	program := tea.NewProgram(model)
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
