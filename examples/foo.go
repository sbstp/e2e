package main

import (
	"context"
	"time"

	"github.com/sbstp/e2e"
)

func init() {
	e2e.Register("test 1", func(ctx context.Context, log *e2e.Logger) {
		time.Sleep(time.Second * 1)
		log.Printf("oh no")
	})

	e2e.Register("test 2", func(ctx context.Context, log *e2e.Logger) {
		time.Sleep(time.Second * 2)
	})
}

func main() {
	e2e.Run(10)
}
