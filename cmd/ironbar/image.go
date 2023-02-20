package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/ipfs-shipyard/thunderdome/cmd/ironbar/build"
	"github.com/ipfs-shipyard/thunderdome/cmd/ironbar/exp"
)

var ImageCommand = &cli.Command{
	Name:   "image",
	Usage:  "Build a docker image for an experiment",
	Action: Image,
	Flags: flags([]cli.Flag{
		&cli.StringFlag{
			Name:        "from-repo",
			Aliases:     []string{"r"},
			Usage:       "Build Thunderdome image from this Git repository.",
			Destination: &imageOpts.gitRepo,
		},
		&cli.StringFlag{
			Name:        "from-image",
			Usage:       "Build the Thunderdome image using this image as the base.",
			Destination: &imageOpts.fromImage,
		},
		&cli.StringFlag{
			Name:        "branch",
			Usage:       "Switch to this branch in the Git repository.",
			Destination: &imageOpts.branch,
		},
		&cli.StringFlag{
			Name:        "commit",
			Usage:       "Switch to this commit in the Git repository.",
			Destination: &imageOpts.commit,
		},
		&cli.StringFlag{
			Name:        "git-tag",
			Usage:       "Switch to this tag in the Git repository.",
			Destination: &imageOpts.gitTag,
		},
		&cli.StringFlag{
			Name:        "tag",
			Usage:       "Tag to apply to image. All tags are prefixed by '" + imageBaseName + ":'.",
			Required:    true,
			Destination: &imageOpts.tag,
		},
		&cli.StringFlag{
			Name:        "push-to",
			Usage:       "Push built image to this docker repo.",
			Destination: &imageOpts.dockerRepo,
		},
		&cli.StringFlag{
			Name:        "base-config",
			Usage:       "Name of a built-in base config to apply. One of " + baseConfigNames,
			Destination: &imageOpts.baseConfig,
			Value:       "bifrost",
		},
		&cli.StringSliceFlag{
			Name:        "env-config",
			Usage:       "Map an environment variable to a kubo config option. Use format 'EnvVar:ConfigOption'.",
			Destination: &imageOpts.envConfig,
		},
		&cli.StringSliceFlag{
			Name:        "env-config-quoted",
			Usage:       "Map an environment variable to a kubo config option that requires quotes (such as a string or duration). Use format 'EnvVar:ConfigOption'.",
			Destination: &imageOpts.envConfigQuoted,
		},
		&cli.BoolFlag{
			Name:   "keeptemp",
			Usage:  "Keep the temporary working directory after ironbar exits.",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:        "description",
			Usage:       "Human readable description of the image and its purpose.",
			Destination: &imageOpts.description,
		},
		&cli.StringFlag{
			Name:        "maintainer",
			Usage:       "Email address of the maintainer.",
			Destination: &imageOpts.maintainer,
			Value:       "ian.davis@protocol.ai",
		},
		// TODO: support  ARG IPFS_PLUGINS
	}),
}

var imageOpts struct {
	gitRepo         string
	branch          string
	commit          string
	gitTag          string
	tag             string
	dockerRepo      string
	fromImage       string
	envConfig       cli.StringSlice
	envConfigQuoted cli.StringSlice
	description     string
	maintainer      string
	baseConfig      string
}

const imageBaseName = "thunderdome"

