package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"

	"gotunnel/internal/config"
)

func runConfig(args []string) error {
	if len(args) == 0 || args[0] != "validate" {
		return errors.New("usage: gotunnel config validate --file gotunnel.yaml")
	}

	fs := flag.NewFlagSet("config validate", flag.ContinueOnError)
	configPath := fs.String("file", "gotunnel.yaml", "Path to GoTunnel config")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Println(string(data))
	return nil
}
