package checker

import (
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/config"
	"github.com/tengattack/unified-ci/store"
)

func TestMain(m *testing.M) {
	common.Conf = config.BuildDefaultConf()
	err := common.InitLog(common.Conf)
	if err != nil {
		panic(err)
	}

	fileDB := "file name.db"
	err = store.Init(fileDB)
	if err != nil {
		panic(err)
	}

	gin.SetMode(gin.TestMode)

	code := m.Run()

	// clean up
	store.Deinit()
	os.Remove(fileDB)

	os.Exit(code)
}
