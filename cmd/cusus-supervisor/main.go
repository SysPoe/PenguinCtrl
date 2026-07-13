package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/syspoe/cusus/internal/processgroup"
)

func main() {
	appPath := flag.String("app", "", "path to cusus executable")
	flag.Parse()
	if *appPath == "" {
		executable, err := os.Executable()
		if err != nil {
			log.Print(err)
			return
		}
		*appPath = filepath.Join(filepath.Dir(executable), "cusus.exe")
	}
	backoff := 250 * time.Millisecond
	for {
		started := time.Now()
		command := processgroup.CommandContext(context.Background(), *appPath)
		command.Env = append(os.Environ(), "CUSUS_SUPERVISED=1")
		command.Stdout, command.Stderr = os.Stdout, os.Stderr
		err := processgroup.Start(command)
		if err == nil {
			err = command.Wait()
		}
		if err == nil {
			return
		}
		log.Printf("CuSus exited unexpectedly: %v; restarting safe and silent in %s", err, backoff)
		time.Sleep(backoff)
		if time.Since(started) > 30*time.Second {
			backoff = 250 * time.Millisecond
		} else {
			backoff = min(5*time.Second, backoff*2)
		}
	}
}
