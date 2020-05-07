package config

import (
	"io/ioutil"

	mqredis "github.com/tengattack/unified-ci/mq/redis"
	"gopkg.in/yaml.v2"
)

// Config is the main config struct.
type Config struct {
	Core         SectionCore         `yaml:"core"`
	API          SectionAPI          `yaml:"api"`
	GitHub       SectionGitHub       `yaml:"github"`
	Log          SectionLog          `yaml:"log"`
	MessageQueue SectionMessageQueue `yaml:"mq"`
	Concurrency  SectionConcurrency  `yaml:"concurrency"`
}

// SectionCore is a sub section of config.
type SectionCore struct {
	EnableRetries bool   `yaml:"enable_retries"`
	MaxRetries    int64  `yaml:"max_retries"`
	Socks5Proxy   string `yaml:"socks5_proxy"`
	GitCommand    string `yaml:"git_command"`
	DBFile        string `yaml:"db_file"`
	WorkDir       string `yaml:"work_dir"`
	LogsDir       string `yaml:"logs_dir"`
	CheckLogURI   string `yaml:"check_log_uri"`
	GolangCILint  string `yaml:"golangcilint"`
	RemarkLint    string `yaml:"remarklint"`
	CPPLint       string `yaml:"cpplint"`
	OCLint        string `yaml:"oclint"`
	ClangLint     string `yaml:"clanglint"`
	Ktlint        string `yaml:"ktlint"`
	PHPLint       string `yaml:"phplint"`
	ESLint        string `yaml:"eslint"`
	TSLint        string `yaml:"tslint"`
	SCSSLint      string `yaml:"scsslint"`
	APIDoc        string `yaml:"apidoc"`
	AndroidLint   string `yaml:"androidlint"`
}

// SectionAPI is a sub section of config.
type SectionAPI struct {
	Enabled    bool   `yaml:"enabled"`
	Mode       string `yaml:"mode"`
	Address    string `yaml:"address"`
	Port       int    `yaml:"port"`
	WebHookURI string `yaml:"webhook_uri"`
}

// SectionGitHub is a sub section of config.
type SectionGitHub struct {
	AppID         int64            `yaml:"app_id"`
	Secret        string           `yaml:"secret"`
	PrivateKey    string           `yaml:"private_key"`
	Installations map[string]int64 `yaml:"installations"`
}

// SectionLog is a sub section of config.
type SectionLog struct {
	Format      string `yaml:"format"`
	AccessLog   string `yaml:"access_log"`
	AccessLevel string `yaml:"access_level"`
	ErrorLog    string `yaml:"error_log"`
	ErrorLevel  string `yaml:"error_level"`
}

// SectionMessageQueue is a sub section of config.
type SectionMessageQueue struct {
	Engine string         `yaml:"engine"`
	Redis  mqredis.Config `yaml:"redis"`
}

// SectionConcurrency is a sub section of config.
type SectionConcurrency struct {
	Lint int `yaml:"lint"`
	Test int `yaml:"test"`
}

// BuildDefaultConf is the default config setting.
func BuildDefaultConf() Config {
	var conf Config

	// Core
	conf.Core.EnableRetries = true
	conf.Core.MaxRetries = 50
	conf.Core.Socks5Proxy = ""
	conf.Core.GitCommand = "git"
	conf.Core.DBFile = "file.db"
	conf.Core.WorkDir = "tmp"
	conf.Core.LogsDir = "logs"
	conf.Core.CheckLogURI = ""
	conf.Core.RemarkLint = "remark"
	conf.Core.CPPLint = "cpplint"
	conf.Core.ClangLint = "clang-format"
	conf.Core.Ktlint = "ktlint"
	conf.Core.PHPLint = "phplint"
	conf.Core.ESLint = ""
	conf.Core.TSLint = ""
	conf.Core.SCSSLint = ""
	conf.Core.APIDoc = "apidoc"

	// API
	conf.API.Enabled = true
	conf.API.Mode = "release"
	conf.API.Address = ""
	conf.API.Port = 8098
	conf.API.WebHookURI = "/api/webhook"

	// GitHub
	conf.GitHub.AppID = 0
	conf.GitHub.Secret = ""
	conf.GitHub.PrivateKey = ""
	conf.GitHub.Installations = make(map[string]int64)

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

	// Concurrency
	conf.Concurrency.Lint = 4
	conf.Concurrency.Test = 1
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
