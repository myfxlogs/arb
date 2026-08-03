package main

import (
	"flag"
	"log"

	"arb/desk"
)

func main() {
	addr := flag.String("addr", "qalfa.org:443", "core gRPC address")
	flag.Parse()

	app, err := desk.NewApp(*addr)
	if err != nil {
		log.Fatalf("create desk app: %v", err)
	}
	app.Run()
}
