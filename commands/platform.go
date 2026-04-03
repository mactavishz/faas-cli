package commands

import (
	"strings"

	"github.com/spf13/cobra"
)

const (
	platformFaasd    = "faasd"
	platformTinyFaaS = "tinyfaas"
)

func getEffectivePlatform(cmd *cobra.Command, selectedPlatform, providerName string) string {
	if platformFlagSet(cmd) {
		return normalizePlatform(selectedPlatform)
	}

	switch strings.ToLower(strings.TrimSpace(providerName)) {
	case platformTinyFaaS:
		return platformTinyFaaS
	case "openfaas":
		return platformFaasd
	default:
		return normalizePlatform(selectedPlatform)
	}
}

func platformFlagSet(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}

	if flag := cmd.Flag("platform"); flag != nil && flag.Changed {
		return true
	}

	if flag := cmd.Flags().Lookup("platform"); flag != nil && flag.Changed {
		return true
	}

	if flag := cmd.InheritedFlags().Lookup("platform"); flag != nil && flag.Changed {
		return true
	}

	if root := cmd.Root(); root != nil {
		if flag := root.PersistentFlags().Lookup("platform"); flag != nil && flag.Changed {
			return true
		}
	}

	return false
}

func normalizePlatform(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return platformFaasd
	}

	return value
}
