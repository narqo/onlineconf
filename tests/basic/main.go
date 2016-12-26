package main

import (
	"fmt"
	"log"
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

	printConfig()

	go func() {
		tick := time.Tick(7 * time.Second)
		for {
			select {
			case <-tick:
				printConfig()
			default:
			}
		}
	}()

	fmt.Printf("exiting: %v\n", <-errc)
}

func printConfig() {
	cfg, ok := onlineconf.GlobalConfig().(*Config)
	if !ok {
		log.Fatalln("can't cast interface")
	}
	fmt.Printf("config: %#v %+v\n", cfg, cfg)
}
