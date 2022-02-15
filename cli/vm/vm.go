package vm

import (
	"bufio"
	"fmt"
	"os"
	"strings"

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
	afterF := func(context *cli.Context) error {
		reader := bufio.NewReader(os.Stdin)
		text, _ := reader.ReadString('\n')
		text = strings.TrimRight(text, "\n")
		commands := strings.Split(text, " ")
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

	ctl.Commands = []cli.Command{
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
			Name:  "someCmd",
			Usage: "Print line",
			Action: func(ctx *cli.Context) error {
				fmt.Fprintln(ctx.App.Writer, "do someCmd")
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
	return ctl.Run([]string{"vm", "logo"})
	/*
		p := vmcli.NewWithConfig(true, os.Exit, &readline.Config{
			Stdout: ctx.App.Writer,
			Stderr: ctx.App.ErrWriter,
		})
		return p.Run()
	*/
}
