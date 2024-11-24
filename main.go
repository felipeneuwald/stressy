package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const appName = "stressy"
const version = "0.2.0"

func main() {
	fmt.Println(appName, version)

	p := flag.Int("p", 1, "Qty of parallel CPU stress tests")
	t := flag.Int("t", 0, "Test execution time (seconds)\nIf not specified will run indefinitely")
	flag.Parse()

	if *t > 0 {
		go endTest(*t)
	}

	for i := 0; i < *p; i++ {
		go stressTestCPU()
	}

	select {}
}

func stressTestCPU() {
	for {
		_, err := bcrypt.GenerateFromPassword([]byte(fmt.Sprintf("%v", time.Now())), bcrypt.MaxCost)
		if err != nil {
			log.Fatalf("bcrypt.GenerateFromPassword(): %v", err)
		}
	}
}

func endTest(t int) {
	time.Sleep(time.Duration(t) * time.Second)
	os.Exit(0)
}
