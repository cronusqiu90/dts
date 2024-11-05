package db

import (
	"time"
)

type ImTask struct {
	ID             int       `gorm:"column:id;primary_key;AUTO_INCREMENT"`
	BatchID        string    `gorm:"column:batch_id;NOT NULL"`
	TaskID         string    `gorm:"column:task_id;NOT NULL"`
	Target         string    `gorm:"column:target;NOT NULL"`
	Category       int       `gorm:"column:category;NOT NULL"`
	Domain         int       `gorm:"column:domain;NOT NULL"`
	Param1         string    `gorm:"column:param1"`
	Param2         string    `gorm:"column:param2"`
	TaskStatus     int       `gorm:"column:task_status;default:0"`
	UpdateTime     time.Time `gorm:"column:update_time;NOT NULL"`
	ExceptionInfo  string    `gorm:"column:exception_info"`
	IsAutoJoin     int       `gorm:"column:is_auto_join;default:0"`
	Account        string    `gorm:"column:account"`
	Remark         string    `gorm:"column:remark"`
	TaskPriority   int       `gorm:"column:task_priority;default:0"`
	Command        int       `gorm:"column:command;default:0"`
	IsGathered     int       `gorm:"column:is_gathered;default:0"`
	IsDeleted      int       `gorm:"column:is_deleted;default:0"`
	ServerIP       string    `gorm:"column:server_ip"`
	UseCount       int       `gorm:"column:use_count;default:0"`
	IsDispatched   int       `gorm:"column:is_dispatched;default:1"`
	ParentGroupID  string    `gorm:"column:parent_group_id"`
	RootGroupID    string    `gorm:"column:root_group_id"`
	ChatPos        string    `gorm:"column:chat_pos"`
	ResultTaskID   string    `gorm:"column:result_task_id"`
	SysGroupID     string    `gorm:"column:sys_group_id"`
	SysUserID      int       `gorm:"column:sys_user_id;default:0"`
	GatherTime     int       `gorm:"column:gather_time;default:0"`
	TaskSource     int       `gorm:"column:task_source;default:0"`
	Source         int       `gorm:"column:source;default:0"`
	NextUpdateTime int       `gorm:"column:next_update_time;default:0"`
}

func (ImTask) TableName() string {
	return "im_task"
}

func CountRemainTasks(category, source int, domains []int) (int, error) {
	var count int64
	// sql: SELECT count(id) FROM im_task WHERE category = ? AND domain in (?) AND source = ? AND task_status < 2;
	tx := db.Model(&ImTask{}).Where("category = ? AND domain IN (?) AND source = ? AND task_status < 2", category, domains, source).Count(&count)
	if tx.Error != nil {
		return 0, tx.Error
	}
	return int(count), nil
}

func GetTask(taskId string, category, domain, source int) ImTask {
	var imTask ImTask
	db.Where(&ImTask{TaskID: taskId, Category: category, Domain: domain, Source: source}).First(&imTask)
	return imTask
}

func AddTask(task ImTask) error {
	return db.Create(&task).Error
}

func DeleteTask(taskId string) error {
	tx := db.Where("task_id = ?", taskId).Delete(&ImTask{})
	return tx.Error
}
