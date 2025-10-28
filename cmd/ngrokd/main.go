package main

import (
	"flag"
	"fmt"
	"github.com/go-logr/logr/funcr"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/daemon"
	"os"
)

func main() {
	configPath := flag.String("config", "/etc/ngrokd/config.yml", "")
	verbose := flag.Bool("v", false, "")
	showVersion := flag.Bool("version", false, "")
	flag.Parse()
	if *showVersion {
		fmt.Println("ngrokd version 0.2.0")
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
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
