package db

import "time"

type ImTaskTrigger struct {
	ID            int       `gorm:"column:id;primary_key;AUTO_INCREMENT"`
	BatchID       string    `json:"batch_id" gorm:"column:batch_id;type:varchar(32);NOT NULL"`
	TaskID        string    `json:"task_id" gorm:"column:task_id;type:varchar(32);NOT NULL"`
	Category      int       `json:"category" gorm:"column:category;NOT NULL"`
	Domain        int       `json:"domain" gorm:"column:domain;NOT NULL"`
	Param1        string    `json:"param1" gorm:"column:param1"`
	Param2        string    `json:"param2" gorm:"column:param2"`
	TaskStatus    int       `json:"task_status" gorm:"column:task_status;default:0"`
	UpdateTime    time.Time `json:"update_time" gorm:"column:update_time;NOT NULL"`
	ExceptionInfo string    `json:"exception_info" gorm:"column:exception_info"`
	Remark        string    `json:"remark" gorm:"column:remark"`
	TaskPriority  int       `json:"task_priority" gorm:"column:task_priority;default:0"`
	Command       int       `json:"command" gorm:"column:command;default:0"`
	ServerIP      string    `json:"server_ip" gorm:"column:server_ip"`
	Source        int       `json:"source" gorm:"column:source;default:0"`
}

type ImTaskTriggers []ImTaskTrigger

func (ImTaskTrigger) TableName() string {
	return "task_monitor"
}

func GetTaskTriggers(category, source int, domain []int) (ImTaskTriggers, error) {
	var imTaskTriggers ImTaskTriggers
	tx := db.Where("category = ? AND domain IN (?) AND source = ?", category, domain, source).Find(&imTaskTriggers)
	if tx.Error != nil {
		return imTaskTriggers, tx.Error
	}
	return imTaskTriggers, nil
}

func DeleteTaskTriggers(rowIds []int) error {
	if tx := db.Delete(&ImTaskTriggers{}, rowIds); tx.Error != nil {
		return tx.Error
	}
	return nil
}
