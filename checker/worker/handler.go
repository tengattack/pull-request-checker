package worker

import (
	"context"
	"fmt"
	"io/ioutil"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/log"
	"github.com/tengattack/unified-ci/store"
)

var httpClient = &http.Client{Timeout: 2 * time.Second}

// TODO: migrate with server
func abortWithError(c *gin.Context, code int, message string) {
	c.AbortWithStatusJSON(code, gin.H{
		"code": code,
		"info": message,
	})
}

// ServerBadgesHandler get project badge route by server worker
func ServerBadgesHandler(c *gin.Context) {
	owner := c.Param("owner")

	var w ServerWorker
	var found bool
	sw.Range(func(key, value interface{}) bool {
		if key.(string) == owner {
			w = value.(ServerWorker)
			found = true
			return false
		}
		return true
	})
	if !found {
		abortWithError(c, 404, "owner not found")
		return
	}
	if w.Addr == "" {
		abortWithError(c, 500, "empty worker addr")
		return
	}

	req, err := http.NewRequest(http.MethodGet, "http://"+w.Addr+c.Request.RequestURI, nil)
	if err != nil {
		abortWithError(c, 500, fmt.Sprintf("new request error: %v", err))
		return
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		abortWithError(c, 500, fmt.Sprintf("http client do error: %v", err))
		return
	}
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		abortWithError(c, 500, fmt.Sprintf("read resp body error: %v", err))
		return
	}
	c.JSON(resp.StatusCode, body)
}

// BadgesHandler get project badge
func BadgesHandler(c *gin.Context) {
	owner := c.Param("owner")
	repo := c.Param("repo")
	badgeType := c.Param("type")

	switch badgeType {
	case "build.svg":
	case "coverage.svg":
	default:
		abortWithError(c, 400, "error params")
		return
	}

	commitsInfo, err := store.GetLatestCommitsInfo(owner, repo)
	if err != nil {
		abortWithError(c, 500, "get latest commits info for "+owner+"/"+repo+" error: "+err.Error())
		return
	}

	build := "unknown"
	coverage := "unknown"
	for _, info := range commitsInfo {
		if info.Passing == 1 {
			build = "passing"
		} else {
			build = "failing"
			break
		}
	}

	var coverageNum int
	if len(commitsInfo) > 0 {
		var pct float64
		coverage = ""
		for _, info := range commitsInfo {
			if info.Coverage == nil {
				coverage = "unknown"
				break
			} else {
				pct += *info.Coverage
			}
		}
		if coverage == "" {
			coverageNum = int(math.Round(100 * pct / float64(len(commitsInfo))))
			coverage = strconv.Itoa(coverageNum) + "%"
		}
	}

	var color string
	unknownColor := "#9f9f9f"
	colors := []string{"#4c1", "#97ca00", "#a4a61d", "#dfb317", "#fe7d37", "#e05d44"}
	// TODO: common template
	buildTemplate := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="88" height="20"><g shape-rendering="crispEdges"><path fill="#555" d="M0 0h37v20H0z"/><path fill="%s" d="M37 0h51v20H37z"/></g><g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="110"> <text x="185" y="140" transform="scale(.1)" textLength="270">build</text><text x="615" y="140" transform="scale(.1)" textLength="410">%s</text></g> </svg>`
	buildFailingTemplate := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="80" height="20"><g shape-rendering="crispEdges"><path fill="#555" d="M0 0h37v20H0z"/><path fill="%s" d="M37 0h43v20H37z"/></g><g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="110"> <text x="185" y="140" transform="scale(.1)" textLength="270">build</text><text x="575" y="140" transform="scale(.1)" textLength="330">%s</text></g> </svg>`
	buildUnknownTemplate := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="98" height="20"><g shape-rendering="crispEdges"><path fill="#555" d="M0 0h37v20H0z"/><path fill="%s" d="M37 0h61v20H37z"/></g><g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="110"> <text x="185" y="140" transform="scale(.1)" textLength="270">build</text><text x="665" y="140" transform="scale(.1)" textLength="510">%s</text></g> </svg>`
	coverageTemplate := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="98" height="20"><g shape-rendering="crispEdges"><path fill="#555" d="M0 0h61v20H0z"/><path fill="%s" d="M61 0h37v20H61z"/></g><g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="110"> <text x="305" y="140" transform="scale(.1)" textLength="510">coverage</text><text x="785" y="140" transform="scale(.1)" textLength="270">%s</text></g> </svg>`
	coverageSmallTemplate := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="90" height="20"><g shape-rendering="crispEdges"><path fill="#555" d="M0 0h61v20H0z"/><path fill="%s" d="M61 0h29v20H61z"/></g><g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="110"> <text x="305" y="140" transform="scale(.1)" textLength="510">coverage</text><text x="745" y="140" transform="scale(.1)" textLength="190">%s</text></g> </svg>`
	coverageUnknownTemplate := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="122" height="20"><g shape-rendering="crispEdges"><path fill="#555" d="M0 0h61v20H0z"/><path fill="%s" d="M61 0h61v20H61z"/></g><g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="110"> <text x="305" y="140" transform="scale(.1)" textLength="510">coverage</text><text x="905" y="140" transform="scale(.1)" textLength="510">%s</text></g> </svg>`

	// make camo do not cache our responses
	c.Header("Cache-Control", "no-cache, max-age=0")

	switch badgeType {
	case "build.svg":
		switch build {
		case "passing":
			color = colors[0]
			c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(fmt.Sprintf(buildTemplate, color, build)))
		case "failing":
			color = colors[len(colors)-1]
			c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(fmt.Sprintf(buildFailingTemplate, color, build)))
		default:
			// unknown
			color = unknownColor
			c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(fmt.Sprintf(buildUnknownTemplate, color, build)))
		}
	case "coverage.svg":
		if coverage == "unknown" {
			color = unknownColor
			c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(fmt.Sprintf(coverageUnknownTemplate, color, coverage)))
			return
		}

		if coverageNum >= 93 { // or starts from 97%
			color = colors[0]
		} else if coverageNum >= 80 {
			color = colors[1]
		} else if coverageNum >= 65 {
			color = colors[2]
		} else if coverageNum >= 45 {
			color = colors[3]
		} else if coverageNum >= 15 {
			color = colors[4]
		} else if coverageNum >= 10 {
			color = colors[5]
		} else {
			// small coverage
			color = colors[5]
			c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(fmt.Sprintf(coverageSmallTemplate, color, coverage)))
			return
		}
		c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(fmt.Sprintf(coverageTemplate, color, coverage)))
	default:
		panic("unexpected params")
	}
}

