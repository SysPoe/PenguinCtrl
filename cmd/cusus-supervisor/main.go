package main

import (
	"context"
	"flag"
	"log"
	"os"
)

func main() {
	appPath := flag.String("app", "", "path to cusus executable")
	flag.Parse()
	if *appPath == "" {
		resolved, err := defaultAppPath()
		if err != nil {
			log.Print(err)
			return
		}
		*appPath = resolved
	}
	runner := newSupervisor(*appPath, os.Stdout, os.Stderr, log.Printf)
	_ = runner.Run(context.Background())
}
