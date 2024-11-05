package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	config "crawlab.org/config"
	"crawlab.org/internal/crypto"
	"crawlab.org/internal/db"
	_ "crawlab.org/internal/log"
	"crawlab.org/internal/message"
	"crawlab.org/internal/util"

	log "github.com/sirupsen/logrus"

	"crawlab.org/internal/amqp"
	amqp091 "github.com/rabbitmq/amqp091-go"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Panicf("%s: %s", msg, err)
	}
}

const (
	VersionInfo = "1.0.2"
)

var (
	showVersion bool
	cfgPath     string
	label       string
)

func main() {
	flag.BoolVar(&showVersion, "V", false, "show version information")
	flag.StringVar(&cfgPath, "c", "", "configuration file path")
	flag.StringVar(&label, "n", "", "label name")
	flag.Parse()
	if showVersion {
		fmt.Println(VersionInfo)
		return
	}

	err := config.Setup(cfgPath)
	failOnError(err, "failed to load config")
	log.Infof("config: %s", util.PrettyShow(config.CONF))

	c := amqp.Config{
		Url:                  config.CONF.AMQP,
		ConnectionName:       fmt.Sprintf("Task.master.%s", label),
		ChannelNotifyTimeout: 30 * time.Second,
	}
	c.Reconnect.Interval = 500 * time.Millisecond
	c.Reconnect.MaxAttempt = 7200
	rabbitmq := amqp.NewRabbitMQ(c)
	if err := rabbitmq.Connect(); err != nil {
		log.Fatal(err)
	}
	defer rabbitmq.Shutdown()

	ch, err := rabbitmq.Channel()
	if err != nil {
		log.Fatal(err)
	}
	defer ch.Close()

	n := fmt.Sprintf("TASK.%s", label)
	if err := rabbitmq.SetupChannel(
		ch,
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
			TTL:        600000,
		},
		true,
		1,
	); err != nil {
		log.Fatal(err)
	}

	ntfCh := ch.NotifyPublish(make(chan amqp091.Confirmation, 1))
	rtnCh := ch.NotifyReturn(make(chan amqp091.Return, 1))

	msgs, err := ch.Consume(
		n,     // queue
		"",    // consumer
		true,  // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	failOnError(err, "Failed to register a consumer")

	wait := sync.WaitGroup{}
	wait.Add(1)
	go func() {
		defer wait.Done()

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		for d := range msgs {
			resp := handleRequest(d)
			body, err := json.Marshal(resp)
			if err != nil {
				log.Errorf("[%s] failed to marshal response: %v", resp.ID, err)
				continue
			}

			if err := ch.PublishWithContext(ctx,
				"Taskmgr", // exchange
				d.ReplyTo, // routing key
				true,      // mandatory
				false,     // immediate
				amqp091.Publishing{
					ContentType:   "text/plain",
					CorrelationId: d.CorrelationId,
					Body:          body,
				},
			); err != nil {
				log.Errorf("[%s] failed to deliver response: %v", resp.ID, err)
				continue
			}

			select {
			case <-ctx.Done():
				log.Errorf("[%s] deliver timeout", resp.ID)
				continue

			case ntf := <-ntfCh:
				if !ntf.Ack {
					log.Errorf("[%s] deliver not confirm", resp.ID)
					continue
				}

				log.Infof("[%s] deliver confirm", resp.ID)
				if err := submitDispatch(resp); err != nil {
					log.Errorf("[%s] failed to submit: %v", resp.ID, err)
					continue
				}
				log.Infof("[%s] submit confirm", resp.ID)
			case rtn := <-rtnCh:
				log.Errorf("[%s] failed to deliver response, errCode=%d, errMsg=%s",
					resp.ID, rtn.ReplyCode, rtn.ReplyText)
				continue
			}
		}
	}()

	log.Info(" [*] Awaiting RPC requests")
	wait.Wait()
	log.Info("stopped")
}

func handleRequest(d amqp091.Delivery) message.Response {
	log.Info("")

	var req message.Request
	var resp message.Response
	resp.Traceback = nil
	resp.Result = nil
	resp.Children = nil

	if err := json.Unmarshal(d.Body, &req); err != nil {
		return resp
	}
	log.Infof("[%s] Tag: %d, CorrelationId: %s", req.ID, d.DeliveryTag, d.CorrelationId)
	log.Infof("[%s] Request: %s", req.ID, util.PrettyShow(req))
	//
	resp.ID = req.ID

	// if req.Kwargs.RemainTasks > 2 {
	// 	resp.Status = "IGNORED"
	// 	resp.Traceback = "remain too many unCompleted tasks."
	// 	return resp
	// }

	r, err := doPost(req.ID, req.Kwargs)
	if err != nil {
		resp.Status = "FAILURE"
		resp.Traceback = err.Error()
		return resp
	}
	resp.Result = r
	resp.Status = "SUCCESS"

	log.Infof("[%s] Response: %s", req.ID, util.PrettyShow(resp))
	return resp
}

