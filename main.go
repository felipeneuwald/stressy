package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"
	// "golang.org/x/crypto/bcrypt"
)

// "flag"

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

	cpuBcrypt()
	// cpuLoop()

}

func cpuLoop() {
	for i := 0; i <= 1; i++ {
		i = 0
	}
}

func cpuBcrypt() {
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
