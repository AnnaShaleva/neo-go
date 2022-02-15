package vm

import (
	"fmt"
	"os"
	"strings"

	"github.com/chzyer/readline"
	cli2 "github.com/nspcc-dev/neo-go/pkg/vm/cli"

	"github.com/urfave/cli"
)

// NewCommands returns 'vm' command.
func NewCommands() []cli.Command {
	return []cli.Command{{
		Name:   "vm",
		Usage:  "start the virtual machine",
		Action: startVMPrompt,
		Flags: []cli.Flag{
			cli.BoolFlag{Name: "debug, d"},
		},
	}}
}

func startVMPrompt(baseCtx *cli.Context) error {

	cmds := []cli.Command{
		{
			Name:  "exit",
			Usage: "Exit the VM prompt",
			Action: func(ctx *cli.Context) error {
				return cli.NewExitError("Bye!", 0) // Need to return an error, otherwise ctl.After will be called.
			},
		},
		{
			Name:   "logo",
			Hidden: true, // Need to be hidden, otherwise it will be shown to the user.
			Usage:  "Usage of loadgo",
			Action: func(ctx *cli.Context) error {
				cli2.PrintLogoS(ctx.App.Writer)
				return nil
			},
		},
		{
			Name:      "greet",
			Usage:     "Greets the user",
			UsageText: "greet <user_name>",
			Action: func(ctx *cli.Context) error {
				args := ctx.Args()
				if !args.Present() {
					fmt.Fprintln(ctx.App.ErrWriter, "No user name provided")
					return nil
				}
				var s string
				for _, arg := range args {
					s += arg
				}
				fmt.Fprintln(ctx.App.Writer, fmt.Sprintf("Hello, %s", s))
				return nil
			},
		},
		{
			Name: "anotherCmdWithSubcmds", // IMPORTANT: do not use subcommands for VM CLI, because
			// Action is not overriden for unknown subcommands, thus, default
			// helpSubcommand.Action will be used which results in error for
			// unknown command topic. // TODO: try to override anotherCmdWithSubcmds.Action
			Usage: "Do subcommand",
			Subcommands: []cli.Command{
				{
					Name:  "subcommand",
					Usage: "Do subcommand",
					Action: func(ctx *cli.Context) error {
						fmt.Fprintln(ctx.App.Writer, "do subcommand")
						return nil
					},
				},
			},
		},
	}
	/*
		var suggestions []prompt.Suggest
		for _, cmd := range cmds {
			if !cmd.Hidden {
				suggestions = append(suggestions, prompt.Suggest{
					Text:        cmd.Name,
					Description: cmd.Usage,
				})
			}
		}
		suggestions = append(suggestions, prompt.Suggest{
			Text:        "help",
			Description: "Print help",
		})
		completer := func(d prompt.Document) []prompt.Suggest {
			return prompt.FilterHasPrefix(suggestions, d.GetWordBeforeCursor(), true)
		}
	*/
	l, err := readline.NewEx(&readline.Config{
		Prompt:      "\033[31m»\033[0m ",
		HistoryFile: "/tmp/readline.tmp",
		// AutoComplete:    completer,
		// InterruptPrompt: "^C",
		EOFPrompt: "exit",

		HistorySearchFold: true,
		// FuncFilterInputRune: filterInput,
	})
	if err != nil {
		panic(err)
	}
	defer l.Close()
	afterF := func(context *cli.Context) error {
		// t := prompt.Input("> ", completer)
		t, err := l.Readline()
		if err != nil {
			return err
		}
		commands := strings.Split(t, " ")
		return context.App.Run(append([]string{"vm"}, commands...))
	}
	ctl := cli.NewApp()
	ctl.Name = "VM CLI"

	// Note: need to set empty `ctl.HelpName`, otherwise `filepath.Base(os.Args[0])`
	// will be used which is `neo-go`, see the example:
	// help
	// NAME:
	//   VM CLI - Official VM CLI for Neo-Go
	//
	// USAGE:
	//   neo-go [global options] command [command options] [arguments...]
	ctl.HelpName = ""

	ctl.Version = "1"
	ctl.Usage = "Official VM CLI for Neo-Go"
	ctl.UsageText = ""
	ctl.ErrWriter = os.Stdout
	ctl.After = func(context *cli.Context) error {
		/*
			fmt.Println("AFTER called")
			fmt.Println(context.Parent() == nil)
		*/
		return afterF(context)

	}

	// If ctl.Action is not set, then default help action will be used. It returns
	// an error in case if there's no topic for the specified command which stops
	// CTL. Override this behaviour.
	ctl.Action = func(c *cli.Context) error {
		args := c.Args()
		if args.Present() {
			err := cli.ShowCommandHelp(c, args.First())
			if err != nil {
				_ = cli.ShowAppHelp(c)
			}
		} else {
			_ = cli.ShowAppHelp(c)
		}
		return nil
		// return afterF(c)
	}

	ctl.Commands = cmds
	return ctl.Run([]string{"vm", "logo"})
	/*
		p := vmcli.NewWithConfig(true, os.Exit, &readline.Config{
			Stdout: ctx.App.Writer,
			Stderr: ctx.App.ErrWriter,
		})
		return p.Run()
	*/
}
