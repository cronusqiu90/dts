package amqp

import (
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	log "github.com/sirupsen/logrus"
)

type Exchange struct {
	Name       string
	Kind       string
	Durable    bool
	AutoDelete bool
}

type Queue struct {
	Name       string
	Durable    bool
	AutoDelete bool
	Exclusive  bool
	TTL        int
}

type Config struct {
	Url                  string
	ConnectionName       string
	ChannelNotifyTimeout time.Duration
	Reconnect            struct {
		Interval   time.Duration
		MaxAttempt int
	}
}

type RabbitMQ struct {
	mux                  sync.RWMutex
	config               Config
	dialConfig           amqp.Config
	connection           *amqp.Connection
	ChannelNotifyTimeout time.Duration
}

func NewRabbitMQ(config Config) *RabbitMQ {
	return &RabbitMQ{
		config: config,
		dialConfig: amqp.Config{
			Heartbeat: 35,
			Properties: amqp.Table{
				"connection_name": config.ConnectionName,
			},
		},
		ChannelNotifyTimeout: config.ChannelNotifyTimeout,
	}
}

// Connect create a new connection, Use once at application startup.
func (r *RabbitMQ) Connect() error {
	conn, err := amqp.DialConfig(r.config.Url, r.dialConfig)
	if err != nil {
		return err
	}
	r.connection = conn

	go r.reconnect()

	return nil
}

// reconnect reconnects to server if the connection or a channel
// is closed unexpectedly. Normal shutdown is ignored. It tries
// maximum of 7200 times and sleeps half a second in between each
// try which equals to 1hour.
func (r *RabbitMQ) reconnect() {
WATCH:
	connErr := <-r.connection.NotifyClose(make(chan *amqp.Error))
	if connErr == nil {
		return
	}

	log.Warnf("connection dropped, reconnecting ...")
	var err error

	for i := 1; i <= r.config.Reconnect.MaxAttempt; i++ {
		r.mux.RLock()
		r.connection, err = amqp.DialConfig(r.config.Url, r.dialConfig)
		r.mux.RUnlock()
		if err == nil {
			log.Infof("[%d/%d] reconnected", i, r.config.Reconnect.MaxAttempt)
			goto WATCH
		}
		time.Sleep(r.config.Reconnect.Interval)
	}
	log.Fatal("failed to reconnect")
}

// Shutdown triggers a normal shutdown. Use this when you wish
// to shutdown your current connection or if you are shutting
// down the application.
func (r *RabbitMQ) Shutdown() error {
	if r.connection != nil {
		return r.connection.Close()
	}
	return nil
}

// Channel returns a new `*amqp.Channel` instance. You must
// call `defer channel.Close()` as soon as you obtain one.
// Sometimes the connection might be closed unintentionally so
// as a graceful handing, try to connect only once.
func (r *RabbitMQ) Channel() (*amqp.Channel, error) {
	if r.connection == nil {
		if err := r.Connect(); err != nil {
			return nil, errors.New("amqp connection is not open")
		}
	}
	channel, err := r.connection.Channel()
	if err != nil {
		return nil, err
	}

	return channel, nil
}

func (r *RabbitMQ) SetupChannel(ch *amqp.Channel, exchange Exchange, queue Queue, confirm bool, qos int) error {
	if err := ch.ExchangeDeclare(
		exchange.Name,
		exchange.Kind,
		exchange.Durable,
		exchange.AutoDelete,
		false,
		false,
		nil,
	); err != nil {
		return err
	}

	args := amqp.Table{}
	if queue.TTL > 0 {
		args["x-message-ttl"] = queue.TTL
	}
	if _, err := ch.QueueDeclare(
		queue.Name,
		queue.Durable,
		queue.AutoDelete,
		queue.Exclusive,
		false,
		args,
	); err != nil {
		return err
	}
	if err := ch.QueueBind(
		queue.Name,
		queue.Name,
		exchange.Name,
		false,
		nil,
	); err != nil {
		return err
	}

	if confirm {
		if err := ch.Confirm(false); err != nil {
			return err
		}
	}

	if qos < 1 {
		qos = 1
	}

	if err := ch.Qos(qos, 0, false); err != nil {
		return err
	}

	return nil

}

func (r *RabbitMQ) Publish(ch *amqp.Channel, exchange, target string, mandatory, immediate, persistent bool, body []byte) error {

	mode := amqp.Transient
	if persistent {
		mode = amqp.Persistent
	}

	err := ch.Publish(
		exchange,
		target,
		mandatory,
		immediate,
		amqp.Publishing{
			DeliveryMode: mode,
			ContentType:  "text/plain",
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to deliver: %v", err)
	}

	select {
	case <-time.After(r.ChannelNotifyTimeout):
		return errors.New("failed to deliver timeout")
	case ntf := <-ch.NotifyPublish(make(chan amqp.Confirmation, 1)):
		if !ntf.Ack {
			return fmt.Errorf("failed to receive confirmation")
		}
	case rtn := <-ch.NotifyReturn(make(chan amqp.Return, 1)):
		return fmt.Errorf("failed to deliver: errCode=%d, errMsg=%s", rtn.ReplyCode, rtn.ReplyText)
	}
	return nil
}
