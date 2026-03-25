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
	"sync/atomic"
)

var wg sync.WaitGroup
var sem = make(chan struct{}, 70)
var mu sync.Mutex
var totalS int
var outputFile *os.File
var (
	totalRequests uint64
	totalErrors   uint64
	totalFounds   uint64
	startTime     time.Time
)

const (
    Green  = "\033[38;5;82m"   
    Red    = "\033[38;5;196m"  
    Yellow = "\033[38;5;226m"  
    Reset  = "\033[0m"
)

const (
    DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64)"
    DefaultTimeout   = 20 * time.Second
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
	count := 0
	for scanner.Scan() {
		word := scanner.Text()
		if !seen[word] && word != "" {
			seen[word] = true
			totalS++
			bufer <- word
			count++
		}
	}
	if count == 0 {
		fmt.Println("Warning: wordlist is empty")
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

func writeOutput(output string) {
	mu.Lock()
	defer mu.Unlock()
	fmt.Print(output)
	if outputFile != nil {
		outputFile.WriteString(output)
	}
}

func getRedirectURL(baseURL string, location string) string {
	if strings.HasPrefix(location, "http") {
		return location
	}
	if strings.HasPrefix(location, "/") {
		u, _ := neturl.Parse(baseURL)
		return fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, location)
	}
	return baseURL + "/" + location
}

func fazz(url string, keywrd string, fc int, fs int, H string, HN string, HV string, method string, dataP string, proxflag string, recrs bool, depthrcs int, depth int, buffr chan string, timeout time.Duration, filename string) {
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
		Timeout: timeout,
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
			} else if dataP != "" {
				req, err = http.NewRequest(method, strings.Replace(url, keywrd, w, 1), strings.NewReader(dataP))
			} else {
				req, err = http.NewRequest(method, strings.Replace(url, keywrd, w, 1), nil)
			}
			var headerName string
			var headerValue string
			if err != nil {
				if strings.Contains(err.Error(), "timeout") {
					writeOutput(fmt.Sprintf("[timeout] %s\n", w))
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
			req.Header.Set("User-Agent", DefaultUserAgent)
			resp, err := client.Do(req)
			if err != nil {
				atomic.AddUint64(&totalErrors, 1)
				return
			}
			
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			
			fuzzedUrl := strings.Replace(url, keywrd, w, 1)
			var output string
			var Color string = Reset
			
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				Color = Green
			} else if resp.StatusCode >= 301 && resp.StatusCode < 400 {
				Color = Yellow
			} else {
				Color = Red
			}
			
			output = fmt.Sprintf("%s[%d]%s %s", Color, resp.StatusCode, Reset, fuzzedUrl)
			
			if resp.StatusCode == 301 || resp.StatusCode == 302 {
				location := resp.Header.Get("location")
				redirectURL := getRedirectURL(fuzzedUrl, location)
				output += fmt.Sprintf(" -> %s", redirectURL)
			}
			
			if H != "" {
				output += fmt.Sprintf(" | %s: %s", headerName, headerValue)
			}

			if (fc != 0 && resp.StatusCode == fc) || (fs != 0 && len(body) == fs) {

			} else {
				atomic.AddUint64(&totalFounds, 1)
				output += fmt.Sprintf(" | len:%d\n", len(body))
				writeOutput(output)
				
				if recrs && depth < depthrcs && resp.StatusCode < 400 {
					newbufer := make(chan string, 100)
					fuzzedUrl += "/" + keywrd
					wg.Add(1)
					go func() {
						defer wg.Done()
						loadfile(filename, newbufer)
						fazz(fuzzedUrl, keywrd, fc, fs, H, HN, HV, method, dataP, proxflag, recrs, depthrcs, depth+1, newbufer, timeout, filename)
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

func main() {
	u := flag.String("u", "", "url")
	w := flag.String("w", "", "wordlist (format: file.txt:FUZZ)")
	fc := flag.Int("fc", 0, "filtrate statuscode")
	fs := flag.Int("fs", 0, "filtrate size")
	h := flag.String("H", "", "header (format: Header-Name: value)")
	M := flag.String("M", "", "method")
	d := flag.String("d", "", "data")
	px := flag.String("px", "", "proxy url")
	recursive := flag.Bool("recursion", false, "recursive mode")
	recursiveD := flag.Int("recursionD", 0, "recursion depth")
	timeout := flag.Int("t", 20, "timeout in seconds")
	output := flag.String("o", "", "output file")
	flag.Parse()

	if *w == "" || *u == "" {
		fmt.Println("Error: -u and -w flags are required")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if !strings.HasPrefix(*u, "http") {
    	fmt.Println("Error: URL must start with http:// or https://")
    	os.Exit(1)
	}

	parts := strings.Split(*w, ":")
	if len(parts) != 2 {
		fmt.Println("Error: wordlist format must be 'file.txt:FUZZ'")
		os.Exit(1)
	}
	filename := parts[0]
	keyword := parts[1]

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		fmt.Printf("Error: wordlist file '%s' not found\n", filename)
		os.Exit(1)
	}

	headerName := ""
	headerValue := ""
	if *h != "" {
		partsH := strings.Split(*h, ": ")
		if len(partsH) != 2 {
			fmt.Println("Error: header format must be 'Header-Name: value'")
			os.Exit(1)
		}
		headerName = partsH[0]
		headerValue = partsH[1]
	}

	if *output != "" {
		var err error
		outputFile, err = os.Create(*output)
		if err != nil {
			fmt.Printf("Error: cannot create output file '%s'\n", *output)
			os.Exit(1)
		}
		defer outputFile.Close()
		fmt.Printf("Output will be saved to: %s\n", *output)
	}

	buffr := make(chan string, 100)
	startTime = time.Now()
	timeoutDuration := time.Duration(*timeout) * time.Second

	go loadfile(filename, buffr)
	fazz(*u, keyword, *fc, *fs, *h, headerName, headerValue, *M, *d, *px, *recursive, *recursiveD, 0, buffr, timeoutDuration, filename)
	wg.Wait()

	mu.Lock()
	fmt.Print("\n")
	printStatus()
	fmt.Print("Done!\n")
	mu.Unlock()
}
