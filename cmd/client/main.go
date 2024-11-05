package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	amqp091 "github.com/rabbitmq/amqp091-go"

	config "crawlab.org/config"
	"crawlab.org/internal/amqp"
	"crawlab.org/internal/db"
	_ "crawlab.org/internal/log"
	"crawlab.org/internal/message"
	"crawlab.org/internal/util"

	log "github.com/sirupsen/logrus"
)

const (
	VersionInfo      = "1.0.2"
	AmqpExchange     = "Taskmgr"
	AmqpExchangeType = "topic"
)

var (
	isClosed            = make(chan struct{})
	domains             = make([]int, 0)
	showVersion         bool
	cfgPath             string
	label               string
	category            int
	domain              string
	dataType            int
	dbName              string
	source              int
	hostname            string
	logLevel            string
	disableAliveAccount bool
	allowRemainTask     int
	rabbitmq            *amqp.RabbitMQ
)

func init() {
	hostname, _ = os.Hostname()

	cwd, _ := os.Getwd()
	cfgPath = filepath.Join(cwd, "etc", "config.cfg")
	flag.StringVar(&cfgPath, "c", cfgPath, "")

	flag.BoolVar(&showVersion, "version", false, "show version")
	flag.StringVar(&dbName, "db", "", "db")
	flag.StringVar(&label, "label", "", "label")
	flag.IntVar(&category, "category", 0, "category")
	flag.StringVar(&domain, "domain", "", "multiple domains if supported,separated by comma")
	flag.IntVar(&dataType, "dataType", 0, "data type")
	flag.StringVar(&hostname, "hostname", hostname, "hostname")
	flag.StringVar(&logLevel, "logLevel", "info", "log level")
	flag.BoolVar(&disableAliveAccount, "disable-alive-account", false, "disable alive account")
	flag.IntVar(&allowRemainTask, "max-remain-task", 2, "allow max un-completed task record")

}

