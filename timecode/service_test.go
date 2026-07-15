package timecode

import (
	"sync"
	"testing"
)

func TestServiceSerializesConcurrentConfigure(t *testing.T) {
	service := NewService(Config{Source: SourceInternal}, "")
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			service.Configure(Config{Source: SourceInternal}, "")
		}()
	}
	wg.Wait()
	service.Close()
}
