package build

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ipfs-shipyard/thunderdome/pkg/exp"
	"golang.org/x/exp/slog"
)

const imageBaseName = "thunderdome"

func LocalImageName(tag string) string {
	return imageBaseName + ":" + tag
}

func Build(ctx context.Context, tag string, spec *exp.ImageSpec) (string, error) {
	imageName := LocalImageName(tag)
	logger := slog.With("component", imageName)
	logger.Debug("building image")

	workDir, err := os.MkdirTemp("", "thunderdome")
	if err != nil {
		return "", fmt.Errorf("could not create temporary work directory")
	}
	logger.Debug(fmt.Sprintf("using work dir %q", workDir))

	var baseImage string
	if spec.Git != nil {
		if nonEmptyCount(spec.Git.Branch, spec.Git.Commit, spec.Git.Tag) > 1 {
			return "", fmt.Errorf("must only specify one of branch, commit or git-tag options")
		}

		if spec.Git.Branch != "" {
			logger.Info(fmt.Sprintf("building image from branch %s in %s", spec.Git.Branch, spec.Git.Repo))
			baseImage, err = BuildImageFromGitBranch(workDir, spec.Git.Repo, spec.Git.Branch, imageName)
			if err != nil {
				return "", err
			}
		} else if spec.Git.Commit != "" {
			logger.Info(fmt.Sprintf("building image from commit %s in %s", spec.Git.Commit, spec.Git.Repo))
			baseImage, err = BuildImageFromGitCommit(workDir, spec.Git.Repo, spec.Git.Commit, imageName)
			if err != nil {
				return "", err
			}
		} else if spec.Git.Tag != "" {
			logger.Info(fmt.Sprintf("building image from tag %s in %s", spec.Git.Tag, spec.Git.Repo))
			baseImage, err = BuildImageFromGitTag(workDir, spec.Git.Repo, spec.Git.Tag, imageName)
			if err != nil {
				return "", err
			}
		} else {
			logger.Info(fmt.Sprintf("building image from %s", spec.Git.Repo))
			baseImage, err = BuildImageFromGit(workDir, spec.Git.Repo, imageName)
			if err != nil {
				return "", err
			}
		}
	} else if spec.BaseImage != "" {
		logger.Info(fmt.Sprintf("building from base image %s", spec.BaseImage))
		baseImage = spec.BaseImage
	} else {
		return "", fmt.Errorf("must specify base image or git spec")
	}

	labels := map[string]string{
		"org.opencontainers.image.created": time.Now().Format(time.RFC3339),
	}
	if spec.Maintainer != "" {
		labels["maintainer"] = spec.Maintainer
	}
	if spec.Description != "" {
		labels["org.opencontainers.image.description"] = spec.Description
	}

	if err := ConfigureImage(workDir, baseImage, imageName, labels, spec.InitCommands); err != nil {
		return "", err
	}

	return imageName, nil
}

