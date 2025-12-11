// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for opengslb-cli.

To load completions:

Bash:
  $ source <(opengslb-cli completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ opengslb-cli completion bash > /etc/bash_completion.d/opengslb-cli
  # macOS:
  $ opengslb-cli completion bash > $(brew --prefix)/etc/bash_completion.d/opengslb-cli

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc
  # To load completions for each session, execute once:
  $ opengslb-cli completion zsh > "${fpath[1]}/_opengslb-cli"
  # You will need to start a new shell for this setup to take effect.

Fish:
  $ opengslb-cli completion fish | source
  # To load completions for each session, execute once:
  $ opengslb-cli completion fish > ~/.config/fish/completions/opengslb-cli.fish

PowerShell:
  PS> opengslb-cli completion powershell | Out-String | Invoke-Expression
  # To load completions for every new session, run:
  PS> opengslb-cli completion powershell > opengslb-cli.ps1
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
