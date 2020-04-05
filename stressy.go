package main

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"sync"
	"time"

	// "flag"

	"golang.org/x/crypto/bcrypt"
)

const parallel = 2

func main() {
	// cpuTest := flag.String("cpu", "bcrypt", "type of CPU test")
	// infinite := flag.Bool("infinite", false, "infinte?")
	// threads := flag.Int("threads", 1, "number of threads")
	//
	// flag.Parse()
	//
	// fmt.Println(cpuTest, *cpuTest)
	// fmt.Println(infinite, *infinite)
	// fmt.Println(threads, *threads)
	// fmt.Println(flag.Args())

	// // it works. have to study, understand that and see why it should be used.
	// c := make(chan os.Signal, 1)
	// signal.Notify(c, os.Interrupt)
	// go func() {
	// 	for sig := range c {
	// 		fmt.Println("^C, Bye.", sig)
	// 		os.Exit(1)
	// 	}
	// }()

	cpuBcrypt()
	// cpuLoop()

}

func cpuLoop() {
	for i := 0; i <= 1; i++ {
		i = 0
	}
}

func cpuBcrypt() {
	// defer bye()

	var wg sync.WaitGroup
	wg.Add(parallel)

	for p := 0; p < parallel; p++ {
		go func() {

			runtime.Gosched()

			for i := 0; i <= 1; i++ {
				fmt.Printf(".")
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
				fmt.Printf("+")
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

// OLD func cpuBcrypt, no concurrency. Not in use.
func cpuBcryptOLD() {
	// defer bye()

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

// func bye() {
// 	fmt.Println("Bye.")
// }
