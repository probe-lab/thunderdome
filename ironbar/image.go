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
	"strings"
	"time"

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
			Name:        "repo",
			Aliases:     []string{"g"},
			Usage:       "Build image from this Git repository",
			Destination: &imageOpts.gitRepo,
		},
		&cli.StringFlag{
			Name:        "branch",
			Aliases:     []string{"b"},
			Usage:       "Use this branch in the Git repository",
			Destination: &imageOpts.branch,
		},
		&cli.StringFlag{
			Name:        "commit",
			Aliases:     []string{"c"},
			Usage:       "Use this commit in the Git repository",
			Destination: &imageOpts.commit,
		},
		&cli.StringFlag{
			Name:        "tag",
			Aliases:     []string{"t"},
			Usage:       "Tag to apply to image. All tags are prefixed by '" + imageBaseName + ":'",
			Required:    true,
			Destination: &imageOpts.tag,
		},
		&cli.StringFlag{
			Name:        "from-image",
			Aliases:     []string{"f"},
			Usage:       "Build the image using this as the base",
			Destination: &imageOpts.fromImage,
		},
		&cli.StringFlag{
			Name:        "push-to",
			Aliases:     []string{"p"},
			Usage:       "Push built image to this docker repo",
			Destination: &imageOpts.dockerRepo,
		},
	},
}

var imageOpts struct {
	gitRepo    string
	branch     string
	commit     string
	tag        string
	dockerRepo string
	fromImage  string
}

const imageBaseName = "thunderdome"

func Image(cc *cli.Context) error {
	imageName := imageBaseName + ":" + imageOpts.tag
	log.Printf("building image %s", imageName)

	if imageOpts.gitRepo == "" && imageOpts.fromImage == "" {
		return fmt.Errorf("must specify repo or from-image")
	}

	workDir, err := os.MkdirTemp("", "thunderdome")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(workDir)
	log.Printf("using work dir %q", workDir)

	if imageOpts.gitRepo != "" {
		if imageOpts.branch != "" && imageOpts.commit != "" {
			return fmt.Errorf("must not specify both branch and commit")
		}

		if imageOpts.branch != "" {
			if err := buildImageFromGitBranch(workDir, imageOpts.gitRepo, imageOpts.branch, imageName); err != nil {
				return err
			}
		} else if imageOpts.commit != "" {
			if err := buildImageFromGitCommit(workDir, imageOpts.gitRepo, imageOpts.commit, imageName); err != nil {
				return err
			}
		} else {
			if err := buildImageFromGit(workDir, imageOpts.gitRepo, imageName); err != nil {
				return err
			}
		}
	} else if imageOpts.fromImage != "" {
		if err := buildImageFromImage(workDir, imageOpts.fromImage, imageName); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("must specify repo or from-image")
	}

	if imageOpts.dockerRepo != "" {
		if err := pushImage(imageName, imageOpts.dockerRepo); err != nil {
			return err
		}
		log.Printf("pushed to %s/%s", imageOpts.dockerRepo, imageName)
	}
	return nil
}

