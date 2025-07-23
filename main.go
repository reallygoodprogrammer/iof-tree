package main

import (
	"bufio"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/PuerkitoBio/goquery"
)

func main() {
	concurrency := flag.Int("c", 10, "concurrency level")
	files_only := flag.Bool("f", false, "only display files (no directories)")
	flag.Parse()

	urls := flag.Args()

	input := make(chan string)
	output := make(chan string)
	err_output := make(chan error)
	var count int64 = 0

	diminish := func() {
		if atomic.AddInt64(&count, -1) == 0 {
			close(input)
		}
	}

	passes := func(val string) bool {
		return val != "/" && !strings.HasPrefix(val, "?") &&
			!strings.HasPrefix(val, "http://") && !strings.HasPrefix(val, "https://") &&
			!strings.HasPrefix(val, "/")
	}

	var process func(u string)
	process = func(u string) {
		defer diminish()

		resp, err := http.Get(u)
		if err != nil {
			err_output <- err
			return
		}
		defer resp.Body.Close()

		contentType := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "text/html") {
			return
		}

		doc, err := goquery.NewDocumentFromReader(resp.Body)
		if err != nil {
			err_output <- err
			return
		}

		if !strings.Contains(doc.Find("title").Text(), "Index of") {
			return
		}

		doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
			val, valid := s.Attr("href")
			if valid && passes(val) {
				if strings.HasSuffix(val, "/") {
					atomic.AddInt64(&count, 1)
					select {
					case input <- u + val:
						// swagged up
					default:
						process(u + val)
					}
				}
				if !(*files_only) {
					output <- u + val
				}
			}
		})
	}

	var inwg sync.WaitGroup
	for i := 0; i < *concurrency; i++ {
		inwg.Add(1)
		go func() {
			defer inwg.Done()
			for u := range input {
				process(u)
			}
		}()
	}

	var outwg sync.WaitGroup
	outwg.Add(1)
	go func() {
		defer outwg.Done()
		for o := range output {
			fmt.Println(o)
		}
	}()

	var errwg sync.WaitGroup
	errwg.Add(1)
	go func() {
		defer errwg.Done()
		for e := range err_output {
			fmt.Fprintln(os.Stderr, e)
		}
	}()

	if len(urls) == 0 {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			atomic.AddInt64(&count, 1)
			input <- scanner.Text()
		}
	} else {
		for _, u := range urls {
			atomic.AddInt64(&count, 1)
			input <- u
		}
	}

	inwg.Wait()
	close(output)
	close(err_output)
	outwg.Wait()
	errwg.Wait()
}
