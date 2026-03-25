# fuxxxx

A fast, lightweight web fuzzer written in Go inspired by ffuf.

## Installation

```bash
git clone https://github.com/iluaii/fuxxxx.git
cd fuxxxx
go build -o fuxxxx fuzzz.go
```

## Usage

```bash
./fuxxxx -u <url> -w <wordlist:keyword> [options]
```

### Required Flags

- `-u` — Target URL (must start with http:// or https://)
- `-w` — Wordlist file with keyword (format: `wordlist.txt:FUZZ`)

### Optional Flags

- `-fc` — Filter by status code (e.g., `-fc 404`)
- `-fs` — Filter by response size (e.g., `-fs 1024`)
- `-H` — Custom header (format: `Header-Name: value`)
- `-M` — HTTP method (default: GET)
- `-d` — POST data
- `-px` — Proxy URL (e.g., `http://127.0.0.1:8080`)
- `-t` — Timeout in seconds (default: 20)
- `-o` — Output file for results
- `-recursion` — Enable recursive fuzzing
- `-recursionD` — Recursion depth (default: 0)

## Examples

### Basic fuzzing
```bash
./fuxxxx -u "http://localhost:3000/api/FUZZ" -w wordlist.txt:FUZZ
```

### With filtering
```bash
./fuxxxx -u "http://localhost:3000/api/FUZZ" -w wordlist.txt:FUZZ -fc 404
```

### With custom header and output
```bash
./fuxxxx -u "http://localhost:3000/api/FUZZ" -w wordlist.txt:FUZZ -H "Authorization: Bearer token" -o results.txt
```

### POST request with data
```bash
./fuxxxx -u "http://localhost:3000/login" -w wordlist.txt:FUZZ -M POST -d "username=FUZZ&password=test"
```

### Recursive fuzzing
```bash
./fuxxxx -u "http://localhost:3000/FUZZ" -w wordlist.txt:FUZZ -recursion -recursionD 2
```

## Features

✅ Fast multi-threaded fuzzing (70 concurrent requests)
✅ Custom headers support
✅ POST/PUT/DELETE methods
✅ Proxy support
✅ Status code filtering
✅ Response size filtering
✅ Recursive fuzzing
✅ Colored output (status codes)
✅ Results export to file
✅ Custom timeout
✅ Relative redirect handling

## Output

Results show:
- HTTP status code (color-coded)
- Request URL
- Response size
- Redirects (if any)
- Custom headers (if set)

Example:
```
[200] http://localhost:3000/api/user | len:128
[404] http://localhost:3000/api/admin | len:45
[301] http://localhost:3000/api/test -> http://localhost:3000/api/test/ | len:0
```

## License

MIT