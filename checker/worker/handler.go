package worker

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tengattack/unified-ci/common"
)

// JoinHandler call when worker startup
func JoinHandler(c *gin.Context) {
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

// RequestHandler call for worker request new jobs
func RequestHandler(c *gin.Context) {
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
