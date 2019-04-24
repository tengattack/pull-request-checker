package checker

import "github.com/tengattack/unified-ci/log"

func InitLog() (err error) {
	LogAccess, LogError, err = log.InitLog(Conf)
	return err
}
