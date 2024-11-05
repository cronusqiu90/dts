package db

import "time"

type ImResult struct {
	ID                 int       `gorm:"column:id;primary_key;AUTO_INCREMENT"`
	BatchID            string    `gorm:"column:batch_id;NOT NULL"`
	TaskID             string    `gorm:"column:task_id;NOT NULL"`
	Category           int       `gorm:"column:category;NOT NULL"`
	Domain             int       `gorm:"column:domain;NOT NULL"`
	DetectAccount      string    `gorm:"column:detect_account"`
	DetectTime         time.Time `gorm:"column:detect_time;NOT NULL"`
	Target             string    `gorm:"column:target;NOT NULL"`
	UserID             string    `gorm:"column:user_id;NOT NULL"`
	UserName           string    `gorm:"column:user_name"`
	FirstName          string    `gorm:"column:first_name"`
	LastName           string    `gorm:"column:last_name"`
	Email              string    `gorm:"column:email"`
	Description        string    `gorm:"column:description"`
	Location           string    `gorm:"column:location"`
	Language           string    `gorm:"column:language"`
	RegisterTime       string    `gorm:"column:register_time"`
	Birthday           string    `gorm:"column:birthday"`
	ImageName          string    `gorm:"column:image_name"`
	HomeUrl            string    `gorm:"column:home_url"`
	TimeZone           string    `gorm:"column:time_zone"`
	LastOnlineTime     string    `gorm:"column:last_online_time"`
	Followers          int       `gorm:"column:followers"`
	Friends            int       `gorm:"column:friends"`
	Subscribes         int       `gorm:"column:subscribes"`
	Favourites         int       `gorm:"column:favourites"`
	PostedCount        int       `gorm:"column:posted_count"`
	IsGeoEnable        int       `gorm:"column:is_geo_enable"`
	IsVerified         int       `gorm:"column:is_verified"`
	Education          string    `gorm:"column:education"`
	Hometown           string    `gorm:"column:hometown"`
	RelationshipStatus string    `gorm:"column:relationship_status"`
	Work               string    `gorm:"column:work"`
	Quotes             string    `gorm:"column:quotes"`
	ServerIP           string    `gorm:"column:server_ip"`
	IsGathered         int       `gorm:"column:is_gathered;NOT NULL"`
	Country            string    `gorm:"column:country"`
	Province           string    `gorm:"column:province"`
	City               string    `gorm:"column:city"`
	PhoneOperators     string    `gorm:"column:phone_operators"`
	GatherTime         int       `gorm:"column:gather_time"`
}

func (ImResult) TableName() string {
	return "im_result"
}
