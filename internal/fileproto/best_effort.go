package fileproto

import (
	"errors"
	"log"
	"os"
)

func closeAndLog(context string, closeFn func() error) {
	if err := closeFn(); err != nil {
		log.Printf("fileproto: %s: %v", context, err)
	}
}

func removeAndLog(context string, removeFn func() error) {
	if err := removeFn(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("fileproto: %s: %v", context, err)
	}
}
