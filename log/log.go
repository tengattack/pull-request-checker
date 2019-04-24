package log

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/mattn/go-isatty"
	"github.com/sirupsen/logrus"
	"github.com/tengattack/unified-ci/config"
)

var (
	green   = string([]byte{27, 91, 57, 55, 59, 52, 50, 109})
	white   = string([]byte{27, 91, 57, 48, 59, 52, 55, 109})
	yellow  = string([]byte{27, 91, 57, 55, 59, 52, 51, 109})
	red     = string([]byte{27, 91, 57, 55, 59, 52, 49, 109})
	blue    = string([]byte{27, 91, 57, 55, 59, 52, 52, 109})
	magenta = string([]byte{27, 91, 57, 55, 59, 52, 53, 109})
	cyan    = string([]byte{27, 91, 57, 55, 59, 52, 54, 109})
	reset   = string([]byte{27, 91, 48, 109})
)

// Req is http request log
type Req struct {
	URI         string `json:"uri"`
	Method      string `json:"method"`
	IP          string `json:"ip"`
	ContentType string `json:"content_type"`
	Agent       string `json:"agent"`
}

// Message is message log
type Message struct {
	Type string
	ID   int64
}

var isTerm bool

func init() {
	isTerm = isatty.IsTerminal(os.Stdout.Fd())
}

var (
	// LogAccess is log server request log
	LogAccess *logrus.Logger
	// LogError is log server error log
	LogError *logrus.Logger
)

// InitLog use for initial log module
func InitLog(conf config.Config) (accessLog, errorLog *logrus.Logger, err error) {

	// init logger
	LogAccess = logrus.New()
	LogError = logrus.New()

	LogAccess.Formatter = &logrus.TextFormatter{
		TimestampFormat: "2006/01/02 - 15:04:05",
		FullTimestamp:   true,
	}

	LogError.Formatter = &logrus.TextFormatter{
		TimestampFormat: "2006/01/02 - 15:04:05",
		FullTimestamp:   true,
	}

	// set logger
	if err = SetLogLevel(LogAccess, conf.Log.AccessLevel); err != nil {
		return nil, nil, errors.New("Set access log level error: " + err.Error())
	}

	if err = SetLogLevel(LogError, conf.Log.ErrorLevel); err != nil {
		return nil, nil, errors.New("Set error log level error: " + err.Error())
	}

	if err = SetLogOut(LogAccess, conf.Log.AccessLog); err != nil {
		return nil, nil, errors.New("Set access log path error: " + err.Error())
	}

	if err = SetLogOut(LogError, conf.Log.ErrorLog); err != nil {
		return nil, nil, errors.New("Set error log path error: " + err.Error())
	}

	return LogAccess, LogError, nil
}

// SetLogOut provide log stdout and stderr output
func SetLogOut(log *logrus.Logger, outString string) error {
	switch outString {
	case "stdout":
		log.Out = os.Stdout
	case "stderr":
		log.Out = os.Stderr
	default:
		f, err := os.OpenFile(outString, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)

		if err != nil {
			return err
		}

		log.Out = f
	}

	return nil
}

// SetLogLevel is define log level what you want
// log level: panic, fatal, error, warn, info and debug
func SetLogLevel(log *logrus.Logger, levelString string) error {
	level, err := logrus.ParseLevel(levelString)

	if err != nil {
		return err
	}

	log.Level = level

	return nil
}

// Request record http request
func Request(uri string, method string, ip string, contentType string, agent string, format string) {
	var output string
	log := &Req{
		URI:         uri,
		Method:      method,
		IP:          ip,
		ContentType: contentType,
		Agent:       agent,
	}

	if format == "json" {
		logJSON, _ := json.Marshal(log)

		output = string(logJSON)
	} else {
		var headerColor, resetColor string

		if isTerm {
			headerColor = magenta
			resetColor = reset
		}

		// format is string
		output = fmt.Sprintf("|%s header %s| %s %s %s %s %s",
			headerColor, resetColor,
			log.Method,
			log.URI,
			log.IP,
			log.ContentType,
			log.Agent,
		)
	}

	LogAccess.Info(output)
}

// Middleware provide gin router handler.
func Middleware(format string) gin.HandlerFunc {
	return func(c *gin.Context) {
		Request(c.Request.URL.Path, c.Request.Method, c.ClientIP(), c.ContentType(), c.Request.Header.Get("User-Agent"),
			format)
		c.Next()
	}
}