func Image(cc *cli.Context) error {
	ctx := cc.Context
	setupLogging()
	if err := checkBuildEnv(); err != nil {
		return err
	}
	imageName := imageBaseName + ":" + imageOpts.tag

	envConfigMappings, err := parseEnvConfigMappings(imageOpts.envConfig.Value(), false)
	if err != nil {
		return err
	}

	envConfigMappingsQuoted, err := parseEnvConfigMappings(imageOpts.envConfigQuoted.Value(), true)
	if err != nil {
		return err
	}

	if imageOpts.gitRepo == "" && imageOpts.fromImage == "" {
		return fmt.Errorf("must specify one of repo or from-image options")
	}

	spec := &exp.ImageSpec{
		Maintainer:  imageOpts.maintainer,
		Description: imageOpts.description,
	}

	if imageOpts.gitRepo != "" {
		if nonEmptyCount(imageOpts.branch, imageOpts.commit, imageOpts.gitTag) > 1 {
			return fmt.Errorf("must only specify one of branch, commit or git-tag options")
		}
		spec.Git = &exp.GitSpec{
			Repo:   imageOpts.gitRepo,
			Branch: imageOpts.branch,
			Commit: imageOpts.commit,
			Tag:    imageOpts.gitTag,
		}
	} else if imageOpts.fromImage != "" {
		spec.BaseImage = imageOpts.fromImage
	} else {
		return fmt.Errorf("must specify repo or from-image")
	}

	baseConfig, ok := build.BaseConfigs[imageOpts.baseConfig]
	if !ok {
		return fmt.Errorf("unknown base config specified: %s (expected one of %s)", imageOpts.baseConfig, baseConfigNames)
	}

	spec.InitCommands = append(spec.InitCommands, baseConfig...)
	spec.InitCommands = append(spec.InitCommands, envConfigMappings...)
	spec.InitCommands = append(spec.InitCommands, envConfigMappingsQuoted...)

	finalImage, err := build.Build(ctx, imageOpts.tag, spec)

	if imageOpts.dockerRepo != "" {
		if err := build.PushImage(imageName, imageOpts.dockerRepo); err != nil {
			return err
		}
		finalImage = fmt.Sprintf("%s/%s", imageOpts.dockerRepo, imageName)
	}

	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Printf("Image build complete: %s\n", finalImage)
	if imageOpts.gitRepo != "" {
		ref := "default branch"
		if imageOpts.branch != "" {
			ref = "branch " + imageOpts.branch
		} else if imageOpts.commit != "" {
			ref = "commit " + imageOpts.commit
		} else if imageOpts.gitTag != "" {
			ref = "tag " + imageOpts.gitTag
		}
		fmt.Printf("Built from %s in %s\n", ref, imageOpts.gitRepo)
	} else if imageOpts.fromImage != "" {
		fmt.Printf("Used %s as base image\n", imageOpts.fromImage)
	}
	if imageOpts.description != "" {
		fmt.Printf("Description: %s\n", imageOpts.description)
	}
	if len(spec.InitCommands) > 0 {
		fmt.Printf("Commands executed at init time:\n")
		for _, cmd := range spec.InitCommands {
			fmt.Printf("  %s\n", cmd)
		}
	}
	fmt.Println("--------------------------------------------------------------------------------")

	return nil
}

var baseConfigNames string

func init() {
	names := make([]string, 0, len(build.BaseConfigs))
	for name := range build.BaseConfigs {
		names = append(names, name)
	}
	sort.Strings(names)
	baseConfigNames = strings.Join(names, ",")
}

func parseEnvConfigMappings(strs []string, quote bool) ([]string, error) {
	var cmds []string
	for _, s := range strs {
		parts := strings.SplitN(s, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed mapping, expecting format 'EnvVar:ConfigOption', got '%s'", s)
		}
		envVar := parts[0]
		configOption := parts[1]

		cmds = append(cmds, fmt.Sprintf(`if [ -n "$%s" ]; then`, envVar))
		cmds = append(cmds, fmt.Sprintf(`  echo "setting %s to $%s"`, configOption, envVar))
		if quote {
			cmds = append(cmds, fmt.Sprintf(`  ipfs config --json %s "\"$%s\""`, configOption, envVar))
		} else {
			cmds = append(cmds, fmt.Sprintf(`  ipfs config --json %s "$%s"`, configOption, envVar))
		}
		cmds = append(cmds, fmt.Sprintf(`fi`))
	}

	return cmds, nil
}

// nonEmptyCount returns the number the passed strings that are not empty
func nonEmptyCount(strs ...string) int {
	nonEmpty := 0
	for _, s := range strs {
		if len(s) > 0 {
			nonEmpty++
		}
	}
	return nonEmpty
}