type EsApiResponse struct {
	Code int `json:"code"`
	Data struct {
		TaskList []struct {
			BatchId  string `json:"baid"`
			Category int    `json:"cagy"`
			Command  int    `json:"coman"`
			Domain   int    `json:"daso"`
			TaskId   string `json:"unid"`
			Param1   string `json:"param1"`
			Target   string `json:"target"`
			Priority int    `json:"prio"`
		} `json:"taskList"`
	} `json:"data"`
}

type EsApiPayload struct {
	Category int    `json:"category"`
	Domain   int    `json:"domain"`
	Size     int    `json:"size"`
	Token    string `json:"token"`
}

func doPost(reqID string, kwargs message.Kwargs) (message.Result, error) {
	var r message.Result
	r.Code = 200
	r.Message = "success"

	category := kwargs.Category
	for _, domain := range kwargs.Domain {
		tasks, err := doPostByCategoryAndDomain(reqID, category, domain)
		if err != nil {
			log.Errorf("[%s] failed to POST category=%d, domain=%d, err=%v", reqID, category, domain, err)
			continue
		}

		for _, task := range tasks {
			task.ServerIP = kwargs.Hostname
			task.Source = kwargs.Source
			task.TaskSource = kwargs.Source
			task.IsDispatched = 1
			log.Infof("[%s] Task(task_id=%s, category=%d, domain=%d, host=%s)", reqID, task.TaskID, task.Category, task.Domain, task.ServerIP)
			r.Tasks = append(r.Tasks, task)
		}

	}
	return r, nil
}

func doPostByCategoryAndDomain(reqID string, category, domain int) ([]db.ImTask, error) {
	tasks := make([]db.ImTask, 0)

	payload := EsApiPayload{
		Category: category,
		Domain:   domain,
		Size:     1,
		Token:    getEsApiToken(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return tasks, err
	}

	log.Debugf("[%s] POST %s", reqID, config.CONF.ES.GetTask)
	log.Debugf("[%s] %s", reqID, string(raw))
	req, err := http.NewRequest("POST", config.CONF.ES.GetTask, bytes.NewBuffer(raw))
	if err != nil {
		return tasks, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range req.Header {
		log.Debugf("[%s] Header %s: %v", reqID, k, v)
	}

	client := &http.Client{Timeout: time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return tasks, err
	}
	defer func() {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	log.Debugf("[%s] API Response(code=%d, status=%s)", reqID, resp.StatusCode, resp.Status)
	if resp.StatusCode != http.StatusOK {
		return tasks, fmt.Errorf("statusCode=%d", resp.StatusCode)
	}

	raw, err = io.ReadAll(resp.Body)
	if err != nil {
		return tasks, err
	}

	var esApiResp EsApiResponse
	if err := json.Unmarshal(raw, &esApiResp); err != nil {
		return tasks, err
	}
	if esApiResp.Code != 200 {
		return tasks, fmt.Errorf("ES statusCode=%d", esApiResp.Code)
	}
	for _, esTask := range esApiResp.Data.TaskList {
		t := db.ImTask{
			BatchID:      esTask.BatchId,
			TaskID:       esTask.TaskId,
			Category:     esTask.Category,
			Domain:       esTask.Domain,
			Command:      esTask.Command,
			Param1:       esTask.Param1,
			Target:       esTask.Target,
			TaskPriority: esTask.Priority,
		}
		log.Debugf("[%s] %v", reqID, util.PrettyShow(t))
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func getEsApiToken() string {
	raw, err := json.Marshal(map[string]interface{}{
		"requestTime": time.Now().UnixMilli(),
		"userId":      "154",
		"userName":    "extName",
		"userUid":     "dsaeweqwdsadasfq15452544",
	})
	if err != nil {
		log.Error(err)
		return ""
	}

	token, err := crypto.Encrypt(raw)
	if err != nil {
		log.Error(err)
		return ""
	}
	return base64.StdEncoding.EncodeToString(token)

}

func submitDispatch(response message.Response) error {
	result := response.Result.(message.Result)
	if result.Tasks == nil {
		return errors.New("empty response")
	}

	rows := make([]interface{}, 0)
	for _, task := range result.Tasks {
		rows = append(rows, map[string]interface{}{
			"serip": task.ServerIP,
			"expt":  task.ExceptionInfo,
			"stat":  task.TaskStatus,
			"unid":  task.TaskID,
			"baid":  task.BatchID,
			"daso":  task.Domain,
			"cagy":  task.Category,
			"dspch": 2,
			"rctm":  task.UpdateTime.Format("2006-01-02 15:04:05.000"),
		})
	}
	raw, err := json.Marshal(map[string]interface{}{
		"data":  rows,
		"token": getEsApiToken(),
	})
	if err != nil {
		return err
	}

	log.Debugf("[%s] POST %s", response.ID, config.CONF.ES.UpdateTask)
	log.Debugf("[%s] %s", response.ID, string(raw))
	req, err := http.NewRequest("POST", config.CONF.ES.UpdateTask, bytes.NewBuffer(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update task, status code: %d", resp.StatusCode)
	}
	raw, err = io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	log.Infof("[%s] %s", response.ID, string(raw))
	return nil
}
