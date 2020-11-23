package main

import (
	"context"
	"fmt"
	"time"

	"github.com/sbstp/e2e"
)

func init() {
	e2e.Register("test 1", func(ctx context.Context, log *e2e.Logger) {
		select {
		case <-ctx.Done():
			time.Sleep(time.Second * 3)
			panic("canceled xxx")
		case <-time.After(time.Second * 5):
			fmt.Println("over")
		}
	})

	e2e.Register("test 2", func(ctx context.Context, log *e2e.Logger) {
		time.Sleep(time.Second * 2)
	})
}

func main() {
	e2e.Run(1)
}
