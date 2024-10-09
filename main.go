package main

import (
	"fmt"
	"os"

	"github.com/nlpfollower/deltamind/backend/cmd"
)

func main() {
	rootCmd := cmd.NewRootCommand()

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
