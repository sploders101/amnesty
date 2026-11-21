package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"charm.land/huh/v2"
	"github.com/sploders101/amnesty/internal/blueprint"
	"github.com/sploders101/amnesty/internal/install"
	"github.com/urfave/cli/v3"
)

func installCli() *cli.Command {
	var osconfig string
	hostname, err := os.ReadFile("/etc/hostname")
	if err == nil {
		osconfig = strings.TrimSpace(string(hostname))
	}
	modules := "/etc/amnesty/modules"

	return &cli.Command{
		Name:  "install",
		Usage: "Installs an Arch sysroot in the specified directory",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "osconfig",
				Usage: "The OSConfig profile to use for the installation",
				Value: osconfig,
			},
			&cli.StringFlag{
				Name:  "modules",
				Usage: "Path to the modules file or directory",
				Value: modules,
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "rootfs",
				Required:  true,
				UsageText: "<rootfs>",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			osconfig := cmd.String("osconfig")
			modules := cmd.String("modules")
			rootfs := cmd.StringArg("rootfs")

			osTemplate, err := blueprint.LoadOSTemplate(ctx, modules)
			if err != nil {
				return err
			}

			blueprint, err := blueprint.AssembleBlueprint(osTemplate, osconfig)
			if err != nil {
				return err
			}

			var password string
			if err := huh.NewInput().
				EchoMode(huh.EchoModePassword).
				Title(fmt.Sprintf("Password for user %q", blueprint.Username)).
				Value(&password).
				Run(); err != nil {
				return fmt.Errorf("failed to get password: %w", err)
			}

			if err := install.InstallBlueprint(
				ctx,
				blueprint,
				rootfs,
				password,
			); err != nil {
				return err
			}

			return nil
		},
	}
}
