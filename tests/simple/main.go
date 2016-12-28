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

var testVar1 = onlineconf.String("test1", "default value", "test var 1")
var testVar2 = onlineconf.Int("test2", 100, "test var 2")
var testVar3 = onlineconf.Bool("test3", true, "test var 3")

func main() {
	errc := make(chan error, 1)
	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT)
		errc <- fmt.Errorf("%s", <-c)
	}()

	cfgFile, err := filepath.Abs("tests/onlineconf.conf")
	if err != nil {
		log.Fatalln(err)
	}

	onlineconf.MustInit(cfgFile, nil)

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
	v1 := testVar1.Get(ctx)
	v2 := testVar2.Get(ctx)
	v3 := testVar3.Get(ctx)
	fmt.Println("req:", r.URL.Path, "var1:", v1, "var2:", v2, "var3:", v3)

	time.Sleep(10 * time.Second)

	v1 = testVar1.Get(ctx)
	v2 = testVar2.Get(ctx)
	v3 = testVar3.Get(ctx)
	fmt.Println("after req:", r.URL.Path, "var1:", v1, "var2:", v2, "var3:", v3)

	fmt.Fprint(w, v1)
}
