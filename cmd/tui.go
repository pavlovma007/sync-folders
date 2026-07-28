package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync-folders/core"
)

// RunTUI запускает простое текстовое меню (без внешних зависимостей).
func RunTUI() {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("\033[H\033[2J") // clear screen
		fmt.Println("╔══════════════════════════════╗")
		fmt.Println("║  sync-folders                ║")
		fmt.Println("╚══════════════════════════════╝")
		fmt.Println()
		fmt.Println("1) List folders")
		fmt.Println("2) Add folder")
		fmt.Println("3) Remove folder")
		fmt.Println("4) List configs")
		fmt.Println("5) Add config")
		fmt.Println("6) Remove config")
		fmt.Println("7) Run sync")
		fmt.Println("8) Config template")
		fmt.Println("0) Exit")
		fmt.Print("\nChoice: ")

		if !scanner.Scan() {
			break
		}
		choice := strings.TrimSpace(scanner.Text())

		switch choice {
		case "1":
			folders, _ := core.ListFolders()
			fmt.Println("\nFolders:")
			for _, f := range folders {
				fmt.Printf("  %s → %s\n", f.Name, f.Path)
			}
			pause()

		case "2":
			fmt.Print("Folder name: ")
			scanner.Scan()
			name := strings.TrimSpace(scanner.Text())
			fmt.Print("Folder path: ")
			scanner.Scan()
			path := strings.TrimSpace(scanner.Text())
			if err := core.AddFolder(name, path); err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Println("OK")
			}
			pause()

		case "3":
			fmt.Print("Folder name to remove: ")
			scanner.Scan()
			name := strings.TrimSpace(scanner.Text())
			if err := core.RemoveFolder(name); err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Println("OK")
			}
			pause()

		case "4":
			cfgs, _ := core.ListConfigs()
			fmt.Println("\nConfigs:")
			for name, sc := range cfgs {
				fmt.Printf("  %s → folder %q (%s)\n", name, sc.Folder, sc.Transport.Type)
			}
			pause()

		case "5":
			fmt.Print("Config YAML path: ")
			scanner.Scan()
			path := strings.TrimSpace(scanner.Text())
			if err := core.AddConfig(path); err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Println("OK")
			}
			pause()

		case "6":
			fmt.Print("Config name to remove: ")
			scanner.Scan()
			name := strings.TrimSpace(scanner.Text())
			if err := core.RemoveConfig(name); err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Println("OK")
			}
			pause()

		case "7":
			fmt.Print("Config name (or --all): ")
			scanner.Scan()
			name := strings.TrimSpace(scanner.Text())
			if name == "--all" || name == "" {
				core.Daemon(0)
			} else {
				cfg, _ := core.LoadConfig()
				if sc, ok := cfg.Syncs[name]; ok {
					engine, _ := core.NewSyncEngine(name, sc)
					engine.RunOnce()
				}
			}
			fmt.Println("Sync done")
			pause()

		case "8":
			fmt.Println("\n# sync-folders config template")
			fmt.Println("# See: sync-folders config template in CLI")
			pause()

		case "0":
			fmt.Println("Bye!")
			return
		}
	}
}

func pause() {
	fmt.Print("\nPress Enter to continue...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}
