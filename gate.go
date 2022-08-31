package main

import (
	"go.minekube.com/gate/cmd/gate"
)

func main() {

	//if err := agent.Listen(agent.ServeOptions{
	//	ShutdownCleanup: true,
	//}); err != nil {
	//	log.Fatal(err)
	//}
	//
	//if profiling {
	//	go func() {
	//		http.ListenAndServe("localhost:8080", pprof.Handler("heap"))
	//	}()
	//}

	gate.Execute()
}
