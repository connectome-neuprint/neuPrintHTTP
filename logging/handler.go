// +build kafka

package logging

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/connectome-neuprint/neuPrintHTTP/config"
	"gopkg.in/confluentinc/confluent-kafka-go.v1/kafka"
)

type kafkaLog struct {
	Producer *kafka.Producer
	Topic    string
}

func (k *kafkaLog) Write(p []byte) (int, error) {
	kafkaMsg := &kafka.Message{

		TopicPartition: kafka.TopicPartition{Topic: &k.Topic, Partition: kafka.PartitionAny},

		Value: p,

		Timestamp: time.Now(),
	}

	if err := k.Producer.Produce(kafkaMsg, nil); err != nil {
		return 0, err
	}

	return len(p), nil

}

// GetLogger gets a logging handler
func GetLogger(port int, options config.Config) (io.Writer, error) {

	logFile := os.Stdout
	var err error

	if options.LoggerFile != "" {
		if logFile, err = os.OpenFile(options.LoggerFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err != nil {
			fmt.Println(err)
			return nil, err
		}
		defer logFile.Close()
	}
	logWriter := io.Writer(logFile)

	// use kafka for logging if available
	if len(options.KafkaServers) > 0 {
		serverstr := strings.Join(options.KafkaServers, ",")
		kp, _ := kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": serverstr})

		portstr := strconv.Itoa(port)
		logWriter = &kafkaLog{kp, "neuprint_" + options.Hostname + "_" + portstr}
	}

	return logWriter, nil
}
