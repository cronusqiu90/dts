package db

import (
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	log "github.com/sirupsen/logrus"
)

const (
	user     = "root"
	password = "admin@2018"
	host     = "127.0.0.1"
	port     = "3306"
)

var (
	db *gorm.DB
)

func SetupDB(dbName string, debug bool) error {
	var err error
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8&parseTime=True&loc=Local",
		user, password, host, port, dbName,
	)

	l := logger.Discard
	if debug {
		l = logger.Default.LogMode(logger.Info)
	}

	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: l,
	})
	if err != nil {
		return err
	}

	rawDb, _ := db.DB()
	rawDb.SetMaxIdleConns(5)
	rawDb.SetMaxOpenConns(5)
	rawDb.SetConnMaxIdleTime(3600)
	rawDb.SetConnMaxLifetime(3600)

	// if err := db.AutoMigrate(&ImTask{}, &ImAccount{}, &ImResult{}); err != nil {
	// 	return err
	// }

	if err := setupTrigger(dbName); err != nil {
		return err
	}

	return nil
}

func setupTrigger(dbName string) error {
	// create trigger table
	if err := db.AutoMigrate(&ImTaskTrigger{}); err != nil {
		return err
	}

	// check trigger exists ?
	if err := ensureTrigger(dbName); err != nil {
		return err
	}

	return nil
}

func ensureTrigger(dbName string) error {
	var rows []map[string]interface{}
	stmt := "SELECT TRIGGER_NAME, EVENT_OBJECT_TABLE FROM INFORMATION_SCHEMA.TRIGGERS WHERE EVENT_OBJECT_SCHEMA = ?;"
	tx := db.Raw(stmt, dbName).Scan(&rows)
	if tx.Error != nil {
		return tx.Error
	}

	for _, row := range rows {
		log.Debug(row)
		if row["EVENT_OBJECT_TABLE"].(string) != "im_task" {
			continue
		}
		if row["TRIGGER_NAME"] == "crawlab_task_trigger" {
			return nil
		}
	}

	if err := createTrigger(); err != nil {
		return err
	}

	return nil

}

func createTrigger() error {
	stmt := `
	CREATE TRIGGER crawlab_task_trigger AFTER UPDATE ON im_task FOR EACH ROW 
BEGIN 
	IF old.source > 0 AND new.is_gathered = 0 AND new.task_status > 0 THEN 
		REPLACE INTO task_monitor (
			batch_id, task_id, category, domain, 
			param1, param2, task_status, update_time, 
			exception_info, task_priority, command, 
			server_ip, remark, source
		) 
		VALUES 
		(
			new.batch_id, new.task_id, new.category, 
			new.domain, new.param1, new.param2, 
			new.task_status, new.update_time, 
			new.exception_info, new.task_priority, 
			new.command, new.server_ip, new.remark, 
			new.source
		);
	END IF;
END;`
	if tx := db.Exec(stmt); tx.Error != nil {
		return tx.Error
	}
	return nil

}
