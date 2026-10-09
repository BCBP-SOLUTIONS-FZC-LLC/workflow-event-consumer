package topology

type Config struct {
	Version int                 `yaml:"version"`
	Targets []Target            `yaml:"targets"`
	Queues  []Queue             `yaml:"queues"`
	Routes  map[string][]string `yaml:"routes"`
}

type Target struct {
	Name       string `yaml:"name"`
	Addr       string `yaml:"addr"`
	TokenEnv   string `yaml:"tokenEnv"`
	PathPrefix string `yaml:"pathPrefix,omitempty"`
}

type Queue struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}
