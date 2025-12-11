/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

var nodeName string
var nodeType string
var purpose string

// nodeCmd represents the node command.
var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Node related commands",
	Long: `
Node related commands.
`,
}

func init() {
	rootCmd.AddCommand(nodeCmd)
	nodeCmd.PersistentFlags().StringVarP(&nodeName, "node", "n", "", "Name of node")
	nodeCmd.PersistentFlags().StringVarP(&nodeType, "type", "t", "", "Node Type")
	nodeCmd.PersistentFlags().StringVarP(&purpose, "purpose", "p", "", "Node Purpose (adds label and taint)")
}
