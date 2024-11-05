package log

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

type CustomLogEntryFormatter struct{}

func (f *CustomLogEntryFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var w *bytes.Buffer
	if entry.Buffer != nil {
		w = entry.Buffer
	} else {
		w = &bytes.Buffer{}
	}

	fileName := filepath.Base(entry.Caller.File)

	fmt.Fprintf(w, "%s | %7s | %s:%3d | %s.\n",
		entry.Time.Format("2006-01-02 15:04:05.000"),
		strings.ToUpper(entry.Level.String()),
		strings.TrimSuffix(fileName, ".go"),
		entry.Caller.Line,
		entry.Message,
	)
	return w.Bytes(), nil
}

func init() {
	logrus.SetReportCaller(true)
	logrus.SetOutput(os.Stdout)
	logrus.SetFormatter(&CustomLogEntryFormatter{})
	logrus.SetLevel(logrus.InfoLevel)
}
