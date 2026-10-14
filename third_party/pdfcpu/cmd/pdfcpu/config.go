/*
Copyright 2026 The pdfcpu Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/spf13/cobra"
)

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "List, reset configuration",
		Long:  usageLongConfig,
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List configuration",
			RunE:  wrapHandler(printConfiguration),
		},
		&cobra.Command{
			Use:   "reset",
			Short: "Reset configuration",
			RunE:  wrapHandler(resetConfiguration),
		},
	)

	return cmd
}

func completionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion script",
		Long: `Generate shell completion script for pdfcpu.

To load completions:

Bash:

  $ source <(pdfcpu completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ pdfcpu completion bash > /etc/bash_completion.d/pdfcpu
  # macOS:
  $ pdfcpu completion bash > $(brew --prefix)/etc/bash_completion.d/pdfcpu

Zsh:

  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ pdfcpu completion zsh > "${fpath[1]}/_pdfcpu"

  # You will need to start a new shell for this setup to take effect.

Fish:

  $ pdfcpu completion fish | source

  # To load completions for each session, execute once:
  $ pdfcpu completion fish > ~/.config/fish/completions/pdfcpu.fish

PowerShell:

  PS> pdfcpu completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> pdfcpu completion powershell > pdfcpu.ps1
  # and source this file from your PowerShell profile.
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			}
			return nil
		},
	}
	return cmd
}

func paperCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "paper",
		Short: "Print list of supported paper sizes",
		Long:  usageLongPaper,
		RunE:  wrapHandler(printPaperSizes),
	}
}

func selectedpagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "selectedpages",
		Short: "Print definition of the -pages flag",
		Long:  usageLongSelectedPages,
		RunE:  wrapHandler(printSelectedPages),
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Long:  usageLongVersion,
		Args:  cobra.NoArgs,
		RunE:  wrapHandler(printVersion),
	}
}

func printConfiguration(conf *model.Configuration, args []string) error {
	fmt.Fprintf(os.Stdout, "config: %s\n", conf.Path)
	f, err := os.Open(conf.Path)
	if err != nil {
		return fmt.Errorf("can't open %s", conf.Path)
	}
	defer f.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, f); err != nil {
		return fmt.Errorf("can't read %s", conf.Path)
	}

	fmt.Print(string(buf.String()))
	return nil
}

func confirmed() bool {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("(yes/no): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading input. Please try again.")
			continue
		}

		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "yes":
			return true
		case "no":
			return false
		default:
			fmt.Println("Invalid input. Please type 'yes' or 'no'.")
		}
	}
}

func resetConfiguration(conf *model.Configuration, args []string) error {
	fmt.Printf("Did you make a backup of %s ?\n", conf.Path)
	if confirmed() {
		fmt.Printf("Are you ready to reset your config.yml to %s ?\n", model.VersionStr)
		if confirmed() {
			fmt.Println("resetting..")
			if err := model.ResetConfig(); err != nil {
				return fmt.Errorf("config problem: %v", err)
			}
			fmt.Println("Finished - Don't forget to update config.yml with your modifications.")
		} else {
			fmt.Println("Operation canceled.")
		}
	} else {
		fmt.Println("Operation canceled.")
	}
	return nil
}

func printPaperSizes(conf *model.Configuration, args []string) error {
	fmt.Fprintln(os.Stdout, paperSizes)
	return nil
}

func printSelectedPages(conf *model.Configuration, args []string) error {
	fmt.Fprintln(os.Stdout, usagePageSelection)
	return nil
}

func printVersion(conf *model.Configuration, args []string) error {
	updateVersionInfoFromBuildInfo()
	writeVersionInfo(os.Stdout, conf.Path)
	return nil
}

func writeVersionInfo(w io.Writer, configPath string) {
	fmt.Fprintf(w, "version: %s\n", version)
	fmt.Fprintf(w, " config: %s\n", configPath)
	fmt.Fprintf(w, " commit: %s\n", commit)
	fmt.Fprintf(w, "   date: %s\n", formatVersionDate(date))
	fmt.Fprintf(w, "     go: %s\n", runtime.Version())
}

func formatVersionDate(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.UTC().Format("2006-01-02 15:04:05 MST")
}
