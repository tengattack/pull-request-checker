package worker

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/config"
)

// Mode working mode
type Mode string

// modes const
const (
	ModeLocal  Mode = "local"
	ModeServer Mode = "server"
	ModeWorker Mode = "worker"
)

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

// Projects list local repo
func (w *Worker) Projects() []WorkerProjectConfig {
	if w.projects == nil {
		projects, err := ListLocalRepo()
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
