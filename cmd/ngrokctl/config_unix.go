//go:build !windows

package main

func getConfigPath() string {
	return "/etc/ngrokd/config.yml"
}
