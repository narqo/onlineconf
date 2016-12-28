package onlineconf

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type OnlineConf interface {
	Version() string
	Config() map[string]interface{}
	Close() error
}

type Options struct {
	CheckInterval time.Duration
	MaxErrors     int
	Prefixes      []string
}

var DefaultOptions = &Options{
	CheckInterval: 5 * time.Second,
}

var globalOnlineConf OnlineConf = &onlineConf{
	parsers: make(map[string]func(v string) (interface{}, error)),
}

// MustInit initialises onlineconf watcher.
func Init(path string, options *Options) error {
	return globalOnlineConf.(*onlineConf).Watch(path, options)
}

// MustInit initialises onlineconf watcher. It panics is failed.
func MustInit(path string, options *Options) {
	if err := Init(path, options); err != nil {
		panic(err)
	}
}

type onlineConf struct {
	path          string
	version string
	data map[string]interface{}

	// prefixes is a set of registered data subtrees
	prefixes []string
	// parsers is a set of registered data parsers
	parsers map[string]func(v string) (interface{}, error)

	checkInterval time.Duration
	maxErrors     int

	mu      sync.RWMutex
	watcher *fsnotify.Watcher
	done    chan struct{}
}

func (c *onlineConf) Watch(path string, options *Options) error {
	path = filepath.Clean(path)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("unabled create watcher: %v", err)
	}

	if options == nil {
		options = DefaultOptions
	}

	c.path = path
	c.prefixes = options.Prefixes
	c.watcher = watcher
	c.done = make(chan struct{})
	c.checkInterval = options.CheckInterval
	c.maxErrors = options.MaxErrors

	err = c.readConfig()
	if err != nil {
		return err
	}

	go c.watch()

	return nil
}

func (c *onlineConf) addParser(key string, fn func(v string) (interface{}, error)) {
	c.mu.Lock()
	c.parsers[key] = fn
	c.mu.Unlock()
}

func (c *onlineConf) readConfig() error {
	config, err := readConfig(c.path)
	if err != nil {
		return err
	}

	data := make(map[string]interface{})

	c.mu.RLock()
	if len(c.prefixes) > 0 {
		for _, prefix := range c.prefixes {
			for k, v := range config.Data {
				if strings.HasPrefix(k, prefix) {
					k = strings.TrimPrefix(k, prefix)
					if parser, ok := c.parsers[k]; ok {
						v, _ = parser(v.(string))
					}
					data[k] = v
				}
			}
		}
	} else {
		for k, v := range config.Data {
			if parser, ok := c.parsers[k]; ok {
				v, _ = parser(v.(string))
			}
			data[k] = v
		}
	}
	c.mu.RUnlock()

	log.Printf("[bg] onlineconf: re-read file: %s version: %v\n", c.path, config.Version)

	c.mu.Lock()
	c.version = config.Version
	c.data = data
	c.mu.Unlock()

	return nil
}

func (c *onlineConf) watch() {
	filedir, _ := filepath.Split(c.path)

	if err := c.watcher.Add(filedir); err != nil {
		log.Printf("[bg] onlineconf: failed to start watching config directory: %s %v", filedir, err)
		return
	}

	tick := time.NewTicker(c.checkInterval)

	var (
		lastWrite *fsnotify.Event
		errs      int
	)

	for {
		select {
		case event := <-c.watcher.Events:
			if filepath.Clean(event.Name) == c.path {
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
					log.Printf("[bg] onlineconf: file: %s event: %v\n", c.path, event)
					lastWrite = &event
				}
			}
		case <-tick.C:
			if lastWrite == nil {
				continue
			}

			err := c.readConfig()
			if err != nil {
				log.Printf("[bg] onlineconf: file: %s conf reader error: %v (%d of %d)\n", c.path, err, errs, c.maxErrors)
				if c.maxErrors > 0 {
					errs++
					if errs == c.maxErrors {
						c.Close()
					}
				}
				continue
			}

			lastWrite = nil
			errs = 0
		case err := <-c.watcher.Errors:
			log.Printf("[bg] onlineconf: file: %s watcher error: %v\n", c.path, err)
			lastWrite = nil
		case <-c.done:
			close(c.done)
			return
		}
	}
}

func (c *onlineConf) Version() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.version
}

func (c *onlineConf) Config() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data
}

func (c *onlineConf) Close() error {
	c.done <- struct{}{}
	return c.watcher.Close()
}

type contextConfigKey struct {}

var defaultNoopConfig = make(map[string]interface{})

// ContextWithConfig stores a snapshot of config into context.
func ContextWithConfig(ctx context.Context) context.Context {
	config := globalOnlineConf.Config()
	return context.WithValue(ctx, contextConfigKey{}, config)
}

// ConfigFromContext retrieves a snapshot of the config from context.
func ConfigFromContext(ctx context.Context) map[string]interface{} {
	config, ok := ctx.Value(contextConfigKey{}).(map[string]interface{})
	if !ok {
		return defaultNoopConfig
	}
	return config
}

type Value interface {
	Get(ctx *context.Context) interface{}
}

func valueFromContext(ctx context.Context, key string, defVal interface{}) interface{} {
	config := ConfigFromContext(ctx)
	val, ok := config[key]
	if !ok {
		val = defVal
	}
	return val
}

type value struct {
	key    string
	defVal interface{}
}

func (g *value) Get(ctx context.Context) interface{} {
	return valueFromContext(ctx, g.key, g.defVal)
}

type intValue struct {
	key    string
	defVal *int
}

func (g *intValue) Get(ctx context.Context) int {
	val := valueFromContext(ctx, g.key, *g.defVal)
	return val.(int)
}

type boolValue struct {
	key    string
	defVal *bool
}

func (g *boolValue) Get(ctx context.Context) bool {
	val := valueFromContext(ctx, g.key, *g.defVal)
	return val.(bool)
}

type stringValue struct {
	key    string
	defVal *string
}

func (g *stringValue) Get(ctx context.Context) string {
	val := valueFromContext(ctx, g.key, *g.defVal)
	return val.(string)
}

func Int(name string, defValue int, desc string) *intValue {
	v := new(int)
	*v = defValue

	globalOnlineConf.(*onlineConf).addParser(name, func(v string) (interface{}, error) {
		return strconv.Atoi(v)
	})

	return &intValue{
		key:    name,
		defVal: v,
	}
}

func Bool(name string, defValue bool, desc string) *boolValue {
	v := new(bool)
	*v = defValue

	globalOnlineConf.(*onlineConf).addParser(name, func(v string) (interface{}, error) {
		return strconv.ParseBool(v)
	})

	return &boolValue{
		key:    name,
		defVal: v,
	}
}

func String(name string, defValue string, desc string) *stringValue {
	v := new(string)
	*v = defValue

	globalOnlineConf.(*onlineConf).addParser(name, func(v string) (interface{}, error) {
		return v, nil
	})

	return &stringValue{
		key:    name,
		defVal: v,
	}
}
