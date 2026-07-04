package cli

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

func completionCmd() *cli.Command {
	return &cli.Command{
		Name:  "completion",
		Usage: "Generate shell completion scripts",
		Commands: []*cli.Command{
			{
				Name:  "bash",
				Usage: "Generate bash completion script",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					fmt.Print(bashCompletion)
					return nil
				},
			},
			{
				Name:  "zsh",
				Usage: "Generate zsh completion script",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					fmt.Print(zshCompletion)
					return nil
				},
			},
			{
				Name:  "fish",
				Usage: "Generate fish completion script",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					fmt.Print(fishCompletion)
					return nil
				},
			},
		},
	}
}

const bashCompletion = `# loadout bash completion
_loadout() {
    local cur="${COMP_WORDS[COMP_CWORD]}"
    local commands="init status diff sync install new add source check update bundle wizard agents doctor version completion"
    COMPREPLY=($(compgen -W "${commands}" -- "${cur}"))
}
complete -F _loadout loadout
`

const zshCompletion = `#compdef loadout
_loadout() {
    local -a commands
    commands=(
        'init:Create the canonical store'
        'status:Show the drift matrix per agent'
        'diff:Show what sync would change'
        'sync:Install or update artifacts in agent homes'
        'install:First-time full install into one agent'
        'new:Scaffold a new artifact in the store'
        'add:Adopt an existing native artifact into the store'
        'source:Track skills from external git repositories'
        'check:Check sources for updates (non-intrusive)'
        'update:Fetch and apply source updates (gated)'
        'bundle:Export/import the store as a shareable artifact'
        'wizard:Guided builder for skills and conventions'
        'agents:List supported agents'
        'doctor:Verify store, agent homes, blocks and state'
        'version:Print version information'
        'completion:Generate shell completion scripts'
    )
    _describe 'loadout commands' commands
}
_loadout
`

const fishCompletion = `# loadout fish completion
complete -c loadout -f
complete -c loadout -n '__fish_use_subcommand' -a init        -d 'Create the canonical store'
complete -c loadout -n '__fish_use_subcommand' -a status      -d 'Show the drift matrix per agent'
complete -c loadout -n '__fish_use_subcommand' -a diff        -d 'Show what sync would change'
complete -c loadout -n '__fish_use_subcommand' -a sync        -d 'Install or update artifacts in agent homes'
complete -c loadout -n '__fish_use_subcommand' -a install     -d 'First-time full install into one agent'
complete -c loadout -n '__fish_use_subcommand' -a new         -d 'Scaffold a new artifact in the store'
complete -c loadout -n '__fish_use_subcommand' -a add         -d 'Adopt an existing native artifact into the store'
complete -c loadout -n '__fish_use_subcommand' -a source      -d 'Track skills from external git repositories'
complete -c loadout -n '__fish_use_subcommand' -a check       -d 'Check sources for updates (non-intrusive)'
complete -c loadout -n '__fish_use_subcommand' -a update      -d 'Fetch and apply source updates (gated)'
complete -c loadout -n '__fish_use_subcommand' -a bundle      -d 'Export/import the store as a shareable artifact'
complete -c loadout -n '__fish_use_subcommand' -a wizard      -d 'Guided builder for skills and conventions'
complete -c loadout -n '__fish_use_subcommand' -a agents      -d 'List supported agents'
complete -c loadout -n '__fish_use_subcommand' -a doctor      -d 'Verify store, agent homes, blocks and state'
complete -c loadout -n '__fish_use_subcommand' -a version     -d 'Print version information'
complete -c loadout -n '__fish_use_subcommand' -a completion  -d 'Generate shell completion scripts'
`
