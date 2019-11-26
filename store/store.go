package store

import (
	"database/sql"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	// import the sqlite driver
	_ "github.com/mattn/go-sqlite3"
)

// CommitsInfo struct
type CommitsInfo struct {
	Owner      string   `db:"owner"`
	Repo       string   `db:"repo"`
	Sha        string   `db:"sha"`
	Author     string   `db:"author"`
	Test       string   `db:"test"`
	Coverage   *float64 `db:"coverage"`
	Passing    int      `db:"passing"`
	Status     int      `db:"status"`
	CreateTime int64    `db:"create_time"`
}

var (
	rwCommitsInfo = new(sync.RWMutex)
	db            *sqlx.DB
)

// Init the sqlite database
func Init(file string) (err error) {
	db, err = sqlx.Connect("sqlite3", "file:"+file+"?cache=shared&mode=rwc&_journal_mode=WAL")
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS commits_tests (
		owner TEXT NOT NULL DEFAULT '',
		repo TEXT NOT NULL DEFAULT '',
		sha TEXT NOT NULL,
		author TEXT NOT NULL DEFAULT '',
		test TEXT NOT NULL DEFAULT '',
		coverage REAL DEFAULT NULL,
		passing INT NOT NULL DEFAULT '0',
		status INT NOT NULL DEFAULT '0',
		create_time INT NOT NULL,
		UNIQUE (owner, repo, sha, test)
	)`)
	if err != nil {
		db.Close()
		return err
	}
	_, err = db.Exec(`ALTER TABLE commits_tests ADD passing INT NOT NULL DEFAULT '0'`)
	if err != nil {
		// PASS
	}
	_, err = db.Exec(`ALTER TABLE commits_tests ADD status INT NOT NULL DEFAULT '0'`)
	if err != nil {
		// PASS
	}
	_, err = db.Exec(`ALTER TABLE commits_tests ADD create_time INT NOT NULL DEFAULT '0'`)
	if err != nil {
		// PASS
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS IDX_OWNER_REPO_SHA ON commits_tests (owner, repo, sha)`)
	if err != nil {
		db.Close()
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS IDX_CREATE_TIME ON commits_tests (create_time)`)
	if err != nil {
		db.Close()
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS IDX_OWNER_REPO_CREATE_TIME ON commits_tests (owner, repo, create_time)`)
	if err != nil {
		db.Close()
		return err
	}
	return nil
}

// Deinit will close the sqlite database
func Deinit() {
	db.Close()
}

// Save to db
func (c *CommitsInfo) Save() error {
	rwCommitsInfo.Lock()
	defer rwCommitsInfo.Unlock()
	t := time.Now().Unix()
	_, err := db.Exec("INSERT OR REPLACE INTO commits_tests (owner, repo, sha, author, test, coverage, passing, status, create_time) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		c.Owner, c.Repo, c.Sha, c.Author, c.Test, c.Coverage, c.Passing, c.Status, t)
	if err != nil {
		return err
	}
	c.CreateTime = t
	return nil
}

// UpdateStatus updates state
func (c *CommitsInfo) UpdateStatus(status int) error {
	rwCommitsInfo.Lock()
	defer rwCommitsInfo.Unlock()
	_, err := db.Exec("UPDATE commits_tests SET status = ? WHERE owner = ? AND repo = ? AND sha = ? AND test = ?",
		status, c.Owner, c.Repo, c.Sha, c.Test)
	if err != nil {
		return err
	}
	c.Status = status
	return nil
}

// GetLatestCommitsInfo gets commits info for latest commit
func GetLatestCommitsInfo(owner, repo string) ([]CommitsInfo, error) {
	rwCommitsInfo.RLock()
	defer rwCommitsInfo.RUnlock()
	var c CommitsInfo
	status := 1
	err := db.Get(&c, "SELECT * FROM commits_tests WHERE owner = ? AND repo = ? AND status = ? ORDER BY create_time DESC LIMIT 1",
		owner, repo, status)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	var cs []CommitsInfo
	err = db.Select(&cs, "SELECT * FROM commits_tests WHERE owner = ? AND repo = ? AND sha = ? AND status = ?",
		owner, repo, c.Sha, status)
	if err != nil {
		return nil, err
	}
	return cs, nil
}

// LoadCommitsInfo gets a CommitsInfo by owner, repo, sha and test
func LoadCommitsInfo(owner, repo, sha, test string) (*CommitsInfo, error) {
	rwCommitsInfo.RLock()
	defer rwCommitsInfo.RUnlock()
	var c CommitsInfo
	err := db.Get(&c, "SELECT * FROM commits_tests WHERE owner = ? AND repo = ? AND sha = ? AND test = ?",
		owner, repo, sha, test)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// ListCommitsInfo lists CommitsInfos by owner, repo and sha
func ListCommitsInfo(owner, repo, sha string) ([]CommitsInfo, error) {
	rwCommitsInfo.RLock()
	defer rwCommitsInfo.RUnlock()
	var c []CommitsInfo
	err := db.Select(&c, "SELECT * FROM commits_tests WHERE owner = ? AND repo = ? AND sha = ?",
		owner, repo, sha)
	if err != nil {
		return nil, err
	}
	return c, nil
}
