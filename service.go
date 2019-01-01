package gowatch

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"mvdan.cc/sh/interp"
	"mvdan.cc/sh/syntax"
)

type service struct {
	// The directory to run the service in
	Dir string

	// The bash script to run
	File *syntax.File

	// The context of the currently running service and the function
	// to cancel it.
	ctx  context.Context
	done context.CancelFunc

	lock sync.Mutex
}

// Run starts the service and keeps it alive. If the service is already
// running, it will be stopped. This method exits when the provided context
// is cancelled or the service is stopped using Stop. Since it is likely that
// the service will be launched in a goroutine, a mutex is used to ensure that
// we only start the new one once the old one has completely shut down.
func (s *service) Run(ctx context.Context, stdout, stderr io.Writer) error {
	if s.done != nil {
		s.done()
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	s.ctx, s.done = context.WithCancel(ctx)
	defer s.done()

	for {
		runner, err := interp.New(
			interp.Dir(s.Dir),
			interp.StdIO(nil, stdout, stderr),
		)
		if err != nil {
			return err
		}

		select {
		case <-s.ctx.Done():
			break
		default:
			err = runner.Run(s.ctx, s.File)
		}

		if err == context.Canceled {
			break
		}

		// Something went wrong and the program exited. Wait a little
		// bit before restarting it.
		time.Sleep(150 * time.Millisecond)
	}

	// Wait 150ms before returning to let everything clean up
	time.Sleep(150 * time.Millisecond)
	return nil
}

// Stop stops the service. Fails if it is not currently running.
func (s *service) Stop() error {
	if s.ctx == nil {
		return fmt.Errorf("service not started")
	}

	s.done()
	s.ctx = nil
	s.done = nil
	return nil
}
