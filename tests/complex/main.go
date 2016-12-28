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

type Config struct {
	Test1 string `onlineconf:"test1"`
}

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

	var cfg Config

	params := &onlineconf.Params{
		File: cfgFile,
	}
	onlineconf.MustInitGlobalConfig(params, &cfg)

	http.HandleFunc("/", handleRequest)

	go func() {
		addr := ":8080"
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
	var reqCfg = new(Config)
	pcfg, ok := onlineconf.GlobalConfig().(*Config)
	if !ok {
		log.Println("this should not happen")
	}
	*reqCfg = *pcfg

	ctx := context.Background()
	ctx = onlineconf.ContextWithConfig(ctx, reqCfg)

	handleEndpoint(ctx, w, r)
}

func handleEndpoint(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	cfg := onlineconf.ConfigFromContext(ctx).(*Config)

	fmt.Printf("1 req: %s config: %+v\n", r.URL.Path, cfg)
	time.Sleep(10 * time.Second)
	fmt.Printf("2 req: %s config: %+v\n", r.URL.Path, cfg)

	fmt.Fprint(w, cfg.Test1)
}
