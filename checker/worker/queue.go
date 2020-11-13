package worker

import (
	"strings"

	"github.com/tengattack/unified-ci/common"
)

// QueueJob .
type QueueJob struct {
	WorkerName string
	Job        string
}

// GetRunningJobs get running jobs
func GetRunningJobs() (jobs []QueueJob, err error) {
	sw.Range(func(key, value interface{}) bool {
		w := value.(ServerWorker)
		for _, j := range w.RunningJobs {
			jobs = append(jobs, QueueJob{
				WorkerName: w.Info.Name,
				Job:        j,
			})
		}
		return true
	})
	return
}

// GetPendingJobs get pending jobs
func GetPendingJobs() (jobs []QueueJob, err error) {
	var list []string
	list, err = common.MQ.ListAll()
	if err != nil {
		return nil, err
	}
	var prefixMap map[string]WorkerInfo
	sw.Range(func(key, value interface{}) bool {
		w := value.(ServerWorker)
		for _, p := range w.Projects {
			prefixMap[p.Name] = w.Info
		}
		return true
	})
	for _, j := range list {
		parts := strings.Split(j, "/")
		prefix := parts[0]
		if len(parts) > 1 {
			prefix += "/" + parts[2]
		}
		if w, ok := prefixMap[prefix]; ok {
			jobs = append(jobs, QueueJob{
				WorkerName: w.Name,
				Job:        j,
			})
		} else {
			// no worker assigned
			jobs = append(jobs, QueueJob{Job: j})
		}
	}
	return
}
