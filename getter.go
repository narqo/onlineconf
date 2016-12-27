package onlineconf

import (
	"context"
	"errors"
)

var ErrKeyNotExists = errors.New("key does not exists")

type Getter interface {
	Get(ctx context.Context) (interface{}, error)
}

type configKey struct {}

func ContextWithConfig(ctx context.Context) context.Context {
	cfg := globalConf.Config() // TODO(v.varankin): copy map
	return context.WithValue(ctx, configKey{}, cfg)
}

func ConfigFromContext(ctx context.Context) map[string]interface{} {
	cfg, ok := ctx.Value(configKey{}).(map[string]interface{})
	if !ok {
		return make(map[string]interface{})
	}
	return cfg
}

type getter struct {
	key    string
	defVal *interface{}
}

func (g *getter) Get(ctx context.Context) (interface{}, error) {
	cfg := ConfigFromContext(ctx)
	val, ok := cfg[g.key]
	if !ok {
		val = *g.defVal
	}
	if val == nil {
		return nil, ErrKeyNotExists
	}
	return val, nil
}

func Var(name string, defValue interface{}, desc string) Getter {
	v := new(interface{})

	*v = defValue
	//cfg.AddCallback(name, func(v1 string) error {
	//	*v = v1
	//	return nil
	//})
	//cfg.AddFlag(name, desc)

	return &getter{
		key:    name,
		defVal: v,
	}
}
