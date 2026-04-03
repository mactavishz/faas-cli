package commands

import (
	"testing"

	"github.com/spf13/cobra"
)

func Test_getEffectivePlatform_FromProvider(t *testing.T) {
	tests := []struct {
		name             string
		selectedPlatform string
		providerName     string
		expected         string
	}{
		{
			name:             "tinyfaas provider maps to tinyfaas",
			selectedPlatform: "faasd",
			providerName:     "tinyfaas",
			expected:         "tinyfaas",
		},
		{
			name:             "openfaas provider maps to faasd",
			selectedPlatform: "faasd",
			providerName:     "openfaas",
			expected:         "faasd",
		},
		{
			name:             "unknown provider falls back to selected",
			selectedPlatform: "faasd",
			providerName:     "unknown",
			expected:         "faasd",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := getEffectivePlatform(nil, tc.selectedPlatform, tc.providerName)
			if got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func Test_normalizePlatform_DefaultsToFaasd(t *testing.T) {
	if got := normalizePlatform(""); got != platformFaasd {
		t.Fatalf("expected %q, got %q", platformFaasd, got)
	}
}

func Test_platformFlagSet_WithPersistentDefaultValueProvided(t *testing.T) {
	var selected string
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().StringVar(&selected, "platform", platformFaasd, "")

	child := &cobra.Command{Use: "child", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
	root.AddCommand(child)
	root.SetArgs([]string{"child", "--platform=faasd"})

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	if !platformFlagSet(child) {
		t.Fatal("expected platform flag to be detected as set")
	}
}
