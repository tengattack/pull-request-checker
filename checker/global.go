package checker

import (
	"github.com/tengattack/unified-ci/config"
	"github.com/tengattack/unified-ci/mq"

	"github.com/sirupsen/logrus"
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
