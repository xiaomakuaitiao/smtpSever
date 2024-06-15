package main

import (
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"sync"
	"time"
)

type DB struct {
	conn *sql.DB
}

// 创建单列数据库连接，ync.Once 是 Go 的一个同步原语，用于确保某些操作只执行一次
var (
	dbInstance *DB
	once       sync.Once
)

// NewDB 返回一个数据库连接实例
func NewDB(dsn string) (*DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)                 // 设置最大打开连接数
	db.SetMaxIdleConns(25)                 // 设置最大空闲连接数
	db.SetConnMaxLifetime(5 * time.Minute) // 设置连接的最大可复用时间
	return &DB{conn: db}, nil
}

// GetDBInstance 返回一个数据库连接实例
func GetDBInstance(config *Config) (*DB, error) {
	var err error
	once.Do(func() {
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
			config.Database.Username,
			config.Database.Password,
			config.Database.Host,
			config.Database.Port,
			config.Database.Name)
		dbInstance, err = NewDB(dsn)
	})
	return dbInstance, err
}

func (db *DB) Close() {
	db.conn.Close()
}

// UserData 包含从数据库中查询到的用户信息和发送限制
type UserData struct {
	UserPlanID     int64
	Status         int
	DayLimit       int
	HourLimit      int
	TotalLimit     int
	UsedDayLimit   int
	UsedHourLimit  int
	UsedTotalLimit int
	Domains        []string
	ServerHost     string
	ServerPort     string
	ServerUsername string
	ServerPassword string
}

// GetUserAndDomainData 从数据库中查询用户和域名的数据
func (db *DB) GetUserAndDomainData(username, password string) (*UserData, error) {
	var userData UserData
	rows, err := db.conn.Query(`
		SELECT up.id, up.status, up.day_limit, up.hour_limit, up.total_limit, 
		       up.used_day_limit, up.used_hour_limit, up.used_total_limit, 
		       ud.domain, s.host, s.port, s.username, s.password
		FROM user_plans up
		JOIN user_domains ud ON up.id = ud.user_plan_id
		JOIN server_plans sp ON up.server_plan_id = sp.plan_id
		JOIN servers s ON sp.server_id = s.id
		WHERE up.username = ? AND up.password = ? AND up.status = 1 AND ud.status = 1`,
		username, password)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, errors.New("invalid username or password")
	}
	var domain string
	// 读取第一条记录
	err = rows.Scan(&userData.UserPlanID, &userData.Status, &userData.DayLimit, &userData.HourLimit,
		&userData.TotalLimit, &userData.UsedDayLimit, &userData.UsedHourLimit, &userData.UsedTotalLimit,
		&domain, &userData.ServerHost, &userData.ServerPort, &userData.ServerUsername, &userData.ServerPassword)
	if err != nil {
		return nil, err
	}
	userData.Domains = append(userData.Domains, domain)

	for rows.Next() {
		err = rows.Scan(&userData.UserPlanID, &userData.Status, &userData.DayLimit, &userData.HourLimit,
			&userData.TotalLimit, &userData.UsedDayLimit, &userData.UsedHourLimit, &userData.UsedTotalLimit,
			&domain, &userData.ServerHost, &userData.ServerPort, &userData.ServerUsername, &userData.ServerPassword)
		if err != nil {
			return nil, err
		}
		userData.Domains = append(userData.Domains, domain)
	}

	if len(userData.Domains) == 0 {
		return nil, errors.New("not allowed to send emails to any domain")
	}

	return &userData, nil
}

// CheckSendLimits 检查用户的发送限制
func (db *DB) CheckSendLimits(userData *UserData) error {
	if userData.UsedTotalLimit >= userData.TotalLimit {
		return errors.New("total limit exceeded")
	}
	if userData.UsedDayLimit >= userData.DayLimit {
		return errors.New("daily limit exceeded")
	}
	if userData.UsedHourLimit >= userData.HourLimit {
		return errors.New("hourly limit exceeded")
	}
	return nil
}

// UpdateSendLimits 更新用户的发送限制
func (db *DB) UpdateSendLimits(userPlansID int64, increment int) error {
	_, err := db.conn.Exec(`
	UPDATE user_plans SET used_total_limit = used_total_limit + ?, used_day_limit = used_day_limit + ?,
	used_hour_limit = used_hour_limit + ? WHERE id = ?`, increment, increment, increment, userPlansID)
	return err
}
