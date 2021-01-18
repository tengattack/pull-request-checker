package util

import (
	"io/ioutil"
	"os"
	"path/filepath"

	yaml "gopkg.in/yaml.v2"
)

const (
	projectTestsConfigFile = ".unified-ci.yml"
)

// ProjectConfig CI config for project
type ProjectConfig struct {
	LinterAfterTests bool                   `yaml:"linterAfterTests"`
	Tests            map[string]TestsConfig `yaml:"tests"`
	IgnorePatterns   []string               `yaml:"ignorePatterns"`
}

type projectConfigRaw struct {
	LinterAfterTests bool                `yaml:"linterAfterTests"`
	Tests            map[string][]string `yaml:"tests"`
	IgnorePatterns   []string            `yaml:"ignorePatterns"`
}

// TestsConfig config for tests
type TestsConfig struct {
	Coverage      string   `yaml:"coverage"`
	DeltaCoverage string   `yaml:"delta_coverage"`
	Cmds          []string `yaml:"cmds"`
}

// ReadProjectConfig get project config from CI config file
func ReadProjectConfig(cwd string) (config ProjectConfig, err error) {
	content, err := ioutil.ReadFile(filepath.Join(cwd, projectTestsConfigFile))
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil
		}
		return config, err
	}

	err = yaml.Unmarshal(content, &config)
	if err != nil {
		var cfg projectConfigRaw
		err = yaml.Unmarshal(content, &cfg)
		if err != nil {
			return config, err
		}
		config.Tests = make(map[string]TestsConfig)
		for k, v := range cfg.Tests {
			config.Tests[k] = TestsConfig{Cmds: v, Coverage: ""}
		}
	}
	return config, nil
}
