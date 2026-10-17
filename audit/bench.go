package main

import (
	"encoding/json"
	"git.taurusxin.com/taurusxin/ncmdump-go/ncmcrypt"
	"os"
	"strconv"
	"sync"
	"time"
)

func main() {
	t := time.Now()
	var wg sync.WaitGroup
	limit, _ := strconv.Atoi(os.Getenv("WORKERS"))
	if limit < 1 {
		limit = 1
	}
	sem := make(chan struct{}, limit)
	for _, p := range os.Args[2:] {
		wg.Add(1)
		f := func(p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			t := time.Now()
			n, e := ncmcrypt.NewNeteaseCloudMusic(p)
			parse := time.Since(t)
			if n != nil {
				defer n.Close()
			}
			if e != nil {
				panic(e)
			}
			t = time.Now()
			_, e = n.Dump(os.Args[1])
			dump := time.Since(t)
			if e != nil {
				panic(e)
			}
			t = time.Now()
			_, e = n.FixMetadata(false)
			meta := time.Since(t)
			err := ""
			if e != nil {
				err = e.Error()
			}
			json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"file": p, "parse_ms": parse.Seconds() * 1000, "dump_ms": dump.Seconds() * 1000, "metadata_cover_ms": meta.Seconds() * 1000, "error": err})
		}
		if os.Getenv("PARALLEL") == "1" {
			go f(p)
		} else {
			f(p)
		}
	}
	wg.Wait()
	json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"wall_ms": time.Since(t).Seconds() * 1000})
}
