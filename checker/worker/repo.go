package worker

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/store"
	"github.com/tengattack/unified-ci/util"
)

// ListServerWorkerRepo list server workers' repos
func ListServerWorkerRepo() ([]WorkerProjectConfig, error) {
	var projects []WorkerProjectConfig
	sw.Range(func(key, value interface{}) bool {
		projects = append(projects, value.(ServerWorker).Projects...)
		return true
	})
	return projects, nil
}

// ListLocalRepo list local repos
func ListLocalRepo() ([]WorkerProjectConfig, error) {
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

func storePromoteStatus(ref common.GithubRef) (bool, error) {
	// check master commit status
	commitInfos, err := store.ListCommitsInfo(ref.Owner, ref.RepoName, ref.Sha)
	if err != nil {
		common.LogError.Errorf("WatchLocalRepo:LoadCommitsInfo for master error: %v", err)
		return false, err
	}
	// promote status
	updated := false
	for _, commitInfo := range commitInfos {
		if commitInfo.Status == 0 {
			err = commitInfo.UpdateStatus(1)
			if err != nil {
				common.LogError.Errorf("WatchLocalRepo:CommitInfo:UpdateStatus error: %v", err)
				// PASS
			} else {
				updated = true
			}
		}
	}
	if updated {
		common.LogAccess.Infof("CommitInfo %s/%s %s for master status updated", ref.Owner, ref.RepoName, ref.Sha)
	}
	return updated, nil
}

func checkProjects(ctx context.Context, projects []WorkerProjectConfig, enablePromote bool) {
	for _, project := range projects {
		parts := strings.Split(project.Name, "/")
		if len(parts) != 2 {
			continue
		}
		owner, repo := parts[0], parts[1]
		client, _, err := common.GetDefaultAPIClient(owner)
		if err != nil {
			common.LogError.Errorf("checkProjects:GetDefaultAPIClient for %s error: %v", owner, err)
			continue
		}
		if project.CheckMaster {
			masterBranch, _, err := client.Repositories.GetBranch(ctx, owner, repo, "master")
			if err != nil {
				common.LogError.Errorf("checkProjects:GetBranch for master error: %v", err)
				// PASS
			} else {
				// check master commit status
				masterCommitSHA := *masterBranch.Commit.SHA
				ref := common.GithubRef{
					Owner:    owner,
					RepoName: repo,

					Sha: masterCommitSHA,
				}
				updated := false
				if enablePromote {
					updated, _ = storePromoteStatus(ref)
				}
				if !updated {
					messagePrefix := fmt.Sprintf("%s/%s/tree/%s/commits/", ref.Owner, ref.RepoName, "master")
					message := messagePrefix + masterCommitSHA
					needCheck, err := common.NeedPRChecking(client, &ref, message, common.MQ)
					if err != nil {
						common.LogError.Errorf("checkProjects:NeedPRChecking for master error: %v", err)
						continue
					}
					if needCheck {
						// no statuses, need check
						common.LogAccess.WithField("entry", "local").Info("Push message: " + message)
						err = common.MQ.Push(message, messagePrefix, false)
						if err == nil {
							common.MarkAsPending(client, ref)
						} else {
							common.LogAccess.Error("Add message to queue error: " + err.Error())
							// PASS
						}
					}
				}
			}
		}
		pulls, err := common.GetGithubPulls(ctx, client, owner, repo)
		if err != nil {
			common.LogError.Errorf("checkProjects:GetGithubPulls error: %v", err)
			continue
		}
		for _, pull := range pulls {
			select {
			case <-ctx.Done():
				common.LogAccess.Warn("checkProjects canceled.")
				return
			default:
			}
			ref := common.GithubRef{
				Owner:    owner,
				RepoName: repo,

				Sha: pull.GetHead().GetSHA(),
			}
			messagePrefix := fmt.Sprintf("%s/%s/pull/%d/commits/", ref.Owner, ref.RepoName, pull.GetNumber())
			message := messagePrefix + ref.Sha
			needCheck, err := common.NeedPRChecking(client, &ref, message, common.MQ)
			if err != nil {
				common.LogError.Errorf("checkProjects:NeedPRChecking error: %v", err)
				continue
			}
			if needCheck {
				// no statuses, need check
				common.LogAccess.WithField("entry", "local").Info("Push message: " + message)
				err = common.MQ.Push(message, messagePrefix, false)
				if err == nil {
					common.MarkAsPending(client, ref)
				} else {
					common.LogAccess.Error("Add message to queue error: " + err.Error())
				}
			}
		}
	}
}

// WatchServerWorkerRepo scans server workers' repo periodically and sends a checking request if a opened PR hasn't any checks
func WatchServerWorkerRepo(ctx context.Context) error {
	var err error
	for {
		select {
		case <-ctx.Done():
			common.LogAccess.Warn("WatchServerWorkerRepo canceled.")
			return nil
		case <-time.After(60 * time.Second):
		}
		projects, err := ListServerWorkerRepo()
		if err != nil {
			common.LogError.Errorf("WatchServerWorkerRepo:ListServerWorkerRepo error: %v", err)
			continue
			// PASS
		}
		if len(projects) <= 0 {
			continue
		}
		checkProjects(ctx, projects, true)
	}
	if err != nil {
		common.LogAccess.Error("Local repo watcher error: " + err.Error())
	}
	return err
}

// WatchLocalRepo scans local repo periodically and sends a checking request if a opened PR hasn't any checks
func WatchLocalRepo(ctx context.Context) error {
	var err error
	for {
		select {
		case <-ctx.Done():
			common.LogAccess.Warn("WatchLocalRepo canceled.")
			return nil
		case <-time.After(60 * time.Second):
		}
		projects, err := ListLocalRepo()
		if err != nil {
			common.LogError.Errorf("WatchLocalRepo:ListLocalRepo error: %v", err)
			continue
			// PASS
		}
		if len(projects) <= 0 {
			continue
		}
		checkProjects(ctx, projects, false)
	}
	if err != nil {
		common.LogAccess.Error("Local repo watcher error: " + err.Error())
	}
	return err
}
