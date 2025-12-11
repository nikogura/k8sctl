/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// clusterCmd represents the cluster command.
var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "K8s Cluster Commands",
	Long: `
K8s Cluster Commands
`,
}

func init() {
	rootCmd.AddCommand(clusterCmd)

}
