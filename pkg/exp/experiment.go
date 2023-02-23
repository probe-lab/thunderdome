package exp

import (
	"encoding/base32"
	"hash/fnv"
	"strings"
	"time"
)

type Experiment struct {
	Name        string
	Description string

	Duration       time.Duration
	MaxRequestRate int
	MaxConcurrency int
	RequestFilter  string

	Targets []*TargetSpec
}

type TargetSpec struct {
	Name         string
	Image        string
	ImageSpec    *ImageSpec
	InstanceType string
	Environment  map[string]string
}

type ImageSpec struct {
	Maintainer   string
	Description  string
	BaseImage    string
	Git          *GitSpec
	InitCommands []string
}

type GitSpec struct {
	Repo   string
	Commit string
	Tag    string
	Branch string
}

func (is *ImageSpec) Hash() string {
	h := fnv.New64()

	h.Write([]byte(is.Maintainer))
	h.Write([]byte{0x31})

	h.Write([]byte(is.Description))
	h.Write([]byte{0x31})

	h.Write([]byte(is.BaseImage))
	h.Write([]byte{0x31})

	if is.Git != nil {
		h.Write([]byte(is.Git.Repo))
		h.Write([]byte{0x31})
		h.Write([]byte(is.Git.Commit))
		h.Write([]byte{0x31})
		h.Write([]byte(is.Git.Tag))
		h.Write([]byte{0x31})
		h.Write([]byte(is.Git.Branch))
		h.Write([]byte{0x31})
	}
	h.Write([]byte{0x31})

	for _, cmd := range is.InitCommands {
		h.Write([]byte(cmd))
		h.Write([]byte{0x31})
	}
	h.Write([]byte{0x31})

	sum := h.Sum(nil)
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum))
}
