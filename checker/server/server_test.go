package server

import (
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/config"
	"github.com/tengattack/unified-ci/mq/redis"
	"github.com/tengattack/unified-ci/store"
)

func TestMain(m *testing.M) {
	conf, err := config.LoadConfig("../../testdata/config.yml")
	if err != nil {
		panic(err)
	}
	// WORKAROUND: private key file path location
	conf.GitHub.PrivateKey = "../" + conf.GitHub.PrivateKey
	common.Conf = conf
	err = common.InitLog(common.Conf)
	if err != nil {
		panic(err)
	}

	err = store.Init(":memory:")
	if err != nil {
		panic(err)
	}

	gin.SetMode(gin.TestMode)

	common.MQ = redis.New(common.Conf.MessageQueue.Redis)
	if err := common.MQ.Init(); err != nil {
		panic(err)
	}

	code := m.Run()

	// clean up
	store.Deinit()

	os.Exit(code)
}
