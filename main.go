package main

import (
	"fmt"
	"os"

	"github.com/vitaminmoo/sfpw-tool/internal/cli"

	"github.com/alecthomas/kong"
)

var version = "dev"

func main() {
	var c cli.CLI
	ctx := kong.Parse(&c,
		kong.Name("sfpw"),
		kong.Description("SFP Wizard - BLE Command Tool for SFP module programming"),
		kong.UsageOnError(),
		kong.Vars{
			"version": version,
		},
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
	)

	err := ctx.Run(&c)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
