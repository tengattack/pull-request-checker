package checker

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	uuid "github.com/satori/go.uuid"
	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/config"
	"github.com/tengattack/unified-ci/util"
)

// Mode working mode
type Mode string

// modes const
const (
	ModeLocal  Mode = "local"
	ModeServer Mode = "server"
	ModeWorker Mode = "worker"
)

// WorkingMode current working mode
var WorkingMode Mode = ModeLocal

var workers sync.Map
var sw sync.Map /* map[name]ServerWorker */

type WorkerJobDoneType string

// job done type const
const (
	TypeJobDoneFinish WorkerJobDoneType = "finfish"
	TypeJobDoneError  WorkerJobDoneType = "error"
)

type ServerWorker struct {
	Info        WorkerInfo
	Projects    []WorkerProjectConfig
	RunningJobs []string
}

// Worker ID struct
type Worker struct {
	info WorkerInfo

	projects []WorkerProjectConfig

	serverAddr string
	httpClient *http.Client
	queue      []string
}

type WorkerInfo struct {
	Name     string `json:"name"`
	WorkerID string `json:"worker_id"`
}

type WorkerProjectConfig struct {
	Name        string `json:"name"`
	CheckMaster bool   `json:"check_master"`
}

// NewWorker creates new worker object
func NewWorker(conf *config.SectionWorker) *Worker {
	worker := &Worker{}
	worker.info.Name = conf.Name
	worker.info.WorkerID = uuid.NewV4().String()
	worker.serverAddr = conf.ServerAddr
	worker.httpClient = &http.Client{Timeout: 2 * time.Second}

	return worker
}

func getLocalRepo() ([]WorkerProjectConfig, error) {
	var result []WorkerProjectConfig
	files, err := ioutil.ReadDir(common.Conf.Core.WorkDir)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		isDir := file.IsDir()
		path := filepath.Join(common.Conf.Core.WorkDir, file.Name())
		if !isDir && file.Mode()&os.ModeSymlink == os.ModeSymlink {
			realPath, err := os.Readlink(path)
			if err != nil {
				continue
			}
			st, err := os.Stat(realPath)
			if err != nil {
				continue
			}
			if st.IsDir() {
				isDir = true
				path = realPath
			}
		}
		if isDir {
			subfiles, err := ioutil.ReadDir(path)
			if err != nil {
				continue
			}
			for _, subfile := range subfiles {
				isDir := subfile.IsDir()
				if !isDir && subfile.Mode()&os.ModeSymlink == os.ModeSymlink {
					realPath, err := os.Readlink(filepath.Join(path, subfile.Name()))
					if err != nil {
						continue
					}
					st, err := os.Stat(realPath)
					if err != nil {
						continue
					}
					if st.IsDir() {
						isDir = true
					}
				}
				if isDir {
					owner, repo := file.Name(), subfile.Name()
					projConf, err := util.ReadProjectConfig(filepath.Join(path, subfile.Name()))
					checkMaster := false
					if err == nil {
						checkMaster = len(projConf.Tests) > 0
					}
					result = append(result, WorkerProjectConfig{
						Name:        owner + "/" + repo,
						CheckMaster: checkMaster,
					})
				}
			}
		}
	}
	return result, nil
}

// Projects list local repo
func (w *Worker) Projects() []WorkerProjectConfig {
	if w.projects == nil {
		projects, err := getLocalRepo()
		if err != nil {
			common.LogError.Errorf("worker list projects error: %v", err)
		} else {
			w.projects = projects
		}
	}
	return w.projects
}

// SetRunningQueue set the running queue
func (w *Worker) SetRunningQueue(queue []string) {
	w.queue = queue
}

// Join the master
func (w *Worker) Join() error {
	joinAPI := w.serverAddr + "/api/worker/join"
	data, _ := json.Marshal(map[string]interface{}{
		"worker":   w.info,
		"projects": w.Projects(),
	})
	req, err := http.NewRequest(http.MethodPost, joinAPI, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	// TODO: parse data
	return nil
}

// Request request new jobs
func (w *Worker) Request(request int) ([]string, error) {
	syncAPI := w.serverAddr + "/api/worker/request"
	data, _ := json.Marshal(map[string]interface{}{
		"worker":   w.info,
		"projects": w.Projects(),
		"running":  w.queue,
		"request":  request,
	})
	req, err := http.NewRequest(http.MethodPost, syncAPI, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var requestResponse struct {
		Code int `json:"code"`
		Info struct {
			Jobs []string `json:"jobs"`
		} `json:"info"`
	}
	err = json.Unmarshal(data, &requestResponse)
	if err != nil {
		return nil, err
	}
	return requestResponse.Info.Jobs, nil
}

// JobDone triggers when job done
func (w *Worker) JobDone(typ WorkerJobDoneType, job string) error {
	doneAPI := w.serverAddr + "/api/worker/jobdone"
	data, _ := json.Marshal(map[string]interface{}{
		"worker": w.info,
		"type":   typ,
		"job":    job,
	})
	req, err := http.NewRequest(http.MethodPost, doneAPI, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	// TODO: parse data
	return nil
}

func workerJoinHandler(c *gin.Context) {
	var joinReq struct {
		Worker   WorkerInfo            `json:"worker"`
		Projects []WorkerProjectConfig `json:"projects"`
	}
	err := c.BindJSON(&joinReq)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"code": -1,
			"info": err.Error(),
		})
		return
	}
	sw.Store(joinReq.Worker.Name, ServerWorker{
		Info:     joinReq.Worker,
		Projects: joinReq.Projects,
	})
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"info": "success",
	})
}

func workerRequestHandler(c *gin.Context) {
	var requestReq struct {
		Worker   WorkerInfo            `json:"worker"`
		Projects []WorkerProjectConfig `json:"projects"`
		Running  []string              `json:"running"`
		Request  int                   `json:"request"`
	}
	err := c.BindJSON(&requestReq)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"code": -1,
			"info": err.Error(),
		})
		return
	}
	sw.Store(requestReq.Worker.Name, ServerWorker{
		Info:        requestReq.Worker,
		Projects:    requestReq.Projects,
		RunningJobs: requestReq.Running,
	})
	var within []string
	for _, p := range requestReq.Projects {
		within = append(within, p.Name)
	}
	jobs, err := common.MQ.GetNWithin(context.Background(), requestReq.Request, requestReq.Running, within)
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

func workerJobDoneHandler(c *gin.Context) {
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
