package main

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/urfave/cli/v2"
)

//go:embed dockerassets
var dockerAssets embed.FS

var ImageCommand = &cli.Command{
	Name:   "image",
	Usage:  "Build a docker image for an experiment",
	Action: Image,
	Flags: []cli.Flag{
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
	},
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
	imageName := imageBaseName + ":" + imageOpts.tag
	log.Printf("building image %s", imageName)

	envConfigMappings, err := parseEnvConfigMappings(imageOpts.envConfig.Value())
	if err != nil {
		return err
	}

	envConfigMappingsQuoted, err := parseEnvConfigMappings(imageOpts.envConfigQuoted.Value())
	if err != nil {
		return err
	}

	if imageOpts.gitRepo == "" && imageOpts.fromImage == "" {
		return fmt.Errorf("must specify one of repo or from-image options")
	}

	workDir, err := os.MkdirTemp("", "thunderdome")
	if err != nil {
		return fmt.Errorf("could not create temporary work directory")
	}
	if !cc.Bool("keeptemp") {
		defer os.RemoveAll(workDir)
	}
	log.Printf("using work dir %q\n", workDir)

	labels := map[string]string{
		"maintainer":                       imageOpts.maintainer,
		"org.opencontainers.image.created": time.Now().Format(time.RFC3339),
	}
	if imageOpts.description != "" {
		labels["org.opencontainers.image.description"] = imageOpts.description
	}

	var baseImage string
	if imageOpts.gitRepo != "" {
		if nonEmptyCount(imageOpts.branch, imageOpts.commit, imageOpts.gitTag) > 1 {
			return fmt.Errorf("must only specify one of branch, commit or git-tag options")
		}

		if imageOpts.branch != "" {
			baseImage, err = buildImageFromGitBranch(workDir, imageOpts.gitRepo, imageOpts.branch, imageName)
			if err != nil {
				return err
			}
		} else if imageOpts.commit != "" {
			baseImage, err = buildImageFromGitCommit(workDir, imageOpts.gitRepo, imageOpts.commit, imageName)
			if err != nil {
				return err
			}
		} else if imageOpts.gitTag != "" {
			baseImage, err = buildImageFromGitTag(workDir, imageOpts.gitRepo, imageOpts.gitTag, imageName)
			if err != nil {
				return err
			}
		} else {
			baseImage, err = buildImageFromGit(workDir, imageOpts.gitRepo, imageName)
			if err != nil {
				return err
			}
		}
	} else if imageOpts.fromImage != "" {
		baseImage = imageOpts.fromImage
	} else {
		return fmt.Errorf("must specify repo or from-image")
	}

	baseConfig, ok := baseConfigs[imageOpts.baseConfig]
	if !ok {
		return fmt.Errorf("unknown base config specified: %s (expected one of %s)", imageOpts.baseConfig, baseConfigNames)
	}

	if err := configureImage(workDir, baseImage, imageName, labels, baseConfig, envConfigMappings, envConfigMappingsQuoted); err != nil {
		return err
	}

	finalImage := imageName

	if imageOpts.dockerRepo != "" {
		if err := pushImage(imageName, imageOpts.dockerRepo); err != nil {
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
	fmt.Printf("Uses %s as base configuration\n", imageOpts.baseConfig)
	if len(envConfigMappings) > 0 || len(envConfigMappingsQuoted) > 0 {
		fmt.Println("Additional configuration may be set using environment variables:")
		for _, em := range envConfigMappings {
			fmt.Printf("  $%s sets %s\n", em.EnvVar, em.ConfigOption)
		}
		for _, em := range envConfigMappingsQuoted {
			fmt.Printf("  $%s sets %s\n", em.EnvVar, em.ConfigOption)
		}
	}
	fmt.Println("--------------------------------------------------------------------------------")

	if cc.Bool("keeptemp") {
		fmt.Println("Build files kept in: " + workDir)
	}

	return nil
}

func gitClone(workdir string, repo string, targetDir string) error {
	cmd := exec.Command("git", "clone", repo, targetDir)
	cmd.Dir = workdir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Println(cmd.String())
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func gitSwitch(gitRepoDir string, branch string) error {
	cmd := exec.Command("git", "switch", branch)
	cmd.Dir = gitRepoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Println(cmd.String())
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func gitCheckout(gitRepoDir string, ref string) error {
	cmd := exec.Command("git", "checkout", "--detach", ref)
	cmd.Dir = gitRepoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Println(cmd.String())
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func dockerBuild(gitRepoDir string, imageName string) error {
	cmd := exec.Command("docker", "build", "-t", imageName, ".")
	cmd.Dir = gitRepoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Println(cmd.String())
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func dockerTag(srcImageName, dstImageName string) error {
	cmd := exec.Command("docker", "tag", srcImageName, dstImageName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Println(cmd.String())
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func dockerPush(imageName string) error {
	cmd := exec.Command("docker", "push", imageName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Println(cmd.String())
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func ecrLogin(dockerRepo string, awsRegion string) error {
	awsPwd, err := getAwsEcrPassword(awsRegion)
	if err != nil {
		return fmt.Errorf("get aws ecr password: %w", err)
	}

	// docker login -u AWS -p $(aws ecr get-login-password --region eu-west-1) "$ECR_REPO"
	log.Println("docker", "login", "-u", "AWS", "-p", "REDACTED", dockerRepo)
	cmd := exec.Command("docker", "login", "-u", "AWS", "-p", awsPwd, dockerRepo)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.Env = []string{
		fmt.Sprintf("AWS_PROFILE=%s", os.Getenv("AWS_PROFILE")),
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func getAwsEcrPassword(awsRegion string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := exec.Command("aws", "ecr", "get-login-password", "--region", awsRegion)
	cmd.Stdout = buf
	cmd.Stderr = os.Stderr

	cmd.Env = []string{
		fmt.Sprintf("AWS_PROFILE=%s", os.Getenv("AWS_PROFILE")),
	}

	log.Println(cmd.String())
	if err := cmd.Start(); err != nil {
		return "", err
	}
	if err := cmd.Wait(); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func dockerBuildFromImage(buildDir, srcImageName, imageName string, labels map[string]string) error {
	log.Printf("building docker image %s using %s as base", imageName, srcImageName)
	fromImageNameArg := fmt.Sprintf("FROM_IMAGE_NAME=%s", srcImageName)

	dockerArgs := []string{"build", "-t", imageName, "--build-arg", fromImageNameArg}
	for k, v := range labels {
		dockerArgs = append(dockerArgs, "--label")
		dockerArgs = append(dockerArgs, fmt.Sprintf("%s=%s", k, v))
	}
	dockerArgs = append(dockerArgs, ".")

	cmd := exec.Command("docker", dockerArgs...)
	cmd.Dir = buildDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Println(cmd.String())
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func copyDockerAssets(buildDir string, system string) error {
	log.Printf("copying docker assets for %s to %s", system, buildDir)
	systemPath := filepath.Join("dockerassets", system)
	pathPrefix := systemPath + string(filepath.Separator)
	err := fs.WalkDir(dockerAssets, systemPath, func(path string, d fs.DirEntry, err error) error {
		if !strings.HasPrefix(path, pathPrefix) {
			return nil
		}
		dst := strings.Replace(path, pathPrefix, buildDir+string(filepath.Separator), 1)

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("reading info for %s: %w", d.Name(), err)
		}

		if d.IsDir() {
			log.Printf("creating directory %s", dst)
			if err := os.Mkdir(dst, 0o777); err != nil {
				return fmt.Errorf("make dir %s: %w", dst, err)
			}
		} else {
			fdst, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE, info.Mode())
			if err != nil {
				return fmt.Errorf("open destination file: %w", err)
			}
			defer fdst.Close()

			fsrc, err := dockerAssets.Open(path)
			if err != nil {
				return fmt.Errorf("open source file %s: %w", path, err)
			}
			log.Printf("copying %s to %s", path, dst)
			_, err = io.Copy(fdst, fsrc)
			if err != nil {
				return fmt.Errorf("copying source file %s: %w", path, err)
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("walk error: %w", err)
	}
	return nil
}

func buildImageFromGitBranch(workDir string, gitRepo string, branch string, imageName string) (string, error) {
	log.Printf("building image from %s in %s", branch, gitRepo)
	const cloneName = "code"

	if err := gitClone(workDir, gitRepo, cloneName); err != nil {
		return "", fmt.Errorf("git clone: %w", err)
	}

	gitRepoDir := filepath.Join(workDir, cloneName)
	if err := gitSwitch(gitRepoDir, branch); err != nil {
		return "", fmt.Errorf("git switch: %w", err)
	}

	tmpImageName := fmt.Sprintf("thunderdome:tmp-%d", time.Now().Unix())
	if err := dockerBuild(gitRepoDir, tmpImageName); err != nil {
		return "", fmt.Errorf("docker build: %w", err)
	}

	return tmpImageName, nil
}

func buildImageFromGitCommit(workDir string, gitRepo string, commit string, imageName string) (string, error) {
	log.Printf("building image from %s in %s", commit, gitRepo)
	const cloneName = "code"

	if err := gitClone(workDir, gitRepo, cloneName); err != nil {
		return "", fmt.Errorf("git clone: %w", err)
	}

	gitRepoDir := filepath.Join(workDir, cloneName)
	if err := gitCheckout(gitRepoDir, commit); err != nil {
		return "", fmt.Errorf("git checkout: %w", err)
	}

	tmpImageName := fmt.Sprintf("thunderdome:tmp-%d", time.Now().Unix())
	if err := dockerBuild(gitRepoDir, tmpImageName); err != nil {
		return "", fmt.Errorf("docker build: %w", err)
	}
	return tmpImageName, nil
}

func buildImageFromGitTag(workDir string, gitRepo string, gitTag string, imageName string) (string, error) {
	log.Printf("building image from tag %s in %s", gitTag, gitRepo)
	const cloneName = "code"

	if err := gitClone(workDir, gitRepo, cloneName); err != nil {
		return "", fmt.Errorf("git clone: %w", err)
	}

	if !strings.HasPrefix(gitTag, "tags/") {
		gitTag = "tags/" + gitTag
	}

	gitRepoDir := filepath.Join(workDir, cloneName)
	if err := gitCheckout(gitRepoDir, gitTag); err != nil {
		return "", fmt.Errorf("git checkout: %w", err)
	}

	tmpImageName := fmt.Sprintf("thunderdome:tmp-%d", time.Now().Unix())
	if err := dockerBuild(gitRepoDir, tmpImageName); err != nil {
		return "", fmt.Errorf("docker build: %w", err)
	}
	return tmpImageName, nil
}

func buildImageFromGit(workDir string, gitRepo string, imageName string) (string, error) {
	log.Printf("building image from %s", gitRepo)
	const cloneName = "code"

	if err := gitClone(workDir, gitRepo, cloneName); err != nil {
		return "", fmt.Errorf("git clone: %w", err)
	}

	gitRepoDir := filepath.Join(workDir, cloneName)
	tmpImageName := fmt.Sprintf("thunderdome:tmp-%d", time.Now().Unix())
	if err := dockerBuild(gitRepoDir, tmpImageName); err != nil {
		return "", fmt.Errorf("docker build: %w", err)
	}
	return tmpImageName, nil
}

func configureImage(workDir, fromImage, imageName string, labels map[string]string, baseConfig string, envConfigMappings []EnvConfigMapping, envConfigMappingsQuoted []EnvConfigMapping) error {
	log.Printf("configuring image %s for use in thunderdome", fromImage)
	buildDir := filepath.Join(workDir, "build")
	if err := os.Mkdir(buildDir, 0o777); err != nil {
		return fmt.Errorf("create build dir: %w", err)
	}

	if err := copyDockerAssets(buildDir, "kubo"); err != nil {
		return fmt.Errorf("copy docker assets: %w", err)
	}

	if err := writeInitConfigScript(buildDir, baseConfig, envConfigMappings, envConfigMappingsQuoted); err != nil {
		return fmt.Errorf("write init config script: %w", err)
	}

	if err := dockerBuildFromImage(buildDir, fromImage, imageName, labels); err != nil {
		return fmt.Errorf("docker build from tag docker assets: %w", err)
	}

	return nil
}

func pushImage(imageName, ecrRepo string) error {
	log.Printf("pushing image %s to %s", imageName, ecrRepo)
	if err := dockerTag(imageName, ecrRepo+"/"+imageName); err != nil {
		return fmt.Errorf("docker tag: %w", err)
	}

	if err := ecrLogin(ecrRepo, "eu-west-1"); err != nil {
		return fmt.Errorf("docker login: %w", err)
	}

	if err := dockerPush(ecrRepo + "/" + imageName); err != nil {
		return fmt.Errorf("docker push: %w", err)
	}

	return nil
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

type EnvConfigMapping struct {
	EnvVar       string
	ConfigOption string
}

func parseEnvConfigMappings(strs []string) ([]EnvConfigMapping, error) {
	var ret []EnvConfigMapping

	for _, s := range strs {
		parts := strings.SplitN(s, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed mapping, expecting format 'EnvVar:ConfigOption', got '%s'", s)
		}
		ret = append(ret, EnvConfigMapping{
			EnvVar:       parts[0],
			ConfigOption: parts[1],
		})
	}

	return ret, nil
}

func writeInitConfigScript(buildDir string, baseConfig string, envConfigMappings []EnvConfigMapping, envConfigMappingsQuoted []EnvConfigMapping) error {
	if len(envConfigMappings) == 0 {
		return nil
	}
	b := new(strings.Builder)
	b.WriteString("#!/bin/sh\n")

	if baseConfig != "" {
		fmt.Fprintln(b, baseConfig)
	}

	for _, ec := range envConfigMappings {
		fmt.Fprintf(b, `if [ -n "$%s" ]; then`, ec.EnvVar)
		fmt.Fprintln(b)
		fmt.Fprintf(b, `  echo "setting %s to $%s"`, ec.ConfigOption, ec.EnvVar)
		fmt.Fprintln(b)
		fmt.Fprintf(b, `  ipfs config --json %s "$%s"`, ec.ConfigOption, ec.EnvVar)
		fmt.Fprintln(b)
		fmt.Fprint(b, `fi`)
		fmt.Fprintln(b)
	}
	for _, ec := range envConfigMappingsQuoted {
		fmt.Fprintf(b, `if [ -n "$%s" ]; then`, ec.EnvVar)
		fmt.Fprintln(b)
		fmt.Fprintf(b, `  echo "setting %s to $%s"`, ec.ConfigOption, ec.EnvVar)
		fmt.Fprintln(b)
		fmt.Fprintf(b, `  ipfs config --json %s "\"$%s\""`, ec.ConfigOption, ec.EnvVar)
		fmt.Fprintln(b)
		fmt.Fprint(b, `fi`)
		fmt.Fprintln(b)
	}

	initConfigFilename := filepath.Join(buildDir, "container-init.d", "02-env-config.sh")
	log.Printf("writing init config script %s", initConfigFilename)
	f, err := os.OpenFile(initConfigFilename, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("open init config file: %w", err)
	}
	defer f.Close()

	if _, err := fmt.Fprintln(f, b.String()); err != nil {
		return fmt.Errorf("write init config file: %w", err)
	}

	return nil
}

var baseConfigNames string

func init() {
	names := make([]string, 0, len(baseConfigs))
	for name := range baseConfigs {
		names = append(names, name)
	}
	sort.Strings(names)
	baseConfigNames = strings.Join(names, ",")
}

var baseConfigs = map[string]string{
	"kubo-default": unindent(`
				echo "using kubo-default config"
	`),
	"bifrost": unindent(`
				echo "using bifrost style base config"
				ipfs config --json AutoNAT '{"ServiceMode": "disabled"}'

				ipfs config --json Datastore.BloomFilterSize '268435456'
				ipfs config --json Datastore.StorageGCWatermark 90
				ipfs config --json Datastore.StorageMax '"1000GB"'

				ipfs config --json Pubsub.StrictSignatureVerification false

				ipfs config --json Reprovider.Interval '"0"'

				ipfs config --json Routing.Type '"dhtserver"'

				ipfs config --json Swarm.ConnMgr.GracePeriod '"2m"'
				ipfs config --json Swarm.ConnMgr.HighWater 5000
				ipfs config --json Swarm.ConnMgr.LowWater 3000
				ipfs config --json Swarm.ConnMgr.DisableBandwidthMetrics true

				ipfs config --json Experimental.AcceleratedDHTClient true
				ipfs config --json Experimental.StrategicProviding true
			`),
}

func unindent(s string) string {
	buf := new(strings.Builder)
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		buf.WriteString(strings.TrimLeftFunc(line, unicode.IsSpace))
	}
	return buf.String()
}
