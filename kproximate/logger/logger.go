package logger

import (
	"fmt"
	"log"
	"os"
)

var (
	InfoLog    *log.Logger
	WarningLog *log.Logger
	ErrorLog   *log.Logger
)

func init() {
	controllerName, err := os.Hostname()
	if err != nil {
		log.Panicf("Could not get worker name: %s", err)
	}

	InfoLog = log.New(os.Stdout, fmt.Sprintf("INFO: %s", controllerName), log.Ldate|log.Ltime)
	WarningLog = log.New(os.Stdout, fmt.Sprintf("WARNING: %s", controllerName), log.Ldate|log.Ltime)
	ErrorLog = log.New(os.Stdout, fmt.Sprintf("ERROR: %s", controllerName), log.Ldate|log.Ltime)
}
