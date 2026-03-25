package main

import (
	"fmt"
	"net/http"
	neturl "net/url"
	"sync"
	"os"
	"bufio"
	"time"
	"strings"
	"io"
	"crypto/tls"
	"flag"
	"strconv"
	"sync/atomic"
)

var wg sync.WaitGroup
var sem = make(chan struct{}, 100)
var mu sync.Mutex
var totalS int
var (
	totalRequests uint64
	totalErrors   uint64
	totalFounds   uint64
	startTime     time.Time
)

func loadfile(filename string, bufer chan string) {
	var seen = make(map[string]bool)
	file, err := os.Open(filename)
	if err != nil {
		fmt.Println("error opening file")
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		word := scanner.Text()
		if !seen[word] {
			seen[word] = true
			totalS++
			bufer <- word
		}
	}
	close(bufer)
}

func printStatus() {
	reqs := atomic.LoadUint64(&totalRequests)
	errs := atomic.LoadUint64(&totalErrors)
	found := atomic.LoadUint64(&totalFounds)
	rps := float64(reqs) / time.Since(startTime).Seconds()
	fmt.Printf(" :: Progress: %d/%d [%d errors, %d found] [%.0f req/s]\n", reqs, totalS, errs, found, rps)
}

func fazz(url string, keywrd string, fc int, fs int, H string, HN string, HV string, method string, dataP string, proxflag string, recrs bool, depthrcs int, depth int, buffr chan string) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if proxflag != "" {
		pUrl, err := neturl.Parse(proxflag)
		if err == nil {
			transport.Proxy = http.ProxyURL(pUrl)
		}
	}
	var client = &http.Client{
		Timeout: 10 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	requestCounter := 0
	for word := range buffr {
		sem <- struct{}{}
		wg.Add(1)
		requestCounter++
		
		go func(w string) {
			defer wg.Done()
			defer func() { <-sem }()
			
			var req *http.Request
			var err error
			if method == "" {
				req, err = http.NewRequest("GET", strings.Replace(url, keywrd, w, 1), nil)
			} else {
				req, err = http.NewRequest(method, strings.Replace(url, keywrd, w, 1), strings.NewReader(dataP))
			}
			var headerName string
			var headerValue string
			if err != nil {
				if strings.Contains(err.Error(), "timeout") {
					mu.Lock()
					fmt.Printf("[timeout] %s\n", w)
					mu.Unlock()
				}
				return
			}
			if H != "" {
				if strings.Contains(HN, keywrd) {
					headerName = strings.Replace(HN, keywrd, w, 1)
					headerValue = HV
				} else {
					headerName = HN
					headerValue = strings.Replace(HV, keywrd, w, 1)
				}
				req.Header.Set(headerName, headerValue)
			}
			atomic.AddUint64(&totalRequests, 1)
			resp, err := client.Do(req)
			if err != nil {
				atomic.AddUint64(&totalErrors, 1)
				return
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			location := resp.Header.Get("location")
			fuzzedUrl := strings.Replace(url, keywrd, w, 1)
			output := fmt.Sprintf("[%d] %s", resp.StatusCode, fuzzedUrl)
			if resp.StatusCode == 301 || resp.StatusCode == 302 {
				output += fmt.Sprintf(" -> %s", location)
			}
			if H != "" {
				output += fmt.Sprintf(" | %s: %s", headerName, headerValue)
			}
			if resp.StatusCode != fc && len(body) != fs {
				atomic.AddUint64(&totalFounds, 1)
				output += fmt.Sprintf(" | len:%d\n", len(body))
				mu.Lock()
				fmt.Print(output)
				mu.Unlock()
				if recrs && depth < depthrcs && resp.StatusCode < 400 {
					newbufer := make(chan string, 100)
					fuzzedUrl += "/" + keywrd
					wg.Add(1)
					go func() {
						defer wg.Done()
						loadfile(filename, newbufer)
						fazz(fuzzedUrl, keywrd, fc, fs, H, HN, HV, method, dataP, proxflag, recrs, depthrcs, depth+1, newbufer)
					}()
				}
			}
		}(word)

		if requestCounter%100 == 0 {
			mu.Lock()
			printStatus()
			mu.Unlock()
		}
	}
}

var filename string

func main() {
	u := flag.String("u", "", "url")
	w := flag.String("w", "", "wordlist")
	fc := flag.String("fc", "", "filtrate statuscode")
	fs := flag.String("fs", "", "filtrate size")
	h := flag.String("H", "", "header")
	M := flag.String("M", "", "method")
	d := flag.String("d", "", "data")
	px := flag.String("px", "", "proxy, write url")
	recursive := flag.Bool("recursion", false, "recursive")
	recursiveD := flag.Int("recursionD", 0, "depth recursion")
	flag.Parse()

	if *w == "" || *u == "" {
		fmt.Println("Error: -u and -w flags are required")
		flag.PrintDefaults()
		os.Exit(1)
	}

	filtercode, _ := strconv.Atoi(*fc)
	filtersize, _ := strconv.Atoi(*fs)
	parts := strings.Split(*w, ":")
	filename = parts[0]
	keyword := parts[1]
	headerName := ""
	headerValue := ""
	if *h != "" {
		partsH := strings.Split(*h, ": ")
		headerName = partsH[0]
		headerValue = partsH[1]
	}

	buffr := make(chan string, 100)
	startTime = time.Now()

	go loadfile(filename, buffr)
	fazz(*u, keyword, filtercode, filtersize, *h, headerName, headerValue, *M, *d, *px, *recursive, *recursiveD, 0, buffr)
	wg.Wait()

	mu.Lock()
	fmt.Print("\n")
	printStatus()
	fmt.Print("Done!\n")
	mu.Unlock()
}