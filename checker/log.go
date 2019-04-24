package checker

import (
	"github.com/tengattack/unified-ci/config"
	"github.com/tengattack/unified-ci/log"
)

// InitLog inits the logger in this package
func InitLog(conf config.Config) (err error) {
	LogAccess, LogError, err = log.InitLog(conf)
	return err
}
