package exp

type ExperimentJSON struct {
	Name           string        `json:"name"`
	Duration       int           `json:"duration"`         // in minutes
	MaxRequestRate int           `json:"max_request_rate"` // maximum number of requests per second to send to targets
	MaxConcurrency int           `json:"max_concurrency"`  // maximum number of concurrent requests to have in flight for each target
	RequestFilter  string        `json:"request_filter"`   // filter to apply to incoming requests: "none", "pathonly", "validpathonly"
	Targets        []TargetJSON  `json:"targets"`
	Shared         *SharedJSON   `json:"shared"` // environment variables and init commands provided to all targets
	Defaults       *DefaultsJSON `json:"defaults"`
}

type NVJSON struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type TargetJSON struct {
	Name         string   `json:"name"`
	InstanceType string   `json:"instance_type,omitempty"` // instance type to use. If empty, DefaultInstanceType will be used instead
	Environment  []NVJSON `json:"environment,omitempty"`   // additional environment variables

	BaseImage    string       `json:"base_image,omitempty"`
	BuildFromGit *GitSpecJSON `json:"build_from_git,omitempty"`
	// Commands that should be added to the container's container.init.d directory
	// for example: ipfs config --json Swarm.ConnMgr.GracePeriod '"2m"'
	InitCommands []string `json:"init_commands,omitempty"`

	UseImage string `json:"use_image,omitempty"` // docker image to use. If empty, DefaultImage will be used instead. Must be pre-configured for thunderdome.
}

type DefaultsJSON struct {
	InstanceType string       `json:"instance_type,omitempty"` // instance type to use. If empty, DefaultInstanceType will be used instead
	Environment  []NVJSON     `json:"environment,omitempty"`   // additional environment variables
	BaseImage    string       `json:"base_image,omitempty"`
	BuildFromGit *GitSpecJSON `json:"build_from_git,omitempty"`
	InitCommands []string     `json:"init_commands,omitempty"`
	UseImage     string       `json:"use_image,omitempty"` // docker image to use. If empty, DefaultImage will be used instead. Must be pre-configured for thunderdome.
}

type SharedJSON struct {
	Environment  []NVJSON `json:"environment,omitempty"`
	InitCommands []string `json:"init_commands,omitempty"`
}

type GitSpecJSON struct {
	Repo   string `json:"repo,omitempty"`
	Commit string `json:"commit,omitempty"`
	Tag    string `json:"tag,omitempty"`
	Branch string `json:"branch,omitempty"`
}
