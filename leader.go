package leaderelection

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type ElectionRecord struct {
	ID           uint   `gorm:"primary_key"`
	ElectionName string `gorm:"unique_index:uidx_election_name"`
	LeaderName   string
	LastUpdate   time.Time `gorm:"autoCreateTime"`
}

type Election struct {
	ElectionName string
	LeaderName   string
	db           *gorm.DB
}

// NewElection Starts a new election with the given name, and candidate name. Multiple candidates can try to win a given
// election name, but only one of them would succeed.
// Inspired from https://gist.github.com/ljjjustin/f2213ac9b9b8c31df746f8b56095ea32
func NewElection(name string, candidate string, config map[string]string) (*Election, error) {
	var err error
	election := Election{ElectionName: name, LeaderName: candidate}
	mysqlDSN := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8&parseTime=True&loc=Local",
		config["MYSQL_USER"],
		config["MYSQL_PASSWORD"],
		config["MYSQL_HOST"],
		config["MYSQL_PORT"],
		config["MYSQL_DBNAME"],
	)

	election.db, err = gorm.Open(mysql.New(mysql.Config{
		DSN:               mysqlDSN,
		DefaultStringSize: 256,
	}), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	sqlDB, err := election.db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetConnMaxLifetime(1 * time.Hour)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetMaxOpenConns(10)

	if err = election.db.AutoMigrate(&ElectionRecord{}); err != nil {
		return nil, fmt.Errorf("failed to create/update db tables with error %s", err.Error())
	}

	return &election, nil
}

// Campaign starts to attempt to win an election.
func (e *Election) Campaign(ctx context.Context) (bool, error) {
	sql := `INSERT IGNORE INTO election_records (election_name, leader_name, last_update) VALUES (?, ?, ?)
			ON DUPLICATE KEY UPDATE
			leader_name = IF(last_update < DATE_SUB(VALUES(last_update), INTERVAL 60 SECOND), VALUES(leader_name), leader_name),
			last_update = IF(leader_name = VALUES(leader_name), VALUES(last_update), last_update)`
	affected := e.db.Exec(sql, e.ElectionName, e.LeaderName, time.Now()).RowsAffected
	if affected > 0 {
		//good you are leader
		return true, nil
	} else {
		// wait 20 seconds and campaign again
		return false, nil
	}
}

func (e *Election) IsLeader(ctx context.Context) (bool, error) {
	var count int
	sql := `SELECT COUNT(*) as is_leader FROM election_records where election_name=? and leader_name=?`
	e.db.Raw(sql, e.ElectionName, e.LeaderName).Scan(&count)
	return count > 0, nil
}

type CallbackFunc func()

func ElectLeader(electionName string, becomeLeaderCb CallbackFunc, looseLeadershipCB CallbackFunc) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	workerName := fmt.Sprintf("worker/%s/%s", hostname, getWorkerId())
	appConfig, err := godotenv.Read()
	if err != nil {
		log.Fatalf("Error reading .env file %s", err.Error())
	}

	election, _ := NewElection(electionName, workerName, appConfig)
	ctx, _ := context.WithCancel(context.Background())
	var isLeader int64 = 0
	var wonCampaign bool

	log.Printf("Starting as candidate [%s] in election [%s].\n", workerName, electionName)
	for {
		if wonCampaign, err = election.Campaign(ctx); err != nil {
			log.Fatalf("Failed in election.Campaign, error : %s\n", err.Error())
		}

		if !wonCampaign {
			if atomic.CompareAndSwapInt64(&isLeader, 1, 0) {
				log.Printf("Oh No! [%s] lost leadership.\n", workerName)
				looseLeadershipCB()
			}
			log.Printf("Failed to accuire leadership, will reattempt....\n")
			time.Sleep(60 * time.Second)
			continue
		}

		//double check.
		verifyLeadership, err := election.IsLeader(ctx)
		if err != nil {
			log.Fatalf("Failed in election.Campaign, error : %s\n", err.Error())
		}
		if !verifyLeadership {
			log.Printf("Failed to verify leadership candidate [%s] in election [%s]. Will reattempt...\n", workerName, electionName)
			continue
		}
		if atomic.CompareAndSwapInt64(&isLeader, 0, 1) {
			log.Printf("Yeaaah! [%s] won and is the leader.\n", workerName)
			becomeLeaderCb()
		}
		time.Sleep(15 * time.Second)
		//log.Printf("Ensuring leadership....\n")
	}
}

func getWorkerId() string {
	addrs, err := getMacAddr()
	if err != nil {
		log.Printf("Error geting macAddr %s \n", err.Error())
		addrs = []string{}
	}
	addrs = append(addrs, strconv.Itoa(os.Getpid()))
	hash := md5.New()
	hash.Write([]byte(strings.Join(addrs, ",")))
	sha := hex.EncodeToString(hash.Sum(nil))
	return sha
}

func getMacAddr() ([]string, error) {
	ifas, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var as []string
	for _, ifa := range ifas {
		a := ifa.HardwareAddr.String()
		if a != "" {
			as = append(as, a)
		}
	}
	return as, nil
}
