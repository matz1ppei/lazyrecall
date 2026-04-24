package debuglog

import (
	"io"
	"log"
	"os"
	"sync"
)

var (
	mu     sync.RWMutex
	logger = log.New(io.Discard, "", log.LstdFlags|log.Lmicroseconds)
)

// InitFile switches the shared logger to append to the given file.
func InitFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}

	mu.Lock()
	logger.SetOutput(f)
	mu.Unlock()
	return nil
}

func Infof(format string, args ...any) {
	mu.RLock()
	defer mu.RUnlock()
	logger.Printf("INFO "+format, args...)
}

func Errorf(format string, args ...any) {
	mu.RLock()
	defer mu.RUnlock()
	logger.Printf("ERROR "+format, args...)
}