func newRequest(corrId string) ([]byte, error) {
	maxRemainTasks, err := db.CountRemainTasks(category, source, domains)
	if err != nil {
		return nil, err
	}
	if maxRemainTasks > allowRemainTask {
		return nil, fmt.Errorf("remain too many unCompleted tasks(%d)", maxRemainTasks)
	}

	aliveAccounts, err := db.CountAliveAccounts(category, domains)
	if err != nil {
		return nil, err
	}

	req := message.Request{
		ID: corrId,
		Kwargs: message.Kwargs{
			Category:             category,
			Domain:               domains,
			DataType:             dataType,
			Source:               source,
			Hostname:             hostname,
			RemainTasks:          maxRemainTasks,
			AliveAccounts:        aliveAccounts,
			DisabledAliveAccount: disableAliveAccount,
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func handleTask(task db.ImTask) error {
	task.ServerIP = hostname
	task.UpdateTime = time.Now()
	task.IsGathered = 0
	task.IsDispatched = 1

	t := db.GetTask(task.TaskID, task.Category, task.Domain, task.Source)
	if t.ID > 0 {
		if t.TaskStatus == 1 {
			return errors.New("already in running")
		}
		if err := db.DeleteTask(task.TaskID); err != nil {
			return err
		}
	}

	if err := db.AddTask(task); err != nil {
		return err
	}
	return nil
}

func startRpcRequestService() error {
	ch, err := rabbitmq.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	n := fmt.Sprintf("Client.%s.c%d.%s", label, category, hostname)
	if err := rabbitmq.SetupChannel(
		ch,
		amqp.Exchange{
			Name:       AmqpExchange,
			Kind:       AmqpExchangeType,
			Durable:    true,
			AutoDelete: false,
		},
		amqp.Queue{
			Name:       n,
			Durable:    false,
			AutoDelete: true,
			Exclusive:  true,
		},
		true,
		1,
	); err != nil {
		return err
	}

	ntfCh := ch.NotifyPublish(make(chan amqp091.Confirmation, 1))
	defer close(ntfCh)
	rtnCh := ch.NotifyReturn(make(chan amqp091.Return, 1))
	defer close(rtnCh)

	corrId := uuid.New().String()
	body, err := newRequest(corrId)
	if err != nil {
		return err
	}

	msgs, err := ch.Consume(
		n,      // queue
		corrId, // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)

	if err := ch.Publish(
		"Taskmgr",                     // exchange
		fmt.Sprintf("TASK.%s", label), // routing key
		true,                          // mandatory
		false,                         // immediate
		amqp091.Publishing{
			ContentType:   "text/plain",
			CorrelationId: corrId,
			ReplyTo:       n,
			Body:          body,
			Expiration:    "60000", // 60 seconds
		},
	); err != nil {
		return err
	}
	log.Infof("[%s] %s", corrId, string(body))
	select {
	case <-isClosed:
		return nil

	case <-time.After(30 * time.Second):
		return fmt.Errorf("[%s] failed to deliver request, timeout", corrId)

	case ntf := <-ntfCh:
		if !ntf.Ack {
			return fmt.Errorf("[%s] failed to deliver request, nacked", corrId)
		}

		log.Infof("[%s] request deliver succeeded", corrId)
		break
	case rtn := <-rtnCh:
		return fmt.Errorf("[%s] failed to deliver request, code=%d, err=%s", corrId, rtn.ReplyCode, rtn.ReplyText)
	}

	select {
	case <-isClosed:
		return nil

	case <-time.After(120 * time.Second):
		return fmt.Errorf("[%s] wait for response timeout", corrId)

	case msg, ok := <-msgs:
		if !ok {
			return nil
		}
		if msg.DeliveryTag == 0 {
			return errors.New("invalid delivery tag")
		}
		if corrId == msg.CorrelationId {
			var resp message.Response
			if err := json.Unmarshal(msg.Body, &resp); err != nil {
				return err
			}

			switch resp.Status {
			case "FAILURE":
				return fmt.Errorf("[%s] FAILURE:%v", corrId, resp.Traceback)
			case "IGNORED":
				return fmt.Errorf("[%s] IGNORED", corrId)
			case "STARTED":
				log.Infof("[%s] STARTED", corrId)
			case "SUCCESS":
				var r message.Result

				if raw, err := json.Marshal(resp.Result); err != nil {
					return fmt.Errorf("[%s] %v", corrId, err)
				} else {
					if err := json.Unmarshal(raw, &r); err != nil {
						return fmt.Errorf("[%s] %v", corrId, err)
					}
				}

				if r.Code != 200 {
					return fmt.Errorf("[%s] %v", corrId, r)
				}

				if r.Tasks == nil || len(r.Tasks) == 0 {
					return fmt.Errorf("[%s] no available tasks", corrId)
				}

				total := len(r.Tasks)
				for index, task := range r.Tasks {
					if err := handleTask(task); err != nil {
						log.Errorf("[%d/%d] failed to handle Task(task_id=%s): %v", index+1, total, task.TaskID, err)
						continue
					}
					log.Infof("[%d/%d] Task(task_id=%s) Added", index+1, total, task.TaskID)
				}
			}
			return nil
		}
	}

	return errors.New("unknown error")
}

func startFeedService() error {
	ch, err := rabbitmq.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	n := fmt.Sprintf("FEEDBACK.%s", label)

	if err := rabbitmq.SetupChannel(
		ch,
		amqp.Exchange{
			Name:       AmqpExchange,
			Kind:       AmqpExchangeType,
			Durable:    true,
			AutoDelete: false,
		},
		amqp.Queue{
			Name:       n,
			Durable:    true,
			AutoDelete: false,
			Exclusive:  false,
		},
		true,
		1,
	); err != nil {
		return err
	}

	rows, err := db.GetTaskTriggers(category, source, domains)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	rowIds := make([]int, 0)
	for _, row := range rows {
		row.ServerIP = hostname
		rowIds = append(rowIds, row.ID)
		log.Infof("[Feedback] Task(task_id=%s, category=%d, domain=%d, status=%d, err=%s)",
			row.TaskID, row.Category, row.Domain, row.TaskStatus, row.ExceptionInfo)
	}

	body, err := json.Marshal(rows)
	if err != nil {
		return err
	}

	if err := rabbitmq.Publish(
		ch,
		AmqpExchange,
		n,
		true,
		false,
		true,
		body,
	); err != nil {
		return err
	}

	if err := db.DeleteTaskTriggers(rowIds); err != nil {
		return err
	}
	log.Infof("Cleanup %d taskTriggers", len(rowIds))
	return nil

}

func main() {
	flag.Parse()
	if showVersion {
		fmt.Println(VersionInfo)
		return
	}

	switch strings.ToLower(logLevel) {
	case "trace":
		log.SetLevel(log.TraceLevel)
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	case "fatal":
		log.SetLevel(log.FatalLevel)
	}
	var err error

	// source
	source, err = util.GetSource(label)
	if err != nil {
		log.Fatalf("failed to get source:%v", err)
	}
	// supported domain
	for _, d := range strings.Split(domain, ",") {
		d_, err := strconv.Atoi(d)
		if err != nil {
			log.Fatal(err)
		}
		domains = append(domains, d_)
	}
	// loading config
	err = config.Setup(cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	// init database
	if err := db.SetupDB(dbName, false); err != nil {
		log.Fatal(err)
	}
	// init amqp
	c := amqp.Config{}
	c.Url = config.CONF.AMQP
	c.ConnectionName = fmt.Sprintf("client.%s.%s", label, hostname)
	c.ChannelNotifyTimeout = 10 * time.Second
	c.Reconnect.Interval = 500 * time.Millisecond
	c.Reconnect.MaxAttempt = 7200
	rabbitmq = amqp.NewRabbitMQ(c)
	if err := rabbitmq.Connect(); err != nil {
		log.Fatal(err)
	}
	// register system signal watchdog
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		n := <-quit
		log.Warnf("signal %v received, shutting down", n)
		close(isClosed)
	}()

	wait := sync.WaitGroup{}

	// start feedback modified task records
	wait.Add(1)
	go func() {
		defer wait.Done()
		log.Info("[feedback] service started")

		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-isClosed:
				log.Info("[feedback] service stopped")
				return
			case <-tick.C:
				err := startFeedService()
				if err != nil {
					log.Error(err)
				}
			}
		}
	}()

	// start rpc client
	wait.Add(1)
	go func() {
		defer wait.Done()
		log.Info("[rpc] service starated")

		tick := time.NewTicker(15 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-isClosed:
				log.Info("[rpc] service stopped")
				return
			case <-tick.C:
				err := startRpcRequestService()
				if err != nil {
					log.Error(err)
				}
			}
		}
	}()

	wait.Wait()
	log.Info("stopped")
}