func updateAddr(w *ServerWorker, c *gin.Context) {
	if ip, _, err := net.SplitHostPort(strings.TrimSpace(c.Request.RemoteAddr)); err == nil {
		w.Addr = ip + ":" + strconv.Itoa(common.Conf.API.Port)
	}
}

// JoinHandler call when worker startup
func JoinHandler(c *gin.Context) {
	var req struct {
		Worker   WorkerInfo            `json:"worker"`
		Projects []WorkerProjectConfig `json:"projects"`
	}
	err := c.BindJSON(&req)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"code": -1,
			"info": err.Error(),
		})
		return
	}
	w := ServerWorker{
		Info:     req.Worker,
		Projects: req.Projects,
	}
	updateAddr(&w, c)
	sw.Store(req.Worker.Name, w)
	log.LogAccess.Infof("worker %q joined, addr: %s", w.Info.Name, w.Addr)
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"info": "success",
	})
}

// RequestHandler call for worker request new jobs
func RequestHandler(c *gin.Context) {
	var req struct {
		Worker   WorkerInfo            `json:"worker"`
		Projects []WorkerProjectConfig `json:"projects"`
		Running  []string              `json:"running"`
		Request  int                   `json:"request"`
	}
	err := c.BindJSON(&req)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"code": -1,
			"info": err.Error(),
		})
		return
	}
	w := ServerWorker{
		Info:        req.Worker,
		Projects:    req.Projects,
		RunningJobs: req.Running,
	}
	updateAddr(&w, c)
	sw.Store(req.Worker.Name, w)
	var within []string
	for _, p := range req.Projects {
		within = append(within, p.Name)
	}
	jobs, err := common.MQ.GetNWithin(context.Background(), req.Request, req.Running, within)
	if err != nil {
		common.LogError.Error("mq get within error: " + err.Error())
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"code": -500,
			"info": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"info": gin.H{"jobs": jobs},
	})
}

// JobDoneHandler call when job done (include fails)
func JobDoneHandler(c *gin.Context) {
	var jobDoneReq struct {
		Worker WorkerInfo        `json:"worker"`
		Type   WorkerJobDoneType `json:"type"`
		Job    string            `json:"job"`
	}
	err := c.BindJSON(&jobDoneReq)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"code": -1,
			"info": err.Error(),
		})
		return
	}
	switch jobDoneReq.Type {
	case TypeJobDoneFinish:
		err = common.MQ.Finish(jobDoneReq.Job)
		if err != nil {
			common.LogError.Error("mq finish error: " + err.Error())
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"code": -500,
				"info": err.Error(),
			})
			return
		}
	case TypeJobDoneError:
		err = common.MQ.Error(jobDoneReq.Job)
		if err != nil {
			common.LogError.Error("mark message error failed: " + err.Error())
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"code": -500,
				"info": err.Error(),
			})
			return
		}
	default:
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"code": -1,
			"info": "params error",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"info": "success",
	})
}
