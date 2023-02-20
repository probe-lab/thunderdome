package build

import (
	"embed"
	"os"
	"os/exec"

	"golang.org/x/exp/slog"
)

//go:embed dockerassets
var dockerAssets embed.FS

func DockerBuild(gitRepoDir string, imageName string) error {
	cmd := exec.Command("docker", "build", "-t", imageName, ".")
	cmd.Dir = gitRepoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	slog.Debug(cmd.String())
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func DockerTag(srcImageName, dstImageName string) error {
	cmd := exec.Command("docker", "tag", srcImageName, dstImageName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	slog.Debug(cmd.String())
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func DockerPush(imageName string) error {
	cmd := exec.Command("docker", "push", imageName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	slog.Debug(cmd.String())
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}
