package build

import (
	"os"
	"os/exec"

	"golang.org/x/exp/slog"
)

func GitClone(workdir string, repo string, targetDir string) error {
	cmd := exec.Command("git", "clone", repo, targetDir)
	cmd.Dir = workdir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	slog.Debug(cmd.String())
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func GitSwitch(gitRepoDir string, branch string) error {
	cmd := exec.Command("git", "switch", branch)
	cmd.Dir = gitRepoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	slog.Debug(cmd.String())
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func GitCheckout(gitRepoDir string, ref string) error {
	cmd := exec.Command("git", "checkout", "--detach", ref)
	cmd.Dir = gitRepoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	slog.Debug(cmd.String())
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func GitFetchTags(workdir string) error {
	cmd := exec.Command("git", "fetch", "origin", "--tags")
	cmd.Dir = workdir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	slog.Debug(cmd.String())
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}
