package cmd

import (
	"context"
	"github.com/spf13/cobra"
)

type contextKey string

const configKey contextKey = "config"

type Config struct {
	DBPath string
}

func NewConfig() *Config {
	return &Config{
		DBPath: "chat.db",
	}
}

func NewRootCommand() *cobra.Command {
	cfg := NewConfig()

	rootCmd := &cobra.Command{
		Use:   "",
		Short: "DeltaMind chat server and utilities",
		Long:  `DeltaMind is a chat server with user management and thread import capabilities.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.WithValue(cmd.Context(), configKey, cfg)
			cmd.SetContext(ctx)
			return nil
		},
	}

	// Add shared flags
	rootCmd.PersistentFlags().StringVar(&cfg.DBPath, "db-path", cfg.DBPath, "Path to the BoltDB database file")

	// Add subcommands
	rootCmd.AddCommand(NewServeCommand())
	rootCmd.AddCommand(NewAddUserCommand())
	rootCmd.AddCommand(NewImportThreadsCommand())
	rootCmd.AddCommand(NewGetUserCommand())
	rootCmd.AddCommand(NewDeleteUserCommand())
	rootCmd.AddCommand(NewGetThreadsCommand())

	return rootCmd
}

func getConfig(cmd *cobra.Command) *Config {
	return cmd.Context().Value(configKey).(*Config)
}
