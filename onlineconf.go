package onlineconf

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

var (
	ErrEmptyConfig          = errors.New("config is empty")
	ErrInvalidSpecification = errors.New("specification must be a struct pointer")
)

var defaultCheckInterval = 5 * time.Second

type OnlineConf interface {
	Config() map[string]interface{}
	Close() error
}

type Params struct {
	File          string
	CheckInterval time.Duration
	MaxErrors     int
	Prefixes      []string
}

type onlineConf struct {
	filename      string
	checkInterval time.Duration
	maxErrors     int

	rawConfig *Config

	mu      sync.RWMutex
	watcher *fsnotify.Watcher
	done    chan struct{}
}

func New(params *Params) (OnlineConf, error) {
	filename := filepath.Clean(params.File)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("unabled to create onlineConf: %v", err)
	}

	c := &onlineConf{
		filename:      filename,
		watcher:       watcher,
		checkInterval: params.CheckInterval,
		maxErrors:     params.MaxErrors,
		done:          make(chan struct{}),
	}
	if c.checkInterval == 0 {
		c.checkInterval = defaultCheckInterval
	}

	err = c.readConfig()
	if err != nil {
		return nil, err
	}

	go c.watch()

	return c, nil
}

func (c *onlineConf) readConfig() error {
	config, err := readConfig(c.filename)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.rawConfig = config
	fmt.Printf("re-read file: %s version: %v\n", c.filename, config.Version)
	c.mu.Unlock()
	return nil
}
//
//func (c *onlineConf) unmarshalConfig(rawConfig *Config) error {
//	if rawConfig == nil || len(rawConfig.Data) == 0 {
//		return ErrEmptyConfig
//	}
//	return unmarshalConfigData(rawConfig.Data, c.config)
//}

func (c *onlineConf) watch() {
	filedir, _ := filepath.Split(c.filename)

	if err := c.watcher.Add(filedir); err != nil {
		fmt.Printf("failed to start watching config directory: %s %v", filedir, err)
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
			fmt.Printf("file: %s event: %v\n", event.Name, event)
			if filepath.Clean(event.Name) == c.filename {
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
					fmt.Printf("file: %s event: %v\n", c.filename, event)
					lastWrite = &event
				}
			}
		case <-tick.C:
			if lastWrite == nil {
				continue
			}

			err := c.readConfig()
			if err != nil {
				fmt.Printf("file: %s error: %v (%d of %d)\n", c.filename, err, errs, c.maxErrors)
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
			fmt.Printf("file: %s error: %v\n", c.filename, err)
			lastWrite = nil
		case <-c.done:
			close(c.done)
			return
		}
	}
}

func (c *onlineConf) Config() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rawConfig.Data
}

func (c *onlineConf) Close() error {
	c.done <- struct{}{}
	return c.watcher.Close()
}

func unmarshalConfigData(data map[string]interface{}, spec interface{}) error {
	s := reflect.ValueOf(spec)
	if s.Kind() != reflect.Ptr {
		return ErrInvalidSpecification
	}
	s = s.Elem()
	if s.Kind() != reflect.Struct {
		return ErrInvalidSpecification
	}
	typeOfSpec := s.Type()

	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		ftype := typeOfSpec.Field(i)
		if !f.CanSet() || ftype.Tag.Get("ignored") == "true" {
			continue
		}

		for f.Kind() == reflect.Ptr {
			if f.IsNil() {
				if f.Type().Elem().Kind() != reflect.Struct {
					// nil pointer to a non-struct: leave it alone
					break
				}
				// nil pointer to struct: create a zero instance
				f.Set(reflect.New(f.Type().Elem()))
			}
			f = f.Elem()
		}

		key := ftype.Name

		alt := ftype.Tag.Get("onlineconf")
		if alt != "" {
			key = alt
		}

		// TODO(v.varankin): support inner struct
		//if f.Kind() == reflect.Struct {
		//}

		value, ok := data[key]
		if !ok {
			if def := ftype.Tag.Get("default"); def != "" {
				value = def
			}
		}

		val, ok := value.(string)
		if !ok {
			// skip for now
			continue
		}

		err := processField(val, f)
		if err != nil {
			return err
		}
	}

	return nil
}

func processField(value string, field reflect.Value) error {
	typ := field.Type()

	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
		if field.IsNil() {
			field.Set(reflect.New(typ))
		}
		field = field.Elem()
	}

	switch typ.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var (
			val int64
			err error
		)
		if field.Kind() == reflect.Int64 && typ.PkgPath() == "time" && typ.Name() == "Duration" {
			var d time.Duration
			d, err = time.ParseDuration(value)
			val = int64(d)
		} else {
			val, err = strconv.ParseInt(value, 0, typ.Bits())
		}
		if err != nil {
			return err
		}

		field.SetInt(val)
	case reflect.Bool:
		val, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(val)
	}

	return nil
}

var globalConf OnlineConf

func InitGlobalConfig(params *Params) error {
	var err error
	globalConf, err = New(params)
	if err != nil {
		return err
	}
	return nil
}

func MustInitGlobalConfig(params *Params) {
	if err := InitGlobalConfig(params); err != nil {
		panic(err)
	}
}

func CloseGlobalConfig() error {
	if globalConf != nil {
		return globalConf.Close()
	}
	return nil
}
