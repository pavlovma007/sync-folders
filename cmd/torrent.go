package cmd

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"sync-folders/dht"
)

func handleTorrent(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: sync-folders torrent <subcommand>")
		fmt.Println("  keygen <project>   Generate Ed25519 key pair for DHT")
		fmt.Println("  status             Show torrent client + DHT status")
		return
	}

	switch args[0] {
	case "keygen":
		if len(args) < 2 {
			fmt.Println("Usage: sync-folders torrent keygen <project>")
			return
		}
		pub, priv, err := dht.GenerateKey()
		if err != nil {
			log.Fatalf("keygen: %v", err)
		}
		fmt.Printf("project:     %s\n", args[1])
		fmt.Printf("public_key:  %x\n", pub)
		fmt.Printf("private_key: %x\n", priv)

	case "status":
		fmt.Println("Torrent transport status")
		fmt.Println("  DHT: not connected (use dht commands)")
		fmt.Println("  Client: use qBittorrent Web UI to check")

	default:
		fmt.Printf("Unknown torrent subcommand: %s\n", args[0])
	}
}

func handleDHT(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: sync-folders dht <subcommand>")
		fmt.Println("  put <key> <priv> <salt> <seq> <value>   Publish manifest to DHT")
		fmt.Println("  get <key> <salt>                        Get manifest from DHT")
		fmt.Println("  watch <key> <salt> [interval]            Watch DHT for updates")
		return
	}

	switch args[0] {
	case "put":
		if len(args) < 6 {
			fmt.Println("Usage: sync-folders dht put <key_hex> <priv_hex> <salt> <seq> <value_json>")
			return
		}
		key := mustDecodeHex(args[1])
		priv := mustDecodeHex(args[2])
		salt := args[3]
		seq, err := strconv.ParseInt(args[4], 10, 64)
		if err != nil {
			log.Fatalf("invalid seq: %v", err)
		}
		value := args[5]

		client, err := dht.NewClient()
		if err != nil {
			log.Fatalf("DHT client: %v", err)
		}
		defer client.Close()

		err = client.Put(key, priv, salt, seq, []byte(value))
		if err != nil {
			log.Fatalf("DHT put: %v", err)
		}
		fmt.Printf("Published to DHT (seq=%d, salt=%s, nodes=%d)\n", seq, salt, client.Nodes())

	case "get":
		if len(args) < 3 {
			fmt.Println("Usage: sync-folders dht get <key_hex> <salt>")
			return
		}
		key := mustDecodeHex(args[1])
		salt := args[2]

		client, err := dht.NewClient()
		if err != nil {
			log.Fatalf("DHT client: %v", err)
		}
		defer client.Close()

		value, seq, err := client.Get(key, salt)
		if err != nil {
			log.Fatalf("DHT get: %v", err)
		}
		fmt.Printf("seq=%d\n", seq)
		fmt.Printf("value=%s\n", string(value))

		var pretty interface{}
		if err := json.Unmarshal(value, &pretty); err == nil {
			prettyJSON, _ := json.MarshalIndent(pretty, "", "  ")
			fmt.Printf("pretty:\n%s\n", string(prettyJSON))
		}

	case "watch":
		if len(args) < 3 {
			fmt.Println("Usage: sync-folders dht watch <key_hex> <salt> [interval_sec]")
			return
		}
		key := mustDecodeHex(args[1])
		salt := args[2]
		interval := 30 * time.Second
		if len(args) >= 4 {
			if n, err := strconv.Atoi(args[3]); err == nil && n > 0 {
				interval = time.Duration(n) * time.Second
			}
		}

		client, err := dht.NewClient()
		if err != nil {
			log.Fatalf("DHT client: %v", err)
		}
		defer client.Close()

		fmt.Printf("Watching DHT (interval=%v, salt=%s)\n", interval, salt)
		fmt.Println("Press Ctrl+C to stop")

		var lastSeq int64
		for {
			value, seq, err := client.Get(key, salt)
			if err == nil && seq > lastSeq {
				fmt.Printf("\n--- Update: seq=%d ---\n", seq)
				fmt.Printf("value=%s\n", string(value))
				lastSeq = seq
			}
			time.Sleep(interval)
		}

	default:
		fmt.Printf("Unknown dht subcommand: %s\n", args[0])
	}
}

func mustDecodeHex(s string) []byte {
	s = strings.TrimSpace(s)
	decoded, err := hex.DecodeString(s)
	if err != nil {
		return []byte(s)
	}
	return decoded
}

func handleCLI(args []string) bool {
	if len(args) < 1 {
		return false
	}
	switch args[0] {
	case "torrent":
		handleTorrent(args[1:])
		return true
	case "dht":
		handleDHT(args[1:])
		return true
	}
	return false
}

func initTorrentCLI() {}
