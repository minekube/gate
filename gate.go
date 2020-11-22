package main

import (
	"go.minekube.com/gate/cmd/gate"
	//_ "net/http/pprof"
)

func main() {
	/*if err := agent.Listen(agent.Options{
		ShutdownCleanup: true,
	}); err != nil {
		log.Fatal(err)
	}*/
	/*go func() {
		http.ListenAndServe("localhost:8080", nil)
	}()*/
	gate.Execute()
}