func CopyDockerAssets(buildDir string, system string) error {
	systemPath := filepath.Join("dockerassets", system)
	pathPrefix := systemPath + string(filepath.Separator)
	err := fs.WalkDir(dockerAssets, systemPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk error: %w", err)
		}

		if !strings.HasPrefix(path, pathPrefix) {
			return nil
		}
		dst := strings.Replace(path, pathPrefix, buildDir+string(filepath.Separator), 1)

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("reading info for %s: %w", d.Name(), err)
		}

		if d.IsDir() {
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

func BuildImageFromGitBranch(workDir string, gitRepo string, branch string, imageName string) (string, error) {
	const cloneName = "code"

	if err := GitClone(workDir, gitRepo, cloneName); err != nil {
		return "", fmt.Errorf("git clone: %w", err)
	}

	gitRepoDir := filepath.Join(workDir, cloneName)
	if err := GitSwitch(gitRepoDir, branch); err != nil {
		return "", fmt.Errorf("git switch: %w", err)
	}

	tmpImageName := fmt.Sprintf("thunderdome:tmp-%d", time.Now().Unix())
	if err := DockerBuild(gitRepoDir, tmpImageName); err != nil {
		return "", fmt.Errorf("docker build: %w", err)
	}

	return tmpImageName, nil
}

func BuildImageFromGitCommit(workDir string, gitRepo string, commit string, imageName string) (string, error) {
	const cloneName = "code"

	if err := GitClone(workDir, gitRepo, cloneName); err != nil {
		return "", fmt.Errorf("git clone: %w", err)
	}

	gitRepoDir := filepath.Join(workDir, cloneName)
	if err := GitCheckout(gitRepoDir, commit); err != nil {
		return "", fmt.Errorf("git checkout: %w", err)
	}

	tmpImageName := fmt.Sprintf("thunderdome:tmp-%d", time.Now().Unix())
	if err := DockerBuild(gitRepoDir, tmpImageName); err != nil {
		return "", fmt.Errorf("docker build: %w", err)
	}
	return tmpImageName, nil
}

func BuildImageFromGitTag(workDir string, gitRepo string, gitTag string, imageName string) (string, error) {
	const cloneName = "code"

	if err := GitClone(workDir, gitRepo, cloneName); err != nil {
		return "", fmt.Errorf("git clone: %w", err)
	}

	gitRepoDir := filepath.Join(workDir, cloneName)
	if err := GitFetchTags(gitRepoDir); err != nil {
		return "", fmt.Errorf("git fetch tags: %w", err)
	}

	if !strings.HasPrefix(gitTag, "tags/") {
		gitTag = "tags/" + gitTag
	}
	if err := GitCheckout(gitRepoDir, gitTag); err != nil {
		return "", fmt.Errorf("git checkout: %w", err)
	}

	tmpImageName := fmt.Sprintf("thunderdome:tmp-%d", time.Now().Unix())
	if err := DockerBuild(gitRepoDir, tmpImageName); err != nil {
		return "", fmt.Errorf("docker build: %w", err)
	}
	return tmpImageName, nil
}

func BuildImageFromGit(workDir string, gitRepo string, imageName string) (string, error) {
	const cloneName = "code"

	if err := GitClone(workDir, gitRepo, cloneName); err != nil {
		return "", fmt.Errorf("git clone: %w", err)
	}

	gitRepoDir := filepath.Join(workDir, cloneName)
	tmpImageName := fmt.Sprintf("thunderdome:tmp-%d", time.Now().Unix())
	if err := DockerBuild(gitRepoDir, tmpImageName); err != nil {
		return "", fmt.Errorf("docker build: %w", err)
	}
	return tmpImageName, nil
}

func DockerBuildFromImage(buildDir, srcImageName, imageName string, labels map[string]string) error {
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
	slog.Debug(cmd.String())
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func ConfigureImage(workDir, fromImage, imageName string, labels map[string]string, initCommands []string) error {
	logger := slog.With("component", imageName)
	logger.Info(fmt.Sprintf("configuring image %s for use in thunderdome", fromImage))
	buildDir := filepath.Join(workDir, "build")
	if err := os.Mkdir(buildDir, 0o777); err != nil {
		return fmt.Errorf("create build dir: %w", err)
	}

	logger.Debug("copying docker assets into build")
	if err := CopyDockerAssets(buildDir, "kubo"); err != nil {
		return fmt.Errorf("copy docker assets: %w", err)
	}

	logger.Debug("writing init config script")
	if err := writeInitConfigScript(buildDir, initCommands); err != nil {
		return fmt.Errorf("write init config script: %w", err)
	}

	if err := DockerBuildFromImage(buildDir, fromImage, imageName, labels); err != nil {
		return fmt.Errorf("docker build from tag: %w", err)
	}

	return nil
}

func PushImage(tag, awsRegion, ecrRepo string) (string, error) {
	localName := LocalImageName(tag)
	remoteName := ecrRepo + ":" + tag
	if err := DockerTag(localName, remoteName); err != nil {
		return "", fmt.Errorf("docker tag: %w", err)
	}

	if err := EcrLogin(ecrRepo, awsRegion); err != nil {
		return "", fmt.Errorf("docker login: %w", err)
	}

	if err := DockerPush(remoteName); err != nil {
		return "", fmt.Errorf("docker push: %w", err)
	}

	return remoteName, nil
}

func ImageExists(tag, awsRegion, ecrRepo string) (bool, error) {
	remoteName := ecrRepo + ":" + tag
	slog.Info("checking if image exists in repo", "image", remoteName)
	if err := EcrLogin(ecrRepo, awsRegion); err != nil {
		return false, fmt.Errorf("docker login: %w", err)
	}

	if err := DockerInspect(remoteName); err != nil {
		return false, err
	}

	return true, nil
}

func EcrLogin(dockerRepo string, awsRegion string) error {
	awsPwd, err := GetAwsEcrPassword(awsRegion)
	if err != nil {
		return fmt.Errorf("get aws ecr password: %w", err)
	}

	// docker login -u AWS -p $(aws ecr get-login-password --region eu-west-1) "$ECR_REPO"
	cmd := exec.Command("docker", "login", "-u", "AWS", "-p", awsPwd, dockerRepo)
	slog.Debug("docker login -u AWS -p REDACTED " + dockerRepo)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr

	cmd.Env = []string{
		fmt.Sprintf("AWS_PROFILE=%s", os.Getenv("AWS_PROFILE")),
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func GetAwsEcrPassword(awsRegion string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := exec.Command("aws", "ecr", "get-login-password", "--region", awsRegion)
	cmd.Stdout = buf
	cmd.Stderr = os.Stderr

	cmd.Env = []string{
		fmt.Sprintf("AWS_PROFILE=%s", os.Getenv("AWS_PROFILE")),
	}

	slog.Debug(cmd.String())
	if err := cmd.Start(); err != nil {
		return "", err
	}
	if err := cmd.Wait(); err != nil {
		return "", err
	}

	return buf.String(), nil
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

func writeInitConfigScript(buildDir string, initCommands []string) error {
	if len(initCommands) == 0 {
		return nil
	}
	b := new(strings.Builder)
	b.WriteString("#!/bin/sh\n")

	for _, cmd := range initCommands {
		fmt.Fprintln(b, cmd)
	}

	initConfigFilename := filepath.Join(buildDir, "container-init.d", "02-env-config.sh")
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
