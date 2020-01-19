// +build !kafka

package logging

import (
	"fmt"
	"io"
	"os"

	"github.com/connectome-neuprint/neuPrintHTTP/config"
)

// GetLogger gets a logging handler
func GetLogger(port int, options config.Config) (io.Writer, error) {

	logFile := os.Stdout
	var err error

	if options.LoggerFile != "" {
		if logFile, err = os.OpenFile(options.LoggerFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err != nil {
			fmt.Println(err)
			return nil, err
		}
		//defer logFile.Close()
	}
	logWriter := io.Writer(logFile)
	return logWriter, nil
}
