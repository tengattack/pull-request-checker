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
	conf, err := config.LoadConfig("../testdata/config.yml")
	if err != nil {
		panic(err)
	}
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

	code := m.Run()

	// clean up
	store.Deinit()

	os.Exit(code)
}
