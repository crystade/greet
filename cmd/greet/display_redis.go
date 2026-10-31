package main

import (
	"fmt"
	"sort"

	"github.com/crystade/greet"
	"github.com/crystade/greet/protocols/redis"
)

func printRedisData(data *redis.RedisResult) {
	if data.Info == nil {
		fmt.Printf("  Pong: %v\n", data.Pong)
		return
	}

	keys := make([]string, 0, len(data.Info))
	for k := range data.Info {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Printf("  Server Info:\n")
	for _, k := range keys {
		fmt.Printf("    %s: %s\n", k, data.Info[k])
	}
}

func init() {
	registerLayerPrinter(redis.ProtocolName, func(layer greet.LayerResult) {
		if r, ok := layer.Data.(*redis.RedisResult); ok {
			printRedisData(r)
		}
	})
}
