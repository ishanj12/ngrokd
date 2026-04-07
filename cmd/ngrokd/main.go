package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/go-logr/logr/funcr"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/daemon"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/service"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "install":
			if err := service.Install(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "uninstall":
			if err := service.Uninstall(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "start":
			if err := service.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "stop":
			if err := service.Stop(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "restart":
			if err := service.Restart(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "status":
			if err := service.Status(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "run":
			// Remove "run" from args so flag.Parse works
			os.Args = append(os.Args[:1], os.Args[2:]...)
		}
	}

	// Default: run the daemon (foreground mode)
	configPath := flag.String("config", "/etc/ngrokd/config.yml", "")
	verbose := flag.Bool("v", false, "")
	showVersion := flag.Bool("version", false, "")
	flag.Parse()
	if *showVersion {
		fmt.Println("ngrokd version 0.3.0")
		os.Exit(0)
	}
	logger := funcr.New(func(p, a string) {
		if p != "" {
			fmt.Printf("%s: %s\n", p, a)
		} else {
			fmt.Println(a)
		}
	}, funcr.Options{Verbosity: 0})
	if *verbose {
		logger = funcr.New(func(p, a string) {
			if p != "" {
				fmt.Printf("%s: %s\n", p, a)
			} else {
				fmt.Println(a)
			}
		}, funcr.Options{Verbosity: 1})
	}
	d, err := daemon.New(*configPath, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := d.Start(); err != nil {
		restoreResolvConf()
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// restoreResolvConf is an emergency cleanup that restores /etc/resolv.conf
// from backup if ngrokd exits without going through Shutdown(). This covers
// panics, unexpected errors, and other abnormal exits. On normal shutdown,
// the daemon already restores resolv.conf and removes the backup, so this
// is a no-op.
func restoreResolvConf() {
	const backup = "/etc/ngrokd/resolv.conf.bak"
	data, err := os.ReadFile(backup)
	if err != nil {
		return
	}
	os.WriteFile("/etc/resolv.conf", data, 0644)
	os.Remove(backup)
}
