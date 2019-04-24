package store

import (
	"database/sql"
	"sync"

	"github.com/jmoiron/sqlx"
	// import the sqlite driver
	_ "github.com/mattn/go-sqlite3"
)

// Warn warning
type Warn struct {
	Warn string
}

func (w *Warn) Error() string {
	if w == nil {
		return ""
	}
	return w.Warn
}

// CommitsInfo struct
type CommitsInfo struct {
	Owner    string   `db:"owner"`
	Repo     string   `db:"repo"`
	Sha      string   `db:"sha"`
	Author   string   `db:"author"`
	Test     string   `db:"test"`
	Coverage *float64 `db:"coverage"`
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
		UNIQUE (owner, repo, sha, test)
	)`)
	if err != nil {
		db.Close()
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS IDX_OWNER_REPO_SHA ON commits_tests (owner, repo, sha)`)
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
	r, err := db.Exec("INSERT OR REPLACE INTO commits_tests (owner, repo, sha, author, test, coverage) VALUES (?, ?, ?, ?, ?, ?)",
		c.Owner, c.Repo, c.Sha, c.Author, c.Test, c.Coverage)
	if err != nil {
		return err
	}
	affect, _ := r.RowsAffected()
	if affect == 0 {
		return &Warn{"0 row(s) affected"}
	}
	return nil
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
