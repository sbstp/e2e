package main

import (
	"context"
	"time"

	"github.com/sbstp/e2e"
)

func init() {
	e2e.Register("test 1 (cancellable)", func(ctx context.Context, log *e2e.Logger) {
		select {
		case <-ctx.Done():
			log.Println("context cancelled")
			time.Sleep(time.Second * 3)
			panic("test canceled")
		case <-time.After(time.Second * 5):
			log.Println("over")
		}
	})

	e2e.Register("test 2 (not cancellable)", func(ctx context.Context, log *e2e.Logger) {
		time.Sleep(time.Second * 10)
	})
}

func main() {
	e2e.Run(5)
}
