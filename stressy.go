package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var parallelism int
var executionTime int

func init() {
	flag.IntVar(&parallelism, "parallelism", 1, "Number of parallel operations")
	flag.IntVar(&executionTime, "time", 86400, "Number of seconds to run")
	flag.Parse()
}

func main() {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		time.Sleep(time.Duration(executionTime) * time.Second)
		wg.Done()
		os.Exit(0)
	}()

	wg.Add(parallelism)
	for p := 0; p < parallelism; p++ {
		go func() {
			runtime.Gosched()
			testBcrypt()
			wg.Done()
		}()
	}

	wg.Wait()
}

func testBcrypt() {
	for i := 0; i <= 1; i++ {
		rand.Seed(time.Now().UnixNano())
		i = rand.Int()
		s := fmt.Sprintf("%v", i)
		bs := []byte(s)

		_, err := bcrypt.GenerateFromPassword(bs, bcrypt.MaxCost)
		if err != nil {
			fmt.Println("bcrypt.GenerateFromPassword:", err)
			os.Exit(1)
		}

		i = 0
	}
}
