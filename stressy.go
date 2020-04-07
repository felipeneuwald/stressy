package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var testType string
var parallelism int

func init() {
	flag.StringVar(&testType, "type", "bcrypt", "Test type")
	flag.IntVar(&parallelism, "parallelism", 1, "Number of parallel resource operations")
	flag.Parse()
}

func main() {

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for sig := range c {
			fmt.Println(sig)
			bye(1, "bye bye")
		}
	}()

	fmt.Println("NumGoroutine", runtime.NumGoroutine())
	fmt.Println("NumCPU", runtime.NumCPU())
	fmt.Println("GOMAXPROCS", runtime.GOMAXPROCS(0))

	var wg sync.WaitGroup
	wg.Add(parallelism)
	for p := 0; p < parallelism; p++ {
		go func() {
			switch testType {
			case "bcrypt":
				runtime.Gosched()
				testBcrypt()
			case "loop":
				runtime.Gosched()
				testLoop()
			default:
				fmt.Println("NumGoroutine", runtime.NumGoroutine())
				bye(1, "Unknown test type")
			}
			wg.Done()
		}()
	}

	fmt.Println(runtime.NumGoroutine())

	wg.Wait()

}

func testBcrypt() {
	fmt.Println("bcrypt")
	// fmt.Println(runtime.NumGoroutine())

	for i := 0; i <= 1; i++ {
		// fmt.Printf(".")
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
		// fmt.Printf("+")
	}
}

func testLoop() {
	fmt.Println("loop")
	i := 0
	for {
		i++
	}
}

func bye(i int, s string) {
	fmt.Println(s)
	os.Exit(i)
}
