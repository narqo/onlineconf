package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/narqo/onlineconf"
)

var testVar1 = onlineconf.Var("test1", "default value", "test var 1")

func main() {
	errc := make(chan error, 1)
	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT)
		errc <- fmt.Errorf("%s", <-c)
	}()

	cfgFile, err := filepath.Abs("tests/basic/onlineconf.conf")
	if err != nil {
		log.Fatalln(err)
	}

	params := &onlineconf.Params{
		File: cfgFile,
	}
	onlineconf.MustInitGlobalConfig(params)

	http.HandleFunc("/", handleRequest)

	go func() {
		addr := ":8080"
		fmt.Printf("listening http: %s\n", addr)
		errc <- http.ListenAndServe(addr, nil)
	}()

	//go func() {
	//	tick := time.Tick(7 * time.Second)
	//	for {
	//		select {
	//		case <-tick:
	//			//printConfig()
	//		default:
	//		}
	//	}
	//}()

	fmt.Printf("exiting: %v\n", <-errc)
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	ctx = onlineconf.ContextWithConfig(ctx)

	handleEndpoint(ctx, w, r)
}

func handleEndpoint(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	v1, _ := testVar1.Get(ctx)

	fmt.Printf("1 req: %s config: %+v\n", r.URL.Path, v1)
	time.Sleep(10 * time.Second)
	fmt.Printf("2 req: %s config: %+v\n", r.URL.Path, v1)

	fmt.Fprint(w, v1)
}
