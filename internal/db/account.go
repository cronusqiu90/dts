package db

import "time"

type ImAccount struct {
	ID                int       `gorm:"column:id;primary_key;AUTO_INCREMENT"`
	AccountID         string    `gorm:"column:account_id;NOT NULL"`
	Domain            int       `gorm:"column:domain;NOT NULL"`
	Category          int       `gorm:"column:category;NOT NULL"`
	AreaCode          string    `gorm:"column:area_code"`
	Account           string    `gorm:"column:account"`
	Password          string    `gorm:"column:password"`
	Token1            string    `gorm:"column:token1"`
	Token2            string    `gorm:"column:token2"`
	CreateTime        time.Time `gorm:"column:create_time;NOT NULL"`
	Status            int       `gorm:"column:status;NOT NULL"`
	IsAllocated       int       `gorm:"column:is_allocated;NOT NULL"`
	AllocateServerIP  string    `gorm:"column:allocate_server_ip"`
	DeviceInfo        string    `gorm:"column:device_info"`
	TotalAmount       int       `gorm:"column:total_amount;NOT NULL"`
	ValidAmount       int       `gorm:"column:valid_amount;NOT NULL"`
	TodayAmount       int       `gorm:"column:today_amount;NOT NULL"`
	LastestUpdateTime int       `gorm:"column:lastest_update_time"`
	NextUpdateTime    int       `gorm:"column:next_update_time"`
	Direct            int       `gorm:"column:direct"`
	ProxyType         int       `gorm:"column:proxy_type"`
	ProxyIPAddress    string    `gorm:"column:proxy_ip_address"`
	ProxyPort         string    `gorm:"column:proxy_port"`
	ProxyAccount      string    `gorm:"column:proxy_account"`
	ProxyPassword     string    `gorm:"column:proxy_password"`
	Source            int       `gorm:"column:source;NOT NULL"`
	IsGathered        int       `gorm:"column:is_gathered;NOT NULL"`
	GatherTime        int       `gorm:"column:gather_time"`
	SysGroupID        string    `gorm:"column:sys_group_id"`
	SysUserID         int       `gorm:"column:sys_user_id"`
}

func (ImAccount) TableName() string {
	return "im_self_account"
}

func CountAliveAccounts(category int, domains []int) (int, error) {
	var count int64
	tx := db.Model(&ImAccount{}).Where("category = ? AND domain in (?) AND status = 0", category, domains).Count(&count)
	if tx.Error != nil {
		return 0, tx.Error
	}
	return int(count), nil
}
