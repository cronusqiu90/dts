package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	config "crawlab.org/config"
	"crawlab.org/internal/crypto"
	"crawlab.org/internal/db"
	_ "crawlab.org/internal/log"

	"github.com/google/uuid"
	amqp091 "github.com/rabbitmq/amqp091-go"
	log "github.com/sirupsen/logrus"

	"crawlab.org/internal/amqp"
)

const (
	VersionInfo = "1.0.2"
)

var (
	showVersion bool
	isClosed    = make(chan struct{})
	deliveryCh  = make(chan []byte)

	cfgPath    string
	label      string
	cacheQueue string
	errDir     string
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func getEsApiToken() string {
	raw, err := json.Marshal(map[string]interface{}{
		"requestTime": time.Now().UnixMilli(),
		"userId":      "",
		"userName":    "",
		"userUid":     "",
	})
	if err != nil {
		return ""
	}
	token, err := crypto.Encrypt(raw)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(token)
}

func init() {
	cwd, _ := os.Getwd()
	errDir := filepath.Join(cwd, "data", "error")
	if _, err := os.Stat(errDir); os.IsNotExist(err) {
		_ = os.MkdirAll(errDir, os.ModeDir)
	}

	flag.BoolVar(&showVersion, "V", false, "show version information")
	flag.StringVar(&cfgPath, "c", "", "configuration file path")
	flag.StringVar(&label, "n", "", "label name")
	flag.StringVar(&cacheQueue, "to", "", "cache queue")

}

func main() {
	flag.Parse()
	if showVersion {
		fmt.Println(VersionInfo)
		return
	}

	err := config.Setup(cfgPath)
	failOnError(err, "failed to load config")

	cacheAmqpServer := amqp.NewRabbitMQ(amqp.Config{
		Url:                  config.CONF.Cache,
		ConnectionName:       fmt.Sprintf("feedback.%s", label),
		ChannelNotifyTimeout: 10 * time.Second,
		Reconnect: struct {
			Interval   time.Duration
			MaxAttempt int
		}{
			Interval:   500 * time.Millisecond,
			MaxAttempt: 7200,
		},
	})
	if err := cacheAmqpServer.Connect(); err != nil {
		log.Fatal(err)
	}
	defer cacheAmqpServer.Shutdown()

	transAmpqServer := amqp.NewRabbitMQ(amqp.Config{
		Url:                  config.CONF.AMQP,
		ConnectionName:       fmt.Sprintf("Feedback.master.download.%s", label),
		ChannelNotifyTimeout: 10 * time.Second,
		Reconnect: struct {
			Interval   time.Duration
			MaxAttempt int
		}{
			Interval:   500 * time.Millisecond,
			MaxAttempt: 7200,
		},
	})
	if err := transAmpqServer.Connect(); err != nil {
		log.Fatal(err)
	}
	defer transAmpqServer.Shutdown()
	transAmqpCh, err := transAmpqServer.Channel()
	if err != nil {
		log.Fatal(err)
	}
	defer transAmqpCh.Close()

	n := fmt.Sprintf("FEEDBACK.%s", label)
	if err := transAmpqServer.SetupChannel(
		transAmqpCh,
		amqp.Exchange{
			Name:       "Taskmgr",
			Kind:       "topic",
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
		4,
	); err != nil {
		log.Fatal(err)
	}
	msgs, err := transAmqpCh.Consume(n, n, true, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}
	log.Infof("start synching %s to %s", n, cacheQueue)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		s := <-quit
		log.Warnf("received signal %v, closing...", s)
		close(isClosed)
		if err := transAmqpCh.Cancel(n, true); err != nil {
			log.Error(err)
		}
		return
	}()

	wait := &sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wait.Add(1)
		go handleSubmitService(i, cacheAmqpServer, wait)
	}

	wait.Add(1)
	go func(deliveries <-chan amqp091.Delivery, w *sync.WaitGroup) {
		defer w.Done()
		for {
			select {
			case <-isClosed:
				return
			case d, ok := <-deliveries:
				if !ok {
					return
				}
				if d.DeliveryTag > 0 {
					deliveryCh <- d.Body
				}
			}
		}
	}(msgs, wait)

	wait.Wait()
	log.Info("stopped")
}

func handleSubmitService(no int, amqpServer *amqp.RabbitMQ, wait *sync.WaitGroup) {
	defer wait.Done()
	defer log.Infof("[SUBMIT-%d] stopped", no)

	ch, err := amqpServer.Channel()
	if err != nil {
		log.Error(err)
		return
	}
	defer ch.Close()

	if err := ch.Confirm(false); err != nil {
		log.Error(err)
		return
	}
	ntfCh := ch.NotifyPublish(make(chan amqp091.Confirmation, 1))
	rtnCh := ch.NotifyReturn(make(chan amqp091.Return, 1))

	for {
		select {
		case <-isClosed:
			return
		case body, ok := <-deliveryCh:
			if !ok {
				return
			}
			err := processBody(ch, ntfCh, rtnCh, body)
			if err != nil {
				log.Error(err)
				continue
			}
		}
	}
}

func processBody(ch *amqp091.Channel, ntfCh <-chan amqp091.Confirmation, rtnCh <-chan amqp091.Return, body []byte) error {
	uid := uuid.New().String()

	var tasks []db.ImTaskTrigger
	if err := json.Unmarshal(body, &tasks); err != nil {
		return fmt.Errorf("[%s] failed to unmarshal message: %v", uid, err)
	}

	if len(tasks) == 0 {
		return nil
	}

	rows := make([]map[string]interface{}, 0)
	for _, task := range tasks {
		row := map[string]interface{}{
			"baid":   task.BatchID,
			"unid":   task.TaskID,
			"cagy":   task.Category,
			"daso":   task.Domain,
			"serip":  task.ServerIP,
			"expt":   task.ExceptionInfo,
			"stat":   task.TaskStatus,
			"dspch":  1,
			"rctm":   task.UpdateTime.Format("2006-01-02 15:04:05"),
			"param1": task.Param1,
			"udtm":   time.Now().Format("2006-01-02 15:04:05"),
		}
		log.Infof("[%s] %v", uid, row)
		rows = append(rows, row)
	}

	payload := map[string]interface{}{
		"data":  rows,
		"token": getEsApiToken(),
		"uid":   uid,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	if err := ch.Publish(
		"Task",
		cacheQueue,
		true,
		false,
		amqp091.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	); err != nil {
		return err
	}
	select {
	case <-time.After(10 * time.Second):
		return fmt.Errorf("[%s] publish timeout", uid)
	case ntf := <-ntfCh:
		if !ntf.Ack {
			return fmt.Errorf("[%s] publish not confirmed", uid)
		}
	case rtn := <-rtnCh:
		return fmt.Errorf("[%s] publish failed, code=%d, msg=%s", uid, rtn.ReplyCode, rtn.ReplyText)
	}
	log.Infof("[%s] publish confirmed", uid)
	return nil
}
