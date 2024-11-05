package cache

import (
	"database/sql"
	"encoding/base64"

	_ "github.com/mattn/go-sqlite3"
)

type SQLiteCache struct {
	db *sql.DB
}

func NewSQLiteCache(path string) (*SQLiteCache, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	return &SQLiteCache{db: db}, nil
}

func (c *SQLiteCache) Count() (int, error) {
	row := c.db.QueryRow("SELECT COUNT(*) FROM cache")
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (c *SQLiteCache) Add(key string, value []byte) error {
	raw := base64.StdEncoding.EncodeToString(value)
	_, err := c.db.Exec("INSERT INTO cache (key, value) VALUES (?, ?)", key, raw)
	return err
}

func (c *SQLiteCache) Get(key string) ([]byte, error) {
	row := c.db.QueryRow("SELECT value FROM cache WHERE key =?", key)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	value, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	return value, nil
}
