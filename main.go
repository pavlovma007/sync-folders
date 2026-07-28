package main

import (
	"os"
	"sync-folders/cmd"
)

func main() {
	if len(os.Args) < 2 {
		// Без аргументов — запускаем GUI
		cmd.RunGUI()
		return
	}

	arg := os.Args[1]
	helpFlags := map[string]bool{
		"--help": true, "-h": true, "--h": true, "-?": true, "/?": true,
	}

	if helpFlags[arg] {
		cmd.Run()
		return
	}

	switch arg {
	case "tui":
		cmd.RunTUI()
	case "gui":
		cmd.RunGUI()
	default:
		cmd.Run()
	}
}
