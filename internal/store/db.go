package store

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-faster/errors"
	"github.com/libtnb/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Open opens (creating if needed) the SQLite database and runs migrations.
func Open(path string) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, errors.Wrap(err, "create db dir")
	}
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, errors.Wrap(err, "open sqlite")
	}
	if err := db.AutoMigrate(&Message{}, &ShareUser{}, &Resource{}, &CheckinLog{}, &KV{}); err != nil {
		return nil, errors.Wrap(err, "migrate")
	}
	return db, nil
}

// GetKV returns the string value for key, "" when absent.
func GetKV(db *gorm.DB, key string) string {
	var kv KV
	if err := db.Where("key = ?", key).First(&kv).Error; err != nil {
		return ""
	}
	return kv.Value
}

// SetKV upserts key=value.
func SetKV(db *gorm.DB, key, value string) error {
	return db.Save(&KV{Key: key, Value: value}).Error
}

// GetKVFallback returns the int value for key, or fallback when absent.
func GetKVInt(db *gorm.DB, key string, fallback int) int {
	v, err := strconv.Atoi(GetKV(db, key))
	if err != nil {
		return fallback
	}
	return v
}

// SetKVInt stores an int value.
func SetKVInt(db *gorm.DB, key string, v int) error {
	return SetKV(db, key, strconv.Itoa(v))
}

// Day returns the local YYYY-MM-DD key used by CheckinLog.
func Day(t time.Time) string { return t.Local().Format("2006-01-02") }
