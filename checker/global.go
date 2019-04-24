package checker

import (
	"github.com/sirupsen/logrus"
	"github.com/tengattack/unified-ci/config"
	"github.com/tengattack/unified-ci/mq"
)

var (
	// Conf is the main config
	Conf config.Config

	// LogAccess is log server request log
	LogAccess *logrus.Logger
	// LogError is log server error log
	LogError *logrus.Logger

	// MQ is the message queue
	MQ mq.MessageQueue
)

var userAgent string

// UserAgent is the user agent for this checker
func UserAgent() string {
	if userAgent == "" {
		userAgent = AppName + "/" + GetVersion()
	}
	return userAgent
}
