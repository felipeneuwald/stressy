// Package stressy provides CPU stress testing functionality
package stressy

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Stressy struct {
	workers int
	timeout int
	done    chan struct{}
}

type Cfg struct {
	Workers int
	Timeout int
}

func New(c Cfg) *Stressy {
	return &Stressy{
		workers: c.Workers,
		timeout: c.Timeout,
		done:    make(chan struct{}),
	}
}

func (s *Stressy) Run() error {
	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start CPU stress test goroutines
	for i := 0; i < s.workers; i++ {
		go s.stressTestCPU()
	}

	// Start timer if duration is set
	if s.timeout > 0 {
		go s.timer()
	}

	// Wait for either signal or timer
	select {
	case <-sigChan:
		fmt.Println("Received signal, shutting down...")
		close(s.done)
	case <-s.done:
		fmt.Println("Timer expired, shutting down...")
	}

	return nil
}

func (s *Stressy) timer() {
	timer := time.NewTimer(time.Duration(s.timeout) * time.Second)
	<-timer.C
	close(s.done)
}

func (s *Stressy) stressTestCPU() {
	for {
		select {
		case <-s.done:
			return
		default:
			_, err := bcrypt.GenerateFromPassword([]byte(fmt.Sprintf("%v", time.Now())), bcrypt.MaxCost)
			if err != nil {
				panic(err)
			}
			fmt.Printf(".")
		}
	}
}