func gitClone(workdir string, repo string, target string) error {
	log.Printf("git clone %s", repo)
	cmd := exec.Command("git", "clone", repo, target)
	cmd.Dir = workdir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func gitSwitch(gitRepoDir string, branch string) error {
	log.Printf("git switch %s", branch)
	cmd := exec.Command("git", "switch", branch)
	cmd.Dir = gitRepoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func gitCheckout(gitRepoDir string, commit string) error {
	log.Printf("git checkout %s", commit)
	cmd := exec.Command("git", "checkout", commit)
	cmd.Dir = gitRepoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func dockerBuild(gitRepoDir string, imageName string) error {
	log.Printf("docker build -t %s .", imageName)
	cmd := exec.Command("docker", "build", "-t", imageName, ".")
	cmd.Dir = gitRepoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func dockerTag(srcImageName, dstImageName string) error {
	log.Printf("docker tag %s %s", srcImageName, dstImageName)
	cmd := exec.Command("docker", "tag", srcImageName, dstImageName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func dockerPush(imageName string) error {
	log.Printf("docker push %s", imageName)
	cmd := exec.Command("docker", "push", imageName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func dockerLogin(dockerRepo string) error {
	awsPwd, err := getAwsEcrPassword()
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

func getAwsEcrPassword() (string, error) {
	buf := new(bytes.Buffer)
	// docker login -u AWS -p $(aws ecr get-login-password --region eu-west-1) "$ECR_REPO"
	log.Println("aws", "ecr", "get-login-password", "--region", "eu-west-1")
	cmd := exec.Command("aws", "ecr", "get-login-password", "--region", "eu-west-1")
	cmd.Stdout = buf
	cmd.Stderr = os.Stderr

	cmd.Env = []string{
		fmt.Sprintf("AWS_PROFILE=%s", os.Getenv("AWS_PROFILE")),
	}

	if err := cmd.Start(); err != nil {
		return "", err
	}
	if err := cmd.Wait(); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func dockerBuildFromImage(buildDir, srcImageName, imageName string) error {
	fromImageNameArg := fmt.Sprintf("FROM_IMAGE_NAME=%s", srcImageName)

	log.Printf("docker build -t %s --build-arg %s .", imageName, fromImageNameArg)
	cmd := exec.Command("docker", "build", "-t", imageName, "--build-arg", fromImageNameArg, ".")
	cmd.Dir = buildDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

func copyDockerAssets(buildDir string, system string) error {
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
			if err := os.Mkdir(dst, 0777); err != nil {
				return fmt.Errorf("make dir %s: %w", dst, err)
			}
		} else {
			fdst, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE, info.Mode())
			if err != nil {
				return fmt.Errorf("open destination file: %w", err)
			}

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

func buildImageFromGitBranch(workDir string, gitRepo string, branch string, imageName string) error {
	log.Printf("building image from %s in %s", branch, gitRepo)
	const cloneName = "code"

	if err := gitClone(workDir, gitRepo, cloneName); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}

	gitRepoDir := filepath.Join(workDir, cloneName)
	if err := gitSwitch(gitRepoDir, branch); err != nil {
		return fmt.Errorf("git switch: %w", err)
	}

	tmpImageName := fmt.Sprintf("thunderdome:tmp-%d", time.Now().Unix())
	if err := dockerBuild(gitRepoDir, tmpImageName); err != nil {
		return fmt.Errorf("docker build: %w", err)
	}

	buildDir := filepath.Join(workDir, "build")
	if err := os.Mkdir(buildDir, 0777); err != nil {
		return fmt.Errorf("create build dir: %w", err)
	}

	if err := copyDockerAssets(buildDir, "kubo"); err != nil {
		return fmt.Errorf("copy docker assets: %w", err)
	}

	if err := dockerBuildFromImage(buildDir, tmpImageName, imageName); err != nil {
		return fmt.Errorf("docker build from tag docker assets: %w", err)
	}

	return nil
}

func buildImageFromGitCommit(workDir string, gitRepo string, commit string, imageName string) error {
	log.Printf("building image from %s in %s", commit, gitRepo)
	const cloneName = "code"

	if err := gitClone(workDir, gitRepo, cloneName); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}

	gitRepoDir := filepath.Join(workDir, cloneName)
	if err := gitCheckout(gitRepoDir, commit); err != nil {
		return fmt.Errorf("git checkout: %w", err)
	}

	tmpImageName := fmt.Sprintf("thunderdome:tmp-%d", time.Now().Unix())
	if err := dockerBuild(gitRepoDir, tmpImageName); err != nil {
		return fmt.Errorf("docker build: %w", err)
	}

	buildDir := filepath.Join(workDir, "build")
	if err := os.Mkdir(buildDir, 0777); err != nil {
		return fmt.Errorf("create build dir: %w", err)
	}

	if err := copyDockerAssets(buildDir, "kubo"); err != nil {
		return fmt.Errorf("copy docker assets: %w", err)
	}

	if err := dockerBuildFromImage(buildDir, tmpImageName, imageName); err != nil {
		return fmt.Errorf("docker build from tag docker assets: %w", err)
	}

	return nil
}

func buildImageFromGit(workDir string, gitRepo string, imageName string) error {
	log.Printf("building image from %s", gitRepo)
	const cloneName = "code"

	if err := gitClone(workDir, gitRepo, cloneName); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}

	gitRepoDir := filepath.Join(workDir, cloneName)
	tmpImageName := fmt.Sprintf("thunderdome:tmp-%d", time.Now().Unix())
	if err := dockerBuild(gitRepoDir, tmpImageName); err != nil {
		return fmt.Errorf("docker build: %w", err)
	}

	buildDir := filepath.Join(workDir, "build")
	if err := os.Mkdir(buildDir, 0777); err != nil {
		return fmt.Errorf("create build dir: %w", err)
	}

	if err := copyDockerAssets(buildDir, "kubo"); err != nil {
		return fmt.Errorf("copy docker assets: %w", err)
	}

	if err := dockerBuildFromImage(buildDir, tmpImageName, imageName); err != nil {
		return fmt.Errorf("docker build from tag docker assets: %w", err)
	}

	return nil
}

func buildImageFromImage(workDir, fromImage, imageName string) error {
	log.Printf("building image from %s", fromImage)
	buildDir := filepath.Join(workDir, "build")
	if err := os.Mkdir(buildDir, 0777); err != nil {
		return fmt.Errorf("create build dir: %w", err)
	}

	if err := copyDockerAssets(buildDir, "kubo"); err != nil {
		return fmt.Errorf("copy docker assets: %w", err)
	}

	if err := dockerBuildFromImage(buildDir, fromImage, imageName); err != nil {
		return fmt.Errorf("docker build from tag docker assets: %w", err)
	}

	return nil
}

func pushImage(imageName, dockerRepo string) error {
	log.Printf("pushing image %s to %s", imageName, dockerRepo)
	if err := dockerTag(imageName, dockerRepo+"/"+imageName); err != nil {
		return fmt.Errorf("docker tag: %w", err)
	}

	if err := dockerLogin(dockerRepo); err != nil {
		return fmt.Errorf("docker login: %w", err)
	}

	if err := dockerPush(dockerRepo + "/" + imageName); err != nil {
		return fmt.Errorf("docker push: %w", err)
	}

	return nil
}
