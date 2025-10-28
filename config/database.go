package config

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/cppla/aibbs/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

// InitDatabase establishes a connection to MySQL using configuration values and performs automatic migrations.
func InitDatabase(modelDefs ...interface{}) *gorm.DB {
	if db != nil {
		return db
	}

	cfg := Get()
	var dsn string
	if cfg.DatabaseURI != "" {
		dsn = cfg.DatabaseURI
	} else {
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			cfg.DBUser,
			cfg.DBPassword,
			cfg.DBHost,
			cfg.DBPort,
			cfg.DBName,
		)
	}

	// Configure GORM logger: derive level from app LogLevel and raise slow-sql threshold to reduce noise
	gLogger := logger.New(
		log.New(os.Stdout, "", log.LstdFlags),
		logger.Config{
			SlowThreshold:             2 * time.Second, // consider slower queries only
			LogLevel:                  toGormLogLevel(cfg.LogLevel),
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)

	gormCfg := &gorm.Config{
		Logger:                                   gLogger,
		DisableForeignKeyConstraintWhenMigrating: true,
	}

	var err error
	db, err = gorm.Open(mysql.Open(dsn), gormCfg)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("failed to get sql.DB: %v", err)
	}

	// 连接池参数：适中规模 + 更积极的连接回收，减少“bad idle connection”噪音
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	// 主动限制单连接的最大空闲时长，避免被服务端 wait_timeout 回收导致的“bad connection”日志
	// Go 1.15+ 提供 SetConnMaxIdleTime
	if setIdleTime := sqlDB.SetConnMaxIdleTime; setIdleTime != nil {
		sqlDB.SetConnMaxIdleTime(10 * time.Minute)
	}

	// 启动期做一次 Ping，提前暴露网络/认证问题（否则错误可能延后到第一次查询）
	if err := sqlDB.Ping(); err != nil {
		log.Fatalf("database ping failed: %v", err)
	}

	// If database has no tables, instruct to run migration script and exit
	if empty, err := IsDatabaseEmpty(db); err == nil && empty {
		log.Println("database is empty; please initialize schema first:")
		log.Println("  python3 scripts/init_db.py")
		os.Exit(1)
	}

	// Do not attempt to drop legacy indexes unconditionally; fresh DB from init.sql already has correct indexes.

	if len(modelDefs) > 0 {
		for _, model := range modelDefs {
			// Only migrate when table not exists to avoid intrusive changes on existing schema
			if !db.Migrator().HasTable(model) {
				if err := db.AutoMigrate(model); err != nil {
					log.Printf("auto migration failed for %T: %v", model, err)
				}
			} else {
				// Safe, additive migrations: add missing columns only
				switch m := model.(type) {
				case *models.User:
					if !db.Migrator().HasColumn(&models.User{}, "Signature") {
						if err := db.Migrator().AddColumn(&models.User{}, "Signature"); err != nil {
							log.Printf("failed to add users.signature column: %v", err)
						}
					}
				default:
					_ = m
				}
			}
		}
	}

	return db
}

// toGormLogLevel maps application LogLevel to GORM's logger level.
func toGormLogLevel(level string) logger.LogLevel {
	switch level {
	case "debug":
		// GORM 'Info' shows SQL; use with caution
		return logger.Info
	case "info", "":
		// Suppress per-statement logs; keep warnings (including slow SQL)
		return logger.Warn
	case "warn":
		return logger.Warn
	case "error":
		return logger.Error
	case "silent":
		return logger.Silent
	default:
		return logger.Warn
	}
}

// DB provides access to initialized gorm DB instance.
func DB() *gorm.DB {
	if db == nil {
		log.Fatal("database not initialized, call InitDatabase first")
	}
	return db
}

// IsDatabaseEmpty returns true when current DATABASE() schema has no tables.
func IsDatabaseEmpty(db *gorm.DB) (bool, error) {
	var cnt int
	if err := db.Raw("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE()").Scan(&cnt).Error; err != nil {
		return false, err
	}
	return cnt == 0, nil
}

func needsUniqueUsernameRepair(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "uni_users_username")
}

func repairUserUniqueIndex(db *gorm.DB) {
	// Drop any legacy constraints or indexes named uni_users_username if they exist
	type fk struct {
		ConstraintName string `gorm:"column:CONSTRAINT_NAME"`
	}
	var fks []fk
	_ = db.Raw("SELECT CONSTRAINT_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME='users' AND CONSTRAINT_NAME='uni_users_username'").Scan(&fks)
	for range fks {
		db.Exec("ALTER TABLE `users` DROP FOREIGN KEY `uni_users_username`")
		db.Exec("ALTER TABLE `users` DROP CONSTRAINT `uni_users_username`")
	}
	type idxRow struct {
		KeyName string `gorm:"column:Key_name"`
	}
	var idxs []idxRow
	_ = db.Raw("SHOW INDEX FROM `users` WHERE Key_name='uni_users_username'").Scan(&idxs)
	for range idxs {
		db.Exec("ALTER TABLE `users` DROP INDEX `uni_users_username`")
		db.Exec("DROP INDEX `uni_users_username` ON `users`")
	}

	// Delete all existing UNIQUE indexes on username to let AutoMigrate recreate the correct one
	type idx struct {
		KeyName string `gorm:"column:Key_name"`
	}

	var indexes []idx
	if queryErr := db.Raw("SHOW INDEX FROM `users` WHERE Column_name = 'username' AND Non_unique = 0").Scan(&indexes).Error; queryErr != nil {
		log.Printf("warning: unable to inspect users.username unique indexes: %v", queryErr)
		return
	}

	for _, index := range indexes {
		if dropErr := db.Exec(fmt.Sprintf("ALTER TABLE `users` DROP INDEX `%s`", index.KeyName)).Error; dropErr != nil {
			log.Printf("warning: failed to drop unique index %s: %v", index.KeyName, dropErr)
		} else {
			log.Printf("dropped unique index %s on users.username", index.KeyName)
		}
	}
}
