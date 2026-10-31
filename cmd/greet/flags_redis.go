package main

import (
	"flag"

	"github.com/crystade/greet"
	"github.com/crystade/greet/protocols/redis"
)

func init() {
	registerFlagHandler(redis.ProtocolName, flagHandler{
		register: registerRedisFlags,
		parse:    parseRedisFlags,
	})
}

func registerRedisFlags(fs *flag.FlagSet) {
	fs.Bool("hello", false, "Use HELLO instead of PING (returns server info; RESP version defaults to auto)")
	fs.String("hello-mode", "", "RESP version for HELLO: auto, resp2 or resp3 (implies --hello; default auto)")
}

func parseRedisFlags(fs *flag.FlagSet) ([]greet.GreetOption, error) {
	useHello := false
	if f := fs.Lookup("hello"); f != nil {
		if getter, ok := f.Value.(flag.Getter); ok {
			if v, ok := getter.Get().(bool); ok {
				useHello = v
			}
		}
	}

	var mode redis.HelloMode
	if f := fs.Lookup("hello-mode"); f != nil {
		if s := f.Value.String(); s != "" {
			m, err := redis.ParseHelloMode(s)
			if err != nil {
				return nil, err
			}
			mode = m
		}
	}

	return []greet.GreetOption{
		greet.WithLayerConfig(redis.ProtocolName, &redis.RedisConfig{
			UseHello:  useHello,
			HelloMode: mode,
		}),
	}, nil
}
