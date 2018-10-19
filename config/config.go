package config

import (
	"io/ioutil"

	mqredis "github.com/tengattack/unified-ci/mq/redis"
	"gopkg.in/yaml.v2"
)

// Config is main config struct.
type Config struct {
	Core         SectionCore         `yaml:"core"`
	API          SectionAPI          `yaml:"api"`
	GitHub       SectionGitHub       `yaml:"github"`
	Log          SectionLog          `yaml:"log"`
	MessageQueue SectionMessageQueue `yaml:"mq"`
}

// SectionCore is sub section of config.
type SectionCore struct {
	EnableRetries bool   `yaml:"enable_retries"`
	MaxRetries    int64  `yaml:"max_retries"`
	WorkDir       string `yaml:"work_dir"`
	LogsDir       string `yaml:"logs_dir"`
	CheckLogURI   string `yaml:"check_log_uri"`
	CPPLint       string `yaml:"cpplint"`
	PHPLint       string `yaml:"phplint"`
	PHPLintConfig string `yaml:"phplint_config"`
	ESLint        string `yaml:"eslint"`
	TSLint        string `yaml:"tslint"`
	SCSSLint      string `yaml:"scsslint"`
}

// SectionAPI is sub section of config.
type SectionAPI struct {
	Enabled    bool   `yaml:"enabled"`
	Mode       string `yaml:"mode"`
	Address    string `yaml:"address"`
	Port       int    `yaml:"port"`
	WebHookURI string `yaml:"webhook_uri"`
}

// SectionGitHub is sub section of config.
type SectionGitHub struct {
	Secret      string `yaml:"secret"`
	AccessToken string `yaml:"access_token"`
}

// SectionLog is sub section of config.
type SectionLog struct {
	Format      string `yaml:"format"`
	AccessLog   string `yaml:"access_log"`
	AccessLevel string `yaml:"access_level"`
	ErrorLog    string `yaml:"error_log"`
	ErrorLevel  string `yaml:"error_level"`
}

// SectionMessageQueue is sub section of config.
type SectionMessageQueue struct {
	Engine string         `yaml:"engine"`
	Redis  mqredis.Config `yaml:"redis"`
}

// BuildDefaultConf is default config setting.
func BuildDefaultConf() Config {
	var conf Config

	// Core
	conf.Core.EnableRetries = true
	conf.Core.MaxRetries = 50
	conf.Core.WorkDir = "tmp"
	conf.Core.LogsDir = "logs"
	conf.Core.CheckLogURI = ""
	conf.Core.CPPLint = "cpplint"
	conf.Core.PHPLint = "phplint"
	conf.Core.PHPLintConfig = ""
	conf.Core.ESLint = ""
	conf.Core.TSLint = ""
	conf.Core.SCSSLint = ""

	// API
	conf.API.Enabled = true
	conf.API.Mode = "release"
	conf.API.Address = ""
	conf.API.Port = 8098
	conf.API.WebHookURI = "/api/webhook"

	// GitHub
	conf.GitHub.Secret = ""
	conf.GitHub.AccessToken = ""

	// Log
	conf.Log.Format = "string"
	conf.Log.AccessLog = "stdout"
	conf.Log.AccessLevel = "debug"
	conf.Log.ErrorLog = "stderr"
	conf.Log.ErrorLevel = "error"

	// MessageQueue
	conf.MessageQueue.Engine = "redis"
	conf.MessageQueue.Redis.Addr = "localhost:6379"
	conf.MessageQueue.Redis.Password = ""
	conf.MessageQueue.Redis.DB = 0
	conf.MessageQueue.Redis.PoolSize = 10

	return conf
}

// LoadConfig loads config from file
func LoadConfig(confPath string) (Config, error) {
	conf := BuildDefaultConf()

	configFile, err := ioutil.ReadFile(confPath)

	if err != nil {
		return conf, err
	}

	err = yaml.Unmarshal(configFile, &conf)

	if err != nil {
		return conf, err
	}

	return conf, nil
}
