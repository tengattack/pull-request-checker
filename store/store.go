package store

import (
	"database/sql"
	"sync"

	"github.com/jmoiron/sqlx"
	// import the sqlite driver
	_ "github.com/mattn/go-sqlite3"
)

var db *sqlx.DB

// Init the sqlite database
func Init(file string) (err error) {
	db, err = sqlx.Connect("sqlite3", "file:./"+file+"?cache=shared&mode=rwc&_journal_mode=WAL")
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS commits_info (
		owner TEXT NOT NULL DEFAULT '',
		repo TEXT NOT NULL DEFAULT '',
		sha TEXT NOT NULL,
		coverage REAL DEFAULT NULL,
		author TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		db.Close()
		return err
	}
	_, err = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uniq ON commits_info (owner, repo, sha)`)
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

// CommitsInfo struct
type CommitsInfo struct {
	Owner    string   `db:"owner"`
	Repo     string   `db:"repo"`
	Sha      string   `db:"sha"`
	Coverage *float64 `db:"coverage"`
	Author   string   `db:"author"`
}

var rwCommitsInfo sync.RWMutex

// Save to db
func (c *CommitsInfo) Save() error {
	rwCommitsInfo.Lock()
	defer rwCommitsInfo.Unlock()
	_, err := db.Exec("INSERT OR REPLACE INTO commits_info (owner, repo, sha, coverage, author) VALUES(?,?,?,?,?)",
		c.Owner, c.Repo, c.Sha, c.Coverage, c.Author)
	if err != nil {
		return err
	}
	return nil
}

// LoadCommitsInfo gets a CommitsInfo by owner, repo and sha
func LoadCommitsInfo(owner, repo, sha string) (*CommitsInfo, error) {
	rwCommitsInfo.RLock()
	defer rwCommitsInfo.RUnlock()
	var c CommitsInfo
	err := db.Get(&c, "SELECT * FROM commits_info WHERE owner = ? AND repo = ? AND sha = ?", owner, repo, sha)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}
