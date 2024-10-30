package cmd

import (
	"fmt"
	"log"

	"github.com/nlpfollower/deltamind/backend/api"
	"github.com/spf13/cobra"
)

func NewServeCommand() *cobra.Command {
	var nexusPort int

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the chat server",
		Long:  `Start the DeltaMind chat server`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := getConfig(cmd)
			return serve(cfg.DBPath, nexusPort)
		},
	}

	cmd.Flags().IntVar(&nexusPort, "nexus-port", 8081, "Port for the Nexus server")

	return cmd
}

func serve(dbPath string, nexusPort int) error {
	srv, err := api.NewServer(dbPath, nexusPort)
	if err != nil {
		return fmt.Errorf("error creating server: %v", err)
	}

	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}

	return nil
}